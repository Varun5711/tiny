package qrcode

import (
	"encoding/base64"
	"fmt"
	"strings"

	"github.com/skip2/go-qrcode"
)

func GenerateQRCode(url string) (string, error) {
	png, err := qrcode.Encode(url, qrcode.Medium, 256)
	if err != nil {
		return "", fmt.Errorf("failed to generate QR code: %w", err)
	}

	encoded := base64.StdEncoding.EncodeToString(png)

	return fmt.Sprintf("data:image/png;base64,%s", encoded), nil
}

func GenerateQRCodeASCII(url string) (string, error) {
	qr, err := qrcode.New(url, qrcode.Low)
	if err != nil {
		return "", fmt.Errorf("failed to generate QR code: %w", err)
	}

	bitmap := qr.Bitmap()

	var sb strings.Builder

	for i := 0; i < len(bitmap); i++ {
		for j := 0; j < len(bitmap[i]); j++ {
			if bitmap[i][j] {
				sb.WriteString("██")
			} else {
				sb.WriteString("  ")
			}
		}
		sb.WriteString("\n")
	}

	return sb.String(), nil
}
