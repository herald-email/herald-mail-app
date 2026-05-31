package backend

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/herald-email/herald-mail-app/internal/cache"
	"github.com/herald-email/herald-mail-app/internal/config"
	"github.com/herald-email/herald-mail-app/internal/imap"
	"github.com/herald-email/herald-mail-app/internal/models"
	"github.com/herald-email/herald-mail-app/internal/oauth"
)

const gmailAPIProvider = "gmail"

type GmailAPISourcePlugin struct{}

func (GmailAPISourcePlugin) Kind() models.SourceKind { return models.SourceKindMail }
func (GmailAPISourcePlugin) Provider() string        { return gmailAPIProvider }

func (GmailAPISourcePlugin) Open(ctx context.Context, source config.SourceConfig, deps SourceDeps) (*OpenedSource, error) {
	if err := mailSourceContextErr(ctx); err != nil {
		return nil, err
	}
	if deps.Cache == nil {
		return nil, fmt.Errorf("open %s: cache is required for Gmail API source", source.ID)
	}
	mailSource := NewGmailAPIMailSource(source, deps.Cache, deps.ProgressCh)
	return &OpenedSource{
		SourceID:   models.NormalizeSourceID(models.SourceID(source.ID), models.DefaultMailSourceID),
		Account:    models.NormalizeAccountID(models.AccountID(source.AccountID)),
		SourceKind: models.SourceKindMail,
		Provider:   gmailAPIProvider,
		Name:       displayNameForSource(source),
		Caps: SourceCapabilities{
			Mail:             true,
			MailCollections:  true,
			MailSync:         true,
			MailSearch:       true,
			MailMutations:    true,
			Drafts:           false,
			CacheBypassReads: true,
			Freshness: ProviderFreshnessMetadata{
				Revision: true,
			},
		},
		Mail: mailSource,
	}, nil
}

type GmailAPIMailSource struct {
	id         models.SourceID
	accountID  models.AccountID
	google     config.GoogleConfig
	baseURL    string
	cache      *cache.Cache
	progressCh chan models.ProgressInfo
	client     *http.Client

	labels map[string]gmailAPILabel
}

func NewGmailAPIMailSource(source config.SourceConfig, c *cache.Cache, progressCh chan models.ProgressInfo) *GmailAPIMailSource {
	baseURL := strings.TrimRight(strings.TrimSpace(source.Google.APIBaseURL), "/")
	if baseURL == "" {
		baseURL = "https://gmail.googleapis.com/gmail/v1"
	}
	return &GmailAPIMailSource{
		id:         models.NormalizeSourceID(models.SourceID(source.ID), models.DefaultMailSourceID),
		accountID:  models.NormalizeAccountID(models.AccountID(source.AccountID)),
		google:     source.Google,
		baseURL:    baseURL,
		cache:      c,
		progressCh: progressCh,
		client:     http.DefaultClient,
	}
}

func (s *GmailAPIMailSource) Connect(ctx context.Context) error {
	if s == nil {
		return fmt.Errorf("Gmail API source unavailable")
	}
	if _, err := oauth.RefreshGoogleConfigIfNeeded(ctx, &s.google); err != nil {
		return err
	}
	_, err := s.loadLabels(ctx)
	return err
}

func (s *GmailAPIMailSource) Close() error { return nil }

func (s *GmailAPIMailSource) ListFolders(ctx context.Context) ([]string, error) {
	labels, err := s.loadLabels(ctx)
	if err != nil {
		return nil, err
	}
	var folders []string
	seen := map[string]bool{}
	add := func(folder string) {
		folder = gmailAPIFolderName(folder)
		if folder != "" && !seen[folder] {
			seen[folder] = true
			folders = append(folders, folder)
		}
	}
	for _, id := range []string{"INBOX", "SENT", "DRAFT", "TRASH", "SPAM", "STARRED"} {
		if _, ok := labels[id]; ok {
			add(id)
		}
	}
	add("All Mail")
	for _, label := range labels {
		if strings.EqualFold(label.Type, "user") {
			add(label.Name)
		}
	}
	sort.Strings(folders)
	return folders, nil
}

