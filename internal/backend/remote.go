package backend

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/herald-email/herald-mail-app/internal/models"
)

// RemoteBackend implements Backend by calling the Herald daemon over HTTP.
type RemoteBackend struct {
	baseURL      string
	httpClient   *http.Client
	sseClient    *http.Client
	progressCh   chan models.ProgressInfo
	syncEventsCh chan models.FolderSyncEvent
	newEmailsCh  chan models.NewEmailsNotification
	validIDsCh   chan map[string]bool
	sseCancel    context.CancelFunc
	closeOnce    sync.Once
	wg           sync.WaitGroup
}

// Compile-time check that RemoteBackend implements Backend.
var _ Backend = (*RemoteBackend)(nil)

// NewRemote creates a RemoteBackend connected to baseURL (e.g. "http://127.0.0.1:7272").
func NewRemote(baseURL string) (*RemoteBackend, error) {
	b := &RemoteBackend{
		baseURL:      strings.TrimRight(baseURL, "/"),
		httpClient:   &http.Client{Timeout: 30 * time.Second},
		sseClient:    &http.Client{},
		progressCh:   make(chan models.ProgressInfo, 100),
		syncEventsCh: make(chan models.FolderSyncEvent, 100),
		newEmailsCh:  make(chan models.NewEmailsNotification, 10),
		validIDsCh:   make(chan map[string]bool, 1),
	}
	ctx, cancel := context.WithCancel(context.Background())
	b.sseCancel = cancel
	b.wg.Add(1)
	go func() {
		defer b.wg.Done()
		b.subscribeSSE(ctx)
	}()
	return b, nil
}

// subscribeSSE reconnects to the SSE stream with exponential backoff.
func (b *RemoteBackend) subscribeSSE(ctx context.Context) {
	backoff := time.Second
	for {
		if ctx.Err() != nil {
			return
		}
		if err := b.readSSEStream(ctx); err != nil && ctx.Err() == nil {
			select {
			case <-time.After(backoff):
			case <-ctx.Done():
				return
			}
			backoff = time.Duration(math.Min(float64(backoff*2), float64(30*time.Second)))
		} else {
			backoff = time.Second // reset on clean reconnect
		}
	}
}

func (b *RemoteBackend) readSSEStream(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, "GET", b.baseURL+"/v1/events", nil)
	if err != nil {
		return err
	}
	resp, err := b.sseClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	scanner := bufio.NewScanner(resp.Body)
	var eventType, dataLine string
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "event: ") {
			eventType = strings.TrimPrefix(line, "event: ")
		} else if strings.HasPrefix(line, "data: ") {
			dataLine = strings.TrimPrefix(line, "data: ")
		} else if line == "" && eventType != "" {
			b.handleSSEEvent(eventType, []byte(dataLine))
			eventType, dataLine = "", ""
		}
	}
	return scanner.Err()
}

func (b *RemoteBackend) handleSSEEvent(eventType string, data []byte) {
	switch eventType {
	case "progress":
		var p models.ProgressInfo
		if json.Unmarshal(data, &p) == nil {
			select {
			case b.progressCh <- p:
			default:
			}
			event := models.FolderSyncEvent{
				Folder:  "",
				Message: p.Message,
				Current: p.Current,
				Total:   p.Total,
				Phase:   models.SyncPhaseSyncStarted,
			}
			if p.Phase == "fetching" {
				event.Phase = models.SyncPhaseRowsCached
				event.EventCount = 1
			}
			if p.Phase == "complete" {
				event.Phase = models.SyncPhaseComplete
			}
			select {
			case b.syncEventsCh <- event:
			default:
			}
		}
	case "new_emails":
		var n models.NewEmailsNotification
		if json.Unmarshal(data, &n) == nil {
			select {
			case b.newEmailsCh <- n:
			default:
			}
		}
	case "valid_ids":
		var m map[string]bool
		if json.Unmarshal(data, &m) == nil {
			select {
			case b.validIDsCh <- m:
			default:
			}
		}
	}
}

// ── HTTP helpers ─────────────────────────────────────────────────────────────

