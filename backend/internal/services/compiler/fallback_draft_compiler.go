package compiler

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"unicode"

	"golang.org/x/net/html"

	"github.com/kurodakayn/mpp-backend/internal/models"
)

const (
	fallbackDraftSchemaVersion = 1
	fallbackXMaxWeight         = 280
	fallbackXURLWeight         = 23
)

var fallbackURLPattern = regexp.MustCompile(`https?://[^\s<>"']+`)

type fallbackDraftCompiler struct{}

type fallbackAdaptedContent struct {
	SchemaVersion int    `json:"schema_version"`
	Format        string `json:"format"`
	HTML          string `json:"html,omitempty"`
	Markdown      string `json:"markdown,omitempty"`
	Text          string `json:"text,omitempty"`
	Summary       string `json:"summary,omitempty"`
}

func NewFallbackDraftCompiler() ProjectDraftCompiler {
	return fallbackDraftCompiler{}
}

func (c fallbackDraftCompiler) CompileProjectDrafts(_ context.Context, project *models.Project, _ []models.ProjectPlatformPublication, platforms []string) (map[string][]byte, error) {
	if project == nil {
		return nil, fmt.Errorf("%w: source project is required", errContentPipelineDraftContract)
	}

	outputs := make(map[string][]byte, len(platforms))
	for _, platform := range platforms {
		platform = strings.TrimSpace(platform)
		payload, err := c.compilePlatform(project, platform)
		if err != nil {
			return nil, err
		}
		outputs[platform] = payload
	}
	return outputs, nil
}

func (c fallbackDraftCompiler) compilePlatform(project *models.Project, platform string) ([]byte, error) {
	sourceHTML := project.SourceContent
	text := fallbackHTMLToText(sourceHTML)
	summary := fallbackSummarize(text)

	var content fallbackAdaptedContent
	switch platform {
	case "wechat":
		content = fallbackAdaptedContent{
			SchemaVersion: fallbackDraftSchemaVersion,
			Format:        "html",
			HTML:          sourceHTML,
			Summary:       summary,
		}
	case "zhihu":
		content = fallbackAdaptedContent{
			SchemaVersion: fallbackDraftSchemaVersion,
			Format:        "markdown",
			Markdown:      fallbackHTMLToMarkdown(sourceHTML),
			Summary:       summary,
		}
	case "x":
		postText := fallbackJoinTitleAndBody(project.Title, text)
		postText = fallbackTruncateWeightedTextWithEllipsis(postText, fallbackXMaxWeight)
		content = fallbackAdaptedContent{
			SchemaVersion: fallbackDraftSchemaVersion,
			Format:        "text",
			Text:          postText,
			Summary:       fallbackSummarize(postText),
		}
	case "douyin":
		postText := fallbackTextWithFallback(text, project.Title, sourceHTML)
		content = fallbackAdaptedContent{
			SchemaVersion: fallbackDraftSchemaVersion,
			Format:        "text",
			Text:          postText,
			Summary:       fallbackSummarize(postText),
		}
	default:
		return nil, fmt.Errorf("%w: unsupported draft platform %q", errContentPipelineDraftContract, platform)
	}

	payload, err := json.Marshal(content)
	if err != nil {
		return nil, err
	}
	if err := validateCompiledAdaptedContent(platform, payload); err != nil {
		return nil, err
	}
	return payload, nil
}

func fallbackHTMLToText(value string) string {
	nodes := fallbackParseHTMLFragment(value)
	renderer := fallbackTextRenderer{}
	for _, node := range nodes {
		renderer.render(node)
	}
	return renderer.finish()
}

type fallbackTextRenderer struct {
	output strings.Builder
}

func (r *fallbackTextRenderer) render(node *html.Node) {
	switch node.Type {
	case html.TextNode:
		r.writeText(node.Data)
	case html.ElementNode:
		if node.Data == "br" {
			r.output.WriteByte('\n')
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			r.render(child)
		}
		if fallbackIsBlockElement(node.Data) && r.output.Len() > 0 {
			r.output.WriteByte('\n')
		}
	default:
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			r.render(child)
		}
	}
}

func (r *fallbackTextRenderer) writeText(value string) {
	collapsed := fallbackCollapseInlineWhitespace(value)
	if collapsed == "" {
		return
	}
	current := r.output.String()
	if len(value) > 0 && unicode.IsSpace([]rune(value)[0]) && current != "" && !strings.HasSuffix(current, " ") && !strings.HasSuffix(current, "\n") {
		r.output.WriteByte(' ')
	}
	r.output.WriteString(collapsed)
	if len(value) > 0 && unicode.IsSpace([]rune(value)[len([]rune(value))-1]) {
		r.output.WriteByte(' ')
	}
}

