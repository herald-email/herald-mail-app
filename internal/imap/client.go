package imap

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"log"
	"net/mail"
	"strings"
	"sync"
	"time"

	"github.com/emersion/go-imap"
	"github.com/emersion/go-imap-idle"
	"github.com/emersion/go-imap/client"
	"mail-processor/internal/cache"
	"mail-processor/internal/config"
	"mail-processor/internal/logger"
	"mail-processor/internal/models"
	"mail-processor/internal/oauth"
)

// ErrIDLENotSupported is returned by StartIDLE when the server lacks IDLE capability.
var ErrIDLENotSupported = errors.New("imap: server does not support IDLE")

// xoauth2Client implements the XOAUTH2 SASL mechanism required by Gmail's IMAP server.
// XOAUTH2 is distinct from OAUTHBEARER (RFC 7628); Gmail only supports XOAUTH2.
// Format: "user=<username>\x01auth=Bearer <access_token>\x01\x01"
type xoauth2Client struct {
	username string
	token    string
}

func (c *xoauth2Client) Start() (string, []byte, error) {
	ir := []byte("user=" + c.username + "\x01auth=Bearer " + c.token + "\x01\x01")
	return "XOAUTH2", ir, nil
}

func (c *xoauth2Client) Next(_ []byte) ([]byte, error) {
	// XOAUTH2 is a single-step mechanism. If the server sends a challenge it
	// means authentication failed; respond with an empty string to get the error.
	return []byte{}, nil
}

var discardLogger = log.New(io.Discard, "", 0)

const fetchCacheBatchSize = 50

// Client wraps the IMAP client with business logic.
// mu serializes all operations that use the underlying IMAP connection;
// the go-imap client is not safe for concurrent use.
type Client struct {
	cfg           *config.Config
	configPath    string // path to write refreshed OAuth tokens back to disk
	client        *client.Client
	cache         *cache.Cache
	groupByDomain bool
	progressCh    chan models.ProgressInfo
	mu            sync.Mutex

	// IDLE support
	idleStop chan struct{}
	idleMu   sync.Mutex
}

// New creates a new IMAP client. configPath is used to persist refreshed OAuth tokens;
// pass an empty string if OAuth is not in use.
func New(cfg *config.Config, configPath string, cache *cache.Cache, progressCh chan models.ProgressInfo) *Client {
	return &Client{
		cfg:        cfg,
		configPath: configPath,
		cache:      cache,
		progressCh: progressCh,
	}
}

// isLocalServer reports whether the IMAP server is a local bridge (ProtonMail Bridge,
// local proxy, etc.) that requires InsecureSkipVerify. External providers such as
// Gmail, Fastmail, and Outlook use implicit TLS with valid certificates.
func (c *Client) isLocalServer() bool {
	h := c.cfg.Server.Host
	return h == "127.0.0.1" || h == "localhost" || c.cfg.Vendor == "protonmail"
}

// Connect establishes connection to IMAP server. It is a no-op if already connected.
func (c *Client) Connect() error {
	if c.client != nil {
		return nil // reuse existing connection
	}

	addr := fmt.Sprintf("%s:%d", c.cfg.Server.Host, c.cfg.Server.Port)

	var err error
	if c.isLocalServer() {
		// Local bridge: plain TCP + StartTLS with self-signed cert tolerance.
		c.client, err = client.Dial(addr)
		if err != nil {
			return fmt.Errorf("failed to connect to IMAP server: %w", err)
		}
		c.client.ErrorLog = discardLogger
		tlsCfg := &tls.Config{
			InsecureSkipVerify: true, // intentional for local ProtonMail Bridge
			ServerName:         c.cfg.Server.Host,
		}
		if err := c.client.StartTLS(tlsCfg); err != nil {
			return fmt.Errorf("failed to start TLS: %w", err)
		}
	} else {
		// External provider (Gmail, Fastmail, Outlook…): implicit TLS on port 993
		// with proper certificate verification.
		tlsCfg := &tls.Config{ServerName: c.cfg.Server.Host}
		c.client, err = client.DialTLS(addr, tlsCfg)
		if err != nil {
			return fmt.Errorf("failed to connect to IMAP server: %w", err)
		}
		c.client.ErrorLog = discardLogger
	}

	// Authenticate
	if c.cfg.IsGmailOAuth() {
		logger.Debug("Attempting Gmail XOAUTH2 authentication for: %s", c.cfg.Gmail.Email)
		accessToken, err := oauth.RefreshIfNeeded(context.Background(), c.cfg)
		if err != nil {
			return fmt.Errorf("failed to refresh OAuth token: %w", err)
		}
		// Persist any refreshed tokens so the next launch doesn't need a new refresh.
		if c.configPath != "" {
			if saveErr := c.cfg.Save(c.configPath); saveErr != nil {
				logger.Warn("Failed to persist refreshed OAuth token: %v", saveErr)
			}
		}
		saslClient := &xoauth2Client{username: c.cfg.Gmail.Email, token: accessToken}
		if err := c.client.Authenticate(saslClient); err != nil {
			logger.Error("XOAUTH2 authentication failed: %v", err)
			return fmt.Errorf("IMAP XOAUTH2 authentication failed: %w", err)
		}
	} else {
		logger.Debug("Attempting to authenticate with username: %s", c.cfg.Credentials.Username)
		if err := c.client.Login(c.cfg.Credentials.Username, c.cfg.Credentials.Password); err != nil {
			logger.Error("Authentication failed: %v", err)
			return fmt.Errorf("IMAP login failed: %w", err)
		}
	}

	logger.Info("Successfully connected to IMAP server at %s", addr)
	return nil
}

// Close closes the IMAP connection
func (c *Client) Close() error {
	if c.client != nil {
		return c.client.Logout()
	}
	return nil
}

// Reconnect tears down the current IMAP connection and establishes a new one.
// Must be called with c.mu held.
func (c *Client) Reconnect() error {
	logger.Info("Reconnecting to IMAP server…")
	if c.client != nil {
		_ = c.client.Logout() // best-effort close
		c.client = nil
	}
	return c.Connect()
}

