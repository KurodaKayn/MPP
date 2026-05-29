package media

import (
	"bytes"
	"fmt"
	"image"
	_ "image/gif"
	"image/jpeg"
	_ "image/png"
	"io"
	"net/http"

	"golang.org/x/image/draw"
)

const MaxWechatSize = 2 * 1024 * 1024 // 2MB

// DownloadAndProcess fetches an image from a URL and compresses it if it exceeds WeChat's size limit.
func DownloadAndProcess(url string) ([]byte, error) {
	resp, err := http.Get(url)
	if err != nil {
		return nil, fmt.Errorf("failed to download image: %w", err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read image body: %w", err)
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