func (r *fallbackTextRenderer) finish() string {
	lines := strings.Split(r.output.String(), "\n")
	kept := make([]string, 0, len(lines))
	for _, line := range lines {
		line = fallbackCollapseInlineWhitespace(line)
		if line != "" {
			kept = append(kept, line)
		}
	}
	return strings.Join(kept, "\n")
}

func fallbackHTMLToMarkdown(value string) string {
	nodes := fallbackParseHTMLFragment(value)
	renderer := fallbackMarkdownRenderer{}
	for _, node := range nodes {
		renderer.render(node)
	}
	return strings.TrimSpace(renderer.output.String())
}

type fallbackMarkdownRenderer struct {
	output strings.Builder
}

func (r *fallbackMarkdownRenderer) render(node *html.Node) {
	switch node.Type {
	case html.TextNode:
		r.writeText(node.Data)
	case html.ElementNode:
		r.renderElement(node)
	default:
		r.renderChildren(node)
	}
}

func (r *fallbackMarkdownRenderer) renderElement(node *html.Node) {
	switch node.Data {
	case "h1", "h2", "h3", "h4", "h5", "h6":
		r.ensureBlankLine()
		level, _ := strconv.Atoi(strings.TrimPrefix(node.Data, "h"))
		r.output.WriteString(strings.Repeat("#", max(level, 1)))
		r.output.WriteByte(' ')
		r.renderChildren(node)
		r.ensureBlankLine()
	case "p":
		r.ensureBlankLine()
		r.renderChildren(node)
		r.ensureBlankLine()
	case "strong", "b":
		r.output.WriteString("**")
		r.renderChildren(node)
		r.output.WriteString("**")
	case "em", "i":
		r.output.WriteByte('*')
		r.renderChildren(node)
		r.output.WriteByte('*')
	case "a":
		label := fallbackNodeText(node)
		href := fallbackAttr(node, "href")
		if label != "" && href != "" {
			r.output.WriteByte('[')
			r.output.WriteString(label)
			r.output.WriteString("](")
			r.output.WriteString(href)
			r.output.WriteByte(')')
			return
		}
		r.renderChildren(node)
	case "img":
		src := fallbackAttr(node, "src")
		if src == "" {
			return
		}
		r.ensureBlankLine()
		r.output.WriteString("![")
		r.output.WriteString(fallbackAttr(node, "alt"))
		r.output.WriteString("](")
		r.output.WriteString(src)
		r.output.WriteByte(')')
		r.ensureBlankLine()
	case "blockquote":
		r.ensureBlankLine()
		for _, line := range strings.Split(fallbackNodeText(node), "\n") {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			r.output.WriteString("> ")
			r.output.WriteString(line)
			r.output.WriteByte('\n')
		}
		r.ensureBlankLine()
	case "ul":
		r.ensureBlankLine()
		r.renderListItems(node, "-")
		r.ensureBlankLine()
	case "ol":
		r.ensureBlankLine()
		index := 1
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			if child.Type == html.ElementNode && child.Data == "li" {
				r.output.WriteString(strconv.Itoa(index))
				r.output.WriteString(". ")
				r.renderChildren(child)
				r.output.WriteByte('\n')
				index++
			}
		}
		r.ensureBlankLine()
	case "li":
		r.output.WriteString("- ")
		r.renderChildren(node)
		r.output.WriteByte('\n')
	case "code":
		r.output.WriteByte('`')
		r.renderChildren(node)
		r.output.WriteByte('`')
	case "pre":
		r.ensureBlankLine()
		r.output.WriteString("```\n")
		r.output.WriteString(strings.Trim(fallbackPreformattedText(node), "\n\r"))
		r.output.WriteString("\n```")
		r.ensureBlankLine()
	case "br":
		r.output.WriteByte('\n')
	default:
		r.renderChildren(node)
	}
}

func (r *fallbackMarkdownRenderer) renderChildren(node *html.Node) {
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		r.render(child)
	}
}

func (r *fallbackMarkdownRenderer) renderListItems(node *html.Node, marker string) {
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		if child.Type != html.ElementNode || child.Data != "li" {
			continue
		}
		r.output.WriteString(marker)
		r.output.WriteByte(' ')
		r.renderChildren(child)
		r.output.WriteByte('\n')
	}
}

