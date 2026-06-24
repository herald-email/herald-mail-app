package app

import (
	"sync"
	"testing"
	"time"

	"github.com/herald-email/herald-mail-app/internal/models"
)

type deletionLaneBackend struct {
	stubBackend
	mu      sync.Mutex
	started []string
	block   map[string]chan struct{}
	startCh chan string
}

func newDeletionLaneBackend(block map[string]chan struct{}) *deletionLaneBackend {
	return &deletionLaneBackend{
		block:   block,
		startCh: make(chan string, 8),
	}
}

func (b *deletionLaneBackend) DeleteEmailByRef(ref models.MessageRef) error {
	b.mu.Lock()
	b.started = append(b.started, ref.MessageID)
	b.mu.Unlock()
	b.startCh <- ref.MessageID
	if ch := b.block[ref.MessageID]; ch != nil {
		<-ch
	}
	return nil
}

func (b *deletionLaneBackend) startedIDs() []string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]string(nil), b.started...)
}

func scopedDeletionRequest(sourceID models.SourceID, messageID string) models.DeletionRequest {
	ref := models.MessageRef{
		SourceID:  sourceID,
		AccountID: models.AccountID(sourceID + "-acct"),
		Folder:    "INBOX",
		MessageID: messageID,
	}.WithDefaults()
	return models.DeletionRequest{
		MessageRef: ref,
		SourceID:   ref.SourceID,
		AccountID:  ref.AccountID,
		LocalID:    ref.LocalID,
		MessageID:  ref.MessageID,
		Folder:     ref.Folder,
	}
}

type archiveBatchBackend struct {
	stubBackend
	refs [][]models.MessageRef
}

func (b *archiveBatchBackend) ArchiveEmailsByRef(refs []models.MessageRef) error {
	b.refs = append(b.refs, append([]models.MessageRef(nil), refs...))
	return nil
}

func waitDeletionStart(t *testing.T, ch <-chan string, want string) {
	t.Helper()
	select {
	case got := <-ch:
		if got != want {
			t.Fatalf("started %q, want %q", got, want)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for %q to start", want)
	}
}

func TestDeletionWorkerAllowsDifferentSourcesWhileOneSourceIsBlocked(t *testing.T) {
	firstDone := make(chan struct{})
	backend := newDeletionLaneBackend(map[string]chan struct{}{
		"work-1": firstDone,
	})
	m := newStubModel(&backend.stubBackend)
	m.backend = backend
	requestCh := make(chan models.DeletionRequest, 4)
	resultCh := make(chan models.DeletionResult, 4)
	go m.deletionWorker(requestCh, resultCh)
	defer close(requestCh)

	requestCh <- scopedDeletionRequest("work-mail", "work-1")
	waitDeletionStart(t, backend.startCh, "work-1")

	requestCh <- scopedDeletionRequest("personal-mail", "personal-1")
	waitDeletionStart(t, backend.startCh, "personal-1")

	select {
	case result := <-resultCh:
		if result.MessageID != "personal-1" {
			t.Fatalf("first completed result = %q, want personal-1", result.MessageID)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("different source deletion should not wait behind blocked work source")
	}
	close(firstDone)
}

func TestDeletionWorkerSerializesMutationsWithinOneSource(t *testing.T) {
	firstDone := make(chan struct{})
	backend := newDeletionLaneBackend(map[string]chan struct{}{
		"work-1": firstDone,
	})
	m := newStubModel(&backend.stubBackend)
	m.backend = backend
	requestCh := make(chan models.DeletionRequest, 4)
	resultCh := make(chan models.DeletionResult, 4)
	go m.deletionWorker(requestCh, resultCh)
	defer close(requestCh)

	requestCh <- scopedDeletionRequest("work-mail", "work-1")
	waitDeletionStart(t, backend.startCh, "work-1")
	requestCh <- scopedDeletionRequest("work-mail", "work-2")

	select {
	case got := <-backend.startCh:
		t.Fatalf("same-source deletion %q started before work-1 completed", got)
	case <-time.After(100 * time.Millisecond):
	}

	close(firstDone)
	select {
	case result := <-resultCh:
		if result.MessageID != "work-1" {
			t.Fatalf("first result = %q, want work-1", result.MessageID)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timed out waiting for work-1 result")
	}
	waitDeletionStart(t, backend.startCh, "work-2")
}

func TestExecuteArchiveBatchUsesBulkBackend(t *testing.T) {
	backend := &archiveBatchBackend{}
	m := newStubModel(&backend.stubBackend)
	m.backend = backend
	refs := []models.MessageRef{
		(models.MessageRef{SourceID: "work-mail", AccountID: "work", Folder: "INBOX", MessageID: "archive-1", UID: 101, UIDValidity: 999}).WithDefaults(),
		(models.MessageRef{SourceID: "work-mail", AccountID: "work", Folder: "INBOX", MessageID: "archive-2", UID: 102, UIDValidity: 999}).WithDefaults(),
	}

	count, err := m.executeArchiveBatch(refs)
	if err != nil {
		t.Fatalf("executeArchiveBatch: %v", err)
	}
	if count != 2 {
		t.Fatalf("count = %d, want 2", count)
	}
	if len(backend.refs) != 1 {
		t.Fatalf("ArchiveEmailsByRef calls = %d, want 1", len(backend.refs))
	}
	if len(backend.refs[0]) != 2 || backend.refs[0][0] != refs[0] || backend.refs[0][1] != refs[1] {
		t.Fatalf("ArchiveEmailsByRef refs = %#v, want %#v", backend.refs[0], refs)
	}
}
