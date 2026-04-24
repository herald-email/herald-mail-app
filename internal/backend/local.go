package backend

import (
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"mail-processor/internal/ai"
	"mail-processor/internal/cache"
	"mail-processor/internal/config"
	"mail-processor/internal/imap"
	"mail-processor/internal/logger"
	"mail-processor/internal/models"
	appsmtp "mail-processor/internal/smtp"
)

// LocalBackend implements Backend with a direct IMAP connection and local SQLite cache.
// This is the single-process implementation; a future RemoteBackend will speak to a daemon.
type LocalBackend struct {
	imapClient       *imap.Client
	cache            *cache.Cache
	classifier       ai.AIClient
	cfg              *config.Config
	progressCh       chan models.ProgressInfo
	rawProgressCh    chan models.ProgressInfo
	syncEventsCh     chan models.FolderSyncEvent
	closed           atomic.Bool
	loadCoordinator  *latestWinsLoadCoordinator
	activeLoadMu     sync.RWMutex
	activeLoad       folderLoadRequest
	lastFetchMu      sync.Mutex
	lastFetchCurrent map[int64]int
	foldersMu        sync.RWMutex
	cachedFolders    []string

	// Background polling
	newEmailsCh chan models.NewEmailsNotification
	pollStop    chan struct{}
	pollMu      sync.Mutex

	// validIDs is the live ground-truth set of message IDs known to exist on the server.
	// nil means reconciliation has not run yet — all cache entries are accepted.
	validIDsMu   sync.RWMutex
	validIDs     map[string]bool
	validIDsChSt chan map[string]bool // channel returned by ValidIDsCh()

	// In-memory email body cache to avoid redundant IMAP fetches when
	// the user navigates back-and-forth through the same emails.
	bodyCache   map[string]*models.EmailBody // key: "folder:uid"
	bodyCacheMu sync.Mutex
}

func summarizeTraceMessage(message string) string {
	message = strings.TrimSpace(message)
	if len(message) <= 120 {
		return message
	}
	return message[:117] + "..."
}

func (b *LocalBackend) tracef(format string, args ...interface{}) {
	logger.Debug("LocalBackend: "+format, args...)
}

// filterByValidIDs returns only emails whose MessageID is in validIDs.
// If validIDs is nil (not yet reconciled), the original slice is returned unchanged.
func (b *LocalBackend) filterByValidIDs(emails []*models.EmailData) []*models.EmailData {
	b.validIDsMu.RLock()
	ids := b.validIDs
	b.validIDsMu.RUnlock()
	if ids == nil {
		return emails
	}
	out := make([]*models.EmailData, 0, len(emails))
	for _, e := range emails {
		if ids[e.MessageID] {
			out = append(out, e)
		}
	}
	return out
}

func (b *LocalBackend) filterSemanticResultsByValidIDs(results []*models.SemanticSearchResult) []*models.SemanticSearchResult {
	b.validIDsMu.RLock()
	ids := b.validIDs
	b.validIDsMu.RUnlock()
	if ids == nil {
		return results
	}
	out := make([]*models.SemanticSearchResult, 0, len(results))
	for _, result := range results {
		if result == nil || result.Email == nil {
			continue
		}
		if ids[result.Email.MessageID] {
			out = append(out, result)
		}
	}
	return out
}

// isValidID returns true when the message exists in the valid set, or when no
// valid set has been established yet (nil → accept all).
func (b *LocalBackend) isValidID(msgID string) bool {
	b.validIDsMu.RLock()
	ids := b.validIDs
	b.validIDsMu.RUnlock()
	return ids == nil || ids[msgID]
}

// ValidIDsCh returns the channel that will receive the valid-ID map from
// background reconciliation. Returns nil before Load() is called.
func (b *LocalBackend) ValidIDsCh() <-chan map[string]bool {
	b.validIDsMu.RLock()
	ch := b.validIDsChSt
	b.validIDsMu.RUnlock()
	return ch
}

// NewLocal creates a LocalBackend. configPath is the path to the config file on disk;
// it is used to persist refreshed OAuth tokens. The caller must call Close() when done.
func NewLocal(cfg *config.Config, configPath string, classifier ai.AIClient) (*LocalBackend, error) {
	cachePath, err := config.EnsureCacheDatabasePath(configPath, cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve cache path: %w", err)
	}
	c, err := cache.New(cachePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open cache: %w", err)
	}
	if _, err := c.EnsureEmbeddingModel(cfg.EffectiveEmbeddingModel()); err != nil {
		_ = c.Close()
		return nil, fmt.Errorf("failed to initialize embedding model state: %w", err)
	}

	progressCh := make(chan models.ProgressInfo, 100)
	rawProgressCh := make(chan models.ProgressInfo, 100)
	b := &LocalBackend{
		imapClient:       imap.New(cfg, configPath, c, rawProgressCh),
		cache:            c,
		classifier:       classifier,
		cfg:              cfg,
		progressCh:       progressCh,
		rawProgressCh:    rawProgressCh,
		syncEventsCh:     make(chan models.FolderSyncEvent, 256),
		loadCoordinator:  newLatestWinsLoadCoordinator(),
		lastFetchCurrent: make(map[int64]int),
		newEmailsCh:      make(chan models.NewEmailsNotification, 10),
	}
	go b.fanoutProgressLoop()
	go b.loadWorker()
	return b, nil
}

