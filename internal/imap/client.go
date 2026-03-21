package imap

import (
	"crypto/tls"
	"fmt"
	"io"
	"log"
	"net/mail"
	"strings"
	"time"

	"github.com/emersion/go-imap"
	"github.com/emersion/go-imap/client"
	"mail-processor/internal/cache"
	"mail-processor/internal/config"
	"mail-processor/internal/logger"
	"mail-processor/internal/models"
)

var discardLogger = log.New(io.Discard, "", 0)

// Client wraps the IMAP client with business logic
type Client struct {
	cfg        *config.Config
	client     *client.Client
	cache      *cache.Cache
	groupByDomain bool
	progressCh chan models.ProgressInfo
}

// New creates a new IMAP client
func New(cfg *config.Config, cache *cache.Cache, progressCh chan models.ProgressInfo) *Client {
	return &Client{
		cfg:        cfg,
		cache:      cache,
		progressCh: progressCh,
	}
}

// Connect establishes connection to IMAP server
func (c *Client) Connect() error {
	// Connect to IMAP server
	addr := fmt.Sprintf("%s:%d", c.cfg.Server.Host, c.cfg.Server.Port)

	var err error
	c.client, err = client.Dial(addr)
	if err != nil {
		return fmt.Errorf("failed to connect to IMAP server: %w", err)
	}

	// Suppress IMAP protocol errors that are logged to stderr
	// These are parsing errors on malformed emails that we handle gracefully
	c.client.ErrorLog = discardLogger

	// Create TLS config for localhost/ProtonMail Bridge
	tlsConfig := &tls.Config{
		InsecureSkipVerify: true, // Required for localhost connections
		ServerName:         c.cfg.Server.Host,
	}

	// Start TLS
	if err := c.client.StartTLS(tlsConfig); err != nil {
		return fmt.Errorf("failed to start TLS: %w", err)
	}

	// Authenticate
	logger.Debug("Attempting to authenticate with username: %s", c.cfg.Credentials.Username)
	if err := c.client.Login(c.cfg.Credentials.Username, c.cfg.Credentials.Password); err != nil {
		logger.Error("Authentication failed: %v", err)
		return fmt.Errorf("failed to authenticate: %w", err)
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

// ListFolders returns all mailbox names from the server
func (c *Client) ListFolders() ([]string, error) {
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

// ProcessEmails reads and processes all emails from specified folder
func (c *Client) ProcessEmails(folder string) error {
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
		c.sendProgress(models.ProgressInfo{
			Phase:   "complete",
			Message: "No new emails to process",
		})
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
	items := []imap.FetchItem{imap.FetchEnvelope, imap.FetchUid, imap.FetchRFC822Size, imap.FetchBodyStructure}

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

	// Extract email data from Envelope
	emailData := &models.EmailData{
		MessageID:      messageID,
		Sender:         sender,
		Subject:        msg.Envelope.Subject,
		Date:           msg.Envelope.Date,
		Size:           int(msg.Size),
		HasAttachments: hasAttach,
		Folder:         folder,
	}

	// Cache the email
	return c.cache.CacheEmail(emailData)
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