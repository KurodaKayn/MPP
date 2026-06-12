package html

import (
	"bytes"
	"fmt"
	"strings"

	"golang.org/x/net/html"
)

// UploaderFunc is a function that takes a processed image object ref and returns a permanent URL.
type UploaderFunc func(objectRef string) (string, error)

// ProcessorFunc is a function that takes a URL and returns a processed image object ref.
type ProcessorFunc func(url string) (string, error)

// ProcessHTMLImages parses HTML, finds <img> tags, processes them via external functions,
// and replaces their src attributes with new URLs.
func ProcessHTMLImages(htmlContent string, processor ProcessorFunc, uploader UploaderFunc) (string, error) {
	doc, err := html.Parse(strings.NewReader(htmlContent))
	if err != nil {
		return "", fmt.Errorf("failed to parse html: %w", err)
	}

	var f func(*html.Node)
	f = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "img" {
			for i, attr := range n.Attr {
				if attr.Key == "src" {
					// Process and upload the image.
					objectRef, err := processor(attr.Val)
					if err == nil {
						newURL, err := uploader(objectRef)
						if err == nil {
							n.Attr[i].Val = newURL
						}
					}
					break
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			f(c)
		}
	}
	f(doc)

	var buf bytes.Buffer
	if err := html.Render(&buf, doc); err != nil {
		return "", fmt.Errorf("failed to render html: %w", err)
	}

	return buf.String(), nil
}
