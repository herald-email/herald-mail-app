// Package iterm2 implements the iTerm2 inline image protocol.
// Images are encoded as OSC 1337 escape sequences that iTerm2 renders inline.
// Other terminals silently ignore the sequences, so the feature degrades gracefully.
package iterm2

import (
	"encoding/base64"
	"fmt"
	"os"
	"strings"
)

// IsSupported returns true when the current terminal is iTerm2.
func IsSupported() bool {
	return strings.Contains(os.Getenv("TERM_PROGRAM"), "iTerm")
}

// Render encodes imageData as an iTerm2 inline image escape sequence when the
// current terminal advertises iTerm2 support.
// width specifies the maximum character-cell width (0 = auto).
// height specifies the maximum character-cell height (0 = auto).
// Non-iTerm2 terminals will receive an empty string.
func Render(imageData []byte, width, height int) string {
	if !IsSupported() {
		return ""
	}
	return RenderInline(imageData, width, height)
}

// RenderInline encodes imageData as an iTerm2 inline image escape sequence
// without checking the current terminal. Use this for explicit user overrides.
func RenderInline(imageData []byte, width, height int) string {
	return RenderInlineInCellBox(imageData, width, height, width)
}

// RenderInlineInCellBox renders an image with explicit image dimensions while
// reserving and clearing a wider cell box. The wider clear box prevents stale
// split-view text from leaking beside iTerm2 raster output during full redraws.
func RenderInlineInCellBox(imageData []byte, width, height, clearWidth int) string {
	if len(imageData) == 0 {
		return ""
	}
	b64 := base64.StdEncoding.EncodeToString(imageData)
	args := fmt.Sprintf("inline=1;preserveAspectRatio=0;size=%d", len(imageData))
	if width > 0 {
		args += fmt.Sprintf(";width=%d", width)
	}
	if height > 0 {
		args += fmt.Sprintf(";height=%d", height)
	}
	var placement strings.Builder
	if width > 0 && height > 1 {
		moveRight := fmt.Sprintf("\033[%dC", width)
		clearCells := ""
		if clearWidth > 0 {
			clearCells = strings.Repeat(" ", clearWidth)
		}
		for i := 0; i < height-1; i++ {
			placement.WriteByte('\r')
			placement.WriteString("\033[2K")
			placement.WriteString(clearCells)
			placement.WriteByte('\r')
			placement.WriteString(moveRight)
			placement.WriteByte('\n')
		}
		placement.WriteByte('\r')
		placement.WriteString("\033[2K")
		placement.WriteString(clearCells)
		placement.WriteByte('\r')
		placement.WriteString(fmt.Sprintf("\033[%dA", height-1))
	}
	// OSC 1337 ; File=<args> : <base64data> BEL
	placement.WriteString(fmt.Sprintf("\033]1337;File=%s:%s\a", args, b64))
	return placement.String()
}