// isConnectionError returns true if the error indicates the IMAP TCP connection
// is dead and a reconnect should be attempted.
func isConnectionError(err error) bool {
	if err == nil {
		return false
	}
	s := err.Error()
	return strings.Contains(s, "broken pipe") ||
		strings.Contains(s, "connection reset") ||
		strings.Contains(s, "connection closed") ||
		strings.Contains(s, "i/o timeout") ||
		strings.Contains(s, "use of closed network connection") ||
		strings.Contains(s, "EOF")
}

func retryAfterReconnect[T any](attempt func() (T, error), reconnect func() error) (T, error) {
	value, err := attempt()
	if err == nil || !isConnectionError(err) {
		return value, err
	}
	if reconnectErr := reconnect(); reconnectErr != nil {
		var zero T
		return zero, fmt.Errorf("reconnect after IMAP failure: %w (original error: %v)", reconnectErr, err)
	}
	return attempt()
}

func chunkUint32s(items []uint32, size int) [][]uint32 {
	if len(items) == 0 {
		return nil
	}
	if size <= 0 {
		size = len(items)
	}
	chunks := make([][]uint32, 0, (len(items)+size-1)/size)
	for start := 0; start < len(items); start += size {
		end := start + size
		if end > len(items) {
			end = len(items)
		}
		chunks = append(chunks, items[start:end])
	}
	return chunks
}

// GetFolderStatus fetches MESSAGES and UNSEEN counts for a list of folders via IMAP STATUS
func (c *Client) GetFolderStatus(folders []string) (map[string]models.FolderStatus, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.client == nil {
		return nil, fmt.Errorf("not connected")
	}
	result := make(map[string]models.FolderStatus)
	items := []imap.StatusItem{imap.StatusMessages, imap.StatusUnseen}
	for _, folder := range folders {
		status, err := c.client.Status(folder, items)
		if err != nil {
			logger.Warn("Failed to get status for folder %s: %v", folder, err)
			continue
		}
		result[folder] = models.FolderStatus{
			Total:  int(status.Messages),
			Unseen: int(status.Unseen),
		}
	}
	return result, nil
}

// ListFolders returns all mailbox names from the server
func (c *Client) ListFolders() ([]string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.client == nil {
		return nil, fmt.Errorf("not connected")
	}
	mailboxes := make(chan *imap.MailboxInfo, 20)
	done := make(chan error, 1)
	go func() {
		done <- c.client.List("", "*", mailboxes)
	}()
	var folders []string
	for m := range mailboxes {
		folders = append(folders, m.Name)
	}
	if err := <-done; err != nil {
		return nil, fmt.Errorf("failed to list folders: %w", err)
	}
	return folders, nil
}

// syncStrategy describes how ProcessEmailsIncremental should sync a folder.
type syncStrategy int

const (
	syncStrategyNone        syncStrategy = iota // UIDNEXT unchanged — no new mail
	syncStrategyIncremental                     // fetch only UIDs >= storedNext
	syncStrategyFull                            // UIDVALIDITY changed — clear cache + full fetch
)

// decideSyncStrategy computes which sync strategy to use given stored and server values.
// storedValidity==0 means no prior sync has been recorded (first run).
func decideSyncStrategy(storedValidity, storedNext, serverValidity, serverNext uint32) syncStrategy {
	if storedValidity == 0 || storedValidity != serverValidity {
		return syncStrategyFull
	}
	if storedNext == serverNext {
		return syncStrategyNone
	}
	return syncStrategyIncremental
}

func adjustSyncStrategyForCacheRecovery(strategy syncStrategy, storedValidity, serverValidity uint32, cachedCount, serverMessages int) syncStrategy {
	if strategy != syncStrategyNone {
		return strategy
	}
	if storedValidity == 0 || storedValidity != serverValidity {
		return strategy
	}
	if serverMessages <= 0 {
		return strategy
	}
	if cachedCount <= 0 || cachedCount < serverMessages {
		return syncStrategyFull
	}
	return strategy
}

// uidMsgPair holds a message-ID string and its IMAP UID as returned by the cache.
type uidMsgPair struct {
	MessageID string
	UID       uint32
}

// buildValidIDSet partitions cached entries into valid IDs, stale UIDs, and stale message IDs.
// A cache row is valid only when it has a non-zero UID that still exists on the server.
// Legacy/incomplete rows with uid==0 are invalidated by message ID so they do not linger forever.
// Stale UIDs are returned sorted descending (highest/newest first).
func buildValidIDSet(cached []uidMsgPair, serverUIDs map[uint32]bool) (validIDs map[string]bool, staleUIDs []uint32, staleMessageIDs []string) {
	validIDs = make(map[string]bool, len(cached))
	for _, row := range cached {
		if row.UID == 0 {
			staleMessageIDs = append(staleMessageIDs, row.MessageID)
		} else if serverUIDs[row.UID] {
			validIDs[row.MessageID] = true
		} else {
			staleUIDs = append(staleUIDs, row.UID)
		}
	}
	// Sort descending so newest (highest UID) are deleted first
	for i, j := 0, len(staleUIDs)-1; i < j; i, j = i+1, j-1 {
		staleUIDs[i], staleUIDs[j] = staleUIDs[j], staleUIDs[i]
	}
	// Simple insertion sort to ensure descending order (staleUIDs is small in the common case)
	for i := 1; i < len(staleUIDs); i++ {
		for j := i; j > 0 && staleUIDs[j] > staleUIDs[j-1]; j-- {
			staleUIDs[j], staleUIDs[j-1] = staleUIDs[j-1], staleUIDs[j]
		}
	}
	return validIDs, staleUIDs, staleMessageIDs
}

