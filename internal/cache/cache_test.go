package cache

import (
	"encoding/binary"
	"math"
	"testing"
	"time"

	"mail-processor/internal/models"
)

// newTestCache creates an in-memory SQLite cache for testing.
func newTestCache(t *testing.T) *Cache {
	t.Helper()
	c, err := New(":memory:")
	if err != nil {
		t.Fatalf("failed to create test cache: %v", err)
	}
	t.Cleanup(func() { c.Close() })
	return c
}

func encodeTestEmbedding(vec []float32) []byte {
	buf := make([]byte, len(vec)*4)
	for i, v := range vec {
		binary.LittleEndian.PutUint32(buf[i*4:], math.Float32bits(v))
	}
	return buf
}

// --- extractDomain ---

func TestExtractDomain(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"user@example.com", "example.com"},
		{"user@mail.example.com", "example.com"},
		{"user@a.b.example.com", "example.com"},
		{"user@example.co.uk", "example.co.uk"},
		{"user@foo.example.co.uk", "example.co.uk"},
		{"user@example.com.au", "example.com.au"},
		{"user@single", "single"},
		{"notanemail", "notanemail"},
		{"", ""},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := extractDomain(tt.input)
			if got != tt.want {
				t.Errorf("extractDomain(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// --- CacheEmail / GetCachedIDs ---

func TestCacheEmail_StoresAndRetrieves(t *testing.T) {
	c := newTestCache(t)

	email := &models.EmailData{
		MessageID: "<abc@example.com>",
		UID:       42,
		Sender:    "sender@example.com",
		Subject:   "Hello",
		Date:      time.Now().Truncate(time.Second),
		Size:      1024,
		Folder:    "INBOX",
	}

	if err := c.CacheEmail(email); err != nil {
		t.Fatalf("CacheEmail: %v", err)
	}

	ids, err := c.GetCachedIDs("INBOX")
	if err != nil {
		t.Fatalf("GetCachedIDs: %v", err)
	}
	if !ids[email.MessageID] {
		t.Errorf("expected message ID %q to be cached", email.MessageID)
	}
}

func TestCacheEmail_UIDisPersisted(t *testing.T) {
	c := newTestCache(t)

	email := &models.EmailData{
		MessageID: "<uid-test@example.com>",
		UID:       99,
		Sender:    "sender@example.com",
		Subject:   "UID test",
		Date:      time.Now(),
		Folder:    "INBOX",
	}
	if err := c.CacheEmail(email); err != nil {
		t.Fatalf("CacheEmail: %v", err)
	}

	uids, err := c.GetCachedUIDs("INBOX")
	if err != nil {
		t.Fatalf("GetCachedUIDs: %v", err)
	}
	if !uids[99] {
		t.Errorf("expected UID 99 to be cached, got %v", uids)
	}
}

func TestGetCachedIDs_EmptyFolderReturnsEmpty(t *testing.T) {
	c := newTestCache(t)

	ids, err := c.GetCachedIDs("INBOX")
	if err != nil {
		t.Fatalf("GetCachedIDs: %v", err)
	}
	if len(ids) != 0 {
		t.Errorf("expected empty map, got %v", ids)
	}
}

func TestGetCachedIDs_OnlyReturnsFolderEntries(t *testing.T) {
	c := newTestCache(t)

	inbox := &models.EmailData{MessageID: "<inbox@x.com>", Sender: "a@x.com", Folder: "INBOX", Date: time.Now()}
	sent := &models.EmailData{MessageID: "<sent@x.com>", Sender: "a@x.com", Folder: "Sent", Date: time.Now()}

	for _, e := range []*models.EmailData{inbox, sent} {
		if err := c.CacheEmail(e); err != nil {
			t.Fatalf("CacheEmail: %v", err)
		}
	}

	ids, err := c.GetCachedIDs("INBOX")
	if err != nil {
		t.Fatalf("GetCachedIDs: %v", err)
	}
	if !ids["<inbox@x.com>"] {
		t.Error("expected INBOX message")
	}
	if ids["<sent@x.com>"] {
		t.Error("Sent message should not appear in INBOX query")
	}
}

// --- DeleteSenderEmails ---

func TestDeleteSenderEmails(t *testing.T) {
	c := newTestCache(t)

	emails := []*models.EmailData{
		{MessageID: "<1@x.com>", Sender: "alice@example.com", Folder: "INBOX", Date: time.Now()},
		{MessageID: "<2@x.com>", Sender: "alice@example.com", Folder: "INBOX", Date: time.Now()},
		{MessageID: "<3@x.com>", Sender: "bob@example.com", Folder: "INBOX", Date: time.Now()},
	}
	for _, e := range emails {
		if err := c.CacheEmail(e); err != nil {
			t.Fatalf("CacheEmail: %v", err)
		}
	}

	if err := c.DeleteSenderEmails("alice@example.com", "INBOX"); err != nil {
		t.Fatalf("DeleteSenderEmails: %v", err)
	}

	ids, _ := c.GetCachedIDs("INBOX")
	if ids["<1@x.com>"] || ids["<2@x.com>"] {
		t.Error("alice's emails should be deleted")
	}
	if !ids["<3@x.com>"] {
		t.Error("bob's email should remain")
	}
}

// --- DeleteDomainEmails ---

func TestDeleteDomainEmails_ExactAndSubdomain(t *testing.T) {
	c := newTestCache(t)

	emails := []*models.EmailData{
		{MessageID: "<exact@x.com>", Sender: "user@example.com", Folder: "INBOX", Date: time.Now()},
		{MessageID: "<sub@x.com>", Sender: "user@mail.example.com", Folder: "INBOX", Date: time.Now()},
		{MessageID: "<other@x.com>", Sender: "user@other.com", Folder: "INBOX", Date: time.Now()},
	}
	for _, e := range emails {
		if err := c.CacheEmail(e); err != nil {
			t.Fatalf("CacheEmail: %v", err)
		}
	}

	if err := c.DeleteDomainEmails("example.com", "INBOX"); err != nil {
		t.Fatalf("DeleteDomainEmails: %v", err)
	}

	ids, _ := c.GetCachedIDs("INBOX")
	if ids["<exact@x.com>"] {
		t.Error("exact-domain email should be deleted")
	}
	if ids["<sub@x.com>"] {
		t.Error("subdomain email should be deleted")
	}
	if !ids["<other@x.com>"] {
		t.Error("unrelated domain email should remain")
	}
}

// --- GetNewestCachedDate ---

func TestGetNewestCachedDate_EmptyFolder(t *testing.T) {
	c := newTestCache(t)

	date, err := c.GetNewestCachedDate("INBOX")
	if err != nil {
		t.Fatalf("unexpected error on empty folder: %v", err)
	}
	if !date.IsZero() {
		t.Errorf("expected zero time for empty folder, got %v", date)
	}
}

func TestGetNewestCachedDate_ReturnsNewest(t *testing.T) {
	c := newTestCache(t)

	older := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	newer := time.Date(2025, 6, 15, 12, 0, 0, 0, time.UTC)

	emails := []*models.EmailData{
		{MessageID: "<old@x.com>", Sender: "a@x.com", Folder: "INBOX", Date: older},
		{MessageID: "<new@x.com>", Sender: "a@x.com", Folder: "INBOX", Date: newer},
	}
	for _, e := range emails {
		if err := c.CacheEmail(e); err != nil {
			t.Fatalf("CacheEmail: %v", err)
		}
	}

	got, err := c.GetNewestCachedDate("INBOX")
	if err != nil {
		t.Fatalf("GetNewestCachedDate: %v", err)
	}
	if !got.Equal(newer) {
		t.Errorf("expected %v, got %v", newer, got)
	}
}

func TestEnsureEmbeddingModel_SetsMetadataWithoutInvalidatingLegacyCache(t *testing.T) {
	c := newTestCache(t)
	if _, err := c.db.Exec(
		`INSERT INTO email_embedding_chunks (message_id, chunk_index, embedding, content_hash, embedded_at) VALUES (?, ?, ?, ?, ?)`,
		"msg-1", 0, encodeTestEmbedding([]float32{0.1, 0.2}), "hash", time.Now().Format(time.RFC3339),
	); err != nil {
		t.Fatalf("seed chunk embedding: %v", err)
	}

	invalidated, err := c.EnsureEmbeddingModel(legacyEmbeddingModelDefault)
	if err != nil {
		t.Fatalf("EnsureEmbeddingModel: %v", err)
	}
	if invalidated {
		t.Fatal("expected legacy default model to keep existing embeddings on first metadata write")
	}

	var count int
	if err := c.db.QueryRow(`SELECT COUNT(*) FROM email_embedding_chunks`).Scan(&count); err != nil {
		t.Fatalf("count chunk embeddings: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected chunk embeddings to be preserved, got %d rows", count)
	}
}

func TestEnsureEmbeddingModel_InvalidatesOnModelChange(t *testing.T) {
	c := newTestCache(t)
	if _, err := c.db.Exec(
		`INSERT INTO cache_metadata (key, value, updated_at) VALUES (?, ?, ?)`,
		cacheMetaEmbeddingModelKey, legacyEmbeddingModelDefault, time.Now().Format(time.RFC3339),
	); err != nil {
		t.Fatalf("seed cache metadata: %v", err)
	}
	if _, err := c.db.Exec(
		`INSERT INTO email_embedding_chunks (message_id, chunk_index, embedding, content_hash, embedded_at) VALUES (?, ?, ?, ?, ?)`,
		"msg-1", 0, encodeTestEmbedding([]float32{0.1, 0.2}), "hash", time.Now().Format(time.RFC3339),
	); err != nil {
		t.Fatalf("seed chunk embedding: %v", err)
	}
	if _, err := c.db.Exec(
		`INSERT INTO email_embeddings (message_id, embedding, hash, embedded_at) VALUES (?, ?, ?, ?)`,
		"msg-1", encodeTestEmbedding([]float32{0.1, 0.2}), "hash", time.Now().Format(time.RFC3339),
	); err != nil {
		t.Fatalf("seed legacy embedding: %v", err)
	}
	if _, err := c.db.Exec(
		`INSERT INTO contacts (email, first_seen, last_seen, embedding) VALUES (?, ?, ?, ?)`,
		"alice@example.com", time.Now().Format(time.RFC3339), time.Now().Format(time.RFC3339), encodeTestEmbedding([]float32{0.3, 0.4}),
	); err != nil {
		t.Fatalf("seed contact embedding: %v", err)
	}

	invalidated, err := c.EnsureEmbeddingModel("nomic-embed-text-v2-moe")
	if err != nil {
		t.Fatalf("EnsureEmbeddingModel: %v", err)
	}
	if !invalidated {
		t.Fatal("expected model change to invalidate embeddings")
	}

	for _, query := range []string{
		`SELECT COUNT(*) FROM email_embedding_chunks`,
		`SELECT COUNT(*) FROM email_embeddings`,
		`SELECT COUNT(*) FROM contacts WHERE embedding IS NOT NULL`,
	} {
		var count int
		if err := c.db.QueryRow(query).Scan(&count); err != nil {
			t.Fatalf("count invalidated rows for %q: %v", query, err)
		}
		if count != 0 {
			t.Fatalf("expected query %q to return 0 after invalidation, got %d", query, count)
		}
	}

	got, found, err := c.getMetadata(cacheMetaEmbeddingModelKey)
	if err != nil {
		t.Fatalf("getMetadata: %v", err)
	}
	if !found || got != "nomic-embed-text-v2-moe" {
		t.Fatalf("expected embedding metadata to update, got found=%v value=%q", found, got)
	}
}

func TestEnsureEmbeddingModel_InvalidatesLegacyCacheWithoutMetadataOnNewDefault(t *testing.T) {
	c := newTestCache(t)
	if _, err := c.db.Exec(
		`INSERT INTO email_embedding_chunks (message_id, chunk_index, embedding, content_hash, embedded_at) VALUES (?, ?, ?, ?, ?)`,
		"msg-1", 0, encodeTestEmbedding([]float32{0.1, 0.2}), "hash", time.Now().Format(time.RFC3339),
	); err != nil {
		t.Fatalf("seed chunk embedding: %v", err)
	}

	invalidated, err := c.EnsureEmbeddingModel("nomic-embed-text-v2-moe")
	if err != nil {
		t.Fatalf("EnsureEmbeddingModel: %v", err)
	}
	if !invalidated {
		t.Fatal("expected first run on a non-legacy model to invalidate metadata-less embeddings")
	}
}

// --- GetAllEmails grouping ---

func TestGetAllEmails_GroupBySender(t *testing.T) {
	c := newTestCache(t)

	emails := []*models.EmailData{
		{MessageID: "<1@x.com>", Sender: "alice@example.com", Folder: "INBOX", Date: time.Now()},
		{MessageID: "<2@x.com>", Sender: "alice@example.com", Folder: "INBOX", Date: time.Now()},
		{MessageID: "<3@x.com>", Sender: "bob@other.com", Folder: "INBOX", Date: time.Now()},
	}
	for _, e := range emails {
		if err := c.CacheEmail(e); err != nil {
			t.Fatalf("CacheEmail: %v", err)
		}
	}

	groups, err := c.GetAllEmails("INBOX", false)
	if err != nil {
		t.Fatalf("GetAllEmails: %v", err)
	}
	if len(groups["alice@example.com"]) != 2 {
		t.Errorf("expected 2 emails for alice, got %d", len(groups["alice@example.com"]))
	}
	if len(groups["bob@other.com"]) != 1 {
		t.Errorf("expected 1 email for bob, got %d", len(groups["bob@other.com"]))
	}
}

func TestGetAllEmails_GroupByDomain(t *testing.T) {
	c := newTestCache(t)

	emails := []*models.EmailData{
		{MessageID: "<1@x.com>", Sender: "alice@example.com", Folder: "INBOX", Date: time.Now()},
		{MessageID: "<2@x.com>", Sender: "bob@example.com", Folder: "INBOX", Date: time.Now()},
		{MessageID: "<3@x.com>", Sender: "carol@other.com", Folder: "INBOX", Date: time.Now()},
	}
	for _, e := range emails {
		if err := c.CacheEmail(e); err != nil {
			t.Fatalf("CacheEmail: %v", err)
		}
	}

	groups, err := c.GetAllEmails("INBOX", true)
	if err != nil {
		t.Fatalf("GetAllEmails: %v", err)
	}
	if len(groups["example.com"]) != 2 {
		t.Errorf("expected 2 emails for example.com, got %d", len(groups["example.com"]))
	}
	if len(groups["other.com"]) != 1 {
		t.Errorf("expected 1 email for other.com, got %d", len(groups["other.com"]))
	}
}

// --- folder_sync_state ---

func TestGetSetFolderSyncState(t *testing.T) {
	c := newTestCache(t)

	// Unknown folder returns zeros, no error
	v, n, err := c.GetFolderSyncState("INBOX")
	if err != nil {
		t.Fatalf("GetFolderSyncState on empty: %v", err)
	}
	if v != 0 || n != 0 {
		t.Errorf("expected 0,0 for unknown folder, got %d,%d", v, n)
	}

	// Store and read back
	if err := c.SetFolderSyncState("INBOX", 12345, 999); err != nil {
		t.Fatalf("SetFolderSyncState: %v", err)
	}
	v, n, err = c.GetFolderSyncState("INBOX")
	if err != nil {
		t.Fatalf("GetFolderSyncState after set: %v", err)
	}
	if v != 12345 || n != 999 {
		t.Errorf("expected 12345,999 got %d,%d", v, n)
	}

	// Update replaces
	if err := c.SetFolderSyncState("INBOX", 12345, 1000); err != nil {
		t.Fatalf("SetFolderSyncState update: %v", err)
	}
	_, n, _ = c.GetFolderSyncState("INBOX")
	if n != 1000 {
		t.Errorf("expected updated uidnext=1000, got %d", n)
	}
}

func TestCountEmailsInFolder(t *testing.T) {
	c := newTestCache(t)

	for _, email := range []*models.EmailData{
		{MessageID: "<1@x.com>", UID: 1, Sender: "a@x.com", Folder: "INBOX", Date: time.Now()},
		{MessageID: "<2@x.com>", UID: 2, Sender: "b@x.com", Folder: "INBOX", Date: time.Now()},
		{MessageID: "<3@x.com>", UID: 3, Sender: "c@x.com", Folder: "Sent", Date: time.Now()},
	} {
		if err := c.CacheEmail(email); err != nil {
			t.Fatalf("CacheEmail: %v", err)
		}
	}

	count, err := c.CountEmailsInFolder("INBOX")
	if err != nil {
		t.Fatalf("CountEmailsInFolder: %v", err)
	}
	if count != 2 {
		t.Fatalf("expected 2 INBOX emails, got %d", count)
	}
}

// --- GetCachedUIDsAndMessageIDs ---

func TestGetCachedUIDsAndMessageIDs(t *testing.T) {
	c := newTestCache(t)

	withUID := &models.EmailData{MessageID: "<with-uid@x.com>", UID: 42, Sender: "a@x.com", Folder: "INBOX", Date: time.Now()}
	noUID := &models.EmailData{MessageID: "<no-uid@x.com>", UID: 0, Sender: "b@x.com", Folder: "INBOX", Date: time.Now()}
	otherFolder := &models.EmailData{MessageID: "<other@x.com>", UID: 77, Sender: "c@x.com", Folder: "Sent", Date: time.Now()}

	for _, e := range []*models.EmailData{withUID, noUID, otherFolder} {
		if err := c.CacheEmail(e); err != nil {
			t.Fatalf("CacheEmail: %v", err)
		}
	}

	rows, err := c.GetCachedUIDsAndMessageIDs("INBOX")
	if err != nil {
		t.Fatalf("GetCachedUIDsAndMessageIDs: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows for INBOX, got %d", len(rows))
	}

	byID := make(map[string]uint32)
	for _, r := range rows {
		byID[r.MessageID] = r.UID
	}
	if byID["<with-uid@x.com>"] != 42 {
		t.Errorf("expected UID 42 for <with-uid@x.com>, got %d", byID["<with-uid@x.com>"])
	}
	if byID["<no-uid@x.com>"] != 0 {
		t.Errorf("expected UID 0 for <no-uid@x.com>, got %d", byID["<no-uid@x.com>"])
	}
	if _, ok := byID["<other@x.com>"]; ok {
		t.Error("Sent folder entry should not appear")
	}
}

// --- DeleteEmailsByUIDs ---

func TestDeleteEmailsByUIDs(t *testing.T) {
	c := newTestCache(t)

	emails := []*models.EmailData{
		{MessageID: "<uid1@x.com>", UID: 1, Sender: "a@x.com", Folder: "INBOX", Date: time.Now()},
		{MessageID: "<uid2@x.com>", UID: 2, Sender: "a@x.com", Folder: "INBOX", Date: time.Now()},
		{MessageID: "<uid3@x.com>", UID: 3, Sender: "a@x.com", Folder: "INBOX", Date: time.Now()},
		{MessageID: "<uid4@x.com>", UID: 4, Sender: "a@x.com", Folder: "INBOX", Date: time.Now()},
		{MessageID: "<uid5@x.com>", UID: 5, Sender: "a@x.com", Folder: "INBOX", Date: time.Now()},
	}
	for _, e := range emails {
		if err := c.CacheEmail(e); err != nil {
			t.Fatalf("CacheEmail: %v", err)
		}
	}

	if err := c.DeleteEmailsByUIDs("INBOX", []uint32{1, 3, 5}); err != nil {
		t.Fatalf("DeleteEmailsByUIDs: %v", err)
	}

	ids, _ := c.GetCachedIDs("INBOX")
	for _, gone := range []string{"<uid1@x.com>", "<uid3@x.com>", "<uid5@x.com>"} {
		if ids[gone] {
			t.Errorf("expected %s to be deleted", gone)
		}
	}
	for _, kept := range []string{"<uid2@x.com>", "<uid4@x.com>"} {
		if !ids[kept] {
			t.Errorf("expected %s to remain", kept)
		}
	}
}

func TestDeleteEmailsByUIDs_Empty(t *testing.T) {
	c := newTestCache(t)
	// No-op on empty slice should not error
	if err := c.DeleteEmailsByUIDs("INBOX", nil); err != nil {
		t.Fatalf("DeleteEmailsByUIDs with nil: %v", err)
	}
	if err := c.DeleteEmailsByUIDs("INBOX", []uint32{}); err != nil {
		t.Fatalf("DeleteEmailsByUIDs with empty: %v", err)
	}
}

func TestDeleteEmailsByMessageIDs(t *testing.T) {
	c := newTestCache(t)

	emails := []*models.EmailData{
		{MessageID: "<uid1@x.com>", UID: 1, Sender: "a@x.com", Folder: "INBOX", Date: time.Now()},
		{MessageID: "<uid2@x.com>", UID: 2, Sender: "a@x.com", Folder: "INBOX", Date: time.Now()},
		{MessageID: "<uid3@x.com>", UID: 3, Sender: "a@x.com", Folder: "INBOX", Date: time.Now()},
	}
	for _, e := range emails {
		if err := c.CacheEmail(e); err != nil {
			t.Fatalf("CacheEmail: %v", err)
		}
	}

	if err := c.DeleteEmailsByMessageIDs("INBOX", []string{"<uid1@x.com>", "<uid3@x.com>"}); err != nil {
		t.Fatalf("DeleteEmailsByMessageIDs: %v", err)
	}

	ids, err := c.GetCachedIDs("INBOX")
	if err != nil {
		t.Fatalf("GetCachedIDs: %v", err)
	}
	if ids["<uid1@x.com>"] || ids["<uid3@x.com>"] {
		t.Fatalf("expected deleted message IDs to be removed, got %v", ids)
	}
	if !ids["<uid2@x.com>"] {
		t.Fatalf("expected untouched message to remain, got %v", ids)
	}
}

// --- ClearFolder ---

func TestClearFolder(t *testing.T) {
	c := newTestCache(t)

	inbox1 := &models.EmailData{MessageID: "<i1@x.com>", Sender: "a@x.com", Folder: "INBOX", Date: time.Now()}
	inbox2 := &models.EmailData{MessageID: "<i2@x.com>", Sender: "b@x.com", Folder: "INBOX", Date: time.Now()}
	sent := &models.EmailData{MessageID: "<s1@x.com>", Sender: "a@x.com", Folder: "Sent", Date: time.Now()}

	for _, e := range []*models.EmailData{inbox1, inbox2, sent} {
		if err := c.CacheEmail(e); err != nil {
			t.Fatalf("CacheEmail: %v", err)
		}
	}

	if err := c.ClearFolder("INBOX"); err != nil {
		t.Fatalf("ClearFolder: %v", err)
	}

	inboxIDs, _ := c.GetCachedIDs("INBOX")
	if len(inboxIDs) != 0 {
		t.Errorf("expected INBOX to be empty after ClearFolder, got %d entries", len(inboxIDs))
	}

	sentIDs, _ := c.GetCachedIDs("Sent")
	if !sentIDs["<s1@x.com>"] {
		t.Error("Sent folder should be unaffected by ClearFolder(INBOX)")
	}
}

// --- Rules CRUD ---

func TestSaveAndGetRule(t *testing.T) {
	c := newTestCache(t)

	rule := &models.Rule{
		Name:         "test-rule",
		Enabled:      true,
		Priority:     10,
		TriggerType:  models.TriggerSender,
		TriggerValue: "spam@example.com",
		Actions: []models.RuleAction{
			{Type: models.ActionDelete},
			{Type: models.ActionNotify, NotifyTitle: "Spam!", NotifyBody: "Got one"},
		},
	}

	if err := c.SaveRule(rule); err != nil {
		t.Fatalf("SaveRule: %v", err)
	}
	if rule.ID == 0 {
		t.Fatal("expected rule.ID to be set after insert")
	}

	rules, err := c.GetEnabledRules()
	if err != nil {
		t.Fatalf("GetEnabledRules: %v", err)
	}
	if len(rules) != 1 {
		t.Fatalf("expected 1 rule, got %d", len(rules))
	}

	got := rules[0]
	if got.Name != rule.Name {
		t.Errorf("Name: got %q, want %q", got.Name, rule.Name)
	}
	if !got.Enabled {
		t.Error("expected Enabled=true")
	}
	if got.Priority != 10 {
		t.Errorf("Priority: got %d, want 10", got.Priority)
	}
	if got.TriggerType != models.TriggerSender {
		t.Errorf("TriggerType: got %q, want %q", got.TriggerType, models.TriggerSender)
	}
	if got.TriggerValue != "spam@example.com" {
		t.Errorf("TriggerValue: got %q", got.TriggerValue)
	}
	if len(got.Actions) != 2 {
		t.Fatalf("expected 2 actions, got %d", len(got.Actions))
	}
	if got.Actions[0].Type != models.ActionDelete {
		t.Errorf("Actions[0].Type: got %q, want %q", got.Actions[0].Type, models.ActionDelete)
	}
	if got.Actions[1].NotifyTitle != "Spam!" {
		t.Errorf("Actions[1].NotifyTitle: got %q, want %q", got.Actions[1].NotifyTitle, "Spam!")
	}
	if got.CustomPromptID != nil {
		t.Errorf("expected CustomPromptID to be nil, got %v", got.CustomPromptID)
	}

	// Test with a non-nil CustomPromptID FK value
	prompt := &models.CustomPrompt{
		Name:         "test-prompt",
		SystemText:   "You are a classifier.",
		UserTemplate: "Classify: {{.Subject}}",
		OutputVar:    "category",
	}
	if err := c.SaveCustomPrompt(prompt); err != nil {
		t.Fatalf("SaveCustomPrompt: %v", err)
	}

	ruleWithPrompt := &models.Rule{
		Name:           "rule-with-prompt",
		Enabled:        true,
		Priority:       20,
		TriggerType:    models.TriggerSender,
		TriggerValue:   "promo@example.com",
		CustomPromptID: &prompt.ID,
		Actions:        []models.RuleAction{{Type: models.ActionNotify}},
	}
	if err := c.SaveRule(ruleWithPrompt); err != nil {
		t.Fatalf("SaveRule with prompt: %v", err)
	}

	gotWithPrompt, err := c.GetRuleByID(ruleWithPrompt.ID)
	if err != nil {
		t.Fatalf("GetRuleByID: %v", err)
	}
	if gotWithPrompt == nil {
		t.Fatal("expected rule with prompt, got nil")
	}
	if gotWithPrompt.CustomPromptID == nil {
		t.Fatal("expected CustomPromptID to be non-nil")
	}
	if *gotWithPrompt.CustomPromptID != prompt.ID {
		t.Errorf("CustomPromptID: got %d, want %d", *gotWithPrompt.CustomPromptID, prompt.ID)
	}
}

func TestGetRuleByID(t *testing.T) {
	c := newTestCache(t)

	rule := &models.Rule{
		Name:         "by-id-rule",
		Enabled:      true,
		TriggerType:  models.TriggerDomain,
		TriggerValue: "example.com",
		Actions:      []models.RuleAction{{Type: models.ActionArchive}},
	}
	if err := c.SaveRule(rule); err != nil {
		t.Fatalf("SaveRule: %v", err)
	}

	got, err := c.GetRuleByID(rule.ID)
	if err != nil {
		t.Fatalf("GetRuleByID: %v", err)
	}
	if got == nil {
		t.Fatal("expected rule, got nil")
	}
	if got.ID != rule.ID {
		t.Errorf("ID: got %d, want %d", got.ID, rule.ID)
	}
	if got.Name != "by-id-rule" {
		t.Errorf("Name: got %q", got.Name)
	}
}

func TestDeleteRule(t *testing.T) {
	c := newTestCache(t)

	rule := &models.Rule{
		Name:         "to-delete",
		Enabled:      true,
		TriggerType:  models.TriggerCategory,
		TriggerValue: "spam",
		Actions:      []models.RuleAction{{Type: models.ActionDelete}},
	}
	if err := c.SaveRule(rule); err != nil {
		t.Fatalf("SaveRule: %v", err)
	}

	if err := c.DeleteRule(rule.ID); err != nil {
		t.Fatalf("DeleteRule: %v", err)
	}

	rules, err := c.GetEnabledRules()
	if err != nil {
		t.Fatalf("GetEnabledRules: %v", err)
	}
	if len(rules) != 0 {
		t.Errorf("expected 0 rules after delete, got %d", len(rules))
	}

	got, err := c.GetRuleByID(rule.ID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != nil {
		t.Error("expected rule to be absent after DeleteRule")
	}
}

// --- CustomPrompts CRUD ---

func TestSaveAndGetCustomPrompt(t *testing.T) {
	c := newTestCache(t)

	p := &models.CustomPrompt{
		Name:         "classify-newsletter",
		SystemText:   "You are an email classifier.",
		UserTemplate: "Is this a newsletter? {{.Subject}}",
		OutputVar:    "is_newsletter",
	}
	if err := c.SaveCustomPrompt(p); err != nil {
		t.Fatalf("SaveCustomPrompt: %v", err)
	}
	if p.ID == 0 {
		t.Fatal("expected p.ID to be set after insert")
	}

	got, err := c.GetCustomPrompt(p.ID)
	if err != nil {
		t.Fatalf("GetCustomPrompt: %v", err)
	}
	if got == nil {
		t.Fatal("expected prompt, got nil")
	}
	if got.Name != p.Name {
		t.Errorf("Name: got %q, want %q", got.Name, p.Name)
	}
	if got.SystemText != p.SystemText {
		t.Errorf("SystemText: got %q", got.SystemText)
	}
	if got.UserTemplate != p.UserTemplate {
		t.Errorf("UserTemplate: got %q", got.UserTemplate)
	}
	if got.OutputVar != p.OutputVar {
		t.Errorf("OutputVar: got %q", got.OutputVar)
	}

	all, err := c.GetAllCustomPrompts()
	if err != nil {
		t.Fatalf("GetAllCustomPrompts: %v", err)
	}
	if len(all) != 1 {
		t.Errorf("expected 1 prompt, got %d", len(all))
	}
}

// --- AppendActionLog ---

func TestAppendActionLog(t *testing.T) {
	c := newTestCache(t)

	// Need a rule to satisfy the FK constraint
	rule := &models.Rule{
		Name:         "log-test-rule",
		Enabled:      true,
		TriggerType:  models.TriggerSender,
		TriggerValue: "a@b.com",
		Actions:      []models.RuleAction{{Type: models.ActionNotify}},
	}
	if err := c.SaveRule(rule); err != nil {
		t.Fatalf("SaveRule: %v", err)
	}

	entry := &models.RuleActionLogEntry{
		RuleID:     rule.ID,
		MessageID:  "<test@example.com>",
		ActionType: models.ActionNotify,
		Status:     "ok",
		Detail:     "notification sent",
		ExecutedAt: time.Now(),
	}
	if err := c.AppendActionLog(entry); err != nil {
		t.Fatalf("AppendActionLog: %v", err)
	}

	// verify row was persisted
	var count int
	if err := c.db.QueryRow("SELECT COUNT(*) FROM rule_action_log WHERE rule_id=?", rule.ID).Scan(&count); err != nil {
		t.Fatalf("query error: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 log entry, got %d", count)
	}
}

// --- TouchRuleLastTriggered ---

func TestTouchRuleLastTriggered(t *testing.T) {
	c := newTestCache(t)

	rule := &models.Rule{
		Name:         "touch-test-rule",
		Enabled:      true,
		TriggerType:  models.TriggerSender,
		TriggerValue: "x@y.com",
		Actions:      []models.RuleAction{{Type: models.ActionDelete}},
	}
	if err := c.SaveRule(rule); err != nil {
		t.Fatalf("SaveRule: %v", err)
	}

	if err := c.TouchRuleLastTriggered(rule.ID); err != nil {
		t.Fatalf("TouchRuleLastTriggered: %v", err)
	}

	got, err := c.GetRuleByID(rule.ID)
	if err != nil {
		t.Fatalf("GetRuleByID: %v", err)
	}
	if got.LastTriggered == nil {
		t.Error("expected LastTriggered to be non-nil after touch")
	}
}

// --- Schema migrations: email_embedding_chunks and contacts ---

func TestInitDB_CreatesEmbeddingChunksTable(t *testing.T) {
	c := newTestCache(t)

	// Insert a row to confirm the table and all columns exist
	_, err := c.db.Exec(`
		INSERT INTO email_embedding_chunks (message_id, chunk_index, embedding, content_hash, embedded_at)
		VALUES (?, ?, ?, ?, ?)`,
		"<test@example.com>", 0, []byte{0x01, 0x02}, "abc123", time.Now(),
	)
	if err != nil {
		t.Fatalf("email_embedding_chunks insert failed: %v", err)
	}

	// UNIQUE constraint: duplicate (message_id, chunk_index) must be rejected
	_, err = c.db.Exec(`
		INSERT INTO email_embedding_chunks (message_id, chunk_index, embedding, content_hash, embedded_at)
		VALUES (?, ?, ?, ?, ?)`,
		"<test@example.com>", 0, []byte{0x03}, "def456", time.Now(),
	)
	if err == nil {
		t.Fatal("expected UNIQUE constraint violation for duplicate (message_id, chunk_index), got nil")
	}
}

func TestInitDB_CreatesContactsTable(t *testing.T) {
	c := newTestCache(t)

	now := time.Now()

	// Insert a contact with required fields
	_, err := c.db.Exec(`
		INSERT INTO contacts (email, display_name, company, topics, notes, first_seen, last_seen, email_count, sent_count)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		"alice@example.com", "Alice", "Acme", `["go","email"]`, "", now, now, 5, 1,
	)
	if err != nil {
		t.Fatalf("contacts insert failed: %v", err)
	}

	// UNIQUE constraint on email: duplicate email must be rejected
	_, err = c.db.Exec(`
		INSERT INTO contacts (email, first_seen, last_seen)
		VALUES (?, ?, ?)`,
		"alice@example.com", now, now,
	)
	if err == nil {
		t.Fatal("expected UNIQUE constraint violation for duplicate email, got nil")
	}

	// Nullable columns (carddav_uid, enriched_at, embedding) should accept NULL
	_, err = c.db.Exec(`
		INSERT INTO contacts (email, first_seen, last_seen, carddav_uid, enriched_at, embedding)
		VALUES (?, ?, ?, NULL, NULL, NULL)`,
		"bob@example.com", now, now,
	)
	if err != nil {
		t.Fatalf("contacts insert with NULL optionals failed: %v", err)
	}
}

// --- UpsertContacts ---

func TestUpsertContacts_FromDirection(t *testing.T) {
	c := newTestCache(t)

	addrs := []models.ContactAddr{
		{Email: "alice@example.com", Name: "Alice"},
	}

	if err := c.UpsertContacts(addrs, "from"); err != nil {
		t.Fatalf("UpsertContacts: %v", err)
	}

	var emailCount, sentCount int
	var displayName string
	err := c.db.QueryRow(`SELECT display_name, email_count, sent_count FROM contacts WHERE email = ?`, "alice@example.com").
		Scan(&displayName, &emailCount, &sentCount)
	if err != nil {
		t.Fatalf("query contact: %v", err)
	}
	if displayName != "Alice" {
		t.Errorf("display_name = %q, want %q", displayName, "Alice")
	}
	if emailCount != 1 {
		t.Errorf("email_count = %d, want 1", emailCount)
	}
	if sentCount != 0 {
		t.Errorf("sent_count = %d, want 0", sentCount)
	}
}

func TestUpsertContacts_ToDirection(t *testing.T) {
	c := newTestCache(t)

	addrs := []models.ContactAddr{
		{Email: "bob@example.com", Name: "Bob"},
	}

	if err := c.UpsertContacts(addrs, "to"); err != nil {
		t.Fatalf("UpsertContacts: %v", err)
	}

	var emailCount, sentCount int
	err := c.db.QueryRow(`SELECT email_count, sent_count FROM contacts WHERE email = ?`, "bob@example.com").
		Scan(&emailCount, &sentCount)
	if err != nil {
		t.Fatalf("query contact: %v", err)
	}
	if emailCount != 0 {
		t.Errorf("email_count = %d, want 0", emailCount)
	}
	if sentCount != 1 {
		t.Errorf("sent_count = %d, want 1", sentCount)
	}
}

func TestUpsertContacts_ConflictIncrementsCounters(t *testing.T) {
	c := newTestCache(t)

	addr := []models.ContactAddr{{Email: "carol@example.com", Name: "Carol"}}

	// Insert twice as "from"
	if err := c.UpsertContacts(addr, "from"); err != nil {
		t.Fatalf("first UpsertContacts: %v", err)
	}
	if err := c.UpsertContacts(addr, "from"); err != nil {
		t.Fatalf("second UpsertContacts: %v", err)
	}

	var emailCount int
	if err := c.db.QueryRow(`SELECT email_count FROM contacts WHERE email = ?`, "carol@example.com").Scan(&emailCount); err != nil {
		t.Fatalf("query: %v", err)
	}
	if emailCount != 2 {
		t.Errorf("email_count = %d, want 2", emailCount)
	}
}

func TestUpsertContacts_PreservesExistingDisplayName(t *testing.T) {
	c := newTestCache(t)

	// First insert with a name
	if err := c.UpsertContacts([]models.ContactAddr{{Email: "dave@example.com", Name: "Dave"}}, "from"); err != nil {
		t.Fatalf("first upsert: %v", err)
	}

	// Second upsert with empty name — existing name should be kept
	if err := c.UpsertContacts([]models.ContactAddr{{Email: "dave@example.com", Name: ""}}, "from"); err != nil {
		t.Fatalf("second upsert: %v", err)
	}

	var displayName string
	if err := c.db.QueryRow(`SELECT display_name FROM contacts WHERE email = ?`, "dave@example.com").Scan(&displayName); err != nil {
		t.Fatalf("query: %v", err)
	}
	if displayName != "Dave" {
		t.Errorf("display_name = %q, want %q", displayName, "Dave")
	}
}

func TestUpsertContacts_SkipsEmptyEmail(t *testing.T) {
	c := newTestCache(t)

	addrs := []models.ContactAddr{
		{Email: "", Name: "Nobody"},
		{Email: "valid@example.com", Name: "Valid"},
	}

	if err := c.UpsertContacts(addrs, "from"); err != nil {
		t.Fatalf("UpsertContacts: %v", err)
	}

	var count int
	if err := c.db.QueryRow(`SELECT COUNT(*) FROM contacts`).Scan(&count); err != nil {
		t.Fatalf("count query: %v", err)
	}
	if count != 1 {
		t.Errorf("contacts count = %d, want 1 (empty email should be skipped)", count)
	}
}

func TestUpsertContacts_EmptySlice(t *testing.T) {
	c := newTestCache(t)

	// Should be a no-op without error
	if err := c.UpsertContacts(nil, "from"); err != nil {
		t.Fatalf("UpsertContacts(nil): %v", err)
	}
	if err := c.UpsertContacts([]models.ContactAddr{}, "to"); err != nil {
		t.Fatalf("UpsertContacts(empty): %v", err)
	}

	var count int
	if err := c.db.QueryRow(`SELECT COUNT(*) FROM contacts`).Scan(&count); err != nil {
		t.Fatalf("count query: %v", err)
	}
	if count != 0 {
		t.Errorf("contacts count = %d, want 0", count)
	}
}

func TestUpsertContacts_MixedDirectionAccumulates(t *testing.T) {
	c := newTestCache(t)
	addr := []models.ContactAddr{{Email: "x@example.com", Name: "X"}}
	if err := c.UpsertContacts(addr, "to"); err != nil {
		t.Fatal(err)
	}
	if err := c.UpsertContacts(addr, "from"); err != nil {
		t.Fatal(err)
	}
	var emailCount, sentCount int
	err := c.db.QueryRow(`SELECT email_count, sent_count FROM contacts WHERE email = ?`, "x@example.com").Scan(&emailCount, &sentCount)
	if err != nil {
		t.Fatal(err)
	}
	if emailCount != 1 {
		t.Errorf("email_count = %d, want 1", emailCount)
	}
	if sentCount != 1 {
		t.Errorf("sent_count = %d, want 1", sentCount)
	}
}

func TestUpsertContacts_FillsBlankDisplayName(t *testing.T) {
	c := newTestCache(t)
	// First upsert: no name
	addr1 := []models.ContactAddr{{Email: "y@example.com", Name: ""}}
	if err := c.UpsertContacts(addr1, "from"); err != nil {
		t.Fatal(err)
	}
	// Second upsert: provides name
	addr2 := []models.ContactAddr{{Email: "y@example.com", Name: "Yvonne"}}
	if err := c.UpsertContacts(addr2, "from"); err != nil {
		t.Fatal(err)
	}
	var name string
	err := c.db.QueryRow(`SELECT display_name FROM contacts WHERE email = ?`, "y@example.com").Scan(&name)
	if err != nil {
		t.Fatal(err)
	}
	if name != "Yvonne" {
		t.Errorf("display_name = %q, want %q", name, "Yvonne")
	}
}

// ---------------------------------------------------------------------------
// Helpers for chunked embedding tests
// ---------------------------------------------------------------------------

func insertTestEmail(t *testing.T, c *Cache, messageID, folder, bodyText string) {
	t.Helper()
	_, err := c.db.Exec(
		`INSERT INTO emails (message_id, sender, subject, date, size, has_attachments, folder, body_text)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		messageID, "sender@example.com", "Subject", time.Now(), 100, 0, folder, bodyText,
	)
	if err != nil {
		t.Fatalf("insertTestEmail(%s): %v", messageID, err)
	}
}

func countChunkRows(t *testing.T, c *Cache, messageID string) int {
	t.Helper()
	var n int
	if err := c.db.QueryRow(`SELECT COUNT(*) FROM email_embedding_chunks WHERE message_id = ?`, messageID).Scan(&n); err != nil {
		t.Fatalf("countChunkRows: %v", err)
	}
	return n
}

func makeTestChunks(messageID string, n int) []models.EmbeddingChunk {
	chunks := make([]models.EmbeddingChunk, n)
	for i := range chunks {
		vec := make([]float32, 4)
		vec[i%4] = float32(i + 1)
		chunks[i] = models.EmbeddingChunk{
			MessageID:   messageID,
			ChunkIndex:  i,
			Embedding:   vec,
			ContentHash: "hash" + string(rune('0'+i)),
		}
	}
	return chunks
}

// ---------------------------------------------------------------------------
// StoreEmbeddingChunks
// ---------------------------------------------------------------------------

func TestStoreEmbeddingChunks_Basic(t *testing.T) {
	c := newTestCache(t)
	insertTestEmail(t, c, "msg1", "INBOX", "hello world")

	if err := c.StoreEmbeddingChunks("msg1", makeTestChunks("msg1", 2)); err != nil {
		t.Fatalf("StoreEmbeddingChunks: %v", err)
	}
	if got := countChunkRows(t, c, "msg1"); got != 2 {
		t.Errorf("expected 2 chunks, got %d", got)
	}
}

func TestStoreEmbeddingChunks_Replaces(t *testing.T) {
	c := newTestCache(t)
	insertTestEmail(t, c, "msg1", "INBOX", "hello world")

	if err := c.StoreEmbeddingChunks("msg1", makeTestChunks("msg1", 3)); err != nil {
		t.Fatalf("first StoreEmbeddingChunks: %v", err)
	}
	if got := countChunkRows(t, c, "msg1"); got != 3 {
		t.Errorf("expected 3 chunks after first store, got %d", got)
	}

	// Second store with 1 chunk should replace all 3
	if err := c.StoreEmbeddingChunks("msg1", makeTestChunks("msg1", 1)); err != nil {
		t.Fatalf("second StoreEmbeddingChunks: %v", err)
	}
	if got := countChunkRows(t, c, "msg1"); got != 1 {
		t.Errorf("expected 1 chunk after replace, got %d", got)
	}
}

// ---------------------------------------------------------------------------
// GetUnembeddedIDsWithBody
// ---------------------------------------------------------------------------

func TestGetUnembeddedIDsWithBody(t *testing.T) {
	c := newTestCache(t)

	// Has body AND chunks → must NOT appear
	insertTestEmail(t, c, "has-both", "INBOX", "body text")
	if err := c.StoreEmbeddingChunks("has-both", makeTestChunks("has-both", 1)); err != nil {
		t.Fatalf("StoreEmbeddingChunks: %v", err)
	}

	// Has body, NO chunks → must appear
	insertTestEmail(t, c, "body-only", "INBOX", "another body")

	// No body → must NOT appear
	insertTestEmail(t, c, "no-body", "INBOX", "")

	ids, err := c.GetUnembeddedIDsWithBody("INBOX")
	if err != nil {
		t.Fatalf("GetUnembeddedIDsWithBody: %v", err)
	}
	if len(ids) != 1 || ids[0] != "body-only" {
		t.Errorf("expected [body-only], got %v", ids)
	}
}

// ---------------------------------------------------------------------------
// GetUncachedBodyIDs
// ---------------------------------------------------------------------------

func TestGetUncachedBodyIDs(t *testing.T) {
	c := newTestCache(t)

	// Has body → must NOT appear
	insertTestEmail(t, c, "has-body", "INBOX", "some text")

	// Has chunks but no body → must NOT appear
	insertTestEmail(t, c, "has-chunks", "INBOX", "")
	if err := c.StoreEmbeddingChunks("has-chunks", makeTestChunks("has-chunks", 1)); err != nil {
		t.Fatalf("StoreEmbeddingChunks: %v", err)
	}

	// Neither body nor chunks → must appear
	insertTestEmail(t, c, "naked", "INBOX", "")

	ids, err := c.GetUncachedBodyIDs("INBOX", 10)
	if err != nil {
		t.Fatalf("GetUncachedBodyIDs: %v", err)
	}
	if len(ids) != 1 || ids[0] != "naked" {
		t.Errorf("expected [naked], got %v", ids)
	}
}

// ---------------------------------------------------------------------------
// GetEmbeddingProgress
// ---------------------------------------------------------------------------

func TestGetEmbeddingProgress(t *testing.T) {
	c := newTestCache(t)

	insertTestEmail(t, c, "e1", "INBOX", "body1")
	insertTestEmail(t, c, "e2", "INBOX", "body2")
	insertTestEmail(t, c, "e3", "INBOX", "body3")

	// e1 and e2 get chunks; e3 does not
	if err := c.StoreEmbeddingChunks("e1", makeTestChunks("e1", 2)); err != nil {
		t.Fatalf("StoreEmbeddingChunks e1: %v", err)
	}
	if err := c.StoreEmbeddingChunks("e2", makeTestChunks("e2", 1)); err != nil {
		t.Fatalf("StoreEmbeddingChunks e2: %v", err)
	}

	done, total, err := c.GetEmbeddingProgress("INBOX")
	if err != nil {
		t.Fatalf("GetEmbeddingProgress: %v", err)
	}
	if done != 2 {
		t.Errorf("done = %d, want 2", done)
	}
	if total != 3 {
		t.Errorf("total = %d, want 3", total)
	}
}

// ---------------------------------------------------------------------------
// SearchSemanticChunked
// ---------------------------------------------------------------------------

func TestSearchSemanticChunked(t *testing.T) {
	c := newTestCache(t)

	insertTestEmail(t, c, "close", "INBOX", "very relevant content")
	insertTestEmail(t, c, "far", "INBOX", "unrelated content")

	// "close" chunk: [1,0,0,0] — identical direction to query
	closeChunk := models.EmbeddingChunk{
		MessageID:   "close",
		ChunkIndex:  0,
		Embedding:   []float32{1, 0, 0, 0},
		ContentHash: "close-hash",
	}
	if err := c.StoreEmbeddingChunks("close", []models.EmbeddingChunk{closeChunk}); err != nil {
		t.Fatalf("StoreEmbeddingChunks close: %v", err)
	}

	// "far" chunk: [0,1,0,0] — orthogonal to query (cosine = 0)
	farChunk := models.EmbeddingChunk{
		MessageID:   "far",
		ChunkIndex:  0,
		Embedding:   []float32{0, 1, 0, 0},
		ContentHash: "far-hash",
	}
	if err := c.StoreEmbeddingChunks("far", []models.EmbeddingChunk{farChunk}); err != nil {
		t.Fatalf("StoreEmbeddingChunks far: %v", err)
	}

	queryVec := []float32{1, 0, 0, 0}

	results, err := c.SearchSemanticChunked("INBOX", queryVec, 10, 0.5)
	if err != nil {
		t.Fatalf("SearchSemanticChunked: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result above minScore 0.5, got %d", len(results))
	}
	if results[0].Email.MessageID != "close" {
		t.Errorf("expected first result to be 'close', got %q", results[0].Email.MessageID)
	}
	if results[0].Score <= 0 {
		t.Errorf("expected positive score, got %f", results[0].Score)
	}
}

// --- GetContactByEmail ---

func TestGetContactByEmail(t *testing.T) {
	c := newTestCache(t)

	// Insert a contact via UpsertContacts.
	addrs := []models.ContactAddr{
		{Email: "alice@example.com", Name: "Alice Example"},
	}
	if err := c.UpsertContacts(addrs, "received"); err != nil {
		t.Fatalf("UpsertContacts: %v", err)
	}

	// Should find the contact.
	cd, err := c.GetContactByEmail("alice@example.com")
	if err != nil {
		t.Fatalf("GetContactByEmail: %v", err)
	}
	if cd == nil {
		t.Fatal("expected non-nil contact, got nil")
	}
	if cd.Email != "alice@example.com" {
		t.Errorf("email = %q, want %q", cd.Email, "alice@example.com")
	}
	if cd.DisplayName != "Alice Example" {
		t.Errorf("display_name = %q, want %q", cd.DisplayName, "Alice Example")
	}

	// Should return nil for unknown address.
	cd2, err := c.GetContactByEmail("unknown@nowhere.com")
	if err != nil {
		t.Fatalf("GetContactByEmail unknown: %v", err)
	}
	if cd2 != nil {
		t.Errorf("expected nil for unknown email, got %+v", cd2)
	}
}

// --- UpdateStarred ---

func TestUpdateStarred(t *testing.T) {
	c := newTestCache(t)

	email := &models.EmailData{
		MessageID: "star-test-1",
		UID:       42,
		Sender:    "test@example.com",
		Subject:   "Test Star",
		Date:      time.Now(),
		Size:      1024,
		Folder:    "INBOX",
	}
	if err := c.CacheEmail(email); err != nil {
		t.Fatalf("CacheEmail: %v", err)
	}

	// Initially not starred.
	got, err := c.GetEmailByID(email.MessageID)
	if err != nil {
		t.Fatalf("GetEmailByID: %v", err)
	}
	if got.IsStarred {
		t.Error("expected IsStarred=false initially, got true")
	}

	// Star it.
	if err := c.UpdateStarred(email.MessageID, true); err != nil {
		t.Fatalf("UpdateStarred(true): %v", err)
	}
	got, err = c.GetEmailByID(email.MessageID)
	if err != nil {
		t.Fatalf("GetEmailByID after star: %v", err)
	}
	if !got.IsStarred {
		t.Error("expected IsStarred=true after UpdateStarred(true), got false")
	}

	// Unstar it.
	if err := c.UpdateStarred(email.MessageID, false); err != nil {
		t.Fatalf("UpdateStarred(false): %v", err)
	}
	got, err = c.GetEmailByID(email.MessageID)
	if err != nil {
		t.Fatalf("GetEmailByID after unstar: %v", err)
	}
	if got.IsStarred {
		t.Error("expected IsStarred=false after UpdateStarred(false), got true")
	}
}
