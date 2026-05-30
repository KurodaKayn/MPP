package publisher

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	stdhtml "html"
	"regexp"
	"strings"
	"unicode"

	"github.com/kurodakayn/mpp-backend/internal/models"
	pkgx "github.com/kurodakayn/mpp-backend/internal/pkg/x"
	nethtml "golang.org/x/net/html"
)

const xCharacterLimit = 280
const xURLWeight = 23

var xURLPattern = regexp.MustCompile(`https?://[^\s<>"']+`)

type XPublisher struct{}

type XConfig struct {
	APIKey            string `json:"api_key"`
	APISecret         string `json:"api_secret"`
	AccessToken       string `json:"access_token"`
	AccessTokenSecret string `json:"access_token_secret"`
	Username          string `json:"username"`
}

type xAdaptedContent struct {
	Format  string `json:"format"`
	Summary string `json:"summary"`
	Text    string `json:"text"`
}

func (x *XPublisher) ValidateConfig(config []byte) error {
	var cfg XConfig
	if err := json.Unmarshal(config, &cfg); err != nil {
		return err
	}
	return cfg.credentials().Validate()
}

func (x *XPublisher) AdaptContent(project *models.Project) ([]byte, error) {
	text := buildXPostText(project.Title, htmlToText(project.SourceContent), xCharacterLimit)
	return json.Marshal(xAdaptedContent{
		Format:  "text",
		Summary: text,
		Text:    text,
	})
}

func (x *XPublisher) Publish(ctx context.Context, pub *models.ProjectPlatformPublication, _ *models.PlatformAccount) (string, string, error) {
	var cfg XConfig
	if err := json.Unmarshal(pub.Config, &cfg); err != nil {
		return "", "", fmt.Errorf("failed to parse x config: %w", err)
	}
	if err := cfg.credentials().Validate(); err != nil {
		return "", "", err
	}

	text := extractXText(pub.AdaptedContent)
	if text == "" {
		return "", "", fmt.Errorf("x post text is empty")
	}
	if xWeightedLength(text) > xCharacterLimit {
		return "", "", fmt.Errorf("x post exceeds %d characters", xCharacterLimit)
	}

	tweet, err := pkgx.NewClient(cfg.credentials()).CreateTweet(ctx, text)
	if err != nil {
		return "", "", err
	}

	return tweet.ID, xStatusURL(cfg.Username, tweet.ID), nil
}

func (c XConfig) credentials() pkgx.Credentials {
	return pkgx.Credentials{
		APIKey:            strings.TrimSpace(c.APIKey),
		APISecret:         strings.TrimSpace(c.APISecret),
		AccessToken:       strings.TrimSpace(c.AccessToken),
		AccessTokenSecret: strings.TrimSpace(c.AccessTokenSecret),
	}
}

func extractXText(raw []byte) string {
	var structured xAdaptedContent
	if err := json.Unmarshal(raw, &structured); err == nil {
		if text := strings.TrimSpace(structured.Text); text != "" {
			return text
		}
		if summary := strings.TrimSpace(structured.Summary); summary != "" {
			return summary
		}
	}

	var plain string
	if err := json.Unmarshal(raw, &plain); err == nil {
		return strings.TrimSpace(plain)
	}

	return strings.TrimSpace(string(raw))
}

func buildXPostText(title, body string, limit int) string {
	title = strings.TrimSpace(title)
	body = strings.TrimSpace(body)

	var text string
	switch {
	case title != "" && body != "":
		text = title + "\n\n" + body
	case title != "":
		text = title
	default:
		text = body
	}
	return truncateXTextWithEllipsis(text, limit)
}

func truncateXTextWithEllipsis(value string, limit int) string {
	value = strings.TrimSpace(value)
	if xWeightedLength(value) <= limit {
		return value
	}

	suffix := "..."
	budget := limit - xWeightedLength(suffix)
	if budget <= 0 {
		return truncateXTextByRuneWeight(value, limit)
	}

	return strings.TrimSpace(truncateXTextByRuneWeight(value, budget)) + suffix
}

func truncateXTextByRuneWeight(value string, limit int) string {
	var builder strings.Builder
	used := 0
	for _, r := range value {
		weight := xRuneWeight(r)
		if used+weight > limit {
			break
		}
		builder.WriteRune(r)
		used += weight
	}
	return builder.String()
}

func xWeightedLength(value string) int {
	length := 0
	last := 0
	for _, match := range xURLPattern.FindAllStringIndex(value, -1) {
		length += xWeightedSegmentLength(value[last:match[0]])
		length += xURLWeight
		last = match[1]
	}
	length += xWeightedSegmentLength(value[last:])
	return length
}

func xWeightedSegmentLength(value string) int {
	length := 0
	for _, r := range value {
		length += xRuneWeight(r)
	}
	return length
}

func xRuneWeight(r rune) int {
	if r <= unicode.MaxASCII || unicode.Is(unicode.Latin, r) {
		return 1
	}
	return 2
}

func htmlToText(source string) string {
	doc, err := nethtml.Parse(strings.NewReader(source))
	if err != nil {
		return strings.TrimSpace(stdhtml.UnescapeString(source))
	}

	var buffer bytes.Buffer
	var walk func(*nethtml.Node)
	walk = func(n *nethtml.Node) {
		if n.Type == nethtml.TextNode {
			text := strings.TrimSpace(n.Data)
			if text != "" {
				if buffer.Len() > 0 {
					buffer.WriteByte(' ')
				}
				buffer.WriteString(text)
			}
		}
		if n.Type == nethtml.ElementNode && n.Data == "br" {
			buffer.WriteByte('\n')
		}
		for child := n.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
		if n.Type == nethtml.ElementNode && isBlockElement(n.Data) && buffer.Len() > 0 {
			buffer.WriteByte('\n')
		}
	}
	walk(doc)

	lines := strings.FieldsFunc(stdhtml.UnescapeString(buffer.String()), func(r rune) bool {
		return r == '\n' || r == '\r'
	})
	cleaned := make([]string, 0, len(lines))
	for _, line := range lines {
		if line = strings.Join(strings.Fields(line), " "); line != "" {
			cleaned = append(cleaned, line)
		}
	}
	return strings.Join(cleaned, "\n")
}

func isBlockElement(tag string) bool {
	switch tag {
	case "article", "blockquote", "div", "figcaption", "figure", "h1", "h2", "h3", "h4", "h5", "h6", "li", "p", "section":
		return true
	default:
		return false
	}
}

func xStatusURL(username, tweetID string) string {
	username = strings.Trim(strings.TrimSpace(username), "@")
	if username == "" {
		return "https://x.com/i/web/status/" + tweetID
	}
	return "https://x.com/" + username + "/status/" + tweetID
}
