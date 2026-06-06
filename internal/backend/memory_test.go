package backend

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/herald-email/herald-mail-app/internal/cache"
	"github.com/herald-email/herald-mail-app/internal/memory"
	"github.com/herald-email/herald-mail-app/internal/models"
)

func TestLocalMemoryEmailSourceReadsCachedHeadersAndBodies(t *testing.T) {
	store, err := cache.New(":memory:")
	if err != nil {
		t.Fatalf("cache.New: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	email := &models.EmailData{
		SourceID:         "work-mail",
		AccountID:        "work",
		LocalID:          "mail:work:memory-msg-1",
		MessageID:        "memory-msg-1",
		Sender:           "Sergey <sergey@example.com>",
		Subject:          "Interview follow-up",
		ProviderThreadID: "thread-123",
		Folder:           "INBOX",
		Date:             time.Date(2026, 6, 6, 12, 0, 0, 0, time.UTC),
	}
	if err := store.CacheEmail(email); err != nil {
		t.Fatalf("CacheEmail: %v", err)
	}
	body := strings.Repeat("Can you send availability by Friday? ", 180)
	if err := store.CacheBodyTextByRef(email.MessageRef(), body); err != nil {
		t.Fatalf("CacheBodyText: %v", err)
	}
	if err := store.SetClassificationByRef(email.MessageRef(), "important"); err != nil {
		t.Fatalf("SetClassificationByRef: %v", err)
	}
	if err := store.UpsertContacts([]models.ContactAddr{{Email: "sergey@example.com", Name: "Sergey Petrov"}}, "from"); err != nil {
		t.Fatalf("UpsertContacts: %v", err)
	}
	if err := store.UpdateContactEnrichment("sergey@example.com", "Cobalt Systems", []string{"interview", "platform"}); err != nil {
		t.Fatalf("UpdateContactEnrichment: %v", err)
	}
	if err := store.StoreEmbeddingChunksByRef(email.MessageRef(), []models.EmbeddingChunk{{
		MessageID:   email.MessageID,
		ChunkIndex:  0,
		Embedding:   []float32{1, 0},
		ContentHash: "memory-hash",
	}}); err != nil {
		t.Fatalf("StoreEmbeddingChunksByRef: %v", err)
	}

	source := localMemoryEmailSource{cache: store}
	got, err := source.MemoryEmails("INBOX", 10)
	if err != nil {
		t.Fatalf("MemoryEmails: %v", err)
	}
	if len(got) != 1 || got[0].Email.MessageID != email.MessageID || got[0].BodyText == "" {
		t.Fatalf("memory source rows = %#v", got)
	}
	row := got[0]
	if row.Direction != "inbound" || row.Classification != "important" || !row.HasBodyCache || !row.HasEmbedding {
		t.Fatalf("memory source metadata = %#v", row)
	}
	if row.ContactDisplayName != "Sergey Petrov" || row.ContactCompany != "Cobalt Systems" || len(row.ContactTopics) != 2 {
		t.Fatalf("memory source contact metadata = %#v", row)
	}
	if len([]rune(row.BodyText)) > 4003 {
		t.Fatalf("memory source body was not bounded: %d runes", len([]rune(row.BodyText)))
	}
}

func TestDemoBackendMemoryReplyPrepReturnsSourceBackedNudges(t *testing.T) {
	demoBackend := NewDemoBackend()

	results, err := demoBackend.SearchMemories(context.Background(), memory.Query{
		People:        []string{"sergey@example.com"},
		MinConfidence: 0.35,
		Limit:         10,
	})
	if err != nil {
		t.Fatalf("SearchMemories: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("expected demo memory results for Sergey")
	}

	prep, err := demoBackend.BuildReplyMemoryContext(context.Background(), memory.ReplyPrepQuery{
		Recipient: "sergey@example.com",
		Subject:   "Senior engineer interview",
	})
	if err != nil {
		t.Fatalf("BuildReplyMemoryContext: %v", err)
	}
	if len(prep.Nudges) == 0 {
		t.Fatalf("expected demo Compose Radar nudges, got %#v", prep)
	}
	if prep.Nudges[0].Evidence[0].MessageID == "" {
		t.Fatalf("nudge missing source evidence: %#v", prep.Nudges[0])
	}
}