func (s *GmailAPIMailSource) GetFolderStatus(ctx context.Context, folders []string) (map[string]models.FolderStatus, error) {
	status := make(map[string]models.FolderStatus, len(folders))
	for _, folder := range folders {
		emails, err := s.cache.GetEmailsSortedByDate(folder)
		if err != nil {
			return nil, err
		}
		var unread int
		for _, email := range emails {
			if !email.IsRead {
				unread++
			}
		}
		status[folder] = models.FolderStatus{Total: len(emails), Unseen: unread}
	}
	return status, nil
}

func (s *GmailAPIMailSource) ProcessEmailsIncremental(ctx context.Context, folder string) error {
	emails, err := s.fetchEmails(ctx, folder, "")
	if err != nil {
		return err
	}
	if err := s.cache.BatchCacheEmails(emails); err != nil {
		return err
	}
	if s.progressCh != nil {
		s.progressCh <- models.ProgressInfo{SourceID: s.id, AccountID: s.accountID, Phase: models.SyncPhaseRowsCached, Current: len(emails), Total: len(emails), ProcessedEmails: len(emails), Message: fmt.Sprintf("Cached %d Gmail API messages", len(emails))}
	}
	return nil
}

func (s *GmailAPIMailSource) GetSenderStatistics(_ context.Context, folder string) (map[string]*models.SenderStats, error) {
	grouped, err := s.cache.GetAllEmails(folder, false)
	if err != nil {
		return nil, err
	}
	return senderStatisticsFromGroups(grouped), nil
}

func (s *GmailAPIMailSource) GetEmailsBySender(_ context.Context, folder string) (map[string][]*models.EmailData, error) {
	return s.cache.GetAllEmails(folder, false)
}

func (s *GmailAPIMailSource) StartBackgroundReconcile(ctx context.Context, folder string, validIDsCh chan<- map[string]bool) {
	defer close(validIDsCh)
	emails, err := s.fetchEmails(ctx, folder, "")
	if err != nil {
		return
	}
	valid := make(map[string]bool, len(emails))
	for _, email := range emails {
		valid[email.MessageID] = true
	}
	validIDsCh <- valid
}

func (s *GmailAPIMailSource) GetFolderMessageIDs(ctx context.Context, folders []string) (map[string]map[string]bool, error) {
	out := make(map[string]map[string]bool, len(folders))
	for _, folder := range folders {
		emails, err := s.fetchEmails(ctx, folder, "")
		if err != nil {
			return nil, err
		}
		ids := make(map[string]bool, len(emails))
		for _, email := range emails {
			ids[email.MessageID] = true
		}
		out[folder] = ids
	}
	return out, nil
}

func (s *GmailAPIMailSource) DeleteSenderEmails(ctx context.Context, sender, folder string) error {
	grouped, err := s.cache.GetAllEmails(folder, false)
	if err != nil {
		return err
	}
	for _, email := range grouped[sender] {
		if err := s.DeleteEmail(ctx, email.MessageID, folder); err != nil {
			return err
		}
	}
	return nil
}

