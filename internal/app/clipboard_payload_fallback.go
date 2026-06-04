//go:build !darwin || !cgo

package app

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func writeSystemPreviewClipboard(payload previewClipboardPayload) (string, error) {
	if payload.Image != nil && len(payload.Image.Data) > 0 {
		ext := ".img"
		switch strings.ToLower(payload.Image.MIMEType) {
		case "image/png":
			ext = ".png"
		case "image/jpeg", "image/jpg":
			ext = ".jpg"
		case "image/gif":
			ext = ".gif"
		}
		name := payload.ImageName
		if strings.TrimSpace(name) == "" {
			name = "herald-preview-image"
		}
		path := filepath.Join(os.TempDir(), safePreviewClipboardFilename(name)+ext)
		if err := os.WriteFile(path, payload.Image.Data, 0o600); err != nil {
			return "", err
		}
		if err := copyPlainTextToClipboard(path); err != nil {
			return "", err
		}
		return fmt.Sprintf("Image saved and path copied: %s", path), nil
	}
	if payload.Plain == "" {
		payload.Plain = payload.HTML
	}
	if err := copyPlainTextToClipboard(payload.Plain); err != nil {
		return "", err
	}
	if payload.HTML != "" {
		return "Text copied (plain fallback)", nil
	}
	return "Text copied", nil
}

func safePreviewClipboardFilename(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return "herald-preview-image"
	}
	var b strings.Builder
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '-', r == '_':
			b.WriteRune(r)
		default:
			b.WriteByte('-')
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		out = "herald-preview-image"
	}
	if len(out) > 80 {
		out = out[:80]
	}
	return out
}
