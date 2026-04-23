package backend

import (
	"sort"
	"strings"
	"sync"
	"time"

	"mail-processor/internal/models"
)

// DemoBackendMarker is detected by the app to show [DEMO] in the status bar.
type DemoBackendMarker interface {
	IsDemo() bool
}

// DemoBackend implements Backend with synthetic in-memory data.
// No IMAP connection or SQLite database is required.
type DemoBackend struct {
	mu                  sync.Mutex
	emails              []*models.EmailData
	rules               []*models.Rule
	prompts             []*models.CustomPrompt
	classifications     map[string]string
	savedSearches       []*models.SavedSearch
	deletedIDs          map[string]bool
	progressCh          chan models.ProgressInfo
	syncEventsCh        chan models.FolderSyncEvent
	newEmailsCh         chan models.NewEmailsNotification
	validIDsCh          chan map[string]bool
	groupByDomain       bool
	bodyCache           map[string]string // messageID → cached body text
	nextSearchID        int
	nextRuleID          int64
	nextPromptID        int64
	unsubscribedSenders map[string]bool
	drafts              []*models.Draft
	nextDraftUID        uint32
}

// compile-time check that DemoBackend satisfies the Backend interface.
var _ Backend = (*DemoBackend)(nil)

// NewDemoBackend creates a DemoBackend pre-loaded with synthetic email data.
func NewDemoBackend() *DemoBackend {
	d := &DemoBackend{
		emails:              seedDemoEmails(),
		classifications:     make(map[string]string),
		deletedIDs:          make(map[string]bool),
		bodyCache:           make(map[string]string),
		unsubscribedSenders: make(map[string]bool),
		progressCh:          make(chan models.ProgressInfo, 100),
		syncEventsCh:        make(chan models.FolderSyncEvent, 100),
		newEmailsCh:         make(chan models.NewEmailsNotification, 10),
		validIDsCh:          make(chan map[string]bool, 1),
		nextSearchID:        1,
		nextRuleID:          1,
		nextPromptID:        1,
	}
	return d
}

// IsDemo satisfies DemoBackendMarker.
func (d *DemoBackend) IsDemo() bool { return true }

// --- Backend interface ---

// Load sends fake progress events and then signals completion.
func (d *DemoBackend) Load(folder string) {
	go func() {
		generation := time.Now().UnixNano()
		d.syncEventsCh <- models.FolderSyncEvent{
			Folder:     folder,
			Generation: generation,
			Phase:      models.SyncPhaseSyncStarted,
			Message:    "Connecting to demo backend…",
		}
		d.syncEventsCh <- models.FolderSyncEvent{
			Folder:     folder,
			Generation: generation,
			Phase:      models.SyncPhaseSnapshotReady,
			Message:    "Rendering demo data…",
		}
		phases := []models.ProgressInfo{
			{Phase: "connecting", Message: "Connecting to demo backend…"},
			{Phase: "scanning", Message: "Scanning demo emails…", Current: 25, Total: 50},
			{Phase: "scanning", Message: "Scanning demo emails…", Current: 50, Total: 50},
			{Phase: "complete", Message: "Demo data loaded", ProcessedEmails: len(d.emails), NewEmails: len(d.emails)},
		}
		for _, p := range phases {
			time.Sleep(50 * time.Millisecond)
			d.progressCh <- p
			eventPhase := models.SyncPhaseSyncStarted
			eventCount := 0
			if p.Current > 0 {
				eventPhase = models.SyncPhaseRowsCached
				eventCount = p.Current
			}
			if p.Phase == "complete" {
				eventPhase = models.SyncPhaseComplete
				eventCount = 1
			}
			d.syncEventsCh <- models.FolderSyncEvent{
				Folder:     folder,
				Generation: generation,
				Phase:      eventPhase,
				Message:    p.Message,
				Current:    p.Current,
				Total:      p.Total,
				EventCount: eventCount,
			}
		}
	}()
}

// Progress returns the read-only progress channel.
func (d *DemoBackend) Progress() <-chan models.ProgressInfo {
	return d.progressCh
}

func (d *DemoBackend) SyncEvents() <-chan models.FolderSyncEvent {
	return d.syncEventsCh
}

