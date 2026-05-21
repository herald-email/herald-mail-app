package repoassert

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

func TestVirtualMailLabCoverageDocumentsAllScenarios(t *testing.T) {
	scenarioData, err := os.ReadFile(repoPath(t, "internal", "testmail", "scenario.go"))
	if err != nil {
		t.Fatalf("read scenario.go: %v", err)
	}
	coverageData, err := os.ReadFile(repoPath(t, "engineering", "testplans", "VIRTUAL_MAIL_LAB_COVERAGE.md"))
	if err != nil {
		t.Fatalf("read virtual mail lab coverage matrix: %v", err)
	}
	coverage := string(coverageData)

	re := regexp.MustCompile(`Scenario[A-Za-z0-9]+\s+ScenarioName\s*=\s*"([^"]+)"`)
	matches := re.FindAllStringSubmatch(string(scenarioData), -1)
	if len(matches) == 0 {
		t.Fatal("no ScenarioName constants found")
	}
	for _, match := range matches {
		scenario := match[1]
		if !strings.Contains(coverage, "`"+scenario+"`") {
			t.Fatalf("coverage matrix missing scenario %q", scenario)
		}
	}
}

func TestVirtualMailLabCoverageMentionsReportSurfaces(t *testing.T) {
	templateData, err := os.ReadFile(repoPath(t, "engineering", "testplans", "REPORT_TEMPLATE.md"))
	if err != nil {
		t.Fatalf("read report template: %v", err)
	}
	coverageData, err := os.ReadFile(repoPath(t, "engineering", "testplans", "VIRTUAL_MAIL_LAB_COVERAGE.md"))
	if err != nil {
		t.Fatalf("read virtual mail lab coverage matrix: %v", err)
	}
	coverage := string(coverageData)

	re := regexp.MustCompile(`- \[ \] ` + "`" + `([^` + "`" + `]+)` + "`")
	matches := re.FindAllStringSubmatch(string(templateData), -1)
	if len(matches) == 0 {
		t.Fatal("no report-template surfaces found")
	}
	for _, match := range matches {
		surface := match[1]
		if !strings.Contains(coverage, "`"+surface+"`") {
			t.Fatalf("coverage matrix missing report surface %q", surface)
		}
	}
}

func TestVirtualMailLabClosureLinksReportAndMatrix(t *testing.T) {
	data, err := os.ReadFile(repoPath(t, "docs", "superpowers", "specs", "2026-05-21-virtual-mail-lab-roadmap-closure.md"))
	if err != nil {
		t.Fatalf("read virtual mail lab closure spec: %v", err)
	}
	text := string(data)
	for _, want := range []string{
		"reports/PROCESS_REPORT_2026-05-20_codex-archive-and-virtual-mail-lab.md",
		"engineering/testplans/VIRTUAL_MAIL_LAB_COVERAGE.md",
		"implemented",
		"deferred",
		"intentionally out of scope",
		"herald --testmail-*",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("closure spec missing %q", want)
		}
	}
}