// Load schedules a folder sync and immediately emits a generation-tagged
// sync_started event so the UI can invalidate older work before the worker
// reaches the IMAP lane.
func (b *LocalBackend) Load(folder string) {
	req := b.loadCoordinator.Submit(folder)
	b.tracef("Load requested: folder=%s generation=%d", req.Folder, req.Generation)
	b.emitSyncEvent(models.FolderSyncEvent{
		Folder:     req.Folder,
		Generation: req.Generation,
		Phase:      models.SyncPhaseSyncStarted,
		Message:    fmt.Sprintf("Opening %s...", folder),
	})
	b.emitSyncEvent(models.FolderSyncEvent{
		Folder:     req.Folder,
		Generation: req.Generation,
		Phase:      models.SyncPhaseSnapshotReady,
		Message:    fmt.Sprintf("Showing cached %s while live sync starts", folder),
	})
}

func (b *LocalBackend) sendProgress(p models.ProgressInfo) {
	if b.progressCh == nil || b.closed.Load() {
		return
	}
	b.tracef("forward progress: phase=%s current=%d total=%d message=%q", p.Phase, p.Current, p.Total, summarizeTraceMessage(p.Message))
	defer func() {
		if recover() != nil {
			// Close() can race with an in-flight send during TUI/SSH shutdown.
		}
	}()
	b.progressCh <- p
}

func (b *LocalBackend) emitSyncEvent(event models.FolderSyncEvent) {
	if b.syncEventsCh == nil || b.closed.Load() {
		return
	}
	b.tracef(
		"emit sync event: folder=%s generation=%d phase=%s current=%d total=%d delta=%d error=%q message=%q",
		event.Folder,
		event.Generation,
		event.Phase,
		event.Current,
		event.Total,
		event.EventCount,
		event.Error,
		summarizeTraceMessage(event.Message),
	)
	defer func() {
		if recover() != nil {
			// Close() can race with an in-flight send during shutdown.
		}
	}()
	select {
	case b.syncEventsCh <- event:
	default:
	}
}

func (b *LocalBackend) setActiveLoad(req folderLoadRequest) {
	b.activeLoadMu.Lock()
	b.activeLoad = req
	b.activeLoadMu.Unlock()
	b.tracef("active load set: folder=%s generation=%d", req.Folder, req.Generation)
}

func (b *LocalBackend) clearActiveLoad(req folderLoadRequest) {
	b.activeLoadMu.Lock()
	if b.activeLoad.Generation == req.Generation {
		b.activeLoad = folderLoadRequest{}
	}
	b.activeLoadMu.Unlock()
	b.lastFetchMu.Lock()
	delete(b.lastFetchCurrent, req.Generation)
	b.lastFetchMu.Unlock()
	b.tracef("active load cleared: folder=%s generation=%d", req.Folder, req.Generation)
}

func (b *LocalBackend) currentActiveLoad() (folderLoadRequest, bool) {
	b.activeLoadMu.RLock()
	defer b.activeLoadMu.RUnlock()
	if b.activeLoad.Generation == 0 {
		return folderLoadRequest{}, false
	}
	return b.activeLoad, true
}

func (b *LocalBackend) fanoutProgressLoop() {
	for info := range b.rawProgressCh {
		if b.closed.Load() {
			return
		}
		b.sendProgress(info)
		req, ok := b.currentActiveLoad()
		if !ok {
			b.tracef("raw progress had no active load: phase=%s current=%d total=%d message=%q", info.Phase, info.Current, info.Total, summarizeTraceMessage(info.Message))
			continue
		}
		if event, ok := b.syncEventFromProgress(req, info); ok {
			b.emitSyncEvent(event)
		}
	}
}

func (b *LocalBackend) syncEventFromProgress(req folderLoadRequest, info models.ProgressInfo) (models.FolderSyncEvent, bool) {
	event := models.FolderSyncEvent{
		Folder:     req.Folder,
		Generation: req.Generation,
		Message:    info.Message,
		Current:    info.Current,
		Total:      info.Total,
	}

	switch info.Phase {
	case "error":
		event.Phase = models.SyncPhaseError
		event.Error = info.Message
		return event, true
	case "complete":
		event.Phase = models.SyncPhaseComplete
		return event, true
	case "fetching":
		event.Phase = models.SyncPhaseRowsCached
		b.lastFetchMu.Lock()
		prev := b.lastFetchCurrent[req.Generation]
		if info.Current < prev {
			prev = 0
		}
		event.EventCount = info.Current - prev
		if event.EventCount <= 0 {
			event.EventCount = 1
		}
		b.lastFetchCurrent[req.Generation] = info.Current
		b.lastFetchMu.Unlock()
		return event, true
	default:
		event.Phase = models.SyncPhaseSyncStarted
		return event, true
	}
}

func (b *LocalBackend) loadWorker() {
	for range b.loadCoordinator.Wake() {
		for {
			req, ok := b.loadCoordinator.DrainPending()
			if !ok {
				break
			}
			b.runLoad(req)
		}
	}
}