// GetSenderStatistics computes stats from the in-memory email slice.
func (d *DemoBackend) GetSenderStatistics(folder string) (map[string]*models.SenderStats, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	stats := make(map[string]*models.SenderStats)
	for _, e := range d.emails {
		if d.deletedIDs[e.MessageID] {
			continue
		}
		if folder != "" && folder != "INBOX" && e.Folder != folder {
			continue
		}
		key := e.Sender
		if d.groupByDomain {
			key = extractDemoEmailDomain(e.Sender)
		}
		s, ok := stats[key]
		if !ok {
			s = &models.SenderStats{
				FirstEmail: e.Date,
				LastEmail:  e.Date,
			}
			stats[key] = s
		}
		s.TotalEmails++
		s.AvgSize = (s.AvgSize*float64(s.TotalEmails-1) + float64(e.Size)) / float64(s.TotalEmails)
		if e.HasAttachments {
			s.WithAttachments++
		}
		if e.Date.Before(s.FirstEmail) {
			s.FirstEmail = e.Date
		}
		if e.Date.After(s.LastEmail) {
			s.LastEmail = e.Date
		}
	}
	return stats, nil
}

// GetEmailsBySender returns emails grouped by sender.
func (d *DemoBackend) GetEmailsBySender(folder string) (map[string][]*models.EmailData, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	result := make(map[string][]*models.EmailData)
	for _, e := range d.emails {
		if d.deletedIDs[e.MessageID] {
			continue
		}
		if folder != "" && folder != "INBOX" && e.Folder != folder {
			continue
		}
		key := e.Sender
		if d.groupByDomain {
			key = extractDemoEmailDomain(e.Sender)
		}
		result[key] = append(result[key], e)
	}
	return result, nil
}

// GetTimelineEmails returns all emails sorted by date descending.
func (d *DemoBackend) GetTimelineEmails(folder string) ([]*models.EmailData, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	var out []*models.EmailData
	for _, e := range d.emails {
		if d.deletedIDs[e.MessageID] {
			continue
		}
		if folder != "" && folder != "INBOX" && e.Folder != folder {
			continue
		}
		out = append(out, e)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Date.After(out[j].Date)
	})
	return out, nil
}

// GetAllMailOnlyView returns an unsupported diagnostic result in demo mode.
func (d *DemoBackend) GetAllMailOnlyView() (*models.VirtualFolderResult, error) {
	return &models.VirtualFolderResult{
		Name:      "All Mail only",
		Supported: false,
		Reason:    "Demo mode does not expose All Mail only diagnostics",
		Emails:    []*models.EmailData{},
	}, nil
}

// GetEmailByID returns a single email by message ID.
func (d *DemoBackend) GetEmailByID(messageID string) (*models.EmailData, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	for _, e := range d.emails {
		if e.MessageID == messageID {
			return e, nil
		}
	}
	return nil, nil
}

// FetchEmailBody returns a synthetic email body.
func (d *DemoBackend) FetchEmailBody(folder string, uid uint32) (*models.EmailBody, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	// Find the email for context
	var subject, sender string
	for _, e := range d.emails {
		if e.UID == uid {
			subject = e.Subject
			sender = e.Sender
			break
		}
	}

	body := "## " + subject + "\n\n"
	body += "**From:** " + sender + "\n\n"
	body += "Lorem ipsum dolor sit amet, consectetur adipiscing elit. " +
		"Sed do eiusmod tempor incididunt ut labore et dolore magna aliqua. " +
		"Ut enim ad minim veniam, quis nostrud exercitation ullamco laboris.\n\n" +
		"Duis aute irure dolor in reprehenderit in voluptate velit esse cillum " +
		"dolore eu fugiat nulla pariatur. Excepteur sint occaecat cupidatat non " +
		"proident, sunt in culpa qui officia deserunt mollit anim id est laborum.\n\n" +
		"This is a **demo email** generated by Herald's demo mode. " +
		"No real IMAP connection is required.\n"

	return &models.EmailBody{
		TextPlain: body,
	}, nil
}

// SaveAttachment is a no-op in demo mode.
func (d *DemoBackend) SaveAttachment(attachment *models.Attachment, destPath string) error {
	return nil
}

