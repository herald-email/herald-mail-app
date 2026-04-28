package filesafe

import (
	"os"
	"path/filepath"
	"testing"
)

func touchFile(t *testing.T, path string) {
	t.Helper()
	if err := os.WriteFile(path, []byte("existing"), 0o644); err != nil {
		t.Fatalf("write existing file %s: %v", path, err)
	}
}

func TestNextAvailablePathSuggestsNumberedNameBeforeExtension(t *testing.T) {
	dir := t.TempDir()
	existing := filepath.Join(dir, "report.pdf")
	touchFile(t, existing)

	got, err := NextAvailablePath(existing)
	if err != nil {
		t.Fatalf("NextAvailablePath returned error: %v", err)
	}

	want := filepath.Join(dir, "report (1).pdf")
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestNextAvailablePathSkipsRepeatedCollisions(t *testing.T) {
	dir := t.TempDir()
	existing := filepath.Join(dir, "report.pdf")
	firstSuggestion := filepath.Join(dir, "report (1).pdf")
	touchFile(t, existing)
	touchFile(t, firstSuggestion)

	got, err := NextAvailablePath(existing)
	if err != nil {
		t.Fatalf("NextAvailablePath returned error: %v", err)
	}

	want := filepath.Join(dir, "report (2).pdf")
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestNextAvailablePathKeepsHiddenFileNameTogether(t *testing.T) {
	dir := t.TempDir()
	existing := filepath.Join(dir, ".env")
	touchFile(t, existing)

	got, err := NextAvailablePath(existing)
	if err != nil {
		t.Fatalf("NextAvailablePath returned error: %v", err)
	}

	want := filepath.Join(dir, ".env (1)")
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestWriteFileExclusiveRefusesExistingFileAndLeavesContentAlone(t *testing.T) {
	dir := t.TempDir()
	existing := filepath.Join(dir, "report.pdf")
	if err := os.WriteFile(existing, []byte("original"), 0o644); err != nil {
		t.Fatalf("write existing file: %v", err)
	}

	err := WriteFileExclusive(existing, []byte("replacement"), 0o644)
	if err == nil {
		t.Fatal("expected existing-file error")
	}
	existingErr, ok := AsExistingFileError(err)
	if !ok {
		t.Fatalf("expected ExistingFileError, got %T: %v", err, err)
	}
	if existingErr.Path != existing {
		t.Fatalf("error path got %q, want %q", existingErr.Path, existing)
	}
	if existingErr.SuggestedPath != filepath.Join(dir, "report (1).pdf") {
		t.Fatalf("suggested path got %q", existingErr.SuggestedPath)
	}

	got, err := os.ReadFile(existing)
	if err != nil {
		t.Fatalf("read existing file: %v", err)
	}
	if string(got) != "original" {
		t.Fatalf("existing file was overwritten: %q", got)
	}
}
