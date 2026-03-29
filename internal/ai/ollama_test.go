package ai

import (
	"testing"
)

func TestIsVisionCapable(t *testing.T) {
	tests := []struct {
		model string
		want  bool
	}{
		{"gemma3:4b", true},
		{"gemma3:1b", true},
		{"gemma3:12b", true},
		{"gemma3n:4b", true},
		{"llava", true},
		{"llava:7b", true},
		{"bakllava", true},
		{"bakllava:7b", true},
		{"moondream", true},
		{"moondream:1.8b", true},
		{"minicpm-v", true},
		{"minicpm-v:8b", true},
		// case-insensitive
		{"Gemma3:4b", true},
		{"LLAVA:13b", true},
		// non-vision models
		{"gemma2:2b", false},
		{"gemma2", false},
		{"llama3", false},
		{"mistral", false},
		{"nomic-embed-text", false},
		{"", false},
	}

	for _, tt := range tests {
		got := IsVisionCapable(tt.model)
		if got != tt.want {
			t.Errorf("IsVisionCapable(%q) = %v, want %v", tt.model, got, tt.want)
		}
	}
}
