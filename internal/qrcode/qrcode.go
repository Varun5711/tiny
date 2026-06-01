// Package qrcode generates QR code images for shortened URLs. Two output
// formats are supported: a Base64-encoded PNG data URI for embedding in HTML
// responses and API payloads, and an ASCII-art representation for terminal
// display in the TUI client. Both use the skip2/go-qrcode library under
// the hood.
package qrcode

import (
	"encoding/base64"
	"fmt"
	"strings"

	"github.com/skip2/go-qrcode"
)

// GenerateQRCode encodes the given URL into a 256x256 PNG QR code and
// returns it as a data URI string (data:image/png;base64,...). This format
// can be embedded directly in an HTML <img> tag or returned in a JSON API
// response without requiring the client to make a second request for the
// image. Medium error-correction is used as a balance between data density
// and scan reliability.
func GenerateQRCode(url string) (string, error) {
	png, err := qrcode.Encode(url, qrcode.Medium, 256)
	if err != nil {
		return "", fmt.Errorf("failed to generate QR code: %w", err)
	}

	encoded := base64.StdEncoding.EncodeToString(png)

	return fmt.Sprintf("data:image/png;base64,%s", encoded), nil
}

// GenerateQRCodeASCII produces a text-based QR code using full-block Unicode
// characters. This is designed for the TUI client where bitmap images cannot
// be rendered. Low error-correction is chosen here because terminal fonts
// render blocks at fixed sizes, so the code needs to stay compact to fit
// typical terminal widths. Each module is doubled horizontally ("  " or "██")
// to produce roughly square cells in monospaced fonts.
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