// DeleteSenderEmails removes all emails from a sender.
func (d *DemoBackend) DeleteSenderEmails(sender, folder string) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	for _, e := range d.emails {
		if e.Sender == sender && (folder == "" || e.Folder == folder) {
			d.deletedIDs[e.MessageID] = true
		}
	}
	return nil
}

// DeleteDomainEmails removes all emails from a domain.
func (d *DemoBackend) DeleteDomainEmails(domain, folder string) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	for _, e := range d.emails {
		if extractDemoEmailDomain(e.Sender) == domain && (folder == "" || e.Folder == folder) {
			d.deletedIDs[e.MessageID] = true
		}
	}
	return nil
}

// DeleteEmail removes a single email.
func (d *DemoBackend) DeleteEmail(messageID, folder string) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.deletedIDs[messageID] = true
	return nil
}

// ArchiveEmail marks a single email as archived (removed from active list).
func (d *DemoBackend) ArchiveEmail(messageID, folder string) error {
	return d.DeleteEmail(messageID, folder)
}

// ArchiveSenderEmails archives all emails from a sender.
func (d *DemoBackend) ArchiveSenderEmails(sender, folder string) error {
	return d.DeleteSenderEmails(sender, folder)
}

// MoveEmail marks the email deleted (demo: no real folder move).
func (d *DemoBackend) MoveEmail(messageID, fromFolder, toFolder string) error {
	return d.DeleteEmail(messageID, fromFolder)
}

// ListFolders returns a fixed folder list derived from demo emails.
func (d *DemoBackend) ListFolders() ([]string, error) {
	seen := make(map[string]bool)
	var folders []string
	for _, e := range d.emails {
		if !seen[e.Folder] {
			seen[e.Folder] = true
			folders = append(folders, e.Folder)
		}
	}
	sort.Strings(folders)
	return folders, nil
}

// GetFolderStatus returns synthetic message counts per folder.
func (d *DemoBackend) GetFolderStatus(folders []string) (map[string]models.FolderStatus, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	result := make(map[string]models.FolderStatus)
	for _, f := range folders {
		var total, unseen int
		for _, e := range d.emails {
			if d.deletedIDs[e.MessageID] {
				continue
			}
			if e.Folder == f {
				total++
				if !e.IsRead {
					unseen++
				}
			}
		}
		result[f] = models.FolderStatus{Total: total, Unseen: unseen}
	}
	return result, nil
}

// GetClassifications returns the in-memory classification map.
func (d *DemoBackend) GetClassifications(folder string) (map[string]string, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	out := make(map[string]string)
	for k, v := range d.classifications {
		out[k] = v
	}
	return out, nil
}

// SetClassification stores a classification for a message.
func (d *DemoBackend) SetClassification(messageID, category string) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.classifications[messageID] = category
	return nil
}

// GetUnclassifiedIDs returns message IDs without a classification.
func (d *DemoBackend) GetUnclassifiedIDs(folder string) ([]string, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	var ids []string
	for _, e := range d.emails {
		if d.deletedIDs[e.MessageID] {
			continue
		}
		if folder != "" && folder != "INBOX" && e.Folder != folder {
			continue
		}
		if _, classified := d.classifications[e.MessageID]; !classified {
			ids = append(ids, e.MessageID)
		}
	}
	return ids, nil
}

// SetGroupByDomain toggles domain-level grouping.
func (d *DemoBackend) SetGroupByDomain(v bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.groupByDomain = v
}

// Close is a no-op for the demo backend.
func (d *DemoBackend) Close() error {
	return nil
}

// --- Search ---

func (d *DemoBackend) SearchEmails(folder, query string, bodySearch bool) ([]*models.EmailData, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	q := strings.ToLower(query)
	var out []*models.EmailData
	for _, e := range d.emails {
		if d.deletedIDs[e.MessageID] {
			continue
		}
		if folder != "" && folder != "INBOX" && e.Folder != folder {
			continue
		}
		if strings.Contains(strings.ToLower(e.Subject), q) ||
			strings.Contains(strings.ToLower(e.Sender), q) {
			out = append(out, e)
		}
	}
	return out, nil
}