// ProcessEmailsIncremental performs a UIDVALIDITY-aware incremental sync for folder.
// It replaces the ProcessEmails + CleanupCache combination in the Load() path.
func (c *Client) ProcessEmailsIncremental(folder string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	logger.Info("ProcessEmailsIncremental: starting for folder %s", folder)

	c.sendProgress(models.ProgressInfo{
		Phase:   "scanning",
		Message: fmt.Sprintf("Opening %s...", folder),
	})

	mbox, err := retryAfterReconnect(func() (*imap.MailboxStatus, error) {
		return c.client.Select(folder, false)
	}, c.Reconnect)
	if err != nil {
		return fmt.Errorf("select folder %s: %w", folder, err)
	}

	c.sendProgress(models.ProgressInfo{
		Phase:   "scanning",
		Total:   int(mbox.Messages),
		Message: fmt.Sprintf("Checking sync state in %s (%d messages on server)...", folder, mbox.Messages),
	})

	storedValidity, storedNext, err := c.cache.GetFolderSyncState(folder)
	if err != nil {
		return fmt.Errorf("get folder sync state: %w", err)
	}

	totalMessages := int(mbox.Messages)
	strategy := decideSyncStrategy(storedValidity, storedNext, mbox.UidValidity, mbox.UidNext)
	cachedCount, err := c.cache.CountEmailsInFolder(folder)
	if err != nil {
		return fmt.Errorf("count cached emails in %s: %w", folder, err)
	}
	if recovered := adjustSyncStrategyForCacheRecovery(strategy, storedValidity, mbox.UidValidity, cachedCount, totalMessages); recovered != strategy {
		logger.Warn(
			"ProcessEmailsIncremental: cache recovery triggered for %s (cached=%d server=%d uidvalidity=%d)",
			folder, cachedCount, totalMessages, mbox.UidValidity,
		)
		c.sendProgress(models.ProgressInfo{
			Phase:   "scanning",
			Total:   totalMessages,
			Current: cachedCount,
			Message: fmt.Sprintf("Recovering incomplete %s cache (%d/%d cached)...", folder, cachedCount, totalMessages),
		})
		strategy = recovered
	}
	logger.Info("ProcessEmailsIncremental: strategy=%d stored(v=%d n=%d) server(v=%d n=%d)",
		strategy, storedValidity, storedNext, mbox.UidValidity, mbox.UidNext)

	logger.Info("ProcessEmailsIncremental: server reports %d total messages in %s", totalMessages, folder)

	switch strategy {
	case syncStrategyNone:
		logger.Info("ProcessEmailsIncremental: no new mail in %s", folder)
		c.sendProgress(models.ProgressInfo{
			Phase:   "scanning",
			Total:   totalMessages,
			Current: totalMessages,
			Message: fmt.Sprintf("No new mail in %s — %d messages already current", folder, totalMessages),
		})
		return nil

	case syncStrategyFull:
		logger.Info("ProcessEmailsIncremental: UIDVALIDITY changed or first run — full resync of %s", folder)
		c.sendProgress(models.ProgressInfo{
			Phase:   "scanning",
			Total:   totalMessages,
			Message: fmt.Sprintf("Refreshing %s from the server...", folder),
		})
		if storedValidity != 0 {
			if err := c.cache.ClearFolder(folder); err != nil {
				return fmt.Errorf("clear folder on uidvalidity change: %w", err)
			}
		}
		if totalMessages > 0 {
			if err := c.fetchAndCacheRange(folder, 1, uint32(totalMessages), totalMessages); err != nil {
				return err
			}
		}

	case syncStrategyIncremental:
		logger.Info("ProcessEmailsIncremental: fetching new UIDs [%d:*] in %s", storedNext, folder)
		c.sendProgress(models.ProgressInfo{
			Phase:   "scanning",
			Total:   totalMessages,
			Message: fmt.Sprintf("Checking for new mail in %s...", folder),
		})
		if totalMessages > 0 {
			if err := c.fetchAndCacheUIDRange(folder, storedNext, totalMessages); err != nil {
				return err
			}
		}
	}

	if err := c.cache.SetFolderSyncState(folder, mbox.UidValidity, mbox.UidNext); err != nil {
		logger.Warn("ProcessEmailsIncremental: failed to persist sync state: %v", err)
	}
	return nil
}

// fetchAndCacheRange fetches sequence numbers start:end and caches each message.
// Reuses the existing processMessage path.
func (c *Client) fetchAndCacheRange(folder string, start, end uint32, total int) error {
	seqset := new(imap.SeqSet)
	seqset.AddRange(start, end)
	messages := make(chan *imap.Message, 10)
	done := make(chan error, 1)
	go func() {
		done <- c.client.Fetch(seqset, []imap.FetchItem{imap.FetchEnvelope}, messages)
	}()
	processed := 0
	var newSeqNums []uint32
	cachedIDs, err := c.cache.GetCachedIDs(folder)
	if err != nil {
		return fmt.Errorf("fetch envelopes cached IDs: %w", err)
	}
	for msg := range messages {
		processed++
		if processed%100 == 0 {
			c.sendProgress(models.ProgressInfo{
				Phase:   "scanning",
				Current: processed,
				Total:   total,
				Message: fmt.Sprintf("Scanning messages... [%d%%] (%d/%d)", processed*100/total, processed, total),
			})
		}
		messageID := extractMessageID(msg)
		if messageID != "" && !cachedIDs[messageID] {
			newSeqNums = append(newSeqNums, msg.SeqNum)
		}
	}
	if err := <-done; err != nil {
		return fmt.Errorf("fetch envelopes: %w", err)
	}
	if len(newSeqNums) > 0 {
		logger.Info("fetchAndCacheRange: caching %d uncached messages in %s (server total %d)", len(newSeqNums), folder, total)
		c.sendProgress(models.ProgressInfo{
			Phase:   "fetching",
			Current: 0,
			Total:   len(newSeqNums),
			Message: fmt.Sprintf("Fetching %d new emails for %s...", len(newSeqNums), folder),
		})
		cachedCount := 0
		for _, chunk := range chunkUint32s(newSeqNums, fetchCacheBatchSize) {
			if err := c.batchFetchDetails(chunk, folder); err != nil {
				return err
			}
			cachedCount += len(chunk)
			c.sendProgress(models.ProgressInfo{
				Phase:   "fetching",
				Current: cachedCount,
				Total:   len(newSeqNums),
				Message: fmt.Sprintf("Fetched %d/%d new emails into cache", cachedCount, len(newSeqNums)),
			})
		}
	} else {
		logger.Info("fetchAndCacheRange: no uncached messages to fetch in %s", folder)
	}
	return nil
}

