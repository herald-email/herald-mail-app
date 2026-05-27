package cache

import (
	"testing"
	"time"

	"github.com/herald-email/herald-mail-app/internal/models"
)

func scopedEmbeddingEmail(messageID string) *models.EmailData {
	return &models.EmailData{
		SourceID:  "work-mail",
		AccountID: "work",
		MessageID: messageID,
		Sender:    "sender@example.com",
		Subject:   "Scoped embedding",
		Folder:    "INBOX",
		Date:      time.Now(),
	}
}

func TestStoreEmbeddingChunksByRefStoresScopeColumns(t *testing.T) {
	c := newTestCache(t)
	email := scopedEmbeddingEmail("scoped-embedding")
	if err := c.CacheEmail(email); err != nil {
		t.Fatalf("CacheEmail: %v", err)
	}
	if err := c.CacheBodyText(email.MessageID, "Scoped body text for semantic search."); err != nil {
		t.Fatalf("CacheBodyText: %v", err)
	}
	ref := email.MessageRef()

	if err := c.StoreEmbeddingChunksByRef(ref, []models.EmbeddingChunk{{
		MessageID:   email.MessageID,
		ChunkIndex:  0,
		Embedding:   []float32{1, 0},
		ContentHash: "hash",
	}}); err != nil {
		t.Fatalf("StoreEmbeddingChunksByRef: %v", err)
	}

	var sourceID, accountID, localID string
	if err := c.db.QueryRow(
		`SELECT source_id, account_id, local_id FROM email_embedding_chunks WHERE message_id = ?`,
		email.MessageID,
	).Scan(&sourceID, &accountID, &localID); err != nil {
		t.Fatalf("query embedding scope: %v", err)
	}
	if sourceID != string(ref.SourceID) || accountID != string(ref.AccountID) || localID != ref.LocalID {
		t.Fatalf("scope = (%q,%q,%q), want (%q,%q,%q)", sourceID, accountID, localID, ref.SourceID, ref.AccountID, ref.LocalID)
	}
}

func TestGetUnembeddedRefsWithBodyReturnsScopedRefs(t *testing.T) {
	c := newTestCache(t)
	embedded := scopedEmbeddingEmail("embedded")
	pending := scopedEmbeddingEmail("pending")
	pending.SourceID = "personal-mail"
	pending.AccountID = "personal"
	for _, email := range []*models.EmailData{embedded, pending} {
		if err := c.CacheEmail(email); err != nil {
			t.Fatalf("CacheEmail(%s): %v", email.MessageID, err)
		}
		if err := c.CacheBodyText(email.MessageID, "Scoped body text for semantic search."); err != nil {
			t.Fatalf("CacheBodyText(%s): %v", email.MessageID, err)
		}
	}
	if err := c.StoreEmbeddingChunksByRef(embedded.MessageRef(), []models.EmbeddingChunk{{
		MessageID:   embedded.MessageID,
		ChunkIndex:  0,
		Embedding:   []float32{1, 0},
		ContentHash: "hash",
	}}); err != nil {
		t.Fatalf("StoreEmbeddingChunksByRef: %v", err)
	}

	refs, err := c.GetUnembeddedRefsWithBody("INBOX")
	if err != nil {
		t.Fatalf("GetUnembeddedRefsWithBody: %v", err)
	}
	if len(refs) != 1 {
		t.Fatalf("refs len = %d, want 1: %#v", len(refs), refs)
	}
	want := pending.MessageRef()
	if refs[0].LocalID != want.LocalID || refs[0].SourceID != want.SourceID || refs[0].AccountID != want.AccountID {
		t.Fatalf("ref = %#v, want %#v", refs[0], want)
	}
}

func TestSearchSemanticChunkedReturnsScopedEmail(t *testing.T) {
	c := newTestCache(t)
	email := scopedEmbeddingEmail("semantic-scoped")
	if err := c.CacheEmail(email); err != nil {
		t.Fatalf("CacheEmail: %v", err)
	}
	if err := c.StoreEmbeddingChunksByRef(email.MessageRef(), []models.EmbeddingChunk{{
		MessageID:   email.MessageID,
		ChunkIndex:  0,
		Embedding:   []float32{1, 0},
		ContentHash: "hash",
	}}); err != nil {
		t.Fatalf("StoreEmbeddingChunksByRef: %v", err)
	}

	results, err := c.SearchSemanticChunked("INBOX", []float32{1, 0}, 10, 0.1)
	if err != nil {
		t.Fatalf("SearchSemanticChunked: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("results len = %d, want 1", len(results))
	}
	got := results[0].Email
	if got.SourceID != email.SourceID || got.AccountID != email.AccountID || got.LocalID != email.MessageRef().LocalID {
		t.Fatalf("result email scope = (%q,%q,%q), want (%q,%q,%q)", got.SourceID, got.AccountID, got.LocalID, email.SourceID, email.AccountID, email.MessageRef().LocalID)
	}
}
