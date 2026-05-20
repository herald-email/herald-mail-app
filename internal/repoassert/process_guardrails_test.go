package repoassert

import (
	"os"
	"strings"
	"testing"
)

func TestAgentGuidanceIncludesProcessGuardrails(t *testing.T) {
	for _, path := range []string{"AGENTS.md", "CLAUDE.md"} {
		data, err := os.ReadFile(repoPath(t, path))
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		text := string(data)
		for _, want := range []string{
			"Verification budget",
			"Second-failure rule",
			"Degradation check",
			"No silent scope substitution",
			"Superpowers throttle",
			"Verification surface",
		} {
			if !strings.Contains(text, want) {
				t.Fatalf("%s missing guardrail %q", path, want)
			}
		}
	}
}

func TestReportTemplateRequiresVerificationSurface(t *testing.T) {
	data, err := os.ReadFile(repoPath(t, "engineering", "testplans", "REPORT_TEMPLATE.md"))
	if err != nil {
		t.Fatalf("read report template: %v", err)
	}
	text := string(data)
	for _, want := range []string{"demo", "virtual lab", "live config", "tmux", "ttyd", "SSH", "MCP", "daemon"} {
		if !strings.Contains(text, want) {
			t.Fatalf("report template missing surface %q", want)
		}
	}
}
