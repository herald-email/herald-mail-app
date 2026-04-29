package main

import (
	"os"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestFundingYAMLUsesValidExternalFundingLinks(t *testing.T) {
	data, err := os.ReadFile(".github/FUNDING.yml")
	if err != nil {
		t.Fatalf("read funding file: %v", err)
	}

	var funding map[string]any
	if err := yaml.Unmarshal(data, &funding); err != nil {
		t.Fatalf("parse funding file: %v", err)
	}

	if fundingValueSet(funding["github"]) {
		t.Fatal("github Sponsors usernames must stay empty unless the listed account is enrolled in GitHub Sponsors")
	}

	thanksDev, ok := funding["thanks_dev"].(string)
	if !ok || strings.TrimSpace(thanksDev) == "" {
		t.Fatal("thanks_dev should point to the repository maintainer's thanks.dev profile")
	}
	if !strings.HasPrefix(strings.TrimSpace(thanksDev), "u/gh/") {
		t.Fatalf("thanks_dev = %q, want GitHub's documented u/gh/USERNAME syntax", thanksDev)
	}
}

func fundingValueSet(value any) bool {
	switch typed := value.(type) {
	case nil:
		return false
	case string:
		return strings.TrimSpace(typed) != ""
	case []any:
		for _, item := range typed {
			if fundingValueSet(item) {
				return true
			}
		}
		return false
	case []string:
		for _, item := range typed {
			if strings.TrimSpace(item) != "" {
				return true
			}
		}
		return false
	default:
		return true
	}
}
