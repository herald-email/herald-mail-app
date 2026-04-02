package app

import (
	"encoding/json"
	"testing"

	"mail-processor/internal/models"
)

// stubBackend is a minimal backend.Backend implementation for unit tests.
// Only the methods called by chatToolRegistry dispatch are implemented.
type stubBackend struct {
	searchResult []*models.EmailData
	threadResult []*models.EmailData
	statsResult  map[string]*models.SenderStats
	searchErr    error
}

func (s *stubBackend) Load(_ string)                                     {}
func (s *stubBackend) GetSenderStatistics(_ string) (map[string]*models.SenderStats, error) {
	return s.statsResult, s.searchErr
}
func (s *stubBackend) GetEmailsBySender(_ string) (map[string][]*models.EmailData, error) {
	return nil, nil
}
func (s *stubBackend) DeleteSenderEmails(_, _ string) error  { return nil }
func (s *stubBackend) DeleteDomainEmails(_, _ string) error  { return nil }
func (s *stubBackend) DeleteEmail(_, _ string) error         { return nil }
func (s *stubBackend) ListFolders() ([]string, error)        { return nil, nil }
func (s *stubBackend) GetFolderStatus(_ []string) (map[string]models.FolderStatus, error) {
	return nil, nil
}
func (s *stubBackend) GetTimelineEmails(_ string) ([]*models.EmailData, error) { return nil, nil }
func (s *stubBackend) GetClassifications(_ string) (map[string]string, error)  { return nil, nil }
func (s *stubBackend) SetClassification(_, _ string) error                     { return nil }
func (s *stubBackend) GetUnclassifiedIDs(_ string) ([]string, error)           { return nil, nil }
func (s *stubBackend) GetEmailByID(_ string) (*models.EmailData, error)        { return nil, nil }
func (s *stubBackend) FetchEmailBody(_ string, _ uint32) (*models.EmailBody, error) {
	return nil, nil
}
func (s *stubBackend) SaveAttachment(_ *models.Attachment, _ string) error { return nil }
func (s *stubBackend) SetGroupByDomain(_ bool)                             {}
func (s *stubBackend) Progress() <-chan models.ProgressInfo                { return nil }
func (s *stubBackend) Close() error                                        { return nil }
func (s *stubBackend) ArchiveEmail(_, _ string) error                      { return nil }
func (s *stubBackend) ArchiveSenderEmails(_, _ string) error               { return nil }
func (s *stubBackend) SearchEmails(_, _ string, _ bool) ([]*models.EmailData, error) {
	return s.searchResult, s.searchErr
}
func (s *stubBackend) SearchEmailsCrossFolder(_ string) ([]*models.EmailData, error) {
	return nil, nil
}
func (s *stubBackend) SearchEmailsIMAP(_, _ string) ([]*models.EmailData, error) { return nil, nil }
func (s *stubBackend) SearchEmailsSemantic(_, _ string, _ int, _ float64) ([]*models.EmailData, error) {
	return nil, nil
}
func (s *stubBackend) GetSavedSearches() ([]*models.SavedSearch, error) { return nil, nil }
func (s *stubBackend) SaveSearch(_, _, _ string) error                  { return nil }
func (s *stubBackend) DeleteSavedSearch(_ int) error                    { return nil }
func (s *stubBackend) MarkRead(_, _ string) error                       { return nil }
func (s *stubBackend) MarkUnread(_, _ string) error                     { return nil }
func (s *stubBackend) MarkStarred(_, _ string) error                    { return nil }
func (s *stubBackend) UnmarkStarred(_, _ string) error                  { return nil }
func (s *stubBackend) GetEmailsByThread(_, _ string) ([]*models.EmailData, error) {
	return s.threadResult, s.searchErr
}
func (s *stubBackend) SendEmail(_, _, _, _ string) error                              { return nil }
func (s *stubBackend) UpdateUnsubscribeHeaders(_, _, _ string) error                  { return nil }
func (s *stubBackend) CacheBodyText(_, _ string) error                                { return nil }
func (s *stubBackend) StoreEmbedding(_ string, _ []float32, _ string) error           { return nil }
func (s *stubBackend) GetUnembeddedIDs(_ string) ([]string, error)                    { return nil, nil }
func (s *stubBackend) GetUnembeddedIDsWithBody(_ string) ([]string, error)            { return nil, nil }
func (s *stubBackend) GetUncachedBodyIDs(_ string, _ int) ([]string, error)           { return nil, nil }
func (s *stubBackend) GetEmbeddingProgress(_ string) (int, int, error)                { return 0, 0, nil }
func (s *stubBackend) StoreEmbeddingChunks(_ string, _ []models.EmbeddingChunk) error { return nil }
func (s *stubBackend) SearchSemanticChunked(_ string, _ []float32, _ int, _ float64) ([]*models.SemanticSearchResult, error) {
	return nil, nil
}
func (s *stubBackend) GetBodyText(_ string) (string, error)           { return "", nil }
func (s *stubBackend) FetchAndCacheBody(_ string) (*models.EmailBody, error) { return nil, nil }
func (s *stubBackend) NewEmailsCh() <-chan models.NewEmailsNotification { return nil }
func (s *stubBackend) StartIDLE(_ string) error                         { return nil }
func (s *stubBackend) StopIDLE()                                        {}
func (s *stubBackend) StartPolling(_ string, _ int)                     {}
func (s *stubBackend) StopPolling()                                     {}
func (s *stubBackend) ValidIDsCh() <-chan map[string]bool               { return nil }
func (s *stubBackend) MoveEmail(_, _, _ string) error                   { return nil }
func (s *stubBackend) SaveRule(_ *models.Rule) error                    { return nil }
func (s *stubBackend) GetEnabledRules() ([]*models.Rule, error)         { return nil, nil }
func (s *stubBackend) DeleteRule(_ int64) error                         { return nil }
func (s *stubBackend) GetAllCustomPrompts() ([]*models.CustomPrompt, error) { return nil, nil }
func (s *stubBackend) SaveCustomPrompt(_ *models.CustomPrompt) error    { return nil }
func (s *stubBackend) GetCustomPrompt(_ int64) (*models.CustomPrompt, error) { return nil, nil }
func (s *stubBackend) AppendActionLog(_ *models.RuleActionLogEntry) error { return nil }
func (s *stubBackend) TouchRuleLastTriggered(_ int64) error              { return nil }
func (s *stubBackend) SaveCustomCategory(_ string, _ int64, _ string) error { return nil }
func (s *stubBackend) GetContactsToEnrich(_, _ int) ([]models.ContactData, error) { return nil, nil }
func (s *stubBackend) GetRecentSubjectsByContact(_ string, _ int) ([]string, error) { return nil, nil }
func (s *stubBackend) UpdateContactEnrichment(_, _ string, _ []string) error { return nil }
func (s *stubBackend) UpdateContactEmbedding(_ string, _ []float32) error    { return nil }
func (s *stubBackend) SearchContactsSemantic(_ []float32, _ int, _ float64) ([]*models.ContactSearchResult, error) {
	return nil, nil
}
func (s *stubBackend) ListContacts(_ int, _ string) ([]models.ContactData, error) { return nil, nil }
func (s *stubBackend) SearchContacts(_ string) ([]models.ContactData, error)       { return nil, nil }
func (s *stubBackend) GetContactEmails(_ string, _ int) ([]*models.EmailData, error) {
	return nil, nil
}
func (s *stubBackend) UpsertContacts(_ []models.ContactAddr, _ string) error { return nil }
func (s *stubBackend) GetAllCleanupRules() ([]*models.CleanupRule, error)     { return nil, nil }
func (s *stubBackend) SaveCleanupRule(_ *models.CleanupRule) error             { return nil }
func (s *stubBackend) DeleteCleanupRule(_ int64) error                         { return nil }
func (s *stubBackend) RecordUnsubscribe(_, _, _ string) error                  { return nil }
func (s *stubBackend) IsUnsubscribedSender(_ string) (bool, error)             { return false, nil }
func (s *stubBackend) SaveDraft(_, _, _ string) (uint32, string, error)        { return 0, "", nil }
func (s *stubBackend) ListDrafts() ([]*models.Draft, error)                    { return nil, nil }
func (s *stubBackend) DeleteDraft(_ uint32, _ string) error                    { return nil }
func (s *stubBackend) ReplyToEmail(_, _ string) error                          { return nil }
func (s *stubBackend) ForwardEmail(_, _, _ string) error                       { return nil }
func (s *stubBackend) ListAttachments(_ string) ([]models.Attachment, error)   { return nil, nil }
func (s *stubBackend) GetAttachment(_, _ string) (*models.Attachment, error)   { return nil, nil }
func (s *stubBackend) DeleteThread(_, _ string) error                          { return nil }
func (s *stubBackend) BulkDelete(_ []string) error                             { return nil }
func (s *stubBackend) ArchiveThread(_, _ string) error                         { return nil }
func (s *stubBackend) BulkMove(_ []string, _ string) error                     { return nil }
func (s *stubBackend) UnsubscribeSender(_ string) error                        { return nil }
func (s *stubBackend) SoftUnsubscribeSender(_, _ string) error                 { return nil }
func (s *stubBackend) CreateFolder(_ string) error                             { return nil }
func (s *stubBackend) RenameFolder(_, _ string) error                         { return nil }
func (s *stubBackend) DeleteFolder(_ string) error                             { return nil }
func (s *stubBackend) SyncAllFolders() (int, error)                           { return 0, nil }
func (s *stubBackend) GetSyncStatus() (map[string]models.FolderStatus, error) { return nil, nil }

