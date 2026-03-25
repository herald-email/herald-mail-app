package backend

import (
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"mail-processor/internal/ai"
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
	classifier *ai.Classifier
	progressCh chan models.ProgressInfo
	loading    atomic.Bool

	// Background polling
	newEmailsCh chan models.NewEmailsNotification
	pollStop    chan struct{}
	pollMu      sync.Mutex
}

// NewLocal creates a LocalBackend. The caller must call Close() when done.
func NewLocal(cfg *config.Config, classifier *ai.Classifier) (*LocalBackend, error) {
	c, err := cache.New("email_cache.db")
	if err != nil {
		return nil, fmt.Errorf("failed to open cache: %w", err)
	}

	progressCh := make(chan models.ProgressInfo, 100)
	return &LocalBackend{
		imapClient:  imap.New(cfg, c, progressCh),
		cache:       c,
		classifier:  classifier,
		progressCh:  progressCh,
		newEmailsCh: make(chan models.NewEmailsNotification, 10),
	}, nil
}

// Load runs the full sync sequence in a background goroutine:
// connect → process new emails → cleanup stale cache → send "complete".
// All progress is sent through Progress().
func (b *LocalBackend) Load(folder string) {
	if !b.loading.CompareAndSwap(false, true) {
		logger.Warn("Load already in progress, ignoring request for folder: %s", folder)
		return
	}
	go func() {
		defer b.loading.Store(false)
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

func (b *LocalBackend) GetFolderStatus(folders []string) (map[string]models.FolderStatus, error) {
	return b.imapClient.GetFolderStatus(folders)
}

func (b *LocalBackend) GetTimelineEmails(folder string) ([]*models.EmailData, error) {
	return b.cache.GetEmailsSortedByDate(folder)
}

func (b *LocalBackend) GetClassifications(folder string) (map[string]string, error) {
	return b.cache.GetClassifications(folder)
}

func (b *LocalBackend) SetClassification(messageID, category string) error {
	return b.cache.SetClassification(messageID, category)
}

func (b *LocalBackend) GetUnclassifiedIDs(folder string) ([]string, error) {
	return b.cache.GetUnclassifiedIDs(folder)
}

func (b *LocalBackend) GetEmailByID(messageID string) (*models.EmailData, error) {
	return b.cache.GetEmailByID(messageID)
}

func (b *LocalBackend) FetchEmailBody(folder string, uid uint32) (*models.EmailBody, error) {
	return b.imapClient.FetchEmailBody(uid, folder)
}

func (b *LocalBackend) SaveAttachment(att *models.Attachment, destPath string) error {
	if att.Data == nil {
		return fmt.Errorf("attachment data not loaded")
	}
	return os.WriteFile(destPath, att.Data, 0644)
}

func (b *LocalBackend) SetGroupByDomain(v bool) {
	b.imapClient.SetGroupByDomain(v)
}

func (b *LocalBackend) Progress() <-chan models.ProgressInfo {
	return b.progressCh
}

// Close shuts down the IMAP connection and the cache database.
func (b *LocalBackend) Close() error {
	b.StopPolling()
	imapErr := b.imapClient.Close()
	cacheErr := b.cache.Close()
	if imapErr != nil {
		return imapErr
	}
	return cacheErr
}

func (b *LocalBackend) ArchiveEmail(messageID, folder string) error {
	return b.imapClient.ArchiveEmail(messageID, folder)
}

func (b *LocalBackend) ArchiveSenderEmails(sender, folder string) error {
	return b.imapClient.ArchiveSenderEmails(sender, folder)
}

func (b *LocalBackend) SearchEmails(folder, query string, bodySearch bool) ([]*models.EmailData, error) {
	if bodySearch {
		emails, err := b.cache.SearchEmailsFTS(folder, query)
		if err != nil {
			logger.Warn("FTS search failed, falling back to LIKE: %v", err)
			return b.cache.SearchEmails(folder, query)
		}
		return emails, nil
	}
	return b.cache.SearchEmails(folder, query)
}

func (b *LocalBackend) SearchEmailsCrossFolder(query string) ([]*models.EmailData, error) {
	return b.cache.SearchEmailsCrossFolder(query)
}

func (b *LocalBackend) SearchEmailsIMAP(folder, query string) ([]*models.EmailData, error) {
	return b.imapClient.SearchIMAP(folder, query)
}

func (b *LocalBackend) SearchEmailsSemantic(folder, query string, limit int, minScore float64) ([]*models.EmailData, error) {
	if b.classifier == nil {
		return nil, fmt.Errorf("Ollama classifier not configured")
	}
	vec, err := b.classifier.Embed(query)
	if err != nil {
		return nil, fmt.Errorf("embedding failed: %w", err)
	}
	return b.cache.SearchSemantic(folder, vec, limit, minScore)
}

func (b *LocalBackend) GetSavedSearches() ([]*models.SavedSearch, error) {
	return b.cache.GetSavedSearches()
}

func (b *LocalBackend) SaveSearch(name, query, folder string) error {
	return b.cache.SaveSearch(name, query, folder)
}

func (b *LocalBackend) DeleteSavedSearch(id int) error {
	return b.cache.DeleteSavedSearch(id)
}

func (b *LocalBackend) CacheBodyText(messageID, bodyText string) error {
	return b.cache.CacheBodyText(messageID, bodyText)
}

func (b *LocalBackend) StoreEmbedding(messageID string, embedding []float32, hash string) error {
	return b.cache.StoreEmbedding(messageID, embedding, hash)
}

func (b *LocalBackend) GetUnembeddedIDs(folder string) ([]string, error) {
	return b.cache.GetUnembeddedIDs(folder)
}

func (b *LocalBackend) NewEmailsCh() <-chan models.NewEmailsNotification {
	return b.newEmailsCh
}

func (b *LocalBackend) StartPolling(folder string, interval int) {
	b.pollMu.Lock()
	defer b.pollMu.Unlock()
	if b.pollStop != nil {
		return // already running
	}
	b.pollStop = make(chan struct{})
	stop := b.pollStop
	go func() {
		ticker := time.NewTicker(time.Duration(interval) * time.Second)
		defer ticker.Stop()
		lastDate := time.Now()
		for {
			select {
			case <-stop:
				return
			case <-ticker.C:
				emails, err := b.imapClient.PollForNewEmails(folder, lastDate)
				if err != nil {
					logger.Warn("Polling error: %v", err)
					continue
				}
				if len(emails) > 0 {
					lastDate = time.Now()
					// Cache the new emails
					for _, e := range emails {
						if err := b.cache.CacheEmail(e); err != nil {
							logger.Warn("Failed to cache polled email: %v", err)
						}
					}
					select {
					case b.newEmailsCh <- models.NewEmailsNotification{Emails: emails, Folder: folder}:
					default:
					}
				}
			}
		}
	}()
}

func (b *LocalBackend) StopPolling() {
	b.pollMu.Lock()
	defer b.pollMu.Unlock()
	if b.pollStop != nil {
		close(b.pollStop)
		b.pollStop = nil
	}
}
