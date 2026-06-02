package content

import (
	"bytes"
	"encoding/json"
	"fmt"
	stdhtml "html"
	"strings"

	nethtml "golang.org/x/net/html"
)

func HTMLToMarkdown(source string) (string, error) {
	nodes, err := nethtml.ParseFragment(strings.NewReader(source), nil)
	if err != nil {
		return "", err
	}

	var renderer markdownRenderer
	for _, node := range nodes {
		renderer.render(node)
	}
	return strings.TrimSpace(renderer.buf.String()), nil
}

func HTMLToText(source string) string {
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

func ExtractPublicationTitle(raw []byte) string {
	var config struct {
		Title string `json:"title"`
	}
	if err := json.Unmarshal(raw, &config); err != nil {
		return ""
	}
	return strings.TrimSpace(config.Title)
}

func isBlockElement(tag string) bool {
	switch tag {
	case "article", "blockquote", "div", "figcaption", "figure", "h1", "h2", "h3", "h4", "h5", "h6", "li", "p", "section":
		return true
	default:
		return false
	}
}

type markdownRenderer struct {
	buf bytes.Buffer
}

func (r *markdownRenderer) render(node *nethtml.Node) {
	switch node.Type {
	case nethtml.TextNode:
		r.writeText(node.Data)
	case nethtml.ElementNode:
		r.renderElement(node)
	default:
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			r.render(child)
		}
	}
}

func (r *markdownRenderer) writeText(value string) {
	collapsed := strings.Join(strings.Fields(value), " ")
	if collapsed == "" {
		return
	}
	if strings.HasPrefix(value, " ") && r.buf.Len() > 0 {
		last := r.buf.Bytes()[r.buf.Len()-1]
		if last != ' ' && last != '\n' {
			r.buf.WriteString(" ")
		}
	}
	r.buf.WriteString(collapsed)
	if strings.HasSuffix(value, " ") {
		r.buf.WriteString(" ")
	}
}

func (r *markdownRenderer) renderElement(node *nethtml.Node) {
	switch node.Data {
	case "h1", "h2", "h3", "h4", "h5", "h6":
		r.ensureBlankLine()
		level := int(node.Data[1] - '0')
		r.buf.WriteString(strings.Repeat("#", level))
		r.buf.WriteString(" ")
		r.renderChildren(node)
		r.ensureBlankLine()
	case "p":
		r.ensureBlankLine()
		r.renderChildren(node)
		r.ensureBlankLine()
	case "strong", "b":
		r.buf.WriteString("**")
		r.renderChildren(node)
		r.buf.WriteString("**")
	case "em", "i":
		r.buf.WriteString("*")
		r.renderChildren(node)
		r.buf.WriteString("*")
	case "a":
		label := strings.TrimSpace(markdownText(node))
		href := attrValue(node, "href")
		if label != "" && href != "" {
			r.buf.WriteString("[")
			r.buf.WriteString(label)
			r.buf.WriteString("](")
			r.buf.WriteString(href)
			r.buf.WriteString(")")
			return
		}
		r.renderChildren(node)
	case "img":
		src := attrValue(node, "src")
		if src == "" {
			return
		}
		r.ensureBlankLine()
		r.buf.WriteString("![")
		r.buf.WriteString(attrValue(node, "alt"))
		r.buf.WriteString("](")
		r.buf.WriteString(src)
		r.buf.WriteString(")")
		r.ensureBlankLine()
	case "blockquote":
		r.ensureBlankLine()
		text := strings.TrimSpace(markdownText(node))
		for _, line := range strings.Split(text, "\n") {
			line = strings.TrimSpace(line)
			if line != "" {
				r.buf.WriteString("> ")
				r.buf.WriteString(line)
				r.buf.WriteString("\n")
			}
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
			if child.Type == nethtml.ElementNode && child.Data == "li" {
				r.buf.WriteString(fmt.Sprintf("%d. ", index))
				r.renderChildren(child)
				r.buf.WriteString("\n")
				index++
			}
		}
		r.ensureBlankLine()
	case "li":
		r.buf.WriteString("- ")
		r.renderChildren(node)
		r.buf.WriteString("\n")
	case "code":
		r.buf.WriteString("`")
		r.renderChildren(node)
		r.buf.WriteString("`")
	case "pre":
		r.ensureBlankLine()
		r.buf.WriteString("```\n")
		r.buf.WriteString(trimOuterNewlines(preformattedText(node)))
		r.buf.WriteString("\n```")
		r.ensureBlankLine()
	case "br":
		r.buf.WriteString("\n")
	default:
		r.renderChildren(node)
	}
}

func (r *markdownRenderer) renderChildren(node *nethtml.Node) {
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		r.render(child)
	}
}

func (r *markdownRenderer) renderListItems(node *nethtml.Node, marker string) {
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		if child.Type == nethtml.ElementNode && child.Data == "li" {
			r.buf.WriteString(marker)
			r.buf.WriteString(" ")
			r.renderChildren(child)
			r.buf.WriteString("\n")
		}
	}
}

func (r *markdownRenderer) ensureBlankLine() {
	value := r.buf.String()
	if value == "" || strings.HasSuffix(value, "\n\n") {
		return
	}
	if strings.HasSuffix(value, "\n") {
		r.buf.WriteString("\n")
		return
	}
	r.buf.WriteString("\n\n")
}

func markdownText(node *nethtml.Node) string {
	var buf strings.Builder
	var walk func(*nethtml.Node)
	walk = func(current *nethtml.Node) {
		if current.Type == nethtml.TextNode {
			buf.WriteString(current.Data)
			return
		}
		for child := current.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(node)
	return strings.Join(strings.Fields(buf.String()), " ")
}

func preformattedText(node *nethtml.Node) string {
	var buf strings.Builder
	var walk func(*nethtml.Node)
	walk = func(current *nethtml.Node) {
		if current.Type == nethtml.TextNode {
			buf.WriteString(current.Data)
			return
		}
		for child := current.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(node)
	return buf.String()
}

func trimOuterNewlines(value string) string {
	return strings.Trim(value, "\n\r")
}

func attrValue(node *nethtml.Node, name string) string {
	for _, attr := range node.Attr {
		if attr.Key == name {
			return strings.TrimSpace(attr.Val)
		}
	}
	return ""
}