// newStubModel creates a minimal Model with a stubBackend for testing chat tools.
func newStubModel(b *stubBackend) *Model {
	m := &Model{
		backend:       b,
		currentFolder: "INBOX",
	}
	return m
}

func TestChatToolRegistry_FourTools(t *testing.T) {
	m := newStubModel(&stubBackend{})
	tools, _ := m.chatToolRegistry()
	if len(tools) != 4 {
		t.Fatalf("expected 4 tools, got %d", len(tools))
	}
	names := map[string]bool{}
	for _, tool := range tools {
		names[tool.Name] = true
	}
	expected := []string{"search_emails", "list_emails_by_sender", "get_thread", "get_sender_stats"}
	for _, name := range expected {
		if !names[name] {
			t.Errorf("missing tool: %s", name)
		}
	}
}

func TestChatToolRegistry_DispatchUnknownTool(t *testing.T) {
	m := newStubModel(&stubBackend{})
	_, dispatch := m.chatToolRegistry()
	_, err := dispatch("nonexistent_tool", json.RawMessage(`{}`))
	if err == nil {
		t.Fatal("expected error for unknown tool, got nil")
	}
}

func TestChatToolRegistry_DispatchSearchEmails(t *testing.T) {
	stub := &stubBackend{
		searchResult: []*models.EmailData{
			{MessageID: "msg1", Sender: "test@example.com", Subject: "Invoice #123"},
		},
	}
	m := newStubModel(stub)
	_, dispatch := m.chatToolRegistry()

	result, err := dispatch("search_emails", json.RawMessage(`{"query":"invoice"}`))
	if err != nil {
		t.Fatal(err)
	}
	if result == "" {
		t.Error("expected non-empty result")
	}
	var emails []map[string]string
	if err := json.Unmarshal([]byte(result), &emails); err != nil {
		t.Fatalf("result is not valid JSON: %v", err)
	}
	if len(emails) != 1 {
		t.Errorf("expected 1 email, got %d", len(emails))
	}
}

func TestChatToolRegistry_DispatchGetSenderStats(t *testing.T) {
	stub := &stubBackend{
		statsResult: map[string]*models.SenderStats{
			"sender@example.com": {TotalEmails: 5},
		},
	}
	m := newStubModel(stub)
	_, dispatch := m.chatToolRegistry()

	result, err := dispatch("get_sender_stats", json.RawMessage(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	if result == "" {
		t.Error("expected non-empty result")
	}
}