func (b *LocalBackend) runLoad(req folderLoadRequest) {
	start := time.Now()
	b.setActiveLoad(req)
	defer b.clearActiveLoad(req)
	b.tracef("runLoad started: folder=%s generation=%d", req.Folder, req.Generation)

	b.sendProgress(models.ProgressInfo{
		Phase:   "connecting",
		Message: "Connecting to IMAP server...",
	})
	time.Sleep(200 * time.Millisecond)

	b.tracef("connecting IMAP: folder=%s generation=%d", req.Folder, req.Generation)
	if err := b.imapClient.Connect(); err != nil {
		logger.Error("Failed to connect to IMAP: %v", err)
		b.sendProgress(models.ProgressInfo{
			Phase:   "error",
			Message: fmt.Sprintf("Connection failed: %v", err),
		})
		b.emitSyncEvent(models.FolderSyncEvent{
			Folder:     req.Folder,
			Generation: req.Generation,
			Phase:      models.SyncPhaseError,
			Message:    fmt.Sprintf("Connection failed: %v", err),
			Error:      err.Error(),
		})
		return
	}
	b.tracef("IMAP connected: folder=%s generation=%d", req.Folder, req.Generation)

	b.tracef("priming folder cache after connect: folder=%s generation=%d", req.Folder, req.Generation)
	if folders, err := b.imapClient.ListFolders(); err != nil {
		logger.Warn("Failed to prime folder cache after connect: %v", err)
	} else if len(folders) > 0 {
		b.foldersMu.Lock()
		b.cachedFolders = append(b.cachedFolders[:0], folders...)
		b.foldersMu.Unlock()
		logger.Info("Primed %d folders after connect", len(folders))
		b.tracef("folder cache primed: folder=%s generation=%d count=%d", req.Folder, req.Generation, len(folders))
	}

	b.tracef("starting ProcessEmailsIncremental: folder=%s generation=%d", req.Folder, req.Generation)
	if err := b.imapClient.ProcessEmailsIncremental(req.Folder); err != nil {
		logger.Error("Failed to process emails: %v", err)
		b.sendProgress(models.ProgressInfo{
			Phase:   "error",
			Message: fmt.Sprintf("Processing failed: %v", err),
		})
		b.emitSyncEvent(models.FolderSyncEvent{
			Folder:     req.Folder,
			Generation: req.Generation,
			Phase:      models.SyncPhaseError,
			Message:    fmt.Sprintf("Processing failed: %v", err),
			Error:      err.Error(),
		})
		return
	}
	b.tracef("ProcessEmailsIncremental completed: folder=%s generation=%d elapsed=%s", req.Folder, req.Generation, time.Since(start).Round(10*time.Millisecond))

	b.sendProgress(models.ProgressInfo{
		Phase:   "finalizing",
		Message: "Generating statistics...",
	})
	b.tracef("loading sender statistics: folder=%s generation=%d", req.Folder, req.Generation)
	stats, err := b.imapClient.GetSenderStatistics(req.Folder)
	if err != nil {
		logger.Error("Failed to get statistics: %v", err)
		b.sendProgress(models.ProgressInfo{
			Phase:   "error",
			Message: fmt.Sprintf("Statistics failed: %v", err),
		})
		b.emitSyncEvent(models.FolderSyncEvent{
			Folder:     req.Folder,
			Generation: req.Generation,
			Phase:      models.SyncPhaseError,
			Message:    fmt.Sprintf("Statistics failed: %v", err),
			Error:      err.Error(),
		})
		return
	}
	b.tracef("sender statistics loaded: folder=%s generation=%d senders=%d", req.Folder, req.Generation, len(stats))

	b.emitSyncEvent(models.FolderSyncEvent{
		Folder:     req.Folder,
		Generation: req.Generation,
		Phase:      models.SyncPhaseCountsUpdated,
		Message:    fmt.Sprintf("Found %d senders", len(stats)),
		EventCount: 1,
	})

	b.sendProgress(models.ProgressInfo{
		Phase:   "complete",
		Message: fmt.Sprintf("Found %d senders", len(stats)),
	})
	b.emitSyncEvent(models.FolderSyncEvent{
		Folder:     req.Folder,
		Generation: req.Generation,
		Phase:      models.SyncPhaseComplete,
		Message:    fmt.Sprintf("Found %d senders", len(stats)),
	})
	logger.Info("Load complete: %d senders", len(stats))
	b.tracef("runLoad settled visible bundle: folder=%s generation=%d senders=%d duration=%s", req.Folder, req.Generation, len(stats), time.Since(start).Round(10*time.Millisecond))

	validIDsCh := make(chan map[string]bool, 1)
	b.validIDsMu.Lock()
	b.validIDsChSt = validIDsCh
	b.validIDsMu.Unlock()
	b.tracef("starting background reconcile: folder=%s generation=%d", req.Folder, req.Generation)
	b.emitSyncEvent(models.FolderSyncEvent{
		Folder:     req.Folder,
		Generation: req.Generation,
		Phase:      models.SyncPhaseReconcileStarted,
		Message:    fmt.Sprintf("Reconciling live %s state...", req.Folder),
	})
	b.imapClient.StartBackgroundReconcile(req.Folder, validIDsCh)
}

