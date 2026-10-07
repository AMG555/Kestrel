package ilink

import (
	"encoding/base64"
	"fmt"
	"strings"

	"github.com/skip2/go-qrcode"
)

// QRCodeDataURL encodes scan content (typically a liteapp URL) as a PNG data URL for web display.
// Note: qrcode_img_content is not a direct image URL and cannot be used as <img src>.
func QRCodeDataURL(content string, size int) (string, error) {
	content = strings.TrimSpace(content)
	if content == "" {
		return "", fmt.Errorf("empty qr content")
	}
	if size <= 0 {
		size = 256
	}
	png, err := qrcode.Encode(content, qrcode.Medium, size)
	if err != nil {
		return "", err
	}
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(png), nil
}
