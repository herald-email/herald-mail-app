package app

import (
	"encoding/json"
	"strings"
)

// filterPayload is the JSON structure inside a <filter> block.
type filterPayload struct {
	IDs   []string `json:"ids"`
	Label string   `json:"label"`
}

// parseChatFilter scans text for the first <filter>...</filter> block,
// parses the JSON payload, and returns the ids and label.
// Returns found=false if no block is present, JSON is malformed, or ids is empty.
func parseChatFilter(text string) (ids []string, label string, found bool) {
	const open = "<filter>"
	const close = "</filter>"

	start := strings.Index(text, open)
	if start < 0 {
		return nil, "", false
	}
	end := strings.Index(text[start:], close)
	if end < 0 {
		return nil, "", false
	}
	raw := text[start+len(open) : start+end]
	raw = strings.TrimSpace(raw)

	var payload filterPayload
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return nil, "", false
	}
	if len(payload.IDs) == 0 {
		return nil, "", false
	}
	return payload.IDs, payload.Label, true
}

// stripChatFilter removes all <filter>...</filter> blocks from text,
// for clean display in the chat panel.
func stripChatFilter(text string) string {
	const open = "<filter>"
	const close = "</filter>"
	for {
		start := strings.Index(text, open)
		if start < 0 {
			break
		}
		end := strings.Index(text[start:], close)
		if end < 0 {
			break
		}
		text = text[:start] + text[start+end+len(close):]
	}
	return strings.TrimSpace(text)
}
