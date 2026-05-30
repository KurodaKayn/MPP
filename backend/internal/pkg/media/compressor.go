package media

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	_ "image/gif"
	"image/jpeg"
	_ "image/png"
	"io"
	"net/http"
	neturl "net/url"
	"strings"

	"golang.org/x/image/draw"
)

const MaxWechatSize = 2 * 1024 * 1024 // 2MB

// DownloadAndProcess fetches an image from a URL or data URL and compresses it if it exceeds WeChat's size limit.
func DownloadAndProcess(sourceURL string) ([]byte, error) {
	data, err := loadImageBytes(sourceURL)
	if err != nil {
		return nil, err
	}

	if len(data) < MaxWechatSize {
		return data, nil
	}

	// Start compression pipeline
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("failed to decode image: %w", err)
	}

	// 1. Try quality reduction
	buf := &bytes.Buffer{}
	quality := 80
	for {
		buf.Reset()
		err = jpeg.Encode(buf, img, &jpeg.Options{Quality: quality})
		if err != nil {
			return nil, err
		}
		if buf.Len() < MaxWechatSize || quality <= 20 {
			break
		}
		quality -= 20
	}

	// 2. If still too large, downscale by 50%
	if buf.Len() >= MaxWechatSize {
		bounds := img.Bounds()
		width := bounds.Dx() / 2
		height := bounds.Dy() / 2
		newImg := image.NewRGBA(image.Rect(0, 0, width, height))
		draw.BiLinear.Scale(newImg, newImg.Bounds(), img, bounds, draw.Over, nil)

		buf.Reset()
		if err := jpeg.Encode(buf, newImg, &jpeg.Options{Quality: 60}); err != nil {
			return nil, err
		}
	}

	return buf.Bytes(), nil
}

func loadImageBytes(sourceURL string) ([]byte, error) {
	if strings.HasPrefix(sourceURL, "data:") {
		return decodeDataURL(sourceURL)
	}

	resp, err := http.Get(sourceURL)
	if err != nil {
		return nil, fmt.Errorf("failed to download image: %w", err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read image body: %w", err)
	}

	return data, nil
}

func decodeDataURL(value string) ([]byte, error) {
	commaIndex := strings.Index(value, ",")
	if commaIndex < 0 {
		return nil, fmt.Errorf("invalid data URL")
	}

	metadata := value[:commaIndex]
	payload := value[commaIndex+1:]
	if strings.Contains(metadata, ";base64") {
		data, err := base64.StdEncoding.DecodeString(payload)
		if err != nil {
			return nil, fmt.Errorf("failed to decode data URL: %w", err)
		}
		return data, nil
	}

	decoded, err := neturl.QueryUnescape(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to decode data URL: %w", err)
	}
	return []byte(decoded), nil
}
