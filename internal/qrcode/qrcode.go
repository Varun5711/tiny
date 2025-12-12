package qrcode

import (
	"encoding/base64"
	"fmt"

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