// fetchAndCacheUIDRange fetches messages with UID >= startUID using UidFetch.
func (c *Client) fetchAndCacheUIDRange(folder string, startUID uint32, total int) error {
	seqset := new(imap.SeqSet)
	// AddRange(start, 0) means start:* in go-imap (0 = *)
	seqset.AddRange(startUID, 0)
	messages := make(chan *imap.Message, 10)
	done := make(chan error, 1)
	go func() {
		done <- c.client.UidFetch(seqset, []imap.FetchItem{imap.FetchEnvelope, imap.FetchUid}, messages)
	}()
	var newSeqNums []uint32
	cachedIDs, err := c.cache.GetCachedIDs(folder)
	if err != nil {
		return fmt.Errorf("uid fetch cached IDs: %w", err)
	}
	for msg := range messages {
		messageID := extractMessageID(msg)
		if messageID != "" && !cachedIDs[messageID] {
			newSeqNums = append(newSeqNums, msg.SeqNum)
		}
	}
	if err := <-done; err != nil {
		return fmt.Errorf("uid fetch new range: %w", err)
	}
	if len(newSeqNums) > 0 {
		logger.Info("fetchAndCacheUIDRange: caching %d new messages in %s (server total %d)", len(newSeqNums), folder, total)
		c.sendProgress(models.ProgressInfo{
			Phase:   "fetching",
			Current: 0,
			Total:   len(newSeqNums),
			Message: fmt.Sprintf("Fetching %d new emails for %s...", len(newSeqNums), folder),
		})
		cachedCount := 0
		for _, chunk := range chunkUint32s(newSeqNums, fetchCacheBatchSize) {
			if err := c.batchFetchDetails(chunk, folder); err != nil {
				return err
			}
			cachedCount += len(chunk)
			c.sendProgress(models.ProgressInfo{
				Phase:   "fetching",
				Current: cachedCount,
				Total:   len(newSeqNums),
				Message: fmt.Sprintf("Fetched %d/%d new emails into cache", cachedCount, len(newSeqNums)),
			})
		}
	} else {
		logger.Info("fetchAndCacheUIDRange: no new uncached messages to fetch in %s", folder)
	}
	return nil
}

// fetchAllServerUIDs returns the set of all UIDs currently on the server for the
// already-selected folder. Caller must hold c.mu.
func (c *Client) fetchAllServerUIDs() (map[uint32]bool, error) {
	seqset := new(imap.SeqSet)
	seqset.AddRange(1, 0) // 1:*
	messages := make(chan *imap.Message, 50)
	done := make(chan error, 1)
	go func() {
		done <- c.client.UidFetch(seqset, []imap.FetchItem{imap.FetchUid}, messages)
	}()
	serverUIDs := make(map[uint32]bool)
	for msg := range messages {
		if msg.Uid > 0 {
			serverUIDs[msg.Uid] = true
		}
	}
	if err := <-done; err != nil {
		return nil, fmt.Errorf("uid fetch all: %w", err)
	}
	return serverUIDs, nil
}

// StartBackgroundReconcile fetches all server UIDs for folder, immediately sends
// the valid-ID set on validIDsCh, then gradually deletes stale cache rows
// (newest-first, in batches of 50) before closing the channel.
func (c *Client) StartBackgroundReconcile(folder string, validIDsCh chan<- map[string]bool) {
	go func() {
		// Fetch all server UIDs (lightweight — no envelopes).
		c.mu.Lock()
		if _, err := c.client.Select(folder, true); err != nil {
			c.mu.Unlock()
			logger.Warn("StartBackgroundReconcile: select %s: %v", folder, err)
			close(validIDsCh)
			return
		}
		serverUIDs, err := c.fetchAllServerUIDs()
		c.mu.Unlock()
		if err != nil {
			logger.Warn("StartBackgroundReconcile: fetchAllServerUIDs: %v", err)
			close(validIDsCh)
			return
		}
		logger.Info("StartBackgroundReconcile: %d UIDs on server for %s", len(serverUIDs), folder)

		// Build valid-ID set from cache without holding the IMAP mutex.
		rawRows, err := c.cache.GetCachedUIDsAndMessageIDs(folder)
		if err != nil {
			logger.Warn("StartBackgroundReconcile: GetCachedUIDsAndMessageIDs: %v", err)
			close(validIDsCh)
			return
		}
		pairs := make([]uidMsgPair, len(rawRows))
		for i, r := range rawRows {
			pairs[i] = uidMsgPair{MessageID: r.MessageID, UID: r.UID}
		}
		validIDs, staleUIDs, staleMessageIDs := buildValidIDSet(pairs, serverUIDs)
		logger.Info(
			"StartBackgroundReconcile: %d valid, %d stale uid-backed, %d stale legacy/no-uid in %s",
			len(validIDs), len(staleUIDs), len(staleMessageIDs), folder,
		)

		// Broadcast valid IDs immediately — UI can filter right away.
		validIDsCh <- validIDs

		// Gradually delete stale rows (newest first for UID-backed rows, batches of 50).
		const batchSize = 50
		for i := 0; i < len(staleUIDs); i += batchSize {
			end := i + batchSize
			if end > len(staleUIDs) {
				end = len(staleUIDs)
			}
			if err := c.cache.DeleteEmailsByUIDs(folder, staleUIDs[i:end]); err != nil {
				logger.Warn("StartBackgroundReconcile: delete batch: %v", err)
			}
			time.Sleep(100 * time.Millisecond)
		}
		for i := 0; i < len(staleMessageIDs); i += batchSize {
			end := i + batchSize
			if end > len(staleMessageIDs) {
				end = len(staleMessageIDs)
			}
			if err := c.cache.DeleteEmailsByMessageIDs(folder, staleMessageIDs[i:end]); err != nil {
				logger.Warn("StartBackgroundReconcile: delete legacy batch: %v", err)
			}
			time.Sleep(100 * time.Millisecond)
		}
		logger.Info("StartBackgroundReconcile: done for %s", folder)
		close(validIDsCh)
	}()
}

