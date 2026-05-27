package app

import (
	"context"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/herald-email/herald-mail-app/internal/ai"
	"github.com/herald-email/herald-mail-app/internal/backend"
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

type contactEnrichmentSourceBackend struct {
	stubBackend
	info    backend.AccountInfo
	contact models.ContactData
}

func (b *contactEnrichmentSourceBackend) GetContactsToEnrich(_, _ int) ([]models.ContactData, error) {
	return []models.ContactData{b.contact}, nil
}

func (b *contactEnrichmentSourceBackend) GetRecentSubjectsByContact(_ string, _ int) ([]string, error) {
	return []string{"Roadmap sync", "Budget review"}, nil
}

func (b *contactEnrichmentSourceBackend) Accounts() []backend.AccountInfo {
	return []backend.AccountInfo{b.info}
}

func (b *contactEnrichmentSourceBackend) ActiveAccount() backend.AccountInfo {
	return b.info
}

func (b *contactEnrichmentSourceBackend) HasMultipleAccounts() bool {
	return true
}

func (b *contactEnrichmentSourceBackend) SwitchAccount(models.SourceID) error {
	return nil
}

func (b *contactEnrichmentSourceBackend) AccountStatuses() map[models.SourceID]backend.AccountStatus {
	return map[models.SourceID]backend.AccountStatus{
		b.info.SourceID: {SourceID: b.info.SourceID, State: "live"},
	}
}

type sourceFairContactAI struct {
	started      chan string
	releaseFirst chan struct{}
}

func (s *sourceFairContactAI) Chat([]ai.ChatMessage) (string, error) {
	return "", nil
}

func (s *sourceFairContactAI) ChatWithTools([]ai.ChatMessage, []ai.Tool) (string, []ai.ToolCall, error) {
	return "", nil, ai.ErrToolsNotSupported
}

func (s *sourceFairContactAI) Classify(_, _ string) (ai.Category, error) {
	return "", nil
}

func (s *sourceFairContactAI) Embed(_ string) ([]float32, error) {
	return []float32{1, 2, 3}, nil
}

func (s *sourceFairContactAI) SetEmbeddingModel(string) {}

func (s *sourceFairContactAI) GenerateQuickReplies(_, _, _ string) ([]string, error) {
	return nil, nil
}

func (s *sourceFairContactAI) EnrichContact(email string, _ []string) (string, []string, error) {
	s.started <- email
	if email == "work-1@example.com" {
		<-s.releaseFirst
	}
	return "Example Co", []string{"planning"}, nil
}

func (s *sourceFairContactAI) HasVisionModel() bool { return false }

func (s *sourceFairContactAI) DescribeImage(context.Context, []byte, string) (string, error) {
	return "", nil
}

func (s *sourceFairContactAI) Ping() error { return nil }

func TestRunContactEnrichmentTagsBackgroundAIWithActiveSource(t *testing.T) {
	started := make(chan string, 4)
	releaseFirst := make(chan struct{})
	classifier := ai.NewManagedClient(&sourceFairContactAI{
		started:      started,
		releaseFirst: releaseFirst,
	}, ai.ManagedConfig{
		MaxConcurrency:                  1,
		QueueLimit:                      8,
		PauseBackgroundWhileInteractive: true,
	})
	cfg := &config.Config{}
	cfg.Semantic.Enabled = true

	modelFor := func(sourceID models.SourceID, accountID models.AccountID, contactEmail string) *Model {
		return &Model{
			backend: &contactEnrichmentSourceBackend{
				info: backend.AccountInfo{
					SourceID:    sourceID,
					AccountID:   accountID,
					DisplayName: string(sourceID),
				},
				contact: models.ContactData{Email: contactEmail, DisplayName: contactEmail, EmailCount: 3},
			},
			classifier: classifier,
			cfg:        cfg,
		}
	}
	run := func(cmd tea.Cmd) <-chan tea.Msg {
		done := make(chan tea.Msg, 1)
		go func() {
			done <- cmd()
		}()
		return done
	}
	waitDone := func(done <-chan tea.Msg) {
		t.Helper()
		select {
		case <-done:
		case <-time.After(time.Second):
			t.Fatal("contact enrichment command did not finish")
		}
	}

	done1 := run(modelFor("work-mail", "work", "work-1@example.com").runContactEnrichment(1))
	if first := <-started; first != "work-1@example.com" {
		t.Fatalf("first contact enrichment = %q, want work-1@example.com", first)
	}
	done2 := run(modelFor("work-mail", "work", "work-2@example.com").runContactEnrichment(1))
	time.Sleep(20 * time.Millisecond)
	done3 := run(modelFor("personal-mail", "personal", "personal-1@example.com").runContactEnrichment(1))
	time.Sleep(20 * time.Millisecond)

	close(releaseFirst)
	waitDone(done1)
	waitDone(done2)
	waitDone(done3)

	got := []string{<-started, <-started}
	want := []string{"personal-1@example.com", "work-2@example.com"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("contact enrichment order = %#v, want %#v", got, want)
		}
	}
}
