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

	// --- Background sync ---

	// NewEmailsCh returns a receive-only channel of new-email notifications.
	NewEmailsCh() <-chan models.NewEmailsNotification

	// StartPolling starts background polling for new emails at the given interval.
	StartPolling(folder string, interval int)

	// StopPolling stops background polling.
	StopPolling()

	// ValidIDsCh returns a channel that receives the live valid-ID set from
	// background reconciliation. Returns nil before Load() is called.
	ValidIDsCh() <-chan map[string]bool
}