// ProcessEmails reads and processes all emails from specified folder
func (c *Client) ProcessEmails(folder string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	logger.Info("Starting to process emails in folder: %s", folder)

	// Select folder
	logger.Debug("Selecting folder: %s", folder)
	mbox, err := c.client.Select(folder, false)
	if err != nil {
		logger.Error("Failed to select folder %s: %v", folder, err)
		return fmt.Errorf("failed to select folder %s: %w", folder, err)
	}

	totalMessages := int(mbox.Messages)
	logger.Info("Found %d messages in folder %s", totalMessages, folder)
	logger.Debug("Mailbox status - Messages: %d, Recent: %d, Unseen: %d",
		mbox.Messages, mbox.Recent, mbox.Unseen)

	if totalMessages == 0 {
		logger.Info("Folder %s is empty, skipping fetch", folder)
		return nil
	}

	c.sendProgress(models.ProgressInfo{
		Phase:   "scanning",
		Total:   totalMessages,
		Message: fmt.Sprintf("Found %d emails in %s", totalMessages, folder),
	})

	// Get cached message IDs
	logger.Debug("Getting cached message IDs...")
	cachedIDs, err := c.cache.GetCachedIDs(folder)
	if err != nil {
		logger.Error("Failed to get cached IDs: %v", err)
		return fmt.Errorf("failed to get cached IDs: %w", err)
	}
	logger.Info("Found %d cached message IDs", len(cachedIDs))

	// Find new messages
	newMessages := []uint32{}
	seqset := new(imap.SeqSet)
	seqset.AddRange(1, mbox.Messages)

	// Fetch message IDs for all messages (using Envelope which contains Message-ID)
	messages := make(chan *imap.Message, 10)
	done := make(chan error, 1)

	go func() {
		done <- c.client.Fetch(seqset, []imap.FetchItem{imap.FetchEnvelope}, messages)
	}()

	processed := 0
	for msg := range messages {
		processed++
		if processed%100 == 0 {
			progress := (processed * 100) / totalMessages
			c.sendProgress(models.ProgressInfo{
				Phase:   "scanning",
				Current: processed,
				Total:   totalMessages,
				Message: fmt.Sprintf("Scanning messages... [%d%%] (%d/%d)", progress, processed, totalMessages),
			})
		}

		// Extract message ID
		messageID := extractMessageID(msg)
		if messageID != "" && !cachedIDs[messageID] {
			newMessages = append(newMessages, msg.SeqNum)
			logger.Debug("Found new message: %s (seq: %d)", messageID, msg.SeqNum)
		}
	}

	if err := <-done; err != nil {
		return fmt.Errorf("failed to fetch message headers: %w", err)
	}

	newCount := len(newMessages)
	logger.Info("Found %d new messages to process", newCount)

	if totalMessages > 0 {
		cacheHitRate := float64(len(cachedIDs)) / float64(totalMessages) * 100
		logger.Debug("Cache hit rate: %.1f%% (%d cached / %d total)",
			cacheHitRate, len(cachedIDs), totalMessages)
	}

	if newCount > 0 {
		c.sendProgress(models.ProgressInfo{
			Phase:     "processing",
			Total:     newCount,
			NewEmails: newCount,
			Message:   fmt.Sprintf("Processing %d new emails...", newCount),
		})

		// Process new messages
		for i, seqNum := range newMessages {
			logger.Debug("Processing message %d/%d (seqNum: %d)", i+1, newCount, seqNum)
			if err := c.processMessage(seqNum, folder); err != nil {
				logger.Warn("Error processing message %d: %v", seqNum, err)
				continue
			}

			progress := ((i + 1) * 100) / newCount
			c.sendProgress(models.ProgressInfo{
				Phase:           "processing",
				Current:         i + 1,
				Total:           newCount,
				ProcessedEmails: i + 1,
				Message:         fmt.Sprintf("Processing emails... [%d%%] (%d/%d)", progress, i+1, newCount),
			})
		}
	} else {
		// Do NOT send "complete" here — Load() owns that signal.
		// Sending "complete" from ProcessEmails leaves a stale message in the
		// buffered channel that poisons the next folder switch.
		logger.Info("No new emails in %s (all %d cached)", folder, len(cachedIDs))
	}

	return nil
}