func senderStatisticsFromGroups(grouped map[string][]*models.EmailData) map[string]*models.SenderStats {
	stats := make(map[string]*models.SenderStats, len(grouped))
	for sender, emails := range grouped {
		if len(emails) == 0 {
			continue
		}

		totalSize := 0
		withAttachments := 0
		firstEmail := emails[0].Date
		lastEmail := emails[0].Date

		for _, email := range emails {
			totalSize += email.Size
			if email.HasAttachments {
				withAttachments++
			}
			if email.Date.Before(firstEmail) {
				firstEmail = email.Date
			}
			if email.Date.After(lastEmail) {
				lastEmail = email.Date
			}
		}

		stats[sender] = &models.SenderStats{
			TotalEmails:     len(emails),
			AvgSize:         float64(totalSize) / float64(len(emails)),
			WithAttachments: withAttachments,
			FirstEmail:      firstEmail,
			LastEmail:       lastEmail,
		}
	}
	return stats
}

func (b *LocalBackend) GetSenderStatistics(folder string) (map[string]*models.SenderStats, error) {
	grouped, err := b.GetEmailsBySender(folder)
	if err != nil {
		return nil, err
	}
	return senderStatisticsFromGroups(grouped), nil
}

func (b *LocalBackend) GetEmailsBySender(folder string) (map[string][]*models.EmailData, error) {
	grouped, err := b.imapClient.GetEmailsBySender(folder)
	if err != nil {
		return nil, err
	}
	for sender, emails := range grouped {
		filtered := b.filterByValidIDs(emails)
		if len(filtered) == 0 {
			delete(grouped, sender)
		} else {
			grouped[sender] = filtered
		}
	}
	return grouped, nil
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
	b.foldersMu.RLock()
	if len(b.cachedFolders) > 0 {
		folders := append([]string(nil), b.cachedFolders...)
		b.foldersMu.RUnlock()
		return folders, nil
	}
	b.foldersMu.RUnlock()

	return b.imapClient.ListFolders()
}

func (b *LocalBackend) GetFolderStatus(folders []string) (map[string]models.FolderStatus, error) {
	return b.imapClient.GetFolderStatus(folders)
}

func (b *LocalBackend) GetTimelineEmails(folder string) ([]*models.EmailData, error) {
	emails, err := b.cache.GetEmailsSortedByDate(folder)
	if err != nil {
		return nil, err
	}
	return b.filterByValidIDs(emails), nil
}

func (b *LocalBackend) GetClassifications(folder string) (map[string]string, error) {
	return b.cache.GetClassifications(folder)
}

func (b *LocalBackend) SetClassification(messageID, category string) error {
	return b.cache.SetClassification(messageID, category)
}

func (b *LocalBackend) GetUnclassifiedIDs(folder string) ([]string, error) {
	ids, err := b.cache.GetUnclassifiedIDs(folder)
	if err != nil {
		return nil, err
	}
	out := ids[:0:0]
	for _, id := range ids {
		if b.isValidID(id) {
			out = append(out, id)
		}
	}
	return out, nil
}

func (b *LocalBackend) GetEmailByID(messageID string) (*models.EmailData, error) {
	if !b.isValidID(messageID) {
		return nil, fmt.Errorf("email %s not in valid set", messageID)
	}
	return b.cache.GetEmailByID(messageID)
}

