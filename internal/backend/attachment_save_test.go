package backend

import (
	"os"
	"path/filepath"
	"testing"

	"mail-processor/internal/filesafe"
	"mail-processor/internal/models"
)

func TestLocalBackendSaveAttachmentRefusesExistingDestination(t *testing.T) {
	dir := t.TempDir()
	existing := filepath.Join(dir, "statement.pdf")
	if err := os.WriteFile(existing, []byte("original"), 0o644); err != nil {
		t.Fatalf("write existing destination: %v", err)
	}

	err := (&LocalBackend{}).SaveAttachment(&models.Attachment{
		Filename: "statement.pdf",
		Data:     []byte("replacement"),
	}, existing)
	if err == nil {
		t.Fatal("expected existing-file error")
	}
	existingErr, ok := filesafe.AsExistingFileError(err)
	if !ok {
		t.Fatalf("expected ExistingFileError, got %T: %v", err, err)
	}
	if existingErr.SuggestedPath != filepath.Join(dir, "statement (1).pdf") {
		t.Fatalf("suggested path got %q", existingErr.SuggestedPath)
	}

	contents, err := os.ReadFile(existing)
	if err != nil {
		t.Fatalf("read existing destination: %v", err)
	}
	if string(contents) != "original" {
		t.Fatalf("existing destination was overwritten: %q", contents)
	}
}