// batchFetchDetails fetches Envelope, UID, Flags, Size, and BodyStructure for
// all seqNums in a single IMAP round trip, then persists results in one SQLite
// transaction. This replaces serial processMessage calls for large batches.
func (c *Client) batchFetchDetails(seqNums []uint32, folder string) error {
	if len(seqNums) == 0 {
		return nil
	}
	seqset := new(imap.SeqSet)
	for _, s := range seqNums {
		seqset.AddNum(s)
	}
	items := []imap.FetchItem{
		imap.FetchEnvelope,
		imap.FetchUid,
		imap.FetchRFC822Size,
		imap.FetchBodyStructure,
		imap.FetchFlags,
	}
	messages := make(chan *imap.Message, len(seqNums))
	done := make(chan error, 1)
	go func() {
		done <- c.client.Fetch(seqset, items, messages)
	}()

	var emails []*models.EmailData
	var allFrom []models.ContactAddr
	var allTo []models.ContactAddr

	for msg := range messages {
		if msg == nil {
			continue
		}
		if msg.Envelope == nil {
			logger.Warn("batchFetchDetails: no envelope for seq %d, skipping", msg.SeqNum)
			continue
		}

		sender := ""
		if len(msg.Envelope.From) > 0 && msg.Envelope.From[0] != nil {
			addr := msg.Envelope.From[0]
			if addr.MailboxName != "" && addr.HostName != "" {
				sender = addr.MailboxName + "@" + addr.HostName
			}
		}
		if sender == "" {
			logger.Warn("batchFetchDetails: empty sender for seq %d, skipping", msg.SeqNum)
			continue
		}

		messageID := msg.Envelope.MessageId
		if messageID == "" && msg.Uid > 0 {
			messageID = fmt.Sprintf("uid-%d", msg.Uid)
		}

		hasAttach := false
		if msg.BodyStructure != nil {
			hasAttach = checkBodyStructureForAttachments(msg.BodyStructure)
		}

		isRead := false
		for _, flag := range msg.Flags {
			if flag == imap.SeenFlag {
				isRead = true
				break
			}
		}

		emails = append(emails, &models.EmailData{
			MessageID:      messageID,
			UID:            msg.Uid,
			Sender:         sender,
			Subject:        msg.Envelope.Subject,
			Date:           msg.Envelope.Date,
			Size:           int(msg.Size),
			HasAttachments: hasAttach,
			IsRead:         isRead,
			Folder:         folder,
		})

		allFrom = append(allFrom, extractEnvelopeAddrs(msg.Envelope.From)...)
		var toAddrs []models.ContactAddr
		toAddrs = append(toAddrs, extractEnvelopeAddrs(msg.Envelope.To)...)
		toAddrs = append(toAddrs, extractEnvelopeAddrs(msg.Envelope.Cc)...)
		toAddrs = append(toAddrs, extractEnvelopeAddrs(msg.Envelope.Bcc)...)
		allTo = append(allTo, toAddrs...)
	}

	if err := <-done; err != nil {
		return fmt.Errorf("batchFetchDetails: fetch: %w", err)
	}

	if err := c.cache.BatchCacheEmails(emails); err != nil {
		return fmt.Errorf("batchFetchDetails: cache: %w", err)
	}

	if len(allFrom) > 0 {
		if err := c.cache.UpsertContacts(allFrom, "from"); err != nil {
			logger.Warn("batchFetchDetails: UpsertContacts (from): %v", err)
		}
	}
	if len(allTo) > 0 {
		if err := c.cache.UpsertContacts(allTo, "to"); err != nil {
			logger.Warn("batchFetchDetails: UpsertContacts (to): %v", err)
		}
	}

	return nil
}

// processMessage processes a single message using Envelope (avoids parsing errors)
func (c *Client) processMessage(seqNum uint32, folder string) error {
	seqset := new(imap.SeqSet)
	seqset.AddNum(seqNum)

	messages := make(chan *imap.Message, 1)
	done := make(chan error, 1)

	// Fetch using Envelope + basic fields to avoid RFC822 parsing issues
	items := []imap.FetchItem{imap.FetchEnvelope, imap.FetchUid, imap.FetchRFC822Size, imap.FetchBodyStructure, imap.FetchFlags}

	go func() {
		done <- c.client.Fetch(seqset, items, messages)
	}()

	msg := <-messages
	if err := <-done; err != nil {
		logger.Warn("Error fetching message %d: %v, skipping", seqNum, err)
		return nil // Skip problematic messages
	}

	if msg == nil || msg.Envelope == nil {
		logger.Warn("No envelope for message %d, skipping", seqNum)
		return nil
	}

	// Extract sender from envelope
	sender := ""
	if len(msg.Envelope.From) > 0 && msg.Envelope.From[0] != nil {
		addr := msg.Envelope.From[0]
		if addr.MailboxName != "" && addr.HostName != "" {
			sender = addr.MailboxName + "@" + addr.HostName
		}
	}

	if sender == "" {
		logger.Warn("Empty sender for message %d, skipping", seqNum)
		return nil
	}

	// Check for attachments from body structure
	hasAttach := false
	if msg.BodyStructure != nil {
		hasAttach = checkBodyStructureForAttachments(msg.BodyStructure)
	}

	// Use UID as MessageID if envelope MessageId is empty
	// UID is guaranteed to be unique within a folder
	messageID := msg.Envelope.MessageId
	if messageID == "" && msg.Uid > 0 {
		messageID = fmt.Sprintf("uid-%d", msg.Uid)
	}

	// Check \Seen flag
	isRead := false
	for _, flag := range msg.Flags {
		if flag == imap.SeenFlag {
			isRead = true
			break
		}
	}

	// Extract email data from Envelope
	emailData := &models.EmailData{
		MessageID:      messageID,
		UID:            msg.Uid,
		Sender:         sender,
		Subject:        msg.Envelope.Subject,
		Date:           msg.Envelope.Date,
		Size:           int(msg.Size),
		HasAttachments: hasAttach,
		IsRead:         isRead,
		Folder:         folder,
	}

	// Cache the email
	if err := c.cache.CacheEmail(emailData); err != nil {
		return err
	}

	// Upsert contacts from envelope headers
	fromAddrs := extractEnvelopeAddrs(msg.Envelope.From)
	if err := c.cache.UpsertContacts(fromAddrs, "from"); err != nil {
		logger.Warn("UpsertContacts (from) for message %s: %v", messageID, err)
	}

	var toAddrs []models.ContactAddr
	toAddrs = append(toAddrs, extractEnvelopeAddrs(msg.Envelope.To)...)
	toAddrs = append(toAddrs, extractEnvelopeAddrs(msg.Envelope.Cc)...)
	toAddrs = append(toAddrs, extractEnvelopeAddrs(msg.Envelope.Bcc)...)
	if err := c.cache.UpsertContacts(toAddrs, "to"); err != nil {
		logger.Warn("UpsertContacts (to) for message %s: %v", messageID, err)
	}

	return nil
}

// GetEmailsBySender retrieves all emails grouped by sender
func (c *Client) GetEmailsBySender(folder string) (map[string][]*models.EmailData, error) {
	return c.cache.GetAllEmails(folder, c.groupByDomain)
}

