package cache

import (
	"testing"
	"time"

	"github.com/herald-email/herald-mail-app/internal/models"
)

func requireFTSTable(t *testing.T, c *Cache) {
	t.Helper()
	var count int
	if err := c.db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'emails_fts'`).Scan(&count); err != nil {
		t.Fatalf("query FTS table: %v", err)
	}
	if count == 0 {
		t.Skip("SQLite build does not expose FTS5")
	}
}

func scopedFTSEmail(messageID string) *models.EmailData {
	return &models.EmailData{
		SourceID:    "work-mail",
		AccountID:   "work",
		MessageID:   messageID,
		UID:         42,
		UIDValidity: 77,
		Sender:      "sender@example.com",
		Subject:     "Scoped search",
		Folder:      "INBOX",
		Date:        time.Now().Truncate(time.Second),
	}
}

func TestFTSIndexCarriesSourceScopeColumns(t *testing.T) {
	c := newTestCache(t)
	requireFTSTable(t, c)

	cols := tableColumns(t, c.db, "emails_fts")
	for _, name := range []string{"source_id", "account_id", "local_id"} {
		if !cols[name] {
			t.Fatalf("emails_fts missing scoped column %s", name)
		}
	}
}

func TestCacheBodyTextByRefUpdatesScopedRow(t *testing.T) {
	c := newTestCache(t)
	requireFTSTable(t, c)
	email := scopedFTSEmail("scoped-body@example.test")
	if err := c.CacheEmail(email); err != nil {
		t.Fatalf("CacheEmail: %v", err)
	}
	ref := email.MessageRef()

	if err := c.CacheBodyTextByRef(ref, "roadmap scoped FTS body"); err != nil {
		t.Fatalf("CacheBodyTextByRef: %v", err)
	}

	var bodyText string
	if err := c.db.QueryRow(`SELECT body_text FROM emails WHERE local_id = ?`, ref.LocalID).Scan(&bodyText); err != nil {
		t.Fatalf("query body_text: %v", err)
	}
	if bodyText != "roadmap scoped FTS body" {
		t.Fatalf("body_text = %q, want scoped body", bodyText)
	}
}

func TestSearchEmailsFTSPreservesScopedIdentity(t *testing.T) {
	c := newTestCache(t)
	requireFTSTable(t, c)
	email := scopedFTSEmail("scoped-search@example.test")
	if err := c.CacheEmail(email); err != nil {
		t.Fatalf("CacheEmail: %v", err)
	}
	ref := email.MessageRef()
	if err := c.CacheBodyTextByRef(ref, "roadmap scoped FTS body"); err != nil {
		t.Fatalf("CacheBodyTextByRef: %v", err)
	}

	results, err := c.SearchEmailsFTS("INBOX", "roadmap")
	if err != nil {
		t.Fatalf("SearchEmailsFTS: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("results len = %d, want 1: %#v", len(results), results)
	}
	got := results[0]
	if got.SourceID != ref.SourceID || got.AccountID != ref.AccountID || got.LocalID != ref.LocalID || got.UIDValidity != ref.UIDValidity {
		t.Fatalf("result scope = (%q,%q,%q,%d), want (%q,%q,%q,%d)",
			got.SourceID, got.AccountID, got.LocalID, got.UIDValidity,
			ref.SourceID, ref.AccountID, ref.LocalID, ref.UIDValidity)
	}
}
