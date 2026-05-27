package app

import (
	"testing"
	"time"

	"github.com/herald-email/herald-mail-app/internal/config"
	"github.com/herald-email/herald-mail-app/internal/models"
)

type scopedEmbeddingAppBackend struct {
	stubBackend
	ref               models.MessageRef
	email             *models.EmailData
	body              string
	storedRef         models.MessageRef
	storedChunks      []models.EmbeddingChunk
	legacyStoreCalled bool
}

func (b *scopedEmbeddingAppBackend) GetUnembeddedRefsWithBody(_ string) ([]models.MessageRef, error) {
	return []models.MessageRef{b.ref}, nil
}

func (b *scopedEmbeddingAppBackend) GetUncachedBodyRefs(_ string, _ int) ([]models.MessageRef, error) {
	return nil, nil
}

func (b *scopedEmbeddingAppBackend) GetEmailByRef(ref models.MessageRef) (*models.EmailData, error) {
	if ref.WithDefaults().LocalID == b.ref.WithDefaults().LocalID {
		return b.email, nil
	}
	return nil, nil
}

func (b *scopedEmbeddingAppBackend) GetBodyTextByRef(ref models.MessageRef) (string, error) {
	if ref.WithDefaults().LocalID == b.ref.WithDefaults().LocalID {
		return b.body, nil
	}
	return "", nil
}

func (b *scopedEmbeddingAppBackend) FetchAndCacheBodyByRef(models.MessageRef) (*models.EmailBody, error) {
	return nil, nil
}

func (b *scopedEmbeddingAppBackend) StoreEmbeddingChunksByRef(ref models.MessageRef, chunks []models.EmbeddingChunk) error {
	b.storedRef = ref.WithDefaults()
	b.storedChunks = append([]models.EmbeddingChunk(nil), chunks...)
	return nil
}

func (b *scopedEmbeddingAppBackend) StoreEmbeddingChunks(_ string, _ []models.EmbeddingChunk) error {
	b.legacyStoreCalled = true
	return nil
}

func (b *scopedEmbeddingAppBackend) GetEmbeddingProgress(_ string) (int, int, error) {
	return 1, 1, nil
}

func TestRunEmbeddingBatchStoresChunksByScopedRef(t *testing.T) {
	email := &models.EmailData{
		SourceID:  "work-mail",
		AccountID: "work",
		MessageID: "scoped-embed",
		Sender:    "sender@example.com",
		Subject:   "Scoped embedding",
		Folder:    "INBOX",
		Date:      time.Now(),
	}
	ref := email.MessageRef()
	backend := &scopedEmbeddingAppBackend{
		ref:   ref,
		email: email,
		body:  "This body should be embedded under the scoped message reference.",
	}
	m := &Model{
		backend:    backend,
		classifier: &stubClassifier{},
		cfg:        &config.Config{},
	}
	m.cfg.Semantic.Enabled = true

	cmd := m.runEmbeddingBatch("INBOX", 7)
	if cmd == nil {
		t.Fatal("expected embedding command")
	}
	_ = cmd()

	if backend.legacyStoreCalled {
		t.Fatal("expected scoped embedding store path, got legacy message-id store path")
	}
	if backend.storedRef.LocalID != ref.LocalID || backend.storedRef.SourceID != ref.SourceID || backend.storedRef.AccountID != ref.AccountID {
		t.Fatalf("stored ref = %#v, want %#v", backend.storedRef, ref)
	}
	if len(backend.storedChunks) == 0 {
		t.Fatal("expected embedded chunks to be stored")
	}
}