// GetSenderStatistics generates statistics for each sender
func (c *Client) GetSenderStatistics(folder string) (map[string]*models.SenderStats, error) {
	emailsBySender, err := c.GetEmailsBySender(folder)
	if err != nil {
		return nil, err
	}

	stats := make(map[string]*models.SenderStats)

	for sender, emails := range emailsBySender {
		if len(emails) == 0 {
			continue
		}

		totalSize := 0
		withAttachments := 0
		var firstEmail, lastEmail time.Time

		for i, email := range emails {
			totalSize += email.Size
			if email.HasAttachments {
				withAttachments++
			}

			if i == 0 {
				firstEmail = email.Date
				lastEmail = email.Date
			} else {
				if email.Date.Before(firstEmail) {
					firstEmail = email.Date
				}
				if email.Date.After(lastEmail) {
					lastEmail = email.Date
				}
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

	return stats, nil
}

// SetGroupByDomain sets the grouping mode
func (c *Client) SetGroupByDomain(groupByDomain bool) {
	c.groupByDomain = groupByDomain
}

// SearchIMAP performs a server-side IMAP search for messages matching the query text
// in the From, Subject, or Body fields. Returns matching emails fetched from the server.
func (c *Client) SearchIMAP(folder, query string) ([]*models.EmailData, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.client == nil {
		return nil, fmt.Errorf("not connected")
	}
	if _, err := c.client.Select(folder, true); err != nil {
		return nil, fmt.Errorf("failed to select folder: %w", err)
	}

	criteria := imap.NewSearchCriteria()
	criteria.Text = []string{query}
	seqNums, err := c.client.Search(criteria)
	if err != nil {
		return nil, fmt.Errorf("IMAP search failed: %w", err)
	}
	if len(seqNums) == 0 {
		return nil, nil
	}

	seqset := new(imap.SeqSet)
	seqset.AddNum(seqNums...)
	messages := make(chan *imap.Message, 20)
	done := make(chan error, 1)
	go func() {
		done <- c.client.Fetch(seqset, []imap.FetchItem{imap.FetchEnvelope, imap.FetchRFC822Size, imap.FetchUid}, messages)
	}()

	var emails []*models.EmailData
	for msg := range messages {
		if msg.Envelope == nil {
			continue
		}
		sender := ""
		if len(msg.Envelope.From) > 0 && msg.Envelope.From[0] != nil {
			addr := msg.Envelope.From[0]
			if addr.MailboxName != "" && addr.HostName != "" {
				sender = addr.MailboxName + "@" + addr.HostName
			}
		}
		msgID := msg.Envelope.MessageId
		if msgID == "" {
			msgID = fmt.Sprintf("uid-%d", msg.Uid)
		}
		emails = append(emails, &models.EmailData{
			MessageID: msgID,
			UID:       msg.Uid,
			Sender:    sender,
			Subject:   msg.Envelope.Subject,
			Date:      msg.Envelope.Date,
			Size:      int(msg.Size),
			Folder:    folder,
		})
	}
	if err := <-done; err != nil {
		return nil, err
	}
	return emails, nil
}

// PollForNewEmails checks for messages newer than sinceDate and returns them.
// This is used by the background polling goroutine for auto-refresh.
func (c *Client) PollForNewEmails(folder string, sinceDate time.Time) ([]*models.EmailData, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.client == nil {
		return nil, fmt.Errorf("not connected")
	}
	if _, err := c.client.Select(folder, true); err != nil {
		return nil, fmt.Errorf("failed to select folder: %w", err)
	}

	criteria := imap.NewSearchCriteria()
	criteria.Since = sinceDate
	seqNums, err := c.client.Search(criteria)
	if err != nil {
		return nil, err
	}
	if len(seqNums) == 0 {
		return nil, nil
	}

	seqset := new(imap.SeqSet)
	seqset.AddNum(seqNums...)
	messages := make(chan *imap.Message, 20)
	done := make(chan error, 1)
	go func() {
		done <- c.client.Fetch(seqset, []imap.FetchItem{imap.FetchEnvelope, imap.FetchRFC822Size, imap.FetchUid}, messages)
	}()

	var emails []*models.EmailData
	for msg := range messages {
		if msg.Envelope == nil {
			continue
		}
		sender := ""
		if len(msg.Envelope.From) > 0 && msg.Envelope.From[0] != nil {
			addr := msg.Envelope.From[0]
			if addr.MailboxName != "" && addr.HostName != "" {
				sender = addr.MailboxName + "@" + addr.HostName
			}
		}
		msgID := msg.Envelope.MessageId
		if msgID == "" {
			msgID = fmt.Sprintf("uid-%d", msg.Uid)
		}
		emails = append(emails, &models.EmailData{
			MessageID: msgID,
			UID:       msg.Uid,
			Sender:    sender,
			Subject:   msg.Envelope.Subject,
			Date:      msg.Envelope.Date,
			Size:      int(msg.Size),
			Folder:    folder,
		})
	}
	if err := <-done; err != nil {
		return nil, err
	}
	return emails, nil
}

// sendProgress sends progress update through channel
func (c *Client) sendProgress(info models.ProgressInfo) {
	select {
	case c.progressCh <- info:
	default:
		// Channel is full, skip this update
	}
}

// Helper functions

func extractMessageID(msg *imap.Message) string {
	// Use Envelope.MessageId which is much faster and more reliable
	if msg.Envelope != nil && msg.Envelope.MessageId != "" {
		return msg.Envelope.MessageId
	}

	// Fallback to body parsing if envelope not available
	if len(msg.Body) == 0 {
		return ""
	}

	bodyReader := msg.Body[&imap.BodySectionName{}]
	if bodyReader == nil {
		return ""
	}

	mailMsg, err := mail.ReadMessage(bodyReader)
	if err != nil {
		return ""
	}

	return mailMsg.Header.Get("Message-Id")
}

func extractMessageIDFromMail(msg *mail.Message) string {
	return msg.Header.Get("Message-Id")
}

func extractSender(msg *mail.Message) string {
	from := msg.Header.Get("From")
	if from == "" {
		return ""
	}

	// Parse the From header
	addr, err := mail.ParseAddress(from)
	if err != nil {
		// If parsing fails, return the raw from header
		return from
	}

	return addr.Address
}

func parseDate(dateStr string) time.Time {
	if dateStr == "" {
		return time.Time{}
	}

	// Try various date formats
	formats := []string{
		"Mon, 02 Jan 2006 15:04:05 -0700", // RFC2822
		"Mon, 2 Jan 2006 15:04:05 -0700",
		"2 Jan 2006 15:04:05 -0700",
		time.RFC3339,
		"2006-01-02T15:04:05Z",
	}

	for _, format := range formats {
		if t, err := time.Parse(format, dateStr); err == nil {
			return t
		}
	}

	logger.Warn("Could not parse date '%s'", dateStr)
	return time.Time{}
}

func hasAttachments(msg *mail.Message) bool {
	contentType := msg.Header.Get("Content-Type")
	return strings.Contains(strings.ToLower(contentType), "multipart")
}

// StartIDLE starts an IMAP IDLE session on the given folder, sending a
// NewEmailsNotification on newEmailsCh whenever new messages arrive.
// Returns ErrIDLENotSupported if the server does not support IDLE.
// Note: IDLE holds the connection; no other IMAP commands should be issued
// while IDLE is active. Call StopIDLE before issuing further commands.
func (c *Client) StartIDLE(folder string, newEmailsCh chan<- models.NewEmailsNotification) error {
	// Lock ordering: idleMu must be acquired before c.mu to avoid deadlock.
	// All paths that hold both locks must follow this order.
	c.idleMu.Lock()
	defer c.idleMu.Unlock()

	if c.idleStop != nil {
		return nil // already running
	}

	idleClient := idle.NewClient(c.client)
	supported, err := idleClient.SupportIdle()
	if err != nil {
		return fmt.Errorf("imap idle: capability check: %w", err)
	}
	if !supported {
		return ErrIDLENotSupported
	}

	c.mu.Lock()
	if c.client == nil {
		c.mu.Unlock()
		return fmt.Errorf("imap idle: not connected")
	}
	if _, err := c.client.Select(folder, true); err != nil {
		c.mu.Unlock()
		return fmt.Errorf("imap idle: select folder %s: %w", folder, err)
	}
	// Set up the updates channel before starting IDLE so we don't miss events.
	updates := make(chan client.Update, 10)
	c.client.Updates = updates
	c.mu.Unlock()

	stop := make(chan struct{})
	c.idleStop = stop

	go func() {
		defer func() {
			c.mu.Lock()
			if c.client != nil {
				c.client.Updates = nil
			}
			c.mu.Unlock()
		}()

		// idleErrCh carries the result of IdleWithFallback so we can log it.
		idleErrCh := make(chan error, 1)
		go func() {
			idleErrCh <- idleClient.IdleWithFallback(stop, 0)
		}()

		lastPoll := time.Now()
		for {
			select {
			case <-stop:
				<-idleErrCh // drain
				return
			case err := <-idleErrCh:
				if err != nil {
					logger.Warn("IMAP IDLE ended with error: %v", err)
				}
				return
			case upd, ok := <-updates:
				if !ok {
					return
				}
				if _, isMailbox := upd.(*client.MailboxUpdate); isMailbox {
					emails, err := c.PollForNewEmails(folder, lastPoll)
					if err != nil {
						logger.Warn("IDLE poll for new emails failed: %v", err)
						continue
					}
					if len(emails) > 0 {
						lastPoll = time.Now()
						select {
						case newEmailsCh <- models.NewEmailsNotification{Emails: emails, Folder: folder}:
						default:
						}
					}
				}
			}
		}
	}()

	return nil
}

// StopIDLE stops a running IDLE session.
func (c *Client) StopIDLE() {
	c.idleMu.Lock()
	defer c.idleMu.Unlock()
	if c.idleStop != nil {
		close(c.idleStop)
		c.idleStop = nil
	}
}

// extractEnvelopeAddrs converts an imap envelope address list to ContactAddr slice.
// Entries with empty MailboxName or HostName are skipped.
func extractEnvelopeAddrs(addrs []*imap.Address) []models.ContactAddr {
	out := make([]models.ContactAddr, 0, len(addrs))
	for _, addr := range addrs {
		if addr == nil || addr.MailboxName == "" || addr.HostName == "" {
			continue
		}
		out = append(out, models.ContactAddr{
			Email: addr.MailboxName + "@" + addr.HostName,
			Name:  addr.PersonalName,
		})
	}
	return out
}

// CreateMailbox creates a new IMAP mailbox with the given name.
func (c *Client) CreateMailbox(name string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.client == nil {
		return fmt.Errorf("not connected")
	}
	if err := c.client.Create(name); err != nil {
		return fmt.Errorf("create mailbox %q: %w", name, err)
	}
	return nil
}

// RenameMailbox renames an existing IMAP mailbox.
func (c *Client) RenameMailbox(existingName, newName string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.client == nil {
		return fmt.Errorf("not connected")
	}
	if err := c.client.Rename(existingName, newName); err != nil {
		return fmt.Errorf("rename mailbox %q to %q: %w", existingName, newName, err)
	}
	return nil
}

// DeleteMailbox deletes an IMAP mailbox permanently.
func (c *Client) DeleteMailbox(name string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.client == nil {
		return fmt.Errorf("not connected")
	}
	if err := c.client.Delete(name); err != nil {
		return fmt.Errorf("delete mailbox %q: %w", name, err)
	}
	return nil
}

// checkBodyStructureForAttachments recursively checks if a body structure contains attachments
func checkBodyStructureForAttachments(bs *imap.BodyStructure) bool {
	if bs == nil {
		return false
	}

	// Check if this part is an attachment
	if bs.Disposition == "attachment" {
		return true
	}

	// Check nested parts for multipart messages
	for _, part := range bs.Parts {
		if checkBodyStructureForAttachments(part) {
			return true
		}
	}

	return false
}
