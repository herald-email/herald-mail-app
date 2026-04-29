// Package kittyimg implements the Kitty graphics protocol for inline previews.
package kittyimg

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/png"
	"os"
	"strings"
)

const kittyChunkSize = 4096

// IsSupported returns true when the current terminal looks Kitty-protocol
// capable. This intentionally uses stable environment hints rather than a
// runtime terminal query so it cannot interfere with TUI input handling.
func IsSupported() bool {
	if os.Getenv("KITTY_WINDOW_ID") != "" {
		return true
	}
	term := strings.ToLower(os.Getenv("TERM"))
	if term == "xterm-kitty" || term == "xterm-ghostty" {
		return true
	}
	termProgram := strings.ToLower(os.Getenv("TERM_PROGRAM"))
	return strings.Contains(termProgram, "kitty") || strings.Contains(termProgram, "ghostty")
}

// Render emits a Kitty graphics sequence only when the current terminal looks
// compatible. Unsupported terminals receive an empty string.
func Render(imageData []byte, width, height int) string {
	if !IsSupported() {
		return ""
	}
	rendered, err := RenderInline(imageData, width, height)
	if err != nil {
		return ""
	}
	return rendered
}

// DeleteVisiblePlacements emits a quiet Kitty delete command for all visible
// image placements. Text erases do not affect graphics placements, so callers
// should use this before repainting a Kitty-backed viewport.
func DeleteVisiblePlacements() string {
	return "\033_Ga=d,d=A,q=2\033\\"
}

// RenderInline transcodes imageData to PNG and emits direct-transfer Kitty
// graphics commands. It does not check terminal support and is used for an
// explicit user override.
func RenderInline(imageData []byte, width, height int) (string, error) {
	img, _, err := image.Decode(bytes.NewReader(imageData))
	if err != nil {
		return "", err
	}
	bounds := img.Bounds()
	var pngBuf bytes.Buffer
	if err := png.Encode(&pngBuf, img); err != nil {
		return "", err
	}

	b64 := base64.StdEncoding.EncodeToString(pngBuf.Bytes())
	if b64 == "" {
		return "", nil
	}

	var out strings.Builder
	for offset := 0; offset < len(b64); offset += kittyChunkSize {
		end := offset + kittyChunkSize
		if end > len(b64) {
			end = len(b64)
		}
		more := 0
		if end < len(b64) {
			more = 1
		}
		if offset == 0 {
			out.WriteString("\033_Ga=T,f=100,t=d,q=2,C=1")
			out.WriteString(",s=")
			out.WriteString(itoa(bounds.Dx()))
			out.WriteString(",v=")
			out.WriteString(itoa(bounds.Dy()))
			if width > 0 {
				out.WriteString(",c=")
				out.WriteString(itoa(width))
			}
			if height > 0 {
				out.WriteString(",r=")
				out.WriteString(itoa(height))
			}
			out.WriteString(",m=")
			out.WriteString(itoa(more))
			out.WriteString(";")
		} else {
			out.WriteString("\033_Gq=2,m=")
			out.WriteString(itoa(more))
			out.WriteString(";")
		}
		out.WriteString(b64[offset:end])
		out.WriteString("\033\\")
	}
	return out.String(), nil
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}
