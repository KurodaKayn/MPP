package publisher

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	stdhtml "html"
	"strings"

	"github.com/kurodakayn/mpp-backend/internal/models"
	pkgx "github.com/kurodakayn/mpp-backend/internal/pkg/x"
	nethtml "golang.org/x/net/html"
)

const xCharacterLimit = 280

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

func (x *XPublisher) Publish(ctx context.Context, pub *models.ProjectPlatformPublication) (string, string, error) {
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
	if countRunes(text) > xCharacterLimit {
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
	return truncateRunesWithEllipsis(text, limit)
}

func truncateRunesWithEllipsis(value string, limit int) string {
	runes := []rune(strings.TrimSpace(value))
	if len(runes) <= limit {
		return string(runes)
	}
	if limit <= 3 {
		return string(runes[:limit])
	}
	return strings.TrimSpace(string(runes[:limit-3])) + "..."
}

func countRunes(value string) int {
	return len([]rune(value))
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