func (b *LocalBackend) FetchEmailBody(folder string, uid uint32) (*models.EmailBody, error) {
	key := fmt.Sprintf("%s:%d", folder, uid)

	// Check in-memory cache first.
	b.bodyCacheMu.Lock()
	if b.bodyCache == nil {
		b.bodyCache = make(map[string]*models.EmailBody, 256)
	}
	if cached, ok := b.bodyCache[key]; ok {
		b.bodyCacheMu.Unlock()
		return cached, nil
	}
	b.bodyCacheMu.Unlock()

	body, err := b.imapClient.FetchEmailBody(uid, folder)
	if err != nil {
		return nil, err
	}

	// Store in cache; evict oldest if over 500 entries.
	b.bodyCacheMu.Lock()
	if len(b.bodyCache) >= 500 {
		// Simple eviction: clear the whole cache when full.
		// An LRU would be better but this is good enough.
		b.bodyCache = make(map[string]*models.EmailBody, 256)
	}
	b.bodyCache[key] = body
	b.bodyCacheMu.Unlock()

	return body, nil
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

func (b *LocalBackend) SyncEvents() <-chan models.FolderSyncEvent {
	return b.syncEventsCh
}

// Cache returns the underlying SQLite cache, for use by components that need
// direct cache access (e.g. the cleanup engine in the daemon server).
func (b *LocalBackend) Cache() *cache.Cache {
	return b.cache
}

func (b *LocalBackend) EnsureEmbeddingModel(model string) (bool, error) {
	return b.cache.EnsureEmbeddingModel(model)
}

// Close shuts down the IMAP connection and the cache database.
func (b *LocalBackend) Close() error {
	b.closed.Store(true)
	b.StopIDLE()
	b.StopPolling()
	if b.progressCh != nil {
		close(b.progressCh)
	}
	if b.syncEventsCh != nil {
		close(b.syncEventsCh)
	}
	if b.newEmailsCh != nil {
		close(b.newEmailsCh)
	}
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
	var (
		emails []*models.EmailData
		err    error
	)
	if bodySearch {
		emails, err = b.cache.SearchEmailsFTS(folder, query)
		if err != nil {
			logger.Warn("FTS search failed, falling back to LIKE: %v", err)
			emails, err = b.cache.SearchEmails(folder, query)
		}
	} else {
		emails, err = b.cache.SearchEmails(folder, query)
	}
	if err != nil {
		return nil, err
	}
	return b.filterByValidIDs(emails), nil
}

func (b *LocalBackend) SearchEmailsCrossFolder(query string) ([]*models.EmailData, error) {
	emails, err := b.cache.SearchEmailsCrossFolder(query)
	if err != nil {
		return nil, err
	}
	return b.filterByValidIDs(emails), nil
}

func (b *LocalBackend) SearchEmailsIMAP(folder, query string) ([]*models.EmailData, error) {
	return b.imapClient.SearchIMAP(folder, query)
}

func (b *LocalBackend) SearchEmailsSemantic(folder, query string, limit int, minScore float64) ([]*models.EmailData, error) {
	if b.classifier == nil {
		return nil, nil
	}
	queryText := ai.BuildQueryText(query)
	vec, err := b.classifier.Embed(queryText)
	if err != nil {
		return nil, fmt.Errorf("embedding failed: %w", err)
	}
	results, err := b.cache.SearchSemanticChunked(folder, vec, limit, minScore)
	if err != nil {
		return nil, err
	}
	emails := make([]*models.EmailData, 0, len(results))
	for _, r := range b.filterSemanticResultsByValidIDs(results) {
		emails = append(emails, r.Email)
	}
	return emails, nil
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

func (b *LocalBackend) MarkRead(messageID, folder string) error {
	email, err := b.cache.GetEmailByID(messageID)
	if err != nil {
		return err
	}
	if err := b.imapClient.MarkRead(email.UID, folder); err != nil {
		logger.Warn("MarkRead IMAP failed for %s: %v", messageID, err)
	}
	return b.cache.MarkRead(messageID)
}

func (b *LocalBackend) MarkUnread(messageID, folder string) error {
	email, err := b.cache.GetEmailByID(messageID)
	if err != nil {
		return err
	}
	if err := b.imapClient.MarkUnread(email.UID, folder); err != nil {
		logger.Warn("MarkUnread IMAP failed for %s: %v", messageID, err)
	}
	return b.cache.MarkUnread(messageID)
}

func (b *LocalBackend) MarkStarred(messageID, folder string) error {
	email, err := b.cache.GetEmailByID(messageID)
	if err != nil {
		return err
	}
	if err := b.imapClient.MarkStarred(email.UID, folder); err != nil {
		logger.Warn("MarkStarred IMAP failed for %s: %v", messageID, err)
	}
	return b.cache.UpdateStarred(messageID, true)
}

func (b *LocalBackend) UnmarkStarred(messageID, folder string) error {
	email, err := b.cache.GetEmailByID(messageID)
	if err != nil {
		return err
	}
	if err := b.imapClient.UnmarkStarred(email.UID, folder); err != nil {
		logger.Warn("UnmarkStarred IMAP failed for %s: %v", messageID, err)
	}
	return b.cache.UpdateStarred(messageID, false)
}

func (b *LocalBackend) GetEmailsByThread(folder, subject string) ([]*models.EmailData, error) {
	return b.cache.GetEmailsByThread(folder, subject)
}

func (b *LocalBackend) SendEmail(to, subject, body, from string) error {
	mailer := appsmtp.New(b.cfg)
	return mailer.Send(from, to, subject, body, "")
}

func (b *LocalBackend) UpdateUnsubscribeHeaders(messageID, listUnsub, listUnsubPost string) error {
	return b.cache.UpdateUnsubscribeHeaders(messageID, listUnsub, listUnsubPost)
}

func (b *LocalBackend) StoreEmbedding(messageID string, embedding []float32, hash string) error {
	return b.cache.StoreEmbedding(messageID, embedding, hash)
}

func (b *LocalBackend) GetUnembeddedIDs(folder string) ([]string, error) {
	return b.cache.GetUnembeddedIDs(folder)
}

func (b *LocalBackend) GetUnembeddedIDsWithBody(folder string) ([]string, error) {
	return b.cache.GetUnembeddedIDsWithBody(folder)
}

func (b *LocalBackend) GetUncachedBodyIDs(folder string, limit int) ([]string, error) {
	return b.cache.GetUncachedBodyIDs(folder, limit)
}

func (b *LocalBackend) GetEmbeddingProgress(folder string) (done, total int, err error) {
	return b.cache.GetEmbeddingProgress(folder)
}

func (b *LocalBackend) StoreEmbeddingChunks(messageID string, chunks []models.EmbeddingChunk) error {
	return b.cache.StoreEmbeddingChunks(messageID, chunks)
}

func (b *LocalBackend) SearchSemanticChunked(folder string, queryVec []float32, limit int, minScore float64) ([]*models.SemanticSearchResult, error) {
	results, err := b.cache.SearchSemanticChunked(folder, queryVec, limit, minScore)
	if err != nil {
		return nil, err
	}
	return b.filterSemanticResultsByValidIDs(results), nil
}

func (b *LocalBackend) GetBodyText(messageID string) (string, error) {
	return b.cache.GetBodyText(messageID)
}

func (b *LocalBackend) FetchAndCacheBody(messageID string) (*models.EmailBody, error) {
	email, err := b.cache.GetEmailByID(messageID)
	if err != nil {
		return nil, err
	}
	if email == nil {
		return nil, fmt.Errorf("FetchAndCacheBody: message %s not found in cache", messageID)
	}
	body, err := b.imapClient.FetchEmailBody(email.UID, email.Folder)
	if err != nil {
		return nil, err
	}
	if body.TextPlain != "" {
		if err := b.cache.CacheBodyText(messageID, body.TextPlain); err != nil {
			logger.Warn("FetchAndCacheBody CacheBodyText %s: %v", messageID, err)
		}
	}
	return body, nil
}

func (b *LocalBackend) NewEmailsCh() <-chan models.NewEmailsNotification {
	return b.newEmailsCh
}

func (b *LocalBackend) StartIDLE(folder string) error {
	return b.imapClient.StartIDLE(folder, b.newEmailsCh)
}

func (b *LocalBackend) StopIDLE() {
	b.imapClient.StopIDLE()
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

func (b *LocalBackend) MoveEmail(messageID, fromFolder, toFolder string) error {
	return b.imapClient.MoveEmail(messageID, fromFolder, toFolder)
}

func (b *LocalBackend) SaveRule(r *models.Rule) error {
	return b.cache.SaveRule(r)
}

func (b *LocalBackend) GetAllRules() ([]*models.Rule, error) {
	return b.cache.GetAllRules()
}

func (b *LocalBackend) GetEnabledRules() ([]*models.Rule, error) {
	return b.cache.GetEnabledRules()
}

func (b *LocalBackend) DeleteRule(id int64) error {
	return b.cache.DeleteRule(id)
}

func (b *LocalBackend) GetAllCustomPrompts() ([]*models.CustomPrompt, error) {
	return b.cache.GetAllCustomPrompts()
}

func (b *LocalBackend) SaveCustomPrompt(p *models.CustomPrompt) error {
	return b.cache.SaveCustomPrompt(p)
}

func (b *LocalBackend) GetCustomPrompt(id int64) (*models.CustomPrompt, error) {
	return b.cache.GetCustomPrompt(id)
}

func (b *LocalBackend) AppendActionLog(entry *models.RuleActionLogEntry) error {
	return b.cache.AppendActionLog(entry)
}

func (b *LocalBackend) TouchRuleLastTriggered(ruleID int64) error {
	return b.cache.TouchRuleLastTriggered(ruleID)
}

func (b *LocalBackend) SaveCustomCategory(messageID string, promptID int64, result string) error {
	return b.cache.SaveCustomCategory(messageID, promptID, result)
}

// --- Contacts ---

func (b *LocalBackend) GetContactsToEnrich(minCount, limit int) ([]models.ContactData, error) {
	return b.cache.GetContactsToEnrich(minCount, limit)
}

func (b *LocalBackend) GetRecentSubjectsByContact(email string, limit int) ([]string, error) {
	return b.cache.GetRecentSubjectsByContact(email, limit)
}

func (b *LocalBackend) UpdateContactEnrichment(email, company string, topics []string) error {
	return b.cache.UpdateContactEnrichment(email, company, topics)
}

func (b *LocalBackend) UpdateContactEmbedding(email string, embedding []float32) error {
	return b.cache.UpdateContactEmbedding(email, embedding)
}

func (b *LocalBackend) SearchContactsSemantic(queryVec []float32, limit int, minScore float64) ([]*models.ContactSearchResult, error) {
	return b.cache.SearchContactsSemantic(queryVec, limit, minScore)
}

func (b *LocalBackend) ListContacts(limit int, sortBy string) ([]models.ContactData, error) {
	return b.cache.ListContacts(limit, sortBy)
}

func (b *LocalBackend) SearchContacts(query string) ([]models.ContactData, error) {
	return b.cache.SearchContacts(query)
}

func (b *LocalBackend) GetContactEmails(contactEmail string, limit int) ([]*models.EmailData, error) {
	return b.cache.GetContactEmails(contactEmail, limit)
}

func (b *LocalBackend) UpsertContacts(addrs []models.ContactAddr, direction string) error {
	return b.cache.UpsertContacts(addrs, direction)
}

// --- Unsubscribed senders ---

func (b *LocalBackend) RecordUnsubscribe(sender, method, url string) error {
	return b.cache.RecordUnsubscribe(sender, method, url)
}

func (b *LocalBackend) IsUnsubscribedSender(sender string) (bool, error) {
	return b.cache.IsUnsubscribedSender(sender)
}

// --- Reply / Forward / Attachments ---

func (b *LocalBackend) ReplyToEmail(messageID, replyBody string) error {
	email, err := b.cache.GetEmailByID(messageID)
	if err != nil {
		return fmt.Errorf("get email: %w", err)
	}
	if email == nil {
		return fmt.Errorf("email %s not found", messageID)
	}
	subject := email.Subject
	if !strings.HasPrefix(strings.ToLower(subject), "re:") {
		subject = "Re: " + subject
	}
	from := b.cfg.Credentials.Username
	html, plain := appsmtp.MarkdownToHTMLAndPlain(replyBody)
	mailer := appsmtp.New(b.cfg)
	return mailer.SendReply(from, email.Sender, subject, plain, html, email.MessageID, "")
}

func (b *LocalBackend) ForwardEmail(messageID, to, forwardBody string) error {
	email, err := b.cache.GetEmailByID(messageID)
	if err != nil {
		return fmt.Errorf("get email: %w", err)
	}
	if email == nil {
		return fmt.Errorf("email %s not found", messageID)
	}
	// Build subject: strip existing Re:/Fwd: prefix then prepend Fwd:
	subject := email.Subject
	lower := strings.ToLower(subject)
	if strings.HasPrefix(lower, "re: ") {
		subject = subject[4:]
	} else if strings.HasPrefix(lower, "fwd: ") {
		subject = subject[5:]
	}
	subject = "Fwd: " + subject

	body := forwardBody + "\n\n---------- Forwarded message ----------\nFrom: " + email.Sender + "\nSubject: " + email.Subject
	html, plain := appsmtp.MarkdownToHTMLAndPlain(body)
	from := b.cfg.Credentials.Username
	mailer := appsmtp.New(b.cfg)
	return mailer.Send(from, to, subject, plain, html)
}

func (b *LocalBackend) ListAttachments(messageID string) ([]models.Attachment, error) {
	email, err := b.cache.GetEmailByID(messageID)
	if err != nil {
		return nil, fmt.Errorf("get email: %w", err)
	}
	if email == nil {
		return nil, fmt.Errorf("email %s not found", messageID)
	}
	body, err := b.imapClient.FetchEmailBody(email.UID, email.Folder)
	if err != nil {
		return nil, fmt.Errorf("fetch body: %w", err)
	}
	// Zero out binary data — return metadata only
	for i := range body.Attachments {
		body.Attachments[i].Data = nil
	}
	return body.Attachments, nil
}

func (b *LocalBackend) GetAttachment(messageID, filename string) (*models.Attachment, error) {
	email, err := b.cache.GetEmailByID(messageID)
	if err != nil {
		return nil, fmt.Errorf("get email: %w", err)
	}
	if email == nil {
		return nil, fmt.Errorf("email %s not found", messageID)
	}
	body, err := b.imapClient.FetchEmailBody(email.UID, email.Folder)
	if err != nil {
		return nil, fmt.Errorf("fetch body: %w", err)
	}
	for _, a := range body.Attachments {
		if strings.EqualFold(a.Filename, filename) {
			aCopy := a
			return &aCopy, nil
		}
	}
	return nil, fmt.Errorf("attachment %q not found in message %s", filename, messageID)
}

// --- Drafts ---

func (b *LocalBackend) SaveDraft(to, cc, bcc, subject, body string) (uint32, string, error) {
	from := b.cfg.Credentials.Username
	raw, err := appsmtp.BuildDraftMessage(from, to, cc, bcc, subject, body)
	if err != nil {
		return 0, "", fmt.Errorf("build draft message: %w", err)
	}
	return b.imapClient.AppendDraft(raw)
}

func (b *LocalBackend) ListDrafts() ([]*models.Draft, error) {
	return b.imapClient.ListDrafts()
}

func (b *LocalBackend) DeleteDraft(uid uint32, folder string) error {
	return b.imapClient.DeleteDraft(uid, folder)
}

// --- Bulk operations ---

func (b *LocalBackend) DeleteThread(folder, subject string) error {
	emails, err := b.GetEmailsByThread(folder, subject)
	if err != nil {
		return fmt.Errorf("DeleteThread get thread: %w", err)
	}
	var firstErr error
	for _, email := range emails {
		if err := b.DeleteEmail(email.MessageID, folder); err != nil {
			logger.Warn("DeleteThread: failed to delete %s: %v", email.MessageID, err)
			if firstErr == nil {
				firstErr = err
			}
		}
	}
	return firstErr
}

func (b *LocalBackend) BulkDelete(messageIDs []string) error {
	var firstErr error
	for _, id := range messageIDs {
		email, err := b.cache.GetEmailByID(id)
		if err != nil {
			logger.Warn("BulkDelete: lookup %s: %v", id, err)
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		if email == nil {
			continue
		}
		if err := b.DeleteEmail(id, email.Folder); err != nil {
			logger.Warn("BulkDelete: delete %s: %v", id, err)
			if firstErr == nil {
				firstErr = err
			}
		}
	}
	return firstErr
}

func (b *LocalBackend) ArchiveThread(folder, subject string) error {
	emails, err := b.GetEmailsByThread(folder, subject)
	if err != nil {
		return fmt.Errorf("ArchiveThread get thread: %w", err)
	}
	var firstErr error
	for _, email := range emails {
		if err := b.ArchiveEmail(email.MessageID, folder); err != nil {
			logger.Warn("ArchiveThread: failed to archive %s: %v", email.MessageID, err)
			if firstErr == nil {
				firstErr = err
			}
		}
	}
	return firstErr
}

func (b *LocalBackend) BulkMove(messageIDs []string, toFolder string) error {
	var firstErr error
	for _, id := range messageIDs {
		email, err := b.cache.GetEmailByID(id)
		if err != nil {
			logger.Warn("BulkMove: lookup %s: %v", id, err)
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		if email == nil {
			continue
		}
		if err := b.MoveEmail(id, email.Folder, toFolder); err != nil {
			logger.Warn("BulkMove: move %s: %v", id, err)
			if firstErr == nil {
				firstErr = err
			}
		}
	}
	return firstErr
}

func (b *LocalBackend) UnsubscribeSender(messageID string) error {
	email, err := b.cache.GetEmailByID(messageID)
	if err != nil {
		return fmt.Errorf("UnsubscribeSender lookup: %w", err)
	}
	if email == nil {
		return fmt.Errorf("UnsubscribeSender: message %s not found", messageID)
	}
	body, err := b.imapClient.FetchEmailBody(email.UID, email.Folder)
	if err != nil {
		return fmt.Errorf("UnsubscribeSender fetch body: %w", err)
	}
	raw := body.ListUnsubscribe
	if raw == "" {
		return fmt.Errorf("no List-Unsubscribe header found for message %s", messageID)
	}

	// Parse angle-bracket-delimited URIs: <https://...>, <http://...>, <mailto:...>
	var httpsURL, httpURL string
	parts := strings.Split(raw, ",")
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if len(part) >= 2 && part[0] == '<' && part[len(part)-1] == '>' {
			part = part[1 : len(part)-1]
		}
		if strings.HasPrefix(part, "https://") && httpsURL == "" {
			httpsURL = part
		} else if strings.HasPrefix(part, "http://") && httpURL == "" {
			httpURL = part
		}
	}

	// One-click POST (RFC 8058)
	if body.ListUnsubscribePost == "List-Unsubscribe=One-Click" && httpsURL != "" {
		resp, err := http.Post(httpsURL, "application/x-www-form-urlencoded",
			strings.NewReader("List-Unsubscribe=One-Click"))
		if err != nil {
			return fmt.Errorf("UnsubscribeSender POST failed: %w", err)
		}
		resp.Body.Close()
		_ = b.RecordUnsubscribe(email.Sender, "one-click", httpsURL)
		return nil
	}

	// Browser fallback
	webURL := httpsURL
	if webURL == "" {
		webURL = httpURL
	}
	if webURL == "" {
		return fmt.Errorf("no usable HTTP/HTTPS unsubscribe URL found")
	}

	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", webURL)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", webURL)
	default:
		cmd = exec.Command("xdg-open", webURL)
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("UnsubscribeSender open browser: %w", err)
	}
	_ = b.RecordUnsubscribe(email.Sender, "browser-opened", webURL)
	return nil
}

func (b *LocalBackend) SoftUnsubscribeSender(sender, toFolder string) error {
	if toFolder == "" {
		toFolder = "Disabled Subscriptions"
	}
	rule := &models.Rule{
		Name:         "Soft unsub: " + sender,
		Enabled:      true,
		TriggerType:  models.TriggerSender,
		TriggerValue: sender,
		Actions: []models.RuleAction{{
			Type:       models.ActionMove,
			DestFolder: toFolder,
		}},
	}
	return b.SaveRule(rule)
}

// --- Folder management ---

func (b *LocalBackend) CreateFolder(name string) error {
	return b.imapClient.CreateMailbox(name)
}

func (b *LocalBackend) RenameFolder(existingName, newName string) error {
	return b.imapClient.RenameMailbox(existingName, newName)
}

func (b *LocalBackend) DeleteFolder(name string) error {
	return b.imapClient.DeleteMailbox(name)
}

// SyncAllFolders triggers background sync for all known folders.
// Load() is async (starts a goroutine), so this returns immediately with 0.
func (b *LocalBackend) SyncAllFolders() (int, error) {
	folders, err := b.ListFolders()
	if err != nil {
		return 0, fmt.Errorf("list folders: %w", err)
	}
	for _, folder := range folders {
		b.Load(folder)
	}
	return 0, nil
}

// GetSyncStatus returns per-folder message counts by listing folders then
// fetching their status from the IMAP server.
func (b *LocalBackend) GetSyncStatus() (map[string]models.FolderStatus, error) {
	folders, err := b.ListFolders()
	if err != nil {
		return nil, fmt.Errorf("list folders: %w", err)
	}
	return b.GetFolderStatus(folders)
}

// --- Cleanup rules ---

func (b *LocalBackend) GetAllCleanupRules() ([]*models.CleanupRule, error) {
	return b.cache.GetAllCleanupRules()
}

func (b *LocalBackend) SaveCleanupRule(rule *models.CleanupRule) error {
	return b.cache.SaveCleanupRule(rule)
}

func (b *LocalBackend) DeleteCleanupRule(id int64) error {
	return b.cache.DeleteCleanupRule(id)
}