func (d *DemoBackend) SearchEmailsCrossFolder(query string) ([]*models.EmailData, error) {
	return d.SearchEmails("", query, false)
}

func (d *DemoBackend) SearchEmailsIMAP(folder, query string) ([]*models.EmailData, error) {
	return d.SearchEmails(folder, query, false)
}

func (d *DemoBackend) SearchEmailsSemantic(folder, query string, limit int, minScore float64) ([]*models.EmailData, error) {
	return d.SearchEmails(folder, query, false)
}

// --- Saved Searches ---

func (d *DemoBackend) GetSavedSearches() ([]*models.SavedSearch, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.savedSearches, nil
}

func (d *DemoBackend) SaveSearch(name, query, folder string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.savedSearches = append(d.savedSearches, &models.SavedSearch{
		ID:        d.nextSearchID,
		Name:      name,
		Query:     query,
		Folder:    folder,
		CreatedAt: time.Now(),
	})
	d.nextSearchID++
	return nil
}

func (d *DemoBackend) DeleteSavedSearch(id int) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	for i, ss := range d.savedSearches {
		if ss.ID == id {
			d.savedSearches = append(d.savedSearches[:i], d.savedSearches[i+1:]...)
			return nil
		}
	}
	return nil
}

// --- Read/Unread ---

func (d *DemoBackend) MarkRead(messageID, folder string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	for _, e := range d.emails {
		if e.MessageID == messageID {
			e.IsRead = true
			return nil
		}
	}
	return nil
}

func (d *DemoBackend) MarkUnread(messageID, folder string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	for _, e := range d.emails {
		if e.MessageID == messageID {
			e.IsRead = false
			return nil
		}
	}
	return nil
}

func (d *DemoBackend) MarkStarred(messageID, folder string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	for _, e := range d.emails {
		if e.MessageID == messageID {
			e.IsStarred = true
			return nil
		}
	}
	return nil
}

func (d *DemoBackend) UnmarkStarred(messageID, folder string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	for _, e := range d.emails {
		if e.MessageID == messageID {
			e.IsStarred = false
			return nil
		}
	}
	return nil
}

func (d *DemoBackend) GetEmailsByThread(folder, subject string) ([]*models.EmailData, error) {
	return nil, nil
}

func (d *DemoBackend) SendEmail(to, subject, body, from string) error {
	return nil
}

func (d *DemoBackend) UpdateUnsubscribeHeaders(messageID, listUnsub, listUnsubPost string) error {
	return nil
}

// --- Body text caching ---

func (d *DemoBackend) CacheBodyText(messageID, bodyText string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.bodyCache[messageID] = bodyText
	return nil
}

// --- Embeddings ---

func (d *DemoBackend) StoreEmbedding(messageID string, embedding []float32, hash string) error {
	return nil
}

func (d *DemoBackend) GetUnembeddedIDs(folder string) ([]string, error) {
	return nil, nil
}

func (d *DemoBackend) GetUnembeddedIDsWithBody(folder string) ([]string, error) {
	return nil, nil
}

func (d *DemoBackend) GetUncachedBodyIDs(folder string, limit int) ([]string, error) {
	return nil, nil
}

func (d *DemoBackend) GetEmbeddingProgress(folder string) (done, total int, err error) {
	return 0, 0, nil
}

func (d *DemoBackend) StoreEmbeddingChunks(messageID string, chunks []models.EmbeddingChunk) error {
	return nil
}

func (d *DemoBackend) SearchSemanticChunked(folder string, queryVec []float32, limit int, minScore float64) ([]*models.SemanticSearchResult, error) {
	return nil, nil
}

func (d *DemoBackend) GetBodyText(messageID string) (string, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.bodyCache[messageID], nil
}

func (d *DemoBackend) FetchAndCacheBody(messageID string) (*models.EmailBody, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	for _, e := range d.emails {
		if e.MessageID == messageID {
			body := "Demo body for: " + e.Subject
			d.bodyCache[messageID] = body
			return &models.EmailBody{TextPlain: body}, nil
		}
	}
	return nil, nil
}

// --- Background sync ---

func (d *DemoBackend) NewEmailsCh() <-chan models.NewEmailsNotification {
	return d.newEmailsCh
}

