package cache

import (
	"testing"
	"time"

	"github.com/herald-email/herald-mail-app/internal/models"
)

func TestSetClassificationByRefStoresScopeColumns(t *testing.T) {
	c := newTestCache(t)
	ref := models.MessageRef{
		SourceID:  "work-mail",
		AccountID: "work",
		Folder:    "INBOX",
		MessageID: "scoped-classification",
	}.WithDefaults()

	if err := c.SetClassificationByRef(ref, "finance"); err != nil {
		t.Fatalf("SetClassificationByRef: %v", err)
	}

	var sourceID, accountID, localID string
	if err := c.db.QueryRow(
		`SELECT source_id, account_id, local_id FROM email_classifications WHERE message_id = ?`,
		ref.MessageID,
	).Scan(&sourceID, &accountID, &localID); err != nil {
		t.Fatalf("query classification scope: %v", err)
	}
	if sourceID != string(ref.SourceID) || accountID != string(ref.AccountID) || localID != ref.LocalID {
		t.Fatalf("scope = (%q,%q,%q), want (%q,%q,%q)", sourceID, accountID, localID, ref.SourceID, ref.AccountID, ref.LocalID)
	}
}

func TestGetClassificationsReturnsScopedAndLegacyKeys(t *testing.T) {
	c := newTestCache(t)
	email := &models.EmailData{
		SourceID:  "work-mail",
		AccountID: "work",
		MessageID: "scoped-visible",
		Sender:    "sender@example.com",
		Subject:   "Scoped visible",
		Date:      time.Now(),
		Folder:    "INBOX",
	}
	if err := c.CacheEmail(email); err != nil {
		t.Fatalf("CacheEmail: %v", err)
	}
	ref := email.MessageRef()
	if err := c.SetClassificationByRef(ref, "travel"); err != nil {
		t.Fatalf("SetClassificationByRef: %v", err)
	}

	got, err := c.GetClassifications("INBOX")
	if err != nil {
		t.Fatalf("GetClassifications: %v", err)
	}
	if got[email.MessageID] != "travel" {
		t.Fatalf("legacy key classification = %q, want travel", got[email.MessageID])
	}
	if got[ref.LocalID] != "travel" {
		t.Fatalf("scoped key classification = %q, want travel", got[ref.LocalID])
	}
}
