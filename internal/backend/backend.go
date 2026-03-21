package backend

import "mail-processor/internal/models"

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

	// SetGroupByDomain toggles domain-level grouping for GetEmailsBySender/GetSenderStatistics.
	SetGroupByDomain(bool)

	// Progress returns a read-only channel of processing progress updates.
	Progress() <-chan models.ProgressInfo

	// Close shuts down all connections and releases resources.
	Close() error
}
