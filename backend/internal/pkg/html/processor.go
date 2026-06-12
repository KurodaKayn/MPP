package html

import (
	"bytes"
	"fmt"
	"strings"

	"golang.org/x/net/html"
)

// UploaderFunc is a function that takes image data and returns a permanent URL.
type UploaderFunc func(imgData []byte) (string, error)

// DownloaderFunc is a function that takes a URL and returns image data.
type DownloaderFunc func(url string) ([]byte, error)

// ProcessHTMLImages parses HTML, finds <img> tags, processes them via external functions,
// and replaces their src attributes with new URLs.
func ProcessHTMLImages(htmlContent string, downloader DownloaderFunc, uploader UploaderFunc) (string, error) {
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
					imgData, err := downloader(attr.Val)
					if err == nil {
						newURL, err := uploader(imgData)
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
