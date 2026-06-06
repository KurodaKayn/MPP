package html

import (
	"bytes"
	stdhtml "html"
	"net/url"
	"strings"
	"unicode"

	xhtml "golang.org/x/net/html"
)

var droppedStoredHTMLElements = map[string]struct{}{
	"base":     {},
	"embed":    {},
	"form":     {},
	"iframe":   {},
	"link":     {},
	"math":     {},
	"meta":     {},
	"object":   {},
	"script":   {},
	"style":    {},
	"svg":      {},
	"template": {},
}

var storedHTMLURLAttrs = map[string]struct{}{
	"action":     {},
	"formaction": {},
	"href":       {},
	"poster":     {},
	"src":        {},
	"xlink:href": {},
}

var droppedStoredHTMLAttrs = map[string]struct{}{
	"srcdoc": {},
	"srcset": {},
	"style":  {},
}

// SanitizeStoredHTML strips active content and unsafe URL protocols from stored rich text.
func SanitizeStoredHTML(raw string) string {
	source := strings.TrimSpace(raw)
	if source == "" {
		return ""
	}

	doc, err := xhtml.Parse(strings.NewReader(source))
	if err != nil {
		return stdhtml.EscapeString(source)
	}

	root := storedHTMLBodyNode(doc)
	if root == nil {
		root = doc
	}
	sanitizeStoredHTMLChildren(root)

	var buf bytes.Buffer
	for child := root.FirstChild; child != nil; child = child.NextSibling {
		if err := xhtml.Render(&buf, child); err != nil {
			return stdhtml.EscapeString(source)
		}
	}
	return strings.TrimSpace(buf.String())
}

func storedHTMLBodyNode(node *xhtml.Node) *xhtml.Node {
	if node == nil {
		return nil
	}
	if node.Type == xhtml.ElementNode && strings.EqualFold(node.Data, "body") {
		return node
	}
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		if body := storedHTMLBodyNode(child); body != nil {
			return body
		}
	}
	return nil
}

func sanitizeStoredHTMLChildren(parent *xhtml.Node) {
	for child := parent.FirstChild; child != nil; {
		next := child.NextSibling
		if shouldDropStoredHTMLNode(child) {
			parent.RemoveChild(child)
			child = next
			continue
		}

		if child.Type == xhtml.ElementNode {
			child.Data = strings.ToLower(child.Data)
			child.Attr = sanitizeStoredHTMLAttrs(child.Attr)
		}

		sanitizeStoredHTMLChildren(child)
		child = next
	}
}

func shouldDropStoredHTMLNode(node *xhtml.Node) bool {
	if node.Type == xhtml.CommentNode || node.Type == xhtml.DoctypeNode {
		return true
	}
	if node.Type != xhtml.ElementNode {
		return false
	}
	_, drop := droppedStoredHTMLElements[strings.ToLower(node.Data)]
	return drop
}

func sanitizeStoredHTMLAttrs(attrs []xhtml.Attribute) []xhtml.Attribute {
	safeAttrs := attrs[:0]
	for _, attr := range attrs {
		key := strings.ToLower(strings.TrimSpace(attr.Key))
		if key == "" || strings.HasPrefix(key, "on") || strings.HasPrefix(key, "xmlns") {
			continue
		}
		if _, drop := droppedStoredHTMLAttrs[key]; drop {
			continue
		}
		if _, isURLAttr := storedHTMLURLAttrs[key]; isURLAttr && !isSafeStoredHTMLURL(key, attr.Val) {
			continue
		}

		attr.Key = key
		safeAttrs = append(safeAttrs, attr)
	}
	return safeAttrs
}

func isSafeStoredHTMLURL(attrName, raw string) bool {
	normalized := normalizedStoredHTMLURL(raw)
	if normalized == "" {
		return false
	}

	if strings.HasPrefix(normalized, "#") || strings.HasPrefix(normalized, "/") {
		return true
	}

	parsed, err := url.Parse(normalized)
	if err != nil {
		return false
	}
	if parsed.Scheme == "" {
		return true
	}

	switch strings.ToLower(attrName) {
	case "href", "xlink:href":
		return parsed.Scheme == "http" || parsed.Scheme == "https" || parsed.Scheme == "mailto" || parsed.Scheme == "tel"
	case "src", "poster":
		return parsed.Scheme == "http" ||
			parsed.Scheme == "https" ||
			parsed.Scheme == "blob" ||
			normalizedStoredHTMLMediaRef(normalized) ||
			normalizedStoredHTMLDataImage(normalized)
	default:
		return parsed.Scheme == "http" || parsed.Scheme == "https"
	}
}

func normalizedStoredHTMLURL(raw string) string {
	unescaped := stdhtml.UnescapeString(strings.TrimSpace(raw))
	return strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) || unicode.IsControl(r) {
			return -1
		}
		return unicode.ToLower(r)
	}, unescaped)
}

func normalizedStoredHTMLMediaRef(value string) bool {
	return strings.HasPrefix(value, "mpp://media/")
}

func normalizedStoredHTMLDataImage(value string) bool {
	if !strings.HasPrefix(value, "data:image/") {
		return false
	}
	for _, imageType := range []string{"png", "jpg", "jpeg", "gif", "webp", "avif"} {
		if strings.HasPrefix(value, "data:image/"+imageType+";base64,") {
			return true
		}
	}
	return false
}
