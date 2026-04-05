package render

import (
	"strings"
	"testing"
)

func TestShortenURL(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"https://example.com", "example.com"},
		{"https://example.com/", "example.com"},
		{"https://example.com/path/to/page", "example.com/path/to/page"},
		{"https://example.com/very/long/path/that/exceeds/fifty/characters/total/definitely", "example.com/very/long/path/that/exceeds/fifty/c..."},
		{"not a url at all", "not a url at all"},
	}
	for _, tt := range tests {
		got := ShortenURL(tt.input)
		if got != tt.want {
			t.Errorf("ShortenURL(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestLinkifyURLs(t *testing.T) {
	text := "visit https://example.com/page today"
	result := LinkifyURLs(text)
	// Should contain OSC 8 escape sequence
	if !strings.Contains(result, "\033]8;;") {
		t.Errorf("LinkifyURLs did not produce OSC 8 links: %q", result)
	}
	// Should contain the shortened label
	if !strings.Contains(result, "example.com/page") {
		t.Errorf("LinkifyURLs missing shortened label in: %q", result)
	}
	// Should still contain the surrounding text
	if !strings.Contains(result, "visit ") || !strings.Contains(result, " today") {
		t.Errorf("LinkifyURLs lost surrounding text: %q", result)
	}
}

func TestLinkifyWrappedLines(t *testing.T) {
	lines := []string{"no url here", "see https://example.com for details"}
	result := LinkifyWrappedLines(lines)
	if result[0] != lines[0] {
		t.Errorf("line without URL should be unchanged: got %q", result[0])
	}
	if !strings.Contains(result[1], "\033]8;;") {
		t.Errorf("line with URL should be linkified: got %q", result[1])
	}
}

func TestStripTrackers(t *testing.T) {
	tests := []struct {
		name string
		url  string
		want string
	}{
		{
			"utm params removed",
			"https://example.com/page?utm_source=email&utm_medium=newsletter&id=42",
			"https://example.com/page?id=42",
		},
		{
			"all tracker params removed leaves clean URL",
			"https://example.com/page?utm_source=email&utm_campaign=test",
			"https://example.com/page",
		},
		{
			"no tracking params unchanged",
			"https://example.com/page?id=42&name=test",
			"https://example.com/page?id=42&name=test",
		},
		{
			"empty query string",
			"https://example.com/page",
			"https://example.com/page",
		},
		{
			"facebook click ID",
			"https://example.com/?fbclid=abc123&real=yes",
			"https://example.com/?real=yes",
		},
		{
			"mailchimp IDs",
			"https://example.com/?mc_cid=abc&mc_eid=def",
			"https://example.com/",
		},
		{
			"case insensitive match",
			"https://example.com/?UTM_SOURCE=email",
			"https://example.com/",
		},
		{
			"hubspot params",
			"https://example.com/path?_hsenc=abc&_hsmi=123&page=1",
			"https://example.com/path?page=1",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := StripTrackers(tt.url)
			if got != tt.want {
				t.Errorf("StripTrackers(%q)\n  got  %q\n  want %q", tt.url, got, tt.want)
			}
		})
	}
}

func TestStripTrackersFromText(t *testing.T) {
	text := "Click here: https://example.com/page?utm_source=email&id=42 for more info"
	result := StripTrackersFromText(text)
	if strings.Contains(result, "utm_source") {
		t.Errorf("StripTrackersFromText should remove utm_source: %q", result)
	}
	if !strings.Contains(result, "id=42") {
		t.Errorf("StripTrackersFromText should preserve non-tracker params: %q", result)
	}
	if !strings.Contains(result, "Click here:") || !strings.Contains(result, "for more info") {
		t.Errorf("StripTrackersFromText should preserve surrounding text: %q", result)
	}
}
