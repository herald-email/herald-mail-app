package ai

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

var quickReplyListPrefix = regexp.MustCompile(`^\s*(?:[-*+]\s+|\d+[.)]\s+)`)

func parseQuickReplySuggestions(reply string) ([]string, error) {
	reply = strings.TrimSpace(stripMarkdownFences(reply))
	if reply == "" {
		return nil, fmt.Errorf("empty quick reply response")
	}

	if suggestions, ok := parseJSONStringArray(reply); ok {
		return suggestions, nil
	}

	if start := strings.Index(reply, "["); start >= 0 {
		if end := strings.LastIndex(reply, "]"); end > start {
			if suggestions, ok := parseJSONStringArray(reply[start : end+1]); ok {
				return suggestions, nil
			}
		}
	}

	lines := strings.Split(reply, "\n")
	suggestions := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		line = quickReplyListPrefix.ReplaceAllString(line, "")
		line = strings.TrimSpace(line)
		line = strings.Trim(line, "`\"' ")
		if line == "" {
			continue
		}
		suggestions = append(suggestions, line)
	}

	suggestions = sanitizeSuggestions(suggestions)
	if len(suggestions) == 0 {
		return nil, fmt.Errorf("no quick replies found")
	}
	return suggestions, nil
}

func parseJSONStringArray(raw string) ([]string, bool) {
	var suggestions []string
	if err := json.Unmarshal([]byte(raw), &suggestions); err != nil {
		return nil, false
	}
	suggestions = sanitizeSuggestions(suggestions)
	if len(suggestions) == 0 {
		return nil, false
	}
	return suggestions, true
}

func sanitizeSuggestions(in []string) []string {
	out := make([]string, 0, len(in))
	seen := map[string]bool{}
	for _, item := range in {
		item = strings.TrimSpace(item)
		item = strings.Trim(item, "`\"' ")
		if item == "" {
			continue
		}
		if seen[item] {
			continue
		}
		seen[item] = true
		out = append(out, item)
		if len(out) == 8 {
			break
		}
	}
	return out
}