func (b *RemoteBackend) get(path string, out any) error {
	resp, err := b.httpClient.Get(b.baseURL + path)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		var e struct {
			Error string `json:"error"`
		}
		json.NewDecoder(resp.Body).Decode(&e) //nolint:errcheck
		return fmt.Errorf("daemon: %s", e.Error)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func (b *RemoteBackend) post(path string, body any) error {
	var buf bytes.Buffer
	if body != nil {
		json.NewEncoder(&buf).Encode(body) //nolint:errcheck
	}
	resp, err := b.httpClient.Post(b.baseURL+path, "application/json", &buf)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		var e struct {
			Error string `json:"error"`
		}
		json.NewDecoder(resp.Body).Decode(&e) //nolint:errcheck
		return fmt.Errorf("daemon: %s", e.Error)
	}
	io.Copy(io.Discard, resp.Body) //nolint:errcheck
	return nil
}

func (b *RemoteBackend) postOut(path string, body any, out any) error {
	var buf bytes.Buffer
	if body != nil {
		json.NewEncoder(&buf).Encode(body) //nolint:errcheck
	}
	resp, err := b.httpClient.Post(b.baseURL+path, "application/json", &buf)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		var e struct {
			Error string `json:"error"`
		}
		json.NewDecoder(resp.Body).Decode(&e) //nolint:errcheck
		return fmt.Errorf("daemon: %s", e.Error)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func (b *RemoteBackend) delete(path string) error {
	req, _ := http.NewRequest("DELETE", b.baseURL+path, nil)
	resp, err := b.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 && resp.StatusCode != 404 {
		var e struct {
			Error string `json:"error"`
		}
		json.NewDecoder(resp.Body).Decode(&e) //nolint:errcheck
		return fmt.Errorf("daemon: %s", e.Error)
	}
	io.Copy(io.Discard, resp.Body) //nolint:errcheck
	return nil
}

// ── Channel accessors ─────────────────────────────────────────────────────────

func (b *RemoteBackend) Progress() <-chan models.ProgressInfo {
	return b.progressCh
}

func (b *RemoteBackend) SyncEvents() <-chan models.FolderSyncEvent {
	return b.syncEventsCh
}

func (b *RemoteBackend) NewEmailsCh() <-chan models.NewEmailsNotification {
	return b.newEmailsCh
}

func (b *RemoteBackend) ValidIDsCh() <-chan map[string]bool {
	return b.validIDsCh
}

// GetAllMailOnlyView is not yet exposed by the daemon-backed remote backend.
func (b *RemoteBackend) GetAllMailOnlyView() (*models.VirtualFolderResult, error) {
	return &models.VirtualFolderResult{
		Name:      "All Mail only",
		Supported: false,
		Reason:    "All Mail only inspector is not supported over the daemon yet",
		Emails:    []*models.EmailData{},
	}, nil
}

// ── Lifecycle ─────────────────────────────────────────────────────────────────

// Close cancels the SSE goroutine and drains/closes all channels.
func (b *RemoteBackend) Close() error {
	b.closeOnce.Do(func() {
		b.sseCancel()
		b.wg.Wait()
		close(b.progressCh)
		close(b.syncEventsCh)
		close(b.newEmailsCh)
		close(b.validIDsCh)
	})
	return nil
}

// ── Sync ──────────────────────────────────────────────────────────────────────

// Load triggers background email synchronisation on the daemon for the given folder.
func (b *RemoteBackend) Load(folder string) {
	b.post("/v1/sync", map[string]string{"folder": folder}) //nolint:errcheck
}

// ── Folders ───────────────────────────────────────────────────────────────────

func (b *RemoteBackend) ListFolders() ([]string, error) {
	var folders []string
	return folders, b.get("/v1/folders", &folders)
}

// GetFolderStatus is not exposed by the daemon API; returns an empty map.
func (b *RemoteBackend) GetFolderStatus(folders []string) (map[string]models.FolderStatus, error) {
	return map[string]models.FolderStatus{}, nil
}

// ── Emails ────────────────────────────────────────────────────────────────────

func (b *RemoteBackend) GetTimelineEmails(folder string) ([]*models.EmailData, error) {
	var emails []*models.EmailData
	return emails, b.get("/v1/emails?folder="+url.QueryEscape(folder), &emails)
}

func (b *RemoteBackend) GetEmailByID(messageID string) (*models.EmailData, error) {
	var email models.EmailData
	return &email, b.get("/v1/emails/"+url.PathEscape(messageID), &email)
}

// FetchEmailBody fetches the full MIME body via the daemon.
// The daemon endpoint /v1/emails/{id}/body accepts a message ID, but the Backend
// interface signature takes (folder string, uid uint32). We look up the email by
// UID in the timeline to retrieve the message ID first.
//
// Note: because the daemon resolves the message ID to UID+folder internally, the
// folder and uid parameters here are best-effort — if the daemon cannot find the
// email the call will return a not-found error.
func (b *RemoteBackend) FetchEmailBody(folder string, uid uint32) (*models.EmailBody, error) {
	// Resolve UID to message ID by scanning the folder's timeline.
	emails, err := b.GetTimelineEmails(folder)
	if err != nil {
		return nil, fmt.Errorf("remote: FetchEmailBody: list emails: %w", err)
	}
	var messageID string
	for _, e := range emails {
		if e.UID == uid {
			messageID = e.MessageID
			break
		}
	}
	if messageID == "" {
		return nil, fmt.Errorf("remote: FetchEmailBody: uid %d not found in folder %s", uid, folder)
	}
	var body models.EmailBody
	return &body, b.get("/v1/emails/"+url.PathEscape(messageID)+"/body", &body)
}

// SaveAttachment is not exposed by the daemon API; returns an unsupported error.
func (b *RemoteBackend) SaveAttachment(attachment *models.Attachment, destPath string) error {
	return fmt.Errorf("remote: SaveAttachment: not supported over daemon API")
}

// ── Sender grouping ───────────────────────────────────────────────────────────

// SetGroupByDomain is handled server-side per-request; this is a no-op.
func (b *RemoteBackend) SetGroupByDomain(v bool) {}

func (b *RemoteBackend) GetSenderStatistics(folder string) (map[string]*models.SenderStats, error) {
	var stats map[string]*models.SenderStats
	return stats, b.get("/v1/stats?folder="+url.QueryEscape(folder), &stats)
}

// GetEmailsBySender is not exposed by the daemon API; returns an unsupported error.
func (b *RemoteBackend) GetEmailsBySender(folder string) (map[string][]*models.EmailData, error) {
	return nil, fmt.Errorf("remote: GetEmailsBySender: not supported over daemon API")
}

// ── Deletion ──────────────────────────────────────────────────────────────────

func (b *RemoteBackend) DeleteEmail(messageID, folder string) error {
	return b.delete("/v1/emails/" + url.PathEscape(messageID) + "?folder=" + url.QueryEscape(folder))
}

func (b *RemoteBackend) DeleteSenderEmails(sender, folder string) error {
	return b.delete("/v1/senders/" + url.PathEscape(sender) + "?folder=" + url.QueryEscape(folder))
}

// DeleteDomainEmails is not directly exposed; returns an unsupported error.
func (b *RemoteBackend) DeleteDomainEmails(domain, folder string) error {
	return fmt.Errorf("remote: DeleteDomainEmails: not supported over daemon API")
}

// ── Archive ───────────────────────────────────────────────────────────────────

func (b *RemoteBackend) ArchiveEmail(messageID, folder string) error {
	return b.post("/v1/emails/"+url.PathEscape(messageID)+"/archive?folder="+url.QueryEscape(folder), nil)
}

// ArchiveSenderEmails is not exposed by the daemon API; returns an unsupported error.
func (b *RemoteBackend) ArchiveSenderEmails(sender, folder string) error {
	return fmt.Errorf("remote: ArchiveSenderEmails: not supported over daemon API")
}

// ── Move ──────────────────────────────────────────────────────────────────────

func (b *RemoteBackend) MoveEmail(messageID, fromFolder, toFolder string) error {
	return b.post("/v1/emails/"+url.PathEscape(messageID)+"/move", map[string]string{
		"fromFolder": fromFolder,
		"toFolder":   toFolder,
	})
}

// ── Read/Unread ───────────────────────────────────────────────────────────────

func (b *RemoteBackend) MarkRead(messageID, folder string) error {
	return b.post("/v1/emails/"+url.PathEscape(messageID)+"/read?folder="+url.QueryEscape(folder), nil)
}

func (b *RemoteBackend) MarkUnread(messageID, folder string) error {
	return b.post("/v1/emails/"+url.PathEscape(messageID)+"/unread?folder="+url.QueryEscape(folder), nil)
}

func (b *RemoteBackend) MarkStarred(messageID, folder string) error {
	return b.post("/v1/emails/"+url.PathEscape(messageID)+"/star?folder="+url.QueryEscape(folder), nil)
}

func (b *RemoteBackend) UnmarkStarred(messageID, folder string) error {
	return b.delete("/v1/emails/" + url.PathEscape(messageID) + "/star?folder=" + url.QueryEscape(folder))
}

func (b *RemoteBackend) GetEmailsByThread(folder, subject string) ([]*models.EmailData, error) {
	var emails []*models.EmailData
	path := fmt.Sprintf("/v1/threads?folder=%s&subject=%s", url.QueryEscape(folder), url.QueryEscape(subject))
	return emails, b.get(path, &emails)
}

func (b *RemoteBackend) SendEmail(to, subject, body, from string) error {
	return b.post("/v1/emails/send", map[string]string{
		"to": to, "subject": subject, "body": body, "from": from,
	})
}

// UpdateUnsubscribeHeaders is not exposed by the daemon API; returns nil (best-effort).
func (b *RemoteBackend) UpdateUnsubscribeHeaders(messageID, listUnsub, listUnsubPost string) error {
	return nil
}

// ── Classifications ───────────────────────────────────────────────────────────

func (b *RemoteBackend) GetClassifications(folder string) (map[string]string, error) {
	var m map[string]string
	return m, b.get("/v1/classifications?folder="+url.QueryEscape(folder), &m)
}

func (b *RemoteBackend) SetClassification(messageID, category string) error {
	return b.post("/v1/emails/"+url.PathEscape(messageID)+"/classify", map[string]string{
		"category": category,
	})
}

// GetUnclassifiedIDs is not exposed by the daemon API; returns an empty slice.
func (b *RemoteBackend) GetUnclassifiedIDs(folder string) ([]string, error) {
	return []string{}, nil
}

// ── Search ────────────────────────────────────────────────────────────────────

func (b *RemoteBackend) SearchEmails(folder, query string, bodySearch bool) ([]*models.EmailData, error) {
	path := fmt.Sprintf("/v1/search?folder=%s&q=%s&body=%v",
		url.QueryEscape(folder), url.QueryEscape(query), bodySearch)
	var emails []*models.EmailData
	return emails, b.get(path, &emails)
}

func (b *RemoteBackend) SearchEmailsCrossFolder(query string) ([]*models.EmailData, error) {
	var emails []*models.EmailData
	return emails, b.get("/v1/search/all?q="+url.QueryEscape(query), &emails)
}

// SearchEmailsIMAP is not exposed by the daemon API; returns an unsupported error.
func (b *RemoteBackend) SearchEmailsIMAP(folder, query string) ([]*models.EmailData, error) {
	return nil, fmt.Errorf("remote: SearchEmailsIMAP: not supported over daemon API")
}

func (b *RemoteBackend) SearchEmailsSemantic(folder, query string, limit int, minScore float64) ([]*models.EmailData, error) {
	path := fmt.Sprintf("/v1/search/semantic?folder=%s&q=%s&limit=%d&min_score=%f",
		url.QueryEscape(folder), url.QueryEscape(query), limit, minScore)
	var emails []*models.EmailData
	return emails, b.get(path, &emails)
}

// ── Saved Searches ────────────────────────────────────────────────────────────

// GetSavedSearches is not exposed by the daemon API; returns an empty slice.
func (b *RemoteBackend) GetSavedSearches() ([]*models.SavedSearch, error) {
	return []*models.SavedSearch{}, nil
}

// SaveSearch is not exposed by the daemon API; returns nil (silently dropped).
func (b *RemoteBackend) SaveSearch(name, query, folder string) error {
	return nil
}

// DeleteSavedSearch is not exposed by the daemon API; returns nil (silently dropped).
func (b *RemoteBackend) DeleteSavedSearch(id int) error {
	return nil
}

// ── Body text / FTS caching ───────────────────────────────────────────────────

// CacheBodyText is not exposed by the daemon API; returns nil (silently dropped).
func (b *RemoteBackend) CacheBodyText(messageID, bodyText string) error {
	return nil
}

// ── Embeddings ────────────────────────────────────────────────────────────────

// StoreEmbedding is not exposed by the daemon API; returns nil (silently dropped).
func (b *RemoteBackend) StoreEmbedding(messageID string, embedding []float32, hash string) error {
	return nil
}

// GetUnembeddedIDs is not exposed by the daemon API; returns an empty slice.
func (b *RemoteBackend) GetUnembeddedIDs(folder string) ([]string, error) {
	return []string{}, nil
}

// GetUnembeddedIDsWithBody is not exposed by the daemon API; returns an empty slice.
func (b *RemoteBackend) GetUnembeddedIDsWithBody(folder string) ([]string, error) {
	return []string{}, nil
}

// GetUncachedBodyIDs is not exposed by the daemon API; returns an empty slice.
func (b *RemoteBackend) GetUncachedBodyIDs(folder string, limit int) ([]string, error) {
	return []string{}, nil
}

// GetEmbeddingProgress is not exposed by the daemon API; returns zeros.
func (b *RemoteBackend) GetEmbeddingProgress(folder string) (done, total int, err error) {
	return 0, 0, nil
}

// StoreEmbeddingChunks is not exposed by the daemon API; returns nil (silently dropped).
func (b *RemoteBackend) StoreEmbeddingChunks(messageID string, chunks []models.EmbeddingChunk) error {
	return nil
}

// SearchSemanticChunked is not exposed by the daemon API; returns an unsupported error.
func (b *RemoteBackend) SearchSemanticChunked(folder string, queryVec []float32, limit int, minScore float64) ([]*models.SemanticSearchResult, error) {
	return nil, fmt.Errorf("remote: SearchSemanticChunked: not supported over daemon API")
}

// GetBodyText is not exposed by the daemon API; returns an empty string.
func (b *RemoteBackend) GetBodyText(messageID string) (string, error) {
	return "", nil
}

// FetchAndCacheBody fetches the email body via the daemon for the given message ID.
func (b *RemoteBackend) FetchAndCacheBody(messageID string) (*models.EmailBody, error) {
	var body models.EmailBody
	return &body, b.get("/v1/emails/"+url.PathEscape(messageID)+"/body", &body)
}

// ── Background sync / IDLE / polling ─────────────────────────────────────────

// StartIDLE is a no-op; the daemon handles IDLE internally.
func (b *RemoteBackend) StartIDLE(folder string) error { return nil }

// StopIDLE is a no-op; the daemon handles IDLE internally.
func (b *RemoteBackend) StopIDLE() {}

// StartPolling is a no-op; the daemon handles polling internally.
func (b *RemoteBackend) StartPolling(folder string, interval int) {}

// StopPolling is a no-op; the daemon handles polling internally.
func (b *RemoteBackend) StopPolling() {}

// ── Rules ─────────────────────────────────────────────────────────────────────

func (b *RemoteBackend) GetEnabledRules() ([]*models.Rule, error) {
	var rules []*models.Rule
	return rules, b.get("/v1/rules", &rules)
}

func (b *RemoteBackend) SaveRule(r *models.Rule) error {
	return b.post("/v1/rules", r)
}

func (b *RemoteBackend) DeleteRule(id int64) error {
	return b.delete(fmt.Sprintf("/v1/rules/%d", id))
}

// GetCustomPrompt is not exposed by the daemon API; returns an unsupported error.
func (b *RemoteBackend) GetCustomPrompt(id int64) (*models.CustomPrompt, error) {
	return nil, fmt.Errorf("remote: GetCustomPrompt: not supported over daemon API")
}

// AppendActionLog is not exposed by the daemon API; returns nil (silently dropped).
func (b *RemoteBackend) AppendActionLog(entry *models.RuleActionLogEntry) error {
	return nil
}

// TouchRuleLastTriggered is not exposed by the daemon API; returns nil (silently dropped).
func (b *RemoteBackend) TouchRuleLastTriggered(ruleID int64) error {
	return nil
}

// SaveCustomCategory is not exposed by the daemon API; returns nil (silently dropped).
func (b *RemoteBackend) SaveCustomCategory(messageID string, promptID int64, result string) error {
	return nil
}

// ── Custom prompts ────────────────────────────────────────────────────────────

func (b *RemoteBackend) GetAllCustomPrompts() ([]*models.CustomPrompt, error) {
	var prompts []*models.CustomPrompt
	return prompts, b.get("/v1/prompts", &prompts)
}

func (b *RemoteBackend) SaveCustomPrompt(p *models.CustomPrompt) error {
	return b.post("/v1/prompts", p)
}

// --- Contacts (not yet exposed by daemon API; stubs) ---

func (b *RemoteBackend) GetContactsToEnrich(minCount, limit int) ([]models.ContactData, error) {
	return nil, nil
}

func (b *RemoteBackend) GetRecentSubjectsByContact(email string, limit int) ([]string, error) {
	return nil, nil
}

func (b *RemoteBackend) UpdateContactEnrichment(email, company string, topics []string) error {
	return nil
}

func (b *RemoteBackend) UpdateContactEmbedding(email string, embedding []float32) error {
	return nil
}

func (b *RemoteBackend) SearchContactsSemantic(queryVec []float32, limit int, minScore float64) ([]*models.ContactSearchResult, error) {
	return nil, nil
}

func (b *RemoteBackend) ListContacts(limit int, sortBy string) ([]models.ContactData, error) {
	return nil, nil
}

func (b *RemoteBackend) SearchContacts(query string) ([]models.ContactData, error) {
	return nil, nil
}

func (b *RemoteBackend) GetContactEmails(contactEmail string, limit int) ([]*models.EmailData, error) {
	return nil, nil
}

func (b *RemoteBackend) UpsertContacts(addrs []models.ContactAddr, direction string) error {
	return nil
}

// --- Folder management ---

func (b *RemoteBackend) CreateFolder(name string) error {
	return b.post("/v1/folders", map[string]string{"name": name})
}

func (b *RemoteBackend) RenameFolder(existingName, newName string) error {
	return b.post("/v1/folders/"+url.PathEscape(existingName)+"/rename", map[string]string{"new_name": newName})
}

func (b *RemoteBackend) DeleteFolder(name string) error {
	return b.delete("/v1/folders/" + url.PathEscape(name))
}

func (b *RemoteBackend) SyncAllFolders() (int, error) {
	type syncResp struct {
		NewEmails int `json:"new_emails"`
	}
	var resp syncResp
	if err := b.postOut("/v1/sync/all", nil, &resp); err != nil {
		return 0, err
	}
	return resp.NewEmails, nil
}

func (b *RemoteBackend) GetSyncStatus() (map[string]models.FolderStatus, error) {
	var status map[string]models.FolderStatus
	return status, b.get("/v1/sync/status", &status)
}

// --- Cleanup rules ---

func (b *RemoteBackend) GetAllCleanupRules() ([]*models.CleanupRule, error) {
	return nil, fmt.Errorf("not supported via remote backend")
}

func (b *RemoteBackend) SaveCleanupRule(rule *models.CleanupRule) error {
	return fmt.Errorf("not supported via remote backend")
}

func (b *RemoteBackend) DeleteCleanupRule(id int64) error {
	return fmt.Errorf("not supported via remote backend")
}

// --- Reply / Forward / Attachments ---

func (b *RemoteBackend) ReplyToEmail(messageID, replyBody string) error {
	return b.post("/v1/emails/"+url.PathEscape(messageID)+"/reply", map[string]string{"body": replyBody})
}

func (b *RemoteBackend) ReplyToEmailWithOptions(messageID string, opts models.ReplyEmailOptions) error {
	return b.post("/v1/emails/"+url.PathEscape(messageID)+"/reply", map[string]any{
		"body":              opts.Body,
		"preservation_mode": opts.PreservationMode,
	})
}

func (b *RemoteBackend) ForwardEmail(messageID, to, forwardBody string) error {
	return b.post("/v1/emails/"+url.PathEscape(messageID)+"/forward", map[string]string{"to": to, "body": forwardBody})
}

func (b *RemoteBackend) ForwardEmailWithOptions(messageID string, opts models.ForwardEmailOptions) error {
	return b.post("/v1/emails/"+url.PathEscape(messageID)+"/forward", map[string]any{
		"to":                                opts.To,
		"body":                              opts.Body,
		"preservation_mode":                 opts.PreservationMode,
		"omit_original_attachments":         opts.OmitOriginalAttachments,
		"omitted_original_attachment_names": opts.OmittedOriginalAttachmentNames,
	})
}

func (b *RemoteBackend) ListAttachments(messageID string) ([]models.Attachment, error) {
	var attachments []models.Attachment
	return attachments, b.get("/v1/emails/"+url.PathEscape(messageID)+"/attachments", &attachments)
}

func (b *RemoteBackend) GetAttachment(messageID, filename string) (*models.Attachment, error) {
	var a models.Attachment
	return &a, b.get("/v1/emails/"+url.PathEscape(messageID)+"/attachments/"+url.PathEscape(filename), &a)
}

// --- Drafts ---

func (b *RemoteBackend) SaveDraft(to, cc, bcc, subject, body string) (uint32, string, error) {
	var resp struct {
		UID    uint32 `json:"uid"`
		Folder string `json:"folder"`
	}
	payload := map[string]string{"to": to, "cc": cc, "bcc": bcc, "subject": subject, "body": body}
	if err := b.postOut("/v1/drafts", payload, &resp); err != nil {
		return 0, "", err
	}
	return resp.UID, resp.Folder, nil
}

func (b *RemoteBackend) SaveRawDraft(raw []byte) (uint32, string, error) {
	var resp struct {
		UID    uint32 `json:"uid"`
		Folder string `json:"folder"`
	}
	if err := b.postOut("/v1/drafts/raw", map[string]string{"raw": base64.StdEncoding.EncodeToString(raw)}, &resp); err != nil {
		return 0, "", err
	}
	return resp.UID, resp.Folder, nil
}

func (b *RemoteBackend) ListDrafts() ([]*models.Draft, error) {
	var drafts []*models.Draft
	return drafts, b.get("/v1/drafts", &drafts)
}

func (b *RemoteBackend) DeleteDraft(uid uint32, folder string) error {
	return b.delete(fmt.Sprintf("/v1/drafts/%d?folder=%s", uid, url.QueryEscape(folder)))
}

// --- Unsubscribed senders ---

func (b *RemoteBackend) RecordUnsubscribe(sender, method, url string) error { return nil }
func (b *RemoteBackend) IsUnsubscribedSender(sender string) (bool, error)   { return false, nil }

// --- Bulk operations ---

func (b *RemoteBackend) DeleteThread(folder, subject string) error {
	return b.post("/v1/threads/delete", map[string]string{"folder": folder, "subject": subject})
}

func (b *RemoteBackend) BulkDelete(messageIDs []string) error {
	return b.post("/v1/emails/bulk-delete", map[string]any{"message_ids": messageIDs})
}

func (b *RemoteBackend) ArchiveThread(folder, subject string) error {
	return b.post("/v1/threads/archive", map[string]string{"folder": folder, "subject": subject})
}

func (b *RemoteBackend) BulkMove(messageIDs []string, toFolder string) error {
	return b.post("/v1/emails/bulk-move", map[string]any{"message_ids": messageIDs, "to_folder": toFolder})
}

func (b *RemoteBackend) UnsubscribeSender(messageID string) error {
	return b.post("/v1/emails/"+url.PathEscape(messageID)+"/unsubscribe", nil)
}

func (b *RemoteBackend) SoftUnsubscribeSender(sender, toFolder string) error {
	return b.post("/v1/senders/"+url.PathEscape(sender)+"/soft-unsubscribe", map[string]string{"to_folder": toFolder})
}