func (r *fallbackMarkdownRenderer) writeText(value string) {
	collapsed := fallbackCollapseInlineWhitespace(value)
	if collapsed == "" {
		return
	}
	current := r.output.String()
	if len(value) > 0 && unicode.IsSpace([]rune(value)[0]) && current != "" && !strings.HasSuffix(current, " ") && !strings.HasSuffix(current, "\n") {
		r.output.WriteByte(' ')
	}
	r.output.WriteString(collapsed)
	if len(value) > 0 && unicode.IsSpace([]rune(value)[len([]rune(value))-1]) {
		r.output.WriteByte(' ')
	}
}

func (r *fallbackMarkdownRenderer) ensureBlankLine() {
	current := r.output.String()
	if current == "" || strings.HasSuffix(current, "\n\n") {
		return
	}
	if strings.HasSuffix(current, "\n") {
		r.output.WriteByte('\n')
		return
	}
	r.output.WriteString("\n\n")
}

func fallbackParseHTMLFragment(value string) []*html.Node {
	contextNode := &html.Node{Type: html.ElementNode, Data: "body"}
	nodes, err := html.ParseFragment(strings.NewReader(value), contextNode)
	if err == nil {
		return nodes
	}
	doc, err := html.Parse(strings.NewReader(value))
	if err != nil {
		return []*html.Node{{Type: html.TextNode, Data: value}}
	}
	return []*html.Node{doc}
}

func fallbackNodeText(node *html.Node) string {
	var output strings.Builder
	fallbackCollectText(node, &output)
	return fallbackCollapseInlineWhitespace(output.String())
}

func fallbackCollectText(node *html.Node, output *strings.Builder) {
	switch node.Type {
	case html.TextNode:
		output.WriteString(node.Data)
	case html.ElementNode:
		if node.Data == "br" {
			output.WriteByte('\n')
		}
		fallthrough
	default:
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			fallbackCollectText(child, output)
		}
	}
}

func fallbackPreformattedText(node *html.Node) string {
	var output strings.Builder
	fallbackCollectPreformattedText(node, &output)
	return output.String()
}

func fallbackCollectPreformattedText(node *html.Node, output *strings.Builder) {
	if node.Type == html.TextNode {
		output.WriteString(node.Data)
		return
	}
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		fallbackCollectPreformattedText(child, output)
	}
}

func fallbackAttr(node *html.Node, name string) string {
	for _, attr := range node.Attr {
		if attr.Key == name {
			return attr.Val
		}
	}
	return ""
}

func fallbackIsBlockElement(name string) bool {
	switch name {
	case "article", "blockquote", "div", "figcaption", "figure", "h1", "h2", "h3", "h4", "h5", "h6", "li", "p", "section":
		return true
	default:
		return false
	}
}

func fallbackCollapseInlineWhitespace(value string) string {
	return strings.Join(strings.Fields(value), " ")
}

func fallbackJoinTitleAndBody(title string, text string) string {
	parts := make([]string, 0, 2)
	for _, part := range []string{title, text} {
		if part = strings.TrimSpace(part); part != "" {
			parts = append(parts, part)
		}
	}
	return strings.Join(parts, "\n\n")
}

func fallbackTextWithFallback(text string, title string, sourceContent string) string {
	for _, value := range []string{text, title, sourceContent} {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}

func fallbackSummarize(value string) string {
	runes := []rune(value)
	if len(runes) > 80 {
		runes = runes[:80]
	}
	return string(runes)
}

func fallbackTruncateWeightedTextWithEllipsis(value string, limit int) string {
	value = strings.TrimSpace(value)
	if fallbackWeightedTextLen(value) <= limit {
		return value
	}
	const suffix = "..."
	budget := limit - len(suffix)
	if budget <= 0 {
		return fallbackTruncateTextByWeight(value, limit)
	}
	return strings.TrimSpace(fallbackTruncateTextByWeight(value, budget)) + suffix
}

func fallbackTruncateTextByWeight(value string, limit int) string {
	var output strings.Builder
	used := 0
	for _, r := range value {
		weight := fallbackRuneWeight(r)
		if used+weight > limit {
			break
		}
		output.WriteRune(r)
		used += weight
	}
	return output.String()
}

func fallbackWeightedTextLen(value string) int {
	length := 0
	last := 0
	for _, match := range fallbackURLPattern.FindAllStringIndex(value, -1) {
		length += fallbackWeightedSegmentLen(value[last:match[0]])
		length += fallbackXURLWeight
		last = match[1]
	}
	return length + fallbackWeightedSegmentLen(value[last:])
}

func fallbackWeightedSegmentLen(value string) int {
	length := 0
	for _, r := range value {
		length += fallbackRuneWeight(r)
	}
	return length
}

func fallbackRuneWeight(r rune) int {
	if r <= unicode.MaxASCII || unicode.Is(unicode.Latin, r) {
		return 1
	}
	return 2
}
