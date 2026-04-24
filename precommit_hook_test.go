package main

import (
	"os"
	"strings"
	"testing"
)

func TestPreCommitHookRunsVet(t *testing.T) {
	info, err := os.Stat(".githooks/pre-commit")
	if err != nil {
		t.Fatalf("expected versioned pre-commit hook: %v", err)
	}
	if info.IsDir() {
		t.Fatal("expected .githooks/pre-commit to be a file")
	}
	if info.Mode().Perm()&0o111 == 0 {
		t.Fatalf("expected pre-commit hook to be executable, mode is %04o", info.Mode().Perm())
	}

	data, err := os.ReadFile(".githooks/pre-commit")
	if err != nil {
		t.Fatalf("read pre-commit hook: %v", err)
	}
	if !strings.Contains(string(data), "make vet") {
		t.Fatalf("expected pre-commit hook to run make vet, got:\n%s", string(data))
	}
}
