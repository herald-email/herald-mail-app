package backend

import (
	"fmt"
	"time"

	"mail-processor/internal/cache"
	"mail-processor/internal/config"
	"mail-processor/internal/imap"
	"mail-processor/internal/logger"
	"mail-processor/internal/models"
)

// LocalBackend implements Backend with a direct IMAP connection and local SQLite cache.
// This is the single-process implementation; a future RemoteBackend will speak to a daemon.
type LocalBackend struct {
	imapClient *imap.Client
	cache      *cache.Cache
	progressCh chan models.ProgressInfo
}

// NewLocal creates a LocalBackend. The caller must call Close() when done.
func NewLocal(cfg *config.Config) (*LocalBackend, error) {
	c, err := cache.New("email_cache.db")
	if err != nil {
		return nil, fmt.Errorf("failed to open cache: %w", err)
	}

	progressCh := make(chan models.ProgressInfo, 100)
	return &LocalBackend{
		imapClient: imap.New(cfg, c, progressCh),
		cache:      c,
		progressCh: progressCh,
	}, nil
}

// Load runs the full sync sequence in a background goroutine:
// connect → process new emails → cleanup stale cache → send "complete".
// All progress is sent through Progress().
func (b *LocalBackend) Load(folder string) {
	go func() {
		b.progressCh <- models.ProgressInfo{
			Phase:   "connecting",
			Message: "Connecting to IMAP server...",
		}
		time.Sleep(200 * time.Millisecond) // let the UI render the first frame

		if err := b.imapClient.Connect(); err != nil {
			logger.Error("Failed to connect to IMAP: %v", err)
			b.progressCh <- models.ProgressInfo{
				Phase:   "error",
				Message: fmt.Sprintf("Connection failed: %v", err),
			}
			return
		}

		if err := b.imapClient.ProcessEmails(folder); err != nil {
			logger.Error("Failed to process emails: %v", err)
			b.progressCh <- models.ProgressInfo{
				Phase:   "error",
				Message: fmt.Sprintf("Processing failed: %v", err),
			}
			return
		}

		b.progressCh <- models.ProgressInfo{
			Phase:   "cleanup",
			Message: "Cleaning up cache...",
		}
		if err := b.imapClient.CleanupCache(folder); err != nil {
			logger.Warn("Cache cleanup failed (non-critical): %v", err)
		}

		b.progressCh <- models.ProgressInfo{
			Phase:   "finalizing",
			Message: "Generating statistics...",
		}
		stats, err := b.imapClient.GetSenderStatistics(folder)
		if err != nil {
			logger.Error("Failed to get statistics: %v", err)
			b.progressCh <- models.ProgressInfo{
				Phase:   "error",
				Message: fmt.Sprintf("Statistics failed: %v", err),
			}
			return
		}

		b.progressCh <- models.ProgressInfo{
			Phase:   "complete",
			Message: fmt.Sprintf("Found %d senders", len(stats)),
		}
		logger.Info("Load complete: %d senders", len(stats))
	}()
}

func (b *LocalBackend) GetSenderStatistics(folder string) (map[string]*models.SenderStats, error) {
	return b.imapClient.GetSenderStatistics(folder)
}

func (b *LocalBackend) GetEmailsBySender(folder string) (map[string][]*models.EmailData, error) {
	return b.imapClient.GetEmailsBySender(folder)
}

func (b *LocalBackend) DeleteSenderEmails(sender, folder string) error {
	return b.imapClient.DeleteSenderEmails(sender, folder)
}

func (b *LocalBackend) DeleteDomainEmails(domain, folder string) error {
	return b.imapClient.DeleteDomainEmails(domain, folder)
}

func (b *LocalBackend) DeleteEmail(messageID, folder string) error {
	return b.imapClient.DeleteEmail(messageID, folder)
}

func (b *LocalBackend) ListFolders() ([]string, error) {
	return b.imapClient.ListFolders()
}

func (b *LocalBackend) SetGroupByDomain(v bool) {
	b.imapClient.SetGroupByDomain(v)
}

func (b *LocalBackend) Progress() <-chan models.ProgressInfo {
	return b.progressCh
}

// Close shuts down the IMAP connection and the cache database.
func (b *LocalBackend) Close() error {
	imapErr := b.imapClient.Close()
	cacheErr := b.cache.Close()
	if imapErr != nil {
		return imapErr
	}
	return cacheErr
}
