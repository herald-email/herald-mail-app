package repoassert

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestGitHubActionsUseNode24MajorVersions(t *testing.T) {
	minNode24Major := map[string]int{
		"actions/checkout":            5,
		"actions/setup-go":            6,
		"actions/upload-artifact":     6,
		"actions/download-artifact":   7,
		"softprops/action-gh-release": 3,
	}
	refPattern := regexp.MustCompile(`^([^@]+)@v([0-9]+)(?:\.|$)`)

	files := workflowFiles(t)
	if len(files) == 0 {
		t.Fatal("expected at least one GitHub Actions workflow")
	}

	for _, file := range files {
		var root yaml.Node
		data, err := os.ReadFile(file)
		if err != nil {
			t.Fatalf("read %s: %v", file, err)
		}
		if err := yaml.Unmarshal(data, &root); err != nil {
			t.Fatalf("parse %s: %v", file, err)
		}
		for _, use := range workflowUses(&root) {
			match := refPattern.FindStringSubmatch(use)
			if match == nil {
				continue
			}
			action := match[1]
			minMajor, ok := minNode24Major[action]
			if !ok {
				continue
			}
			major, err := strconv.Atoi(match[2])
			if err != nil {
				t.Fatalf("parse action major from %q in %s: %v", use, file, err)
			}
			if major < minMajor {
				t.Fatalf("%s uses %s; %s must be v%d+ to run on Node 24", file, use, action, minMajor)
			}
		}
	}
}

func workflowFiles(t *testing.T) []string {
	t.Helper()

	var files []string
	for _, pattern := range []string{".github/workflows/*.yml", ".github/workflows/*.yaml"} {
		matches, err := filepath.Glob(repoPath(t, filepath.FromSlash(pattern)))
		if err != nil {
			t.Fatalf("glob %s: %v", pattern, err)
		}
		files = append(files, matches...)
	}
	return files
}

func workflowUses(node *yaml.Node) []string {
	if node == nil {
		return nil
	}
	var uses []string
	if node.Kind == yaml.MappingNode {
		for i := 0; i+1 < len(node.Content); i += 2 {
			key := node.Content[i]
			value := node.Content[i+1]
			if key.Value == "uses" && value.Kind == yaml.ScalarNode {
				uses = append(uses, value.Value)
				continue
			}
			uses = append(uses, workflowUses(value)...)
		}
		return uses
	}
	for _, child := range node.Content {
		uses = append(uses, workflowUses(child)...)
	}
	return uses
}

func TestWorkflowUsesCollectsNestedUses(t *testing.T) {
	yml := []byte(`
jobs:
  test:
    steps:
      - uses: actions/checkout@v5
      - name: run
        run: echo ok
`)
	var root yaml.Node
	if err := yaml.Unmarshal(yml, &root); err != nil {
		t.Fatal(err)
	}
	got := fmt.Sprint(workflowUses(&root))
	if got != "[actions/checkout@v5]" {
		t.Fatalf("workflowUses = %s", got)
	}
}
