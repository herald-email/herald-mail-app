package cleanup

import (
	"mail-processor/internal/models"
)

// noopBackend is a no-op implementation of backend.Backend for tests.
// Individual test mocks embed this and override only the methods they care about.
type noopBackend struct{}

func (noopBackend) Load(folder string)                                                          {}
func (noopBackend) GetSenderStatistics(folder string) (map[string]*models.SenderStats, error)  { return nil, nil }
func (noopBackend) GetEmailsBySender(folder string) (map[string][]*models.EmailData, error)    { return nil, nil }
func (noopBackend) DeleteSenderEmails(sender, folder string) error                             { return nil }
func (noopBackend) DeleteDomainEmails(domain, folder string) error                             { return nil }
func (noopBackend) DeleteEmail(messageID, folder string) error                                 { return nil }
func (noopBackend) ListFolders() ([]string, error)                                             { return nil, nil }
func (noopBackend) GetFolderStatus(folders []string) (map[string]models.FolderStatus, error)  { return nil, nil }
func (noopBackend) GetTimelineEmails(folder string) ([]*models.EmailData, error)               { return nil, nil }
func (noopBackend) GetClassifications(folder string) (map[string]string, error)                { return nil, nil }
func (noopBackend) SetClassification(messageID, category string) error                         { return nil }
func (noopBackend) GetUnclassifiedIDs(folder string) ([]string, error)                        { return nil, nil }
func (noopBackend) GetEmailByID(messageID string) (*models.EmailData, error)                   { return nil, nil }
func (noopBackend) FetchEmailBody(folder string, uid uint32) (*models.EmailBody, error)        { return nil, nil }
func (noopBackend) SaveAttachment(attachment *models.Attachment, destPath string) error        { return nil }
func (noopBackend) SetGroupByDomain(bool)                                                      {}
func (noopBackend) Progress() <-chan models.ProgressInfo                                       { return nil }
func (noopBackend) Close() error                                                               { return nil }
func (noopBackend) ArchiveEmail(messageID, folder string) error                                { return nil }
func (noopBackend) ArchiveSenderEmails(sender, folder string) error                            { return nil }
func (noopBackend) SearchEmails(folder, query string, bodySearch bool) ([]*models.EmailData, error) { return nil, nil }
func (noopBackend) SearchEmailsCrossFolder(query string) ([]*models.EmailData, error)          { return nil, nil }
func (noopBackend) SearchEmailsIMAP(folder, query string) ([]*models.EmailData, error)        { return nil, nil }
func (noopBackend) SearchEmailsSemantic(folder, query string, limit int, minScore float64) ([]*models.EmailData, error) {
	return nil, nil
}
func (noopBackend) GetSavedSearches() ([]*models.SavedSearch, error)         { return nil, nil }
func (noopBackend) SaveSearch(name, query, folder string) error              { return nil }
func (noopBackend) DeleteSavedSearch(id int) error                           { return nil }
func (noopBackend) MarkRead(messageID, folder string) error                  { return nil }
func (noopBackend) MarkUnread(messageID, folder string) error                { return nil }
func (noopBackend) GetEmailsByThread(folder, subject string) ([]*models.EmailData, error) { return nil, nil }
func (noopBackend) SendEmail(to, subject, body, from string) error           { return nil }
func (noopBackend) UpdateUnsubscribeHeaders(messageID, listUnsub, listUnsubPost string) error { return nil }
func (noopBackend) CacheBodyText(messageID, bodyText string) error           { return nil }
func (noopBackend) StoreEmbedding(messageID string, embedding []float32, hash string) error { return nil }
func (noopBackend) GetUnembeddedIDs(folder string) ([]string, error)        { return nil, nil }
func (noopBackend) GetUnembeddedIDsWithBody(folder string) ([]string, error) { return nil, nil }
func (noopBackend) GetUncachedBodyIDs(folder string, limit int) ([]string, error) { return nil, nil }
func (noopBackend) GetEmbeddingProgress(folder string) (done, total int, err error) { return 0, 0, nil }
func (noopBackend) StoreEmbeddingChunks(messageID string, chunks []models.EmbeddingChunk) error { return nil }
func (noopBackend) SearchSemanticChunked(folder string, queryVec []float32, limit int, minScore float64) ([]*models.SemanticSearchResult, error) {
	return nil, nil
}
func (noopBackend) GetBodyText(messageID string) (string, error)             { return "", nil }
func (noopBackend) FetchAndCacheBody(messageID string) (*models.EmailBody, error) { return nil, nil }
func (noopBackend) NewEmailsCh() <-chan models.NewEmailsNotification         { return nil }
func (noopBackend) StartIDLE(folder string) error                            { return nil }
func (noopBackend) StopIDLE()                                                {}
func (noopBackend) StartPolling(folder string, interval int)                 {}
func (noopBackend) StopPolling()                                             {}
func (noopBackend) ValidIDsCh() <-chan map[string]bool                       { return nil }
func (noopBackend) MoveEmail(messageID, fromFolder, toFolder string) error   { return nil }
func (noopBackend) SaveRule(r *models.Rule) error                            { return nil }
func (noopBackend) GetEnabledRules() ([]*models.Rule, error)                 { return nil, nil }
func (noopBackend) DeleteRule(id int64) error                                { return nil }
func (noopBackend) GetAllCustomPrompts() ([]*models.CustomPrompt, error)     { return nil, nil }
func (noopBackend) SaveCustomPrompt(p *models.CustomPrompt) error            { return nil }
func (noopBackend) GetCustomPrompt(id int64) (*models.CustomPrompt, error)   { return nil, nil }
func (noopBackend) AppendActionLog(entry *models.RuleActionLogEntry) error   { return nil }
func (noopBackend) TouchRuleLastTriggered(ruleID int64) error                { return nil }
func (noopBackend) SaveCustomCategory(messageID string, promptID int64, result string) error { return nil }
func (noopBackend) GetContactsToEnrich(minCount, limit int) ([]models.ContactData, error) { return nil, nil }
func (noopBackend) GetRecentSubjectsByContact(email string, limit int) ([]string, error) { return nil, nil }
func (noopBackend) UpdateContactEnrichment(email, company string, topics []string) error { return nil }
func (noopBackend) UpdateContactEmbedding(email string, embedding []float32) error { return nil }
func (noopBackend) SearchContactsSemantic(queryVec []float32, limit int, minScore float64) ([]*models.ContactSearchResult, error) {
	return nil, nil
}
func (noopBackend) ListContacts(limit int, sortBy string) ([]models.ContactData, error) { return nil, nil }
func (noopBackend) SearchContacts(query string) ([]models.ContactData, error) { return nil, nil }
func (noopBackend) GetContactEmails(contactEmail string, limit int) ([]*models.EmailData, error) { return nil, nil }
func (noopBackend) UpsertContacts(addrs []models.ContactAddr, direction string) error { return nil }
func (noopBackend) GetAllCleanupRules() ([]*models.CleanupRule, error)       { return nil, nil }
func (noopBackend) SaveCleanupRule(rule *models.CleanupRule) error           { return nil }
func (noopBackend) DeleteCleanupRule(id int64) error                         { return nil }
