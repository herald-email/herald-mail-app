package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExpandPath(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("could not get home dir: %v", err)
	}

	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{
			name:  "tilde expands to home",
			input: "~/.herald/conf.yaml",
			want:  filepath.Join(home, ".herald/conf.yaml"),
		},
		{
			name:  "tilde-only expands to home",
			input: "~",
			want:  home,
		},
		{
			name:  "absolute path unchanged",
			input: "/etc/herald/conf.yaml",
			want:  "/etc/herald/conf.yaml",
		},
		{
			name:  "relative path unchanged",
			input: "proton.yaml",
			want:  "proton.yaml",
		},
		{
			name:  "relative path with dir unchanged",
			input: "config/proton.yaml",
			want:  "config/proton.yaml",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := expandPath(tc.input)
			if tc.wantErr {
				if err == nil {
					t.Errorf("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Errorf("expandPath(%q) = %q, want %q", tc.input, got, tc.want)
			}
			// Expanded result must not contain a literal tilde
			if strings.Contains(got, "~") {
				t.Errorf("result still contains tilde: %q", got)
			}
		})
	}
}