func (d *DemoBackend) StartIDLE(folder string) error            { return nil }
func (d *DemoBackend) StopIDLE()                                {}
func (d *DemoBackend) StartPolling(folder string, interval int) {}
func (d *DemoBackend) StopPolling()                             {}

func (d *DemoBackend) ValidIDsCh() <-chan map[string]bool {
	return d.validIDsCh
}

// --- Rules ---

func (d *DemoBackend) SaveRule(r *models.Rule) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if r.ID == 0 {
		r.ID = d.nextRuleID
		d.nextRuleID++
		r.CreatedAt = time.Now()
		d.rules = append(d.rules, r)
	} else {
		for i, existing := range d.rules {
			if existing.ID == r.ID {
				d.rules[i] = r
				return nil
			}
		}
		d.rules = append(d.rules, r)
	}
	return nil
}

func (d *DemoBackend) GetAllRules() ([]*models.Rule, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	return append([]*models.Rule(nil), d.rules...), nil
}

func (d *DemoBackend) GetEnabledRules() ([]*models.Rule, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	var out []*models.Rule
	for _, r := range d.rules {
		if r.Enabled {
			out = append(out, r)
		}
	}
	return out, nil
}

func (d *DemoBackend) DeleteRule(id int64) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	for i, r := range d.rules {
		if r.ID == id {
			d.rules = append(d.rules[:i], d.rules[i+1:]...)
			return nil
		}
	}
	return nil
}

// --- Custom Prompts ---

func (d *DemoBackend) GetAllCustomPrompts() ([]*models.CustomPrompt, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.prompts, nil
}

func (d *DemoBackend) SaveCustomPrompt(p *models.CustomPrompt) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if p.ID == 0 {
		p.ID = d.nextPromptID
		d.nextPromptID++
		p.CreatedAt = time.Now()
		d.prompts = append(d.prompts, p)
	} else {
		for i, existing := range d.prompts {
			if existing.ID == p.ID {
				d.prompts[i] = p
				return nil
			}
		}
		d.prompts = append(d.prompts, p)
	}
	return nil
}

func (d *DemoBackend) GetCustomPrompt(id int64) (*models.CustomPrompt, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	for _, p := range d.prompts {
		if p.ID == id {
			return p, nil
		}
	}
	return nil, nil
}

func (d *DemoBackend) AppendActionLog(entry *models.RuleActionLogEntry) error {
	return nil
}

func (d *DemoBackend) TouchRuleLastTriggered(ruleID int64) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	now := time.Now()
	for _, r := range d.rules {
		if r.ID == ruleID {
			r.LastTriggered = &now
			return nil
		}
	}
	return nil
}

func (d *DemoBackend) SaveCustomCategory(messageID string, promptID int64, result string) error {
	return nil
}

// --- Contacts (stubs for DemoBackend) ---

func (d *DemoBackend) GetContactsToEnrich(minCount, limit int) ([]models.ContactData, error) {
	return nil, nil
}

func (d *DemoBackend) GetRecentSubjectsByContact(email string, limit int) ([]string, error) {
	return nil, nil
}

func (d *DemoBackend) UpdateContactEnrichment(email, company string, topics []string) error {
	return nil
}

func (d *DemoBackend) UpdateContactEmbedding(email string, embedding []float32) error {
	return nil
}

func (d *DemoBackend) SearchContactsSemantic(queryVec []float32, limit int, minScore float64) ([]*models.ContactSearchResult, error) {
	return nil, nil
}

func (d *DemoBackend) ListContacts(limit int, sortBy string) ([]models.ContactData, error) {
	contacts := seedDemoContacts()
	if limit > 0 && len(contacts) > limit {
		contacts = contacts[:limit]
	}
	return contacts, nil
}

func (d *DemoBackend) SearchContacts(query string) ([]models.ContactData, error) {
	q := strings.ToLower(query)
	all := seedDemoContacts()
	var out []models.ContactData
	for _, c := range all {
		if strings.Contains(strings.ToLower(c.DisplayName), q) ||
			strings.Contains(strings.ToLower(c.Email), q) ||
			strings.Contains(strings.ToLower(c.Company), q) {
			out = append(out, c)
		}
	}
	return out, nil
}