func (s *GmailAPIMailSource) DeleteDomainEmails(ctx context.Context, domain, folder string) error {
	grouped, err := s.cache.GetAllEmails(folder, false)
	if err != nil {
		return err
	}
	for _, emails := range grouped {
		for _, email := range emails {
			if strings.Contains(strings.ToLower(email.Sender), "@"+strings.ToLower(domain)) {
				if err := s.DeleteEmail(ctx, email.MessageID, folder); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func (s *GmailAPIMailSource) DeleteEmail(ctx context.Context, messageID, folder string) error {
	gmailID, err := s.gmailIDForMessageID(messageID)
	if err != nil {
		return err
	}
	req, err := s.newRequest(ctx, http.MethodPost, s.baseURL+"/users/me/messages/"+url.PathEscape(gmailID)+"/trash", nil)
	if err != nil {
		return err
	}
	if err := s.doJSON(req, nil); err != nil {
		return err
	}
	return s.cache.DeleteEmail(messageID)
}

func (s *GmailAPIMailSource) ArchiveEmail(ctx context.Context, messageID, folder string) error {
	gmailID, err := s.gmailIDForMessageID(messageID)
	if err != nil {
		return err
	}
	if err := s.modifyLabels(ctx, gmailID, nil, []string{"INBOX"}); err != nil {
		return err
	}
	return nil
}

func (s *GmailAPIMailSource) ArchiveSenderEmails(ctx context.Context, sender, folder string) error {
	grouped, err := s.cache.GetAllEmails(folder, false)
	if err != nil {
		return err
	}
	for _, email := range grouped[sender] {
		if err := s.ArchiveEmail(ctx, email.MessageID, folder); err != nil {
			return err
		}
	}
	return nil
}

func (s *GmailAPIMailSource) MoveEmail(ctx context.Context, messageID, fromFolder, toFolder string) error {
	gmailID, err := s.gmailIDForMessageID(messageID)
	if err != nil {
		return err
	}
	toLabel, err := s.labelIDForFolder(ctx, toFolder)
	if err != nil {
		return err
	}
	remove := []string{}
	if fromLabel, err := s.labelIDForFolder(ctx, fromFolder); err == nil && fromLabel != "" {
		remove = append(remove, fromLabel)
	}
	return s.modifyLabels(ctx, gmailID, []string{toLabel}, remove)
}

func (s *GmailAPIMailSource) SearchIMAP(ctx context.Context, folder, query string) ([]*models.EmailData, error) {
	return s.fetchEmails(ctx, folder, query)
}

func (s *GmailAPIMailSource) SetGroupByDomain(bool) {}

func (s *GmailAPIMailSource) MarkRead(ctx context.Context, uid uint32, folder string) error {
	return s.modifyByUID(ctx, uid, folder, nil, []string{"UNREAD"}, func(messageID string) error {
		return s.cache.MarkRead(messageID)
	})
}

func (s *GmailAPIMailSource) MarkUnread(ctx context.Context, uid uint32, folder string) error {
	return s.modifyByUID(ctx, uid, folder, []string{"UNREAD"}, nil, func(messageID string) error {
		return s.cache.MarkUnread(messageID)
	})
}

func (s *GmailAPIMailSource) MarkStarred(ctx context.Context, uid uint32, folder string) error {
	return s.modifyByUID(ctx, uid, folder, []string{"STARRED"}, nil, func(messageID string) error {
		return s.cache.UpdateStarred(messageID, true)
	})
}

func (s *GmailAPIMailSource) UnmarkStarred(ctx context.Context, uid uint32, folder string) error {
	return s.modifyByUID(ctx, uid, folder, nil, []string{"STARRED"}, func(messageID string) error {
		return s.cache.UpdateStarred(messageID, false)
	})
}

func (s *GmailAPIMailSource) StartIDLE(context.Context, string, chan<- models.NewEmailsNotification) error {
	return fmt.Errorf("Gmail API IDLE is not supported")
}

func (s *GmailAPIMailSource) StopIDLE() {}

func (s *GmailAPIMailSource) PollForNewEmails(ctx context.Context, folder string, sinceDate time.Time) ([]*models.EmailData, error) {
	emails, err := s.fetchEmails(ctx, folder, "")
	if err != nil {
		return nil, err
	}
	var out []*models.EmailData
	for _, email := range emails {
		if email.Date.After(sinceDate) {
			out = append(out, email)
		}
	}
	return out, nil
}

func (s *GmailAPIMailSource) AppendDraft(context.Context, []byte) (uint32, string, error) {
	return 0, "", fmt.Errorf("Gmail API draft append is not implemented")
}

func (s *GmailAPIMailSource) ListDrafts(context.Context) ([]*models.Draft, error) {
	return nil, nil
}

func (s *GmailAPIMailSource) DeleteDraft(context.Context, uint32, string) error {
	return fmt.Errorf("Gmail API draft delete is not implemented")
}

func (s *GmailAPIMailSource) CreateMailbox(context.Context, string) error {
	return fmt.Errorf("Gmail API label creation is not implemented")
}

func (s *GmailAPIMailSource) RenameMailbox(context.Context, string, string) error {
	return fmt.Errorf("Gmail API label rename is not implemented")
}

func (s *GmailAPIMailSource) DeleteMailbox(context.Context, string) error {
	return fmt.Errorf("Gmail API label deletion is not implemented")
}

func (s *GmailAPIMailSource) FetchMessageNoCache(ctx context.Context, ref models.MessageRef) (*models.EmailBody, error) {
	gmailID := gmailIDFromLocalID(ref.LocalID)
	if gmailID == "" && ref.MessageID != "" {
		var err error
		gmailID, err = s.gmailIDForMessageID(ref.MessageID)
		if err != nil {
			return nil, err
		}
	}
	if gmailID == "" {
		return nil, fmt.Errorf("Gmail API message id unavailable for %s", ref.MessageID)
	}
	payload, err := s.fetchMessage(ctx, gmailID)
	if err != nil {
		return nil, err
	}
	body, err := gmailAPIMessageBody(payload)
	if err != nil {
		return nil, err
	}
	if body.MessageID == "" {
		body.MessageID = ref.MessageID
	}
	return body, nil
}

func (s *GmailAPIMailSource) FetchMessagePreviewNoCache(ctx context.Context, ref models.MessageRef) (*models.EmailBody, error) {
	return s.FetchMessageNoCache(ctx, ref)
}

func (s *GmailAPIMailSource) SendEmail(ctx context.Context, from, to, subject, body string) error {
	raw := buildGmailAPIRawMessage(from, to, "", subject, body)
	payload := map[string][]string{"raw": []string{base64.RawURLEncoding.EncodeToString([]byte(raw))}}
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	httpReq, err := s.newRequest(ctx, http.MethodPost, s.baseURL+"/users/me/messages/send", bytes.NewReader(data))
	if err != nil {
		return err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	return s.doJSON(httpReq, nil)
}

func (s *GmailAPIMailSource) SendCompose(ctx context.Context, req ComposeSendRequest) error {
	from := strings.TrimSpace(req.From)
	if from == "" {
		from = strings.TrimSpace(s.google.Email)
	}
	raw := buildGmailAPIRawMessage(from, req.To, req.CC, req.Subject, req.MarkdownBody)
	payload := map[string][]string{"raw": []string{base64.RawURLEncoding.EncodeToString([]byte(raw))}}
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	httpReq, err := s.newRequest(ctx, http.MethodPost, s.baseURL+"/users/me/messages/send", bytes.NewReader(data))
	if err != nil {
		return err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	return s.doJSON(httpReq, nil)
}

func (s *GmailAPIMailSource) fetchEmails(ctx context.Context, folder, query string) ([]*models.EmailData, error) {
	ids, err := s.listMessageIDs(ctx, folder, query)
	if err != nil {
		return nil, err
	}
	emails := make([]*models.EmailData, 0, len(ids))
	for _, id := range ids {
		payload, err := s.fetchMessage(ctx, id)
		if err != nil {
			return nil, err
		}
		email, err := s.emailDataFromMessage(folder, payload)
		if err != nil {
			return nil, err
		}
		emails = append(emails, email)
	}
	return emails, nil
}

func (s *GmailAPIMailSource) listMessageIDs(ctx context.Context, folder, query string) ([]string, error) {
	values := url.Values{}
	if label, err := s.labelIDForFolder(ctx, folder); err == nil && label != "" {
		values.Set("labelIds", label)
	}
	if strings.TrimSpace(query) != "" {
		values.Set("q", strings.TrimSpace(query))
	}
	u := s.baseURL + "/users/me/messages"
	if encoded := values.Encode(); encoded != "" {
		u += "?" + encoded
	}
	req, err := s.newRequest(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	var payload gmailAPIMessageList
	if err := s.doJSON(req, &payload); err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(payload.Messages))
	for _, msg := range payload.Messages {
		if strings.TrimSpace(msg.ID) != "" {
			ids = append(ids, msg.ID)
		}
	}
	return ids, nil
}

func (s *GmailAPIMailSource) fetchMessage(ctx context.Context, gmailID string) (gmailAPIMessage, error) {
	req, err := s.newRequest(ctx, http.MethodGet, s.baseURL+"/users/me/messages/"+url.PathEscape(gmailID)+"?format=raw", nil)
	if err != nil {
		return gmailAPIMessage{}, err
	}
	var payload gmailAPIMessage
	if err := s.doJSON(req, &payload); err != nil {
		return gmailAPIMessage{}, err
	}
	if payload.ID == "" {
		payload.ID = gmailID
	}
	return payload, nil
}

func (s *GmailAPIMailSource) emailDataFromMessage(folder string, payload gmailAPIMessage) (*models.EmailData, error) {
	body, err := gmailAPIMessageBody(payload)
	if err != nil {
		return nil, err
	}
	date := time.Now()
	if payload.InternalDate != nil {
		if millis, err := parseGmailAPIInternalDate(payload.InternalDate); err == nil && millis > 0 {
			date = time.UnixMilli(millis)
		}
	}
	messageID := strings.TrimSpace(body.MessageID)
	if messageID == "" {
		messageID = "gmail:" + payload.ID
	}
	ref := models.MessageRef{
		SourceID:  s.id,
		AccountID: s.accountID,
		Folder:    folder,
		UID:       gmailAPISyntheticUID(payload.ID),
		MessageID: messageID,
		LocalID:   gmailAPILocalID(s.id, s.accountID, folder, payload.ID),
	}.WithDefaults()
	return &models.EmailData{
		SourceID:       ref.SourceID,
		AccountID:      ref.AccountID,
		LocalID:        ref.LocalID,
		MessageID:      messageID,
		UID:            ref.UID,
		Sender:         body.From,
		Subject:        body.Subject,
		Date:           date,
		Size:           payload.SizeEstimate,
		HasAttachments: len(body.Attachments) > 0,
		Folder:         folder,
		IsRead:         !containsLabel(payload.LabelIDs, "UNREAD"),
		IsStarred:      containsLabel(payload.LabelIDs, "STARRED"),
		IsDraft:        containsLabel(payload.LabelIDs, "DRAFT"),
	}, nil
}

func (s *GmailAPIMailSource) loadLabels(ctx context.Context) (map[string]gmailAPILabel, error) {
	if s.labels != nil {
		return s.labels, nil
	}
	req, err := s.newRequest(ctx, http.MethodGet, s.baseURL+"/users/me/labels", nil)
	if err != nil {
		return nil, err
	}
	var payload gmailAPILabelList
	if err := s.doJSON(req, &payload); err != nil {
		return nil, err
	}
	labels := make(map[string]gmailAPILabel, len(payload.Labels))
	for _, label := range payload.Labels {
		labels[label.ID] = label
	}
	s.labels = labels
	return labels, nil
}

func (s *GmailAPIMailSource) labelIDForFolder(ctx context.Context, folder string) (string, error) {
	normalized := gmailAPIFolderName(folder)
	switch normalized {
	case "", "All Mail":
		return "", nil
	case "INBOX", "SENT", "DRAFT", "TRASH", "SPAM", "STARRED":
		return normalized, nil
	}
	labels, err := s.loadLabels(ctx)
	if err != nil {
		return "", err
	}
	for _, label := range labels {
		if strings.EqualFold(label.Name, folder) {
			return label.ID, nil
		}
	}
	return "", fmt.Errorf("Gmail label %q not found", folder)
}

func (s *GmailAPIMailSource) gmailIDForMessageID(messageID string) (string, error) {
	email, err := s.cache.GetEmailByID(messageID)
	if err != nil {
		return "", err
	}
	if id := gmailIDFromLocalID(email.LocalID); id != "" {
		return id, nil
	}
	return "", fmt.Errorf("Gmail API id unavailable for message %s", messageID)
}

func (s *GmailAPIMailSource) modifyByUID(ctx context.Context, uid uint32, folder string, add, remove []string, after func(string) error) error {
	email, err := s.cache.GetEmailByFolderUID(folder, uid)
	if err != nil {
		return err
	}
	gmailID := gmailIDFromLocalID(email.LocalID)
	if gmailID == "" {
		return fmt.Errorf("Gmail API id unavailable for uid %d in %s", uid, folder)
	}
	if err := s.modifyLabels(ctx, gmailID, add, remove); err != nil {
		return err
	}
	if after != nil {
		return after(email.MessageID)
	}
	return nil
}

func (s *GmailAPIMailSource) modifyLabels(ctx context.Context, gmailID string, add, remove []string) error {
	payload := map[string][]string{}
	if len(add) > 0 {
		payload["addLabelIds"] = add
	}
	if len(remove) > 0 {
		payload["removeLabelIds"] = remove
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := s.newRequest(ctx, http.MethodPost, s.baseURL+"/users/me/messages/"+url.PathEscape(gmailID)+"/modify", bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	return s.doJSON(req, nil)
}

func (s *GmailAPIMailSource) newRequest(ctx context.Context, method, u string, body io.Reader) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, method, u, body)
	if err != nil {
		return nil, err
	}
	token, err := oauth.RefreshGoogleConfigIfNeeded(ctx, &s.google)
	if err != nil {
		return nil, err
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	return req, nil
}

func (s *GmailAPIMailSource) doJSON(req *http.Request, out any) error {
	resp, err := s.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return fmt.Errorf("gmail api %s %s: status %d: %s", req.Method, req.URL.Path, resp.StatusCode, strings.TrimSpace(string(body)))
	}
	if out == nil {
		io.Copy(io.Discard, resp.Body) //nolint:errcheck
		return nil
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

type gmailAPILabelList struct {
	Labels []gmailAPILabel `json:"labels"`
}

type gmailAPILabel struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Type string `json:"type"`
}

type gmailAPIMessageList struct {
	Messages []struct {
		ID       string `json:"id"`
		ThreadID string `json:"threadId"`
	} `json:"messages"`
}

type gmailAPIMessage struct {
	ID           string   `json:"id"`
	ThreadID     string   `json:"threadId"`
	LabelIDs     []string `json:"labelIds"`
	InternalDate any      `json:"internalDate"`
	SizeEstimate int      `json:"sizeEstimate"`
	Raw          string   `json:"raw"`
}

func gmailAPIMessageBody(payload gmailAPIMessage) (*models.EmailBody, error) {
	raw, err := decodeGmailAPIRaw(payload.Raw)
	if err != nil {
		return nil, err
	}
	body, err := imap.ParseMIMEBody(raw)
	if err != nil {
		return nil, err
	}
	return body, nil
}

func decodeGmailAPIRaw(raw string) ([]byte, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, fmt.Errorf("Gmail API message raw body is empty")
	}
	if data, err := base64.RawURLEncoding.DecodeString(raw); err == nil {
		return data, nil
	}
	return base64.URLEncoding.DecodeString(raw)
}

func gmailAPIFolderName(folder string) string {
	switch strings.ToUpper(strings.TrimSpace(folder)) {
	case "[GMAIL]/ALL MAIL", "ALL MAIL", "ALL":
		return "All Mail"
	case "[GMAIL]/SENT MAIL", "SENT MAIL":
		return "SENT"
	case "[GMAIL]/DRAFTS", "DRAFTS":
		return "DRAFT"
	case "[GMAIL]/TRASH", "TRASH":
		return "TRASH"
	default:
		return strings.TrimSpace(folder)
	}
}

func gmailAPILocalID(sourceID models.SourceID, accountID models.AccountID, folder, gmailID string) string {
	return strings.Join([]string{
		string(models.SourceKindMail),
		string(models.NormalizeSourceID(sourceID, models.DefaultMailSourceID)),
		string(models.NormalizeAccountID(accountID)),
		folder,
		"gmail:" + gmailID,
	}, ":")
}

func gmailIDFromLocalID(localID string) string {
	parts := strings.SplitN(strings.TrimSpace(localID), ":", 6)
	if len(parts) != 6 || parts[0] != string(models.SourceKindMail) || parts[4] != "gmail" {
		return ""
	}
	return parts[5]
}

func gmailAPISyntheticUID(id string) uint32 {
	h := fnv.New32a()
	_, _ = h.Write([]byte(id))
	uid := h.Sum32()
	if uid == 0 {
		return 1
	}
	return uid
}

func parseGmailAPIInternalDate(value any) (int64, error) {
	switch v := value.(type) {
	case string:
		return strconv.ParseInt(strings.TrimSpace(v), 10, 64)
	case float64:
		return int64(v), nil
	case json.Number:
		return v.Int64()
	default:
		return 0, fmt.Errorf("unsupported Gmail internalDate %T", value)
	}
}

func containsLabel(labels []string, want string) bool {
	for _, label := range labels {
		if label == want {
			return true
		}
	}
	return false
}

func buildGmailAPIRawMessage(from, to, cc, subject, body string) string {
	var b strings.Builder
	b.WriteString("From: " + from + "\r\n")
	b.WriteString("To: " + to + "\r\n")
	if strings.TrimSpace(cc) != "" {
		b.WriteString("Cc: " + cc + "\r\n")
	}
	b.WriteString("Subject: " + subject + "\r\n")
	b.WriteString("MIME-Version: 1.0\r\n")
	b.WriteString("Content-Type: text/plain; charset=utf-8\r\n")
	b.WriteString("\r\n")
	b.WriteString(body)
	return b.String()
}
