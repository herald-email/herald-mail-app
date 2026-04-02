package backend

import (
	"mail-processor/internal/models"
)

// Backend defines the contract between the UI and email backend logic.
// This decouples the Bubble Tea model from any specific IMAP implementation
// and enables a future daemon/remote backend without touching the UI.
type Backend interface {
	// Load starts background email synchronization for the given folder.
	// It connects, fetches new messages, cleans up stale cache entries,
	// and streams all progress through Progress(). Non-blocking.
	Load(folder string)

	// GetSenderStatistics returns per-sender stats for the given folder.
	GetSenderStatistics(folder string) (map[string]*models.SenderStats, error)

	// GetEmailsBySender returns emails grouped by sender for the given folder.
	GetEmailsBySender(folder string) (map[string][]*models.EmailData, error)

	// DeleteSenderEmails removes all emails from a sender in both IMAP and cache.
	DeleteSenderEmails(sender, folder string) error

	// DeleteDomainEmails removes all emails from a domain in both IMAP and cache.
	DeleteDomainEmails(domain, folder string) error

	// DeleteEmail removes a single email by Message-ID.
	DeleteEmail(messageID, folder string) error

	// ListFolders returns all mailbox names available on the server.
	ListFolders() ([]string, error)

	// GetFolderStatus returns MESSAGES and UNSEEN counts for the given folders.
	GetFolderStatus(folders []string) (map[string]models.FolderStatus, error)

	// GetTimelineEmails returns all emails for a folder sorted by date descending.
	GetTimelineEmails(folder string) ([]*models.EmailData, error)

	// GetClassifications returns AI category tags for emails in a folder.
	GetClassifications(folder string) (map[string]string, error)

	// SetClassification stores an AI category for a single message.
	SetClassification(messageID, category string) error

	// GetUnclassifiedIDs returns message IDs in a folder without a classification.
	GetUnclassifiedIDs(folder string) ([]string, error)

	// GetEmailByID returns a single cached email by message ID.
	GetEmailByID(messageID string) (*models.EmailData, error)

	// FetchEmailBody fetches the full MIME body of an email by UID from the IMAP server.
	FetchEmailBody(folder string, uid uint32) (*models.EmailBody, error)

	// SaveAttachment writes attachment data to the given destination path.
	SaveAttachment(attachment *models.Attachment, destPath string) error

	// SetGroupByDomain toggles domain-level grouping for GetEmailsBySender/GetSenderStatistics.
	SetGroupByDomain(bool)

	// Progress returns a read-only channel of processing progress updates.
	Progress() <-chan models.ProgressInfo

	// Close shuts down all connections and releases resources.
	Close() error

	// --- Archive ---

	// ArchiveEmail moves a single email to the archive folder.
	ArchiveEmail(messageID, folder string) error

	// ArchiveSenderEmails moves all emails from a sender to the archive folder.
	ArchiveSenderEmails(sender, folder string) error

	// --- Search ---

	// SearchEmails finds emails matching query in a folder (subject/sender LIKE, or FTS body).
	SearchEmails(folder, query string, bodySearch bool) ([]*models.EmailData, error)

	// SearchEmailsCrossFolder finds emails matching query across all folders.
	SearchEmailsCrossFolder(query string) ([]*models.EmailData, error)

	// SearchEmailsIMAP performs a server-side IMAP search for query text.
	SearchEmailsIMAP(folder, query string) ([]*models.EmailData, error)

	// SearchEmailsSemantic returns emails ranked by semantic similarity to query.
	SearchEmailsSemantic(folder, query string, limit int, minScore float64) ([]*models.EmailData, error)

	// --- Saved Searches ---

	// GetSavedSearches returns all persisted searches.
	GetSavedSearches() ([]*models.SavedSearch, error)

	// SaveSearch persists a named search.
	SaveSearch(name, query, folder string) error

	// DeleteSavedSearch removes a saved search by ID.
	DeleteSavedSearch(id int) error

	// --- Read/unread ---

	// MarkRead marks an email as read on the IMAP server and in the local cache.
	MarkRead(messageID, folder string) error

	// MarkUnread marks an email as unread on the IMAP server and in the local cache.
	MarkUnread(messageID, folder string) error

	// --- Star/unstar ---

	// MarkStarred sets the \Flagged flag on an email on the IMAP server and in the local cache.
	MarkStarred(messageID, folder string) error

	// UnmarkStarred removes the \Flagged flag from an email on the IMAP server and in the local cache.
	UnmarkStarred(messageID, folder string) error

	// GetEmailsByThread returns all emails in the given folder with the same thread subject.
	GetEmailsByThread(folder, subject string) ([]*models.EmailData, error)

	// SendEmail sends an email via SMTP.
	SendEmail(to, subject, body, from string) error

	// UpdateUnsubscribeHeaders stores List-Unsubscribe headers for a message in the cache.
	UpdateUnsubscribeHeaders(messageID, listUnsub, listUnsubPost string) error

	// --- Body text caching ---

	// CacheBodyText stores the plain-text body for FTS indexing.
	CacheBodyText(messageID, bodyText string) error

	// --- Embeddings ---

	// StoreEmbedding saves a float32 embedding vector for a message.
	StoreEmbedding(messageID string, embedding []float32, hash string) error

	// GetUnembeddedIDs returns message IDs without embeddings.
	GetUnembeddedIDs(folder string) ([]string, error)

	// GetUnembeddedIDsWithBody returns message IDs that have body_text cached but no embedding chunks yet.
	GetUnembeddedIDsWithBody(folder string) ([]string, error)

	// GetUncachedBodyIDs returns up to limit message IDs that have neither body_text nor embedding chunks.
	GetUncachedBodyIDs(folder string, limit int) ([]string, error)

	// GetEmbeddingProgress returns the count of embedded messages (done) and total messages (total) for a folder.
	GetEmbeddingProgress(folder string) (done, total int, err error)

	// StoreEmbeddingChunks replaces all existing chunks for messageID with the provided chunks.
	StoreEmbeddingChunks(messageID string, chunks []models.EmbeddingChunk) error

	// SearchSemanticChunked returns emails ranked by semantic similarity using per-chunk embeddings.
	// Results are paired with their cosine similarity scores.
	SearchSemanticChunked(folder string, queryVec []float32, limit int, minScore float64) ([]*models.SemanticSearchResult, error)

	// GetBodyText returns the cached plain-text body for a message.
	GetBodyText(messageID string) (string, error)

	// FetchAndCacheBody fetches the email body via IMAP and caches the plain text for future embedding.
	FetchAndCacheBody(messageID string) (*models.EmailBody, error)

	// --- Background sync ---

	// NewEmailsCh returns a receive-only channel of new-email notifications.
	NewEmailsCh() <-chan models.NewEmailsNotification

	// StartIDLE starts an IMAP IDLE session on the given folder.
	// Returns an error (including imap.ErrIDLENotSupported) if IDLE cannot start.
	StartIDLE(folder string) error

	// StopIDLE stops a running IDLE session.
	StopIDLE()

	// StartPolling starts background polling for new emails at the given interval.
	StartPolling(folder string, interval int)

	// StopPolling stops background polling.
	StopPolling()

	// ValidIDsCh returns a channel that receives the live valid-ID set from
	// background reconciliation. Returns nil before Load() is called.
	ValidIDsCh() <-chan map[string]bool

	// --- Move ---

	// MoveEmail copies messageID from fromFolder to toFolder then expunges the original.
	MoveEmail(messageID, fromFolder, toFolder string) error

	// --- Rules persistence ---

	// SaveRule persists a rule (insert or update by ID).
	SaveRule(r *models.Rule) error

	// GetEnabledRules returns all rules that are currently enabled.
	GetEnabledRules() ([]*models.Rule, error)

	// DeleteRule removes a rule by ID.
	DeleteRule(id int64) error

	// GetAllCustomPrompts returns all custom prompts.
	GetAllCustomPrompts() ([]*models.CustomPrompt, error)

	// SaveCustomPrompt persists a custom prompt (insert or update by ID).
	SaveCustomPrompt(p *models.CustomPrompt) error

	// GetCustomPrompt returns a single custom prompt by ID.
	GetCustomPrompt(id int64) (*models.CustomPrompt, error)

	// AppendActionLog inserts a rule action log entry.
	AppendActionLog(entry *models.RuleActionLogEntry) error

	// TouchRuleLastTriggered updates the last_triggered timestamp for a rule.
	TouchRuleLastTriggered(ruleID int64) error

	// SaveCustomCategory persists a custom prompt result for a message.
	SaveCustomCategory(messageID string, promptID int64, result string) error

	// --- Contacts ---

	// GetContactsToEnrich returns contacts with email_count >= minCount that have not been enriched yet.
	GetContactsToEnrich(minCount, limit int) ([]models.ContactData, error)

	// GetRecentSubjectsByContact returns recent email subjects where sender matches the contact email.
	GetRecentSubjectsByContact(email string, limit int) ([]string, error)

	// UpdateContactEnrichment saves LLM-extracted company and topics for a contact.
	UpdateContactEnrichment(email, company string, topics []string) error

	// UpdateContactEmbedding saves the semantic embedding vector for a contact.
	UpdateContactEmbedding(email string, embedding []float32) error

	// SearchContactsSemantic finds contacts by cosine similarity against queryVec.
	SearchContactsSemantic(queryVec []float32, limit int, minScore float64) ([]*models.ContactSearchResult, error)

	// ListContacts returns contacts sorted by the given criterion.
	// sortBy accepts "last_seen" (default), "name", or "email_count".
	ListContacts(limit int, sortBy string) ([]models.ContactData, error)

	// SearchContacts performs a keyword search on display_name, email, company, and topics.
	SearchContacts(query string) ([]models.ContactData, error)

	// GetContactEmails returns recent emails where sender matches the given email address.
	GetContactEmails(contactEmail string, limit int) ([]*models.EmailData, error)

	// UpsertContacts inserts or updates contacts from seen email addresses.
	// direction is "from" or "to".
	UpsertContacts(addrs []models.ContactAddr, direction string) error

	// --- Unsubscribed senders ---

	// RecordUnsubscribe persists an unsubscribe record for a sender.
	RecordUnsubscribe(sender, method, url string) error

	// IsUnsubscribedSender returns true if the sender has previously been unsubscribed from.
	IsUnsubscribedSender(sender string) (bool, error)

	// --- Reply / Forward / Attachments ---

	// ReplyToEmail sends a reply to the given message. replyBody is Markdown.
	ReplyToEmail(messageID, replyBody string) error

	// ForwardEmail forwards the given message to `to` with an optional covering note. forwardBody is Markdown.
	ForwardEmail(messageID, to, forwardBody string) error

	// ListAttachments returns attachment metadata for the given email (no binary data).
	ListAttachments(messageID string) ([]models.Attachment, error)

	// GetAttachment returns the named attachment with Data field populated.
	GetAttachment(messageID, filename string) (*models.Attachment, error)

	// --- Drafts ---

	// SaveDraft saves a draft email to the IMAP Drafts folder.
	// Returns the UID and folder of the saved draft.
	SaveDraft(to, subject, body string) (uid uint32, folder string, err error)

	// ListDrafts returns all draft emails from the IMAP Drafts folder.
	ListDrafts() ([]*models.Draft, error)

	// DeleteDraft removes a draft by UID from the given folder.
	DeleteDraft(uid uint32, folder string) error

	// --- Cleanup rules ---

	// GetAllCleanupRules returns all cleanup rules.
	GetAllCleanupRules() ([]*models.CleanupRule, error)

	// SaveCleanupRule inserts or updates a cleanup rule.
	SaveCleanupRule(rule *models.CleanupRule) error

	// DeleteCleanupRule removes a cleanup rule by ID.
	DeleteCleanupRule(id int64) error
}