func (d *DemoBackend) UpsertContacts(addrs []models.ContactAddr, direction string) error {
	return nil
}

// --- Cleanup rules ---

func (d *DemoBackend) GetAllCleanupRules() ([]*models.CleanupRule, error) {
	return nil, nil
}

func (d *DemoBackend) SaveCleanupRule(rule *models.CleanupRule) error {
	return nil
}

func (d *DemoBackend) DeleteCleanupRule(id int64) error {
	return nil
}

func (d *DemoBackend) GetContactEmails(contactEmail string, limit int) ([]*models.EmailData, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	q := strings.ToLower(contactEmail)
	var out []*models.EmailData
	for _, e := range d.emails {
		if strings.Contains(strings.ToLower(e.Sender), q) {
			out = append(out, e)
			if limit > 0 && len(out) >= limit {
				break
			}
		}
	}
	return out, nil
}

// --- Unsubscribed senders ---

func (d *DemoBackend) RecordUnsubscribe(sender, method, url string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.unsubscribedSenders[sender] = true
	return nil
}

func (d *DemoBackend) IsUnsubscribedSender(sender string) (bool, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.unsubscribedSenders[sender], nil
}

// --- Reply / Forward / Attachments ---

func (d *DemoBackend) ReplyToEmail(_, _ string) error                        { return nil }
func (d *DemoBackend) ForwardEmail(_, _, _ string) error                     { return nil }
func (d *DemoBackend) ListAttachments(_ string) ([]models.Attachment, error) { return nil, nil }
func (d *DemoBackend) GetAttachment(_, _ string) (*models.Attachment, error) { return nil, nil }

// --- Drafts ---

func (d *DemoBackend) SaveDraft(to, cc, bcc, subject, body string) (uint32, string, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.nextDraftUID++
	draft := &models.Draft{
		UID:     d.nextDraftUID,
		Folder:  "Drafts",
		To:      to,
		CC:      cc,
		BCC:     bcc,
		Subject: subject,
		Body:    body,
		Date:    time.Now(),
	}
	d.drafts = append(d.drafts, draft)
	return draft.UID, draft.Folder, nil
}

func (d *DemoBackend) ListDrafts() ([]*models.Draft, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	out := make([]*models.Draft, len(d.drafts))
	copy(out, d.drafts)
	return out, nil
}

func (d *DemoBackend) DeleteDraft(uid uint32, folder string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	for i, dr := range d.drafts {
		if dr.UID == uid {
			d.drafts = append(d.drafts[:i], d.drafts[i+1:]...)
			return nil
		}
	}
	return nil // not found is not an error
}

// --- Bulk operations ---

func (d *DemoBackend) DeleteThread(folder, subject string) error           { return nil }
func (d *DemoBackend) BulkDelete(messageIDs []string) error                { return nil }
func (d *DemoBackend) ArchiveThread(folder, subject string) error          { return nil }
func (d *DemoBackend) BulkMove(messageIDs []string, toFolder string) error { return nil }
func (d *DemoBackend) UnsubscribeSender(messageID string) error            { return nil }
func (d *DemoBackend) SoftUnsubscribeSender(sender, toFolder string) error { return nil }

// --- Folder management ---

func (d *DemoBackend) CreateFolder(_ string) error                            { return nil }
func (d *DemoBackend) RenameFolder(_, _ string) error                         { return nil }
func (d *DemoBackend) DeleteFolder(_ string) error                            { return nil }
func (d *DemoBackend) SyncAllFolders() (int, error)                           { return 0, nil }
func (d *DemoBackend) GetSyncStatus() (map[string]models.FolderStatus, error) { return nil, nil }

// extractDemoEmailDomain extracts the domain part from a sender string like "Name <addr@domain.com>".
func extractDemoEmailDomain(sender string) string {
	addr := sender
	if lt := strings.LastIndex(sender, "<"); lt >= 0 {
		addr = sender[lt+1:]
		if gt := strings.Index(addr, ">"); gt >= 0 {
			addr = addr[:gt]
		}
	}
	if at := strings.LastIndex(addr, "@"); at >= 0 {
		return addr[at+1:]
	}
	return addr
}
