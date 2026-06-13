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

func ProcessHTMLImageSources(htmlContent string, sources []string, processor ProcessorFunc, uploader UploaderFunc) (string, error) {
	sourceSet := make(map[string]struct{}, len(sources))
	for _, source := range sources {
		source = strings.TrimSpace(source)
		if source == "" {
			continue
		}
		sourceSet[source] = struct{}{}
	}
	if len(sourceSet) == 0 {
		return htmlContent, nil
	}

	doc, err := html.Parse(strings.NewReader(htmlContent))
	if err != nil {
		return "", fmt.Errorf("failed to parse html: %w", err)
	}

	replacements := make(map[string]string, len(sourceSet))
	var walk func(*html.Node) error
	walk = func(n *html.Node) error {
		if n.Type == html.ElementNode && n.Data == "img" {
			for i, attr := range n.Attr {
				if attr.Key != "src" {
					continue
				}
				source := strings.TrimSpace(attr.Val)
				if _, ok := sourceSet[source]; !ok {
					break
				}
				replacement, ok := replacements[source]
				if !ok {
					objectRef, err := processor(source)
					if err != nil {
						return err
					}
					replacement, err = uploader(objectRef)
					if err != nil {
						return err
					}
					replacements[source] = replacement
				}
				n.Attr[i].Val = replacement
				break
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			if err := walk(c); err != nil {
				return err
			}
		}
		return nil
	}
	if err := walk(doc); err != nil {
		return "", err
	}

	var buf bytes.Buffer
	if err := html.Render(&buf, doc); err != nil {
		return "", fmt.Errorf("failed to render html: %w", err)
	}

	return buf.String(), nil
}
