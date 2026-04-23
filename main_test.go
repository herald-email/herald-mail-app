package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestConfigNeedsOnboarding_MissingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing.yaml")

	needs, err := configNeedsOnboarding(path)
	if err != nil {
		t.Fatalf("configNeedsOnboarding returned error: %v", err)
	}
	if !needs {
		t.Fatalf("expected missing config to require onboarding")
	}
}

func TestConfigNeedsOnboarding_EmptyOrWhitespaceFile(t *testing.T) {
	cases := map[string]string{
		"empty":      "",
		"whitespace": "   \n\t",
	}
	for name, contents := range cases {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "empty.yaml")
			if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
				t.Fatalf("write temp config: %v", err)
			}

			needs, err := configNeedsOnboarding(path)
			if err != nil {
				t.Fatalf("configNeedsOnboarding returned error: %v", err)
			}
			if !needs {
				t.Fatalf("expected empty or whitespace-only config to require onboarding")
			}
		})
	}
}

func TestConfigNeedsOnboarding_NonEmptyFileDoesNotTriggerOnboarding(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte("credentials:\n"), 0o600); err != nil {
		t.Fatalf("write temp config: %v", err)
	}

	needs, err := configNeedsOnboarding(path)
	if err != nil {
		t.Fatalf("configNeedsOnboarding returned error: %v", err)
	}
	if needs {
		t.Fatalf("expected non-empty config file to skip onboarding and fail later via normal validation")
	}
}
