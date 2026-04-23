package main

import (
	"os"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

type skillFrontMatter struct {
	Name                   string `yaml:"name"`
	Description            string `yaml:"description"`
	DisableModelInvocation bool   `yaml:"disable-model-invocation"`
	ArgumentHint           string `yaml:"argument-hint"`
}

func TestTUITestSkillFrontMatterParses(t *testing.T) {
	data, err := os.ReadFile(".agents/skills/tui-test/SKILL.md")
	if err != nil {
		t.Fatalf("read skill file: %v", err)
	}

	frontMatter, body, ok := extractSkillFrontMatter(string(data))
	if !ok {
		t.Fatal("skill file is missing YAML front matter")
	}

	var meta skillFrontMatter
	if err := yaml.Unmarshal([]byte(frontMatter), &meta); err != nil {
		t.Fatalf("parse front matter: %v", err)
	}

	if meta.Name != "tui-test" {
		t.Fatalf("unexpected skill name %q", meta.Name)
	}

	if meta.Description == "" {
		t.Fatal("description should not be empty")
	}

	if !meta.DisableModelInvocation {
		t.Fatal("disable-model-invocation should be true")
	}

	if meta.ArgumentHint == "" {
		t.Fatal("argument-hint should not be empty")
	}

	if !strings.Contains(body, "# TUI Battle Testing") {
		t.Fatal("skill body should contain the expected heading")
	}
}

func extractSkillFrontMatter(raw string) (frontMatter string, body string, ok bool) {
	rest, ok := strings.CutPrefix(raw, "---\n")
	if !ok {
		return "", "", false
	}

	frontMatter, body, ok = strings.Cut(rest, "\n---\n")
	return frontMatter, body, ok
}
