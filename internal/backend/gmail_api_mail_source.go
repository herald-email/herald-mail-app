package backend

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"hash/fnv"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/herald-email/herald-mail-app/internal/cache"
	"github.com/herald-email/herald-mail-app/internal/config"
	"github.com/herald-email/herald-mail-app/internal/imap"
	"github.com/herald-email/herald-mail-app/internal/models"
	"github.com/herald-email/herald-mail-app/internal/oauth"
	appsmtp "github.com/herald-email/herald-mail-app/internal/smtp"
)

const gmailAPIProvider = "gmail"

const (
	gmailAPIBoundedSyncLimit = 100
	gmailAPIMaxRetryDelay    = 5 * time.Second
)

var gmailAPIRetryAfterBodyRE = regexp.MustCompile(`(?i)retry after ([0-9T:\-.Z]+)`)

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
			Drafts:           true,
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
	drafts map[uint32]string
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
	cursor, hasCursor, err := s.cache.GetMetadata(s.historyCursorKey(folder))
	if err != nil {
		return err
	}
	if hasCursor && strings.TrimSpace(cursor) != "" {
		updated, err := s.applyHistoryDelta(ctx, folder, cursor)
		if err == nil {
			if s.progressCh != nil {
				s.progressCh <- models.ProgressInfo{SourceID: s.id, AccountID: s.accountID, Phase: models.SyncPhaseRowsCached, Current: updated, Total: updated, ProcessedEmails: updated, Message: fmt.Sprintf("Applied %d Gmail API history updates", updated)}
			}
			return nil
		}
		if !isGmailAPIHistoryExpired(err) {
			return err
		}
	}
	emails, nextCursor, err := s.fetchEmailsWithCursorLimit(ctx, folder, "", gmailAPIBoundedSyncLimit)
	if err != nil {
		return err
	}
	if err := s.cache.BatchCacheEmails(emails); err != nil {
		return err
	}
	if nextCursor != "" {
		if err := s.cache.SetMetadata(s.historyCursorKey(folder), nextCursor); err != nil {
			return err
		}
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
	ids, err := s.listMessageIDs(ctx, folder, "", 0)
	if err != nil {
		return
	}
	gmailIDs := make(map[string]bool, len(ids))
	for _, id := range ids {
		gmailIDs[id] = true
	}
	cached, err := s.cache.GetEmailsSortedByDate(folder)
	if err != nil {
		return
	}
	valid := make(map[string]bool, len(cached))
	for _, email := range cached {
		if email != nil && gmailIDs[gmailIDFromLocalID(email.LocalID)] {
			valid[email.MessageID] = true
		}
	}
	validIDsCh <- valid
}

func (s *GmailAPIMailSource) GetFolderMessageIDs(ctx context.Context, folders []string) (map[string]map[string]bool, error) {
	out := make(map[string]map[string]bool, len(folders))
	for _, folder := range folders {
		ids, err := s.listMessageIDs(ctx, folder, "", 0)
		if err != nil {
			return nil, err
		}
		gmailIDs := make(map[string]bool, len(ids))
		for _, id := range ids {
			gmailIDs[id] = true
		}
		cached, err := s.cachedEmailsForGmailIDMapping(folder)
		if err != nil {
			return nil, err
		}
		messageIDs := make(map[string]bool, len(cached))
		for _, email := range cached {
			if email != nil && gmailIDs[gmailIDFromLocalID(email.LocalID)] {
				messageIDs[email.MessageID] = true
			}
		}
		out[folder] = messageIDs
	}
	return out, nil
}

func (s *GmailAPIMailSource) cachedEmailsForGmailIDMapping(folder string) ([]*models.EmailData, error) {
	folders := []string{folder}
	normalized := gmailAPIFolderName(folder)
	if normalized != "All Mail" {
		folders = append(folders, "All Mail")
	}
	seen := map[string]bool{}
	var out []*models.EmailData
	for _, cacheFolder := range folders {
		cacheFolder = gmailAPIFolderName(cacheFolder)
		if cacheFolder == "" || seen[cacheFolder] {
			continue
		}
		seen[cacheFolder] = true
		emails, err := s.cache.GetEmailsSortedByDate(cacheFolder)
		if err != nil {
			return nil, err
		}
		out = append(out, emails...)
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

func (s *GmailAPIMailSource) AppendDraft(ctx context.Context, raw []byte) (uint32, string, error) {
	payload := map[string]map[string]string{"message": {"raw": base64.RawURLEncoding.EncodeToString(raw)}}
	data, err := json.Marshal(payload)
	if err != nil {
		return 0, "", err
	}
	req, err := s.newRequest(ctx, http.MethodPost, s.baseURL+"/users/me/drafts", bytes.NewReader(data))
	if err != nil {
		return 0, "", err
	}
	req.Header.Set("Content-Type", "application/json")
	var draft gmailAPIDraft
	if err := s.doJSON(req, &draft); err != nil {
		return 0, "", err
	}
	messageID := strings.TrimSpace(draft.Message.ID)
	if messageID == "" {
		messageID = strings.TrimSpace(draft.ID)
	}
	uid := gmailAPISyntheticUID(messageID)
	s.rememberDraft(uid, draft.ID)
	return uid, "DRAFT", nil
}

func (s *GmailAPIMailSource) ListDrafts(ctx context.Context) ([]*models.Draft, error) {
	values := url.Values{}
	var drafts []*models.Draft
	for {
		u := s.baseURL + "/users/me/drafts"
		if encoded := values.Encode(); encoded != "" {
			u += "?" + encoded
		}
		req, err := s.newRequest(ctx, http.MethodGet, u, nil)
		if err != nil {
			return nil, err
		}
		var list gmailAPIDraftList
		if err := s.doJSON(req, &list); err != nil {
			return nil, err
		}
		for _, item := range list.Drafts {
			draftID := strings.TrimSpace(item.ID)
			if draftID == "" {
				continue
			}
			draft, err := s.fetchDraft(ctx, draftID)
			if err != nil {
				return nil, err
			}
			body, err := gmailAPIMessageBody(draft.Message)
			if err != nil {
				return nil, err
			}
			messageID := strings.TrimSpace(draft.Message.ID)
			if messageID == "" {
				messageID = draftID
			}
			uid := gmailAPISyntheticUID(messageID)
			s.rememberDraft(uid, draftID)
			date := time.Now()
			if draft.Message.InternalDate != nil {
				if millis, err := parseGmailAPIInternalDate(draft.Message.InternalDate); err == nil && millis > 0 {
					date = time.UnixMilli(millis)
				}
			}
			drafts = append(drafts, &models.Draft{
				UID:     uid,
				Folder:  "DRAFT",
				To:      body.To,
				CC:      body.CC,
				BCC:     body.BCC,
				Subject: body.Subject,
				Body:    firstNonEmpty(body.TextPlain, body.TextHTML),
				Date:    date,
			})
		}
		if strings.TrimSpace(list.NextPageToken) == "" {
			return drafts, nil
		}
		values.Set("pageToken", strings.TrimSpace(list.NextPageToken))
	}
}

func (s *GmailAPIMailSource) DeleteDraft(ctx context.Context, uid uint32, folder string) error {
	draftID, err := s.draftIDForUID(ctx, uid, folder)
	if err != nil {
		return err
	}
	req, err := s.newRequest(ctx, http.MethodDelete, s.baseURL+"/users/me/drafts/"+url.PathEscape(draftID), nil)
	if err != nil {
		return err
	}
	if err := s.doJSON(req, nil); err != nil {
		return err
	}
	delete(s.drafts, uid)
	return nil
}

func (s *GmailAPIMailSource) SendDraft(ctx context.Context, uid uint32, folder string) error {
	draftID, err := s.draftIDForUID(ctx, uid, folder)
	if err != nil {
		return err
	}
	payload := map[string]string{"id": draftID}
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := s.newRequest(ctx, http.MethodPost, s.baseURL+"/users/me/drafts/send", bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if err := s.doJSON(req, nil); err != nil {
		return err
	}
	delete(s.drafts, uid)
	return nil
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
	payload, err := s.fetchMessageRaw(ctx, gmailID)
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
	raw := buildGmailAPIRawMessage(from, to, "", "", subject, body)
	err := s.sendRawMessage(ctx, raw)
	return err
}

func (s *GmailAPIMailSource) SendCompose(ctx context.Context, req ComposeSendRequest) error {
	from := strings.TrimSpace(req.From)
	if from == "" {
		from = strings.TrimSpace(s.google.Email)
	}
	req.From = from
	var raw string
	var err error
	if req.Preserved != nil {
		preserved := *req.Preserved
		if strings.TrimSpace(preserved.From) == "" {
			preserved.From = from
		}
		raw, err = appsmtp.BuildPreservedMIMEMessage(preserved)
		if err == nil && strings.TrimSpace(preserved.BCC) != "" {
			raw = insertGmailAPIBccHeader(raw, preserved.BCC)
		}
	} else {
		raw, err = buildGmailAPIComposeRaw(req)
	}
	if err != nil {
		return err
	}
	return s.sendRawMessage(ctx, raw)
}

func (s *GmailAPIMailSource) sendRawMessage(ctx context.Context, raw string) error {
	payload := map[string][]string{"raw": {base64.RawURLEncoding.EncodeToString([]byte(raw))}}
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
	emails, _, err := s.fetchEmailsWithCursor(ctx, folder, query)
	return emails, err
}

func (s *GmailAPIMailSource) fetchEmailsWithCursor(ctx context.Context, folder, query string) ([]*models.EmailData, string, error) {
	return s.fetchEmailsWithCursorLimit(ctx, folder, query, 0)
}

func (s *GmailAPIMailSource) fetchEmailsWithCursorLimit(ctx context.Context, folder, query string, limit int) ([]*models.EmailData, string, error) {
	ids, err := s.listMessageIDs(ctx, folder, query, limit)
	if err != nil {
		return nil, "", err
	}
	emails := make([]*models.EmailData, 0, len(ids))
	nextCursor := ""
	for _, id := range ids {
		payload, err := s.fetchMessageMetadata(ctx, id)
		if err != nil {
			return nil, "", err
		}
		email, err := s.emailDataFromMessage(folder, payload)
		if err != nil {
			return nil, "", err
		}
		emails = append(emails, email)
		nextCursor = maxGmailAPIHistoryID(nextCursor, payload.HistoryID)
	}
	return emails, nextCursor, nil
}

func (s *GmailAPIMailSource) applyHistoryDelta(ctx context.Context, folder, cursor string) (int, error) {
	labelID, err := s.labelIDForFolder(ctx, folder)
	if err != nil {
		return 0, err
	}
	payload, err := s.listHistory(ctx, cursor, labelID)
	if err != nil {
		return 0, err
	}
	changed := map[string]bool{}
	deleted := map[string]bool{}
	for _, record := range payload.History {
		for _, item := range record.MessagesAdded {
			if id := strings.TrimSpace(item.Message.ID); id != "" {
				changed[id] = true
			}
		}
		for _, item := range record.MessagesDeleted {
			if id := strings.TrimSpace(item.Message.ID); id != "" {
				deleted[id] = true
			}
		}
		for _, item := range record.LabelsAdded {
			id := strings.TrimSpace(item.Message.ID)
			if id == "" {
				continue
			}
			if containsAnyLabel(item.LabelIDs, "TRASH") && !strings.EqualFold(labelID, "TRASH") {
				deleted[id] = true
				continue
			}
			changed[id] = true
		}
		for _, item := range record.LabelsRemoved {
			id := strings.TrimSpace(item.Message.ID)
			if id == "" {
				continue
			}
			if labelID != "" && containsAnyLabel(item.LabelIDs, labelID) {
				deleted[id] = true
				continue
			}
			changed[id] = true
		}
		for _, msg := range record.Messages {
			if id := strings.TrimSpace(msg.ID); id != "" {
				changed[id] = true
			}
		}
	}
	for id := range deleted {
		if err := s.cache.DeleteEmailByLocalID(gmailAPILocalID(s.id, s.accountID, folder, id)); err != nil {
			return 0, err
		}
		delete(changed, id)
	}
	cached := 0
	for id := range changed {
		msg, err := s.fetchMessageMetadata(ctx, id)
		if err != nil {
			return cached, err
		}
		if !s.messageBelongsInFolder(labelID, msg.LabelIDs) {
			if err := s.cache.DeleteEmailByLocalID(gmailAPILocalID(s.id, s.accountID, folder, id)); err != nil {
				return cached, err
			}
			continue
		}
		email, err := s.emailDataFromMessage(folder, msg)
		if err != nil {
			return cached, err
		}
		if err := s.cache.CacheEmail(email); err != nil {
			return cached, err
		}
		cached++
	}
	if strings.TrimSpace(payload.HistoryID) != "" {
		if err := s.cache.SetMetadata(s.historyCursorKey(folder), strings.TrimSpace(payload.HistoryID)); err != nil {
			return cached, err
		}
	}
	return cached + len(deleted), nil
}

func (s *GmailAPIMailSource) listHistory(ctx context.Context, cursor, labelID string) (gmailAPIHistoryList, error) {
	values := url.Values{}
	values.Set("startHistoryId", strings.TrimSpace(cursor))
	if strings.TrimSpace(labelID) != "" {
		values.Set("labelId", strings.TrimSpace(labelID))
	}
	var combined gmailAPIHistoryList
	for {
		u := s.baseURL + "/users/me/history?" + values.Encode()
		req, err := s.newRequest(ctx, http.MethodGet, u, nil)
		if err != nil {
			return gmailAPIHistoryList{}, err
		}
		var payload gmailAPIHistoryList
		if err := s.doJSON(req, &payload); err != nil {
			return gmailAPIHistoryList{}, err
		}
		combined.History = append(combined.History, payload.History...)
		if payload.HistoryID != "" {
			combined.HistoryID = payload.HistoryID
		}
		if strings.TrimSpace(payload.NextPageToken) == "" {
			return combined, nil
		}
		values.Set("pageToken", strings.TrimSpace(payload.NextPageToken))
	}
}

func (s *GmailAPIMailSource) messageBelongsInFolder(labelID string, labels []string) bool {
	labelID = strings.TrimSpace(labelID)
	if labelID == "" {
		return !containsAnyLabel(labels, "TRASH", "SPAM")
	}
	return containsAnyLabel(labels, labelID)
}

func (s *GmailAPIMailSource) historyCursorKey(folder string) string {
	return strings.Join([]string{
		"gmail_api_history",
		string(models.NormalizeSourceID(s.id, models.DefaultMailSourceID)),
		string(models.NormalizeAccountID(s.accountID)),
		gmailAPIFolderName(folder),
	}, ":")
}

func (s *GmailAPIMailSource) listMessageIDs(ctx context.Context, folder, query string, limit int) ([]string, error) {
	values := url.Values{}
	if label, err := s.labelIDForFolder(ctx, folder); err == nil && label != "" {
		values.Set("labelIds", label)
	}
	if strings.TrimSpace(query) != "" {
		values.Set("q", strings.TrimSpace(query))
	}
	if limit > 0 {
		values.Set("maxResults", strconv.Itoa(min(limit, gmailAPIBoundedSyncLimit)))
	}
	var ids []string
	for {
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
		for _, msg := range payload.Messages {
			if strings.TrimSpace(msg.ID) != "" {
				ids = append(ids, msg.ID)
				if limit > 0 && len(ids) >= limit {
					return ids, nil
				}
			}
		}
		if strings.TrimSpace(payload.NextPageToken) == "" {
			return ids, nil
		}
		if limit > 0 {
			remaining := limit - len(ids)
			if remaining <= 0 {
				return ids, nil
			}
			values.Set("maxResults", strconv.Itoa(min(remaining, gmailAPIBoundedSyncLimit)))
		}
		values.Set("pageToken", strings.TrimSpace(payload.NextPageToken))
	}
}

func (s *GmailAPIMailSource) fetchMessageRaw(ctx context.Context, gmailID string) (gmailAPIMessage, error) {
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

func (s *GmailAPIMailSource) fetchMessageMetadata(ctx context.Context, gmailID string) (gmailAPIMessage, error) {
	values := url.Values{}
	values.Set("format", "metadata")
	for _, header := range []string{"From", "To", "Cc", "Bcc", "Subject", "Message-ID", "Date", "List-Unsubscribe", "List-Unsubscribe-Post", "In-Reply-To", "References"} {
		values.Add("metadataHeaders", header)
	}
	req, err := s.newRequest(ctx, http.MethodGet, s.baseURL+"/users/me/messages/"+url.PathEscape(gmailID)+"?"+values.Encode(), nil)
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
	body, err := gmailAPIMessageMetadataBody(payload)
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
		HasAttachments: gmailAPIMessageHasAttachments(payload.Payload),
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
	values := url.Values{}
	labels := map[string]gmailAPILabel{}
	for {
		u := s.baseURL + "/users/me/labels"
		if encoded := values.Encode(); encoded != "" {
			u += "?" + encoded
		}
		req, err := s.newRequest(ctx, http.MethodGet, u, nil)
		if err != nil {
			return nil, err
		}
		var payload gmailAPILabelList
		if err := s.doJSON(req, &payload); err != nil {
			return nil, err
		}
		for _, label := range payload.Labels {
			labels[label.ID] = label
		}
		if strings.TrimSpace(payload.NextPageToken) == "" {
			break
		}
		values.Set("pageToken", strings.TrimSpace(payload.NextPageToken))
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
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			body, err := retryableRequestBody(req)
			if err != nil {
				return lastErr
			}
			req.Body = body
		}
		resp, err := s.client.Do(req)
		if err != nil {
			return err
		}
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
			resp.Body.Close()
			lastErr = gmailAPIHTTPError{Method: req.Method, Path: req.URL.Path, StatusCode: resp.StatusCode, Body: trimGmailAPIErrorBody(string(body))}
			if isGmailAPIRetryableStatus(resp.StatusCode) && attempt < 2 {
				delay := gmailAPIRetryDelay(attempt+1, resp.Header.Get("Retry-After"), string(body), time.Now())
				if err := sleepWithContext(req.Context(), delay); err != nil {
					return err
				}
				continue
			}
			return lastErr
		}
		defer resp.Body.Close()
		if out == nil {
			io.Copy(io.Discard, resp.Body) //nolint:errcheck
			return nil
		}
		return json.NewDecoder(resp.Body).Decode(out)
	}
	return lastErr
}

func gmailAPIRetryDelay(attempt int, retryAfterHeader, body string, now time.Time) time.Duration {
	if delay, ok := parseGmailAPIRetryAfter(retryAfterHeader, now); ok {
		return boundGmailAPIRetryDelay(delay)
	}
	if match := gmailAPIRetryAfterBodyRE.FindStringSubmatch(body); len(match) == 2 {
		if retryAt, err := time.Parse(time.RFC3339Nano, match[1]); err == nil {
			return boundGmailAPIRetryDelay(retryAt.Sub(now))
		}
	}
	if attempt < 1 {
		attempt = 1
	}
	delay := time.Duration(1<<uint(attempt-1)) * 150 * time.Millisecond
	return boundGmailAPIRetryDelay(delay)
}

func parseGmailAPIRetryAfter(value string, now time.Time) (time.Duration, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, false
	}
	if seconds, err := strconv.Atoi(value); err == nil {
		return time.Duration(seconds) * time.Second, true
	}
	if retryAt, err := http.ParseTime(value); err == nil {
		return retryAt.Sub(now), true
	}
	return 0, false
}

func boundGmailAPIRetryDelay(delay time.Duration) time.Duration {
	if delay < 0 {
		return 0
	}
	if delay > gmailAPIMaxRetryDelay {
		return gmailAPIMaxRetryDelay
	}
	return delay
}

func sleepWithContext(ctx context.Context, delay time.Duration) error {
	if delay <= 0 {
		return nil
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func retryableRequestBody(req *http.Request) (io.ReadCloser, error) {
	if req.Body == nil {
		return nil, nil
	}
	if req.GetBody == nil {
		return nil, fmt.Errorf("request body cannot be replayed")
	}
	return req.GetBody()
}

func isGmailAPIRetryableStatus(status int) bool {
	return status == http.StatusTooManyRequests || status >= 500
}

func trimGmailAPIErrorBody(body string) string {
	body = strings.TrimSpace(body)
	if len(body) > 240 {
		return body[:240] + "..."
	}
	return body
}

type gmailAPIHTTPError struct {
	Method     string
	Path       string
	StatusCode int
	Body       string
}

func (e gmailAPIHTTPError) Error() string {
	return fmt.Sprintf("gmail api %s %s: status %d: %s", e.Method, e.Path, e.StatusCode, e.Body)
}

// IsStaleMessageNotFoundError reports provider responses that mean a cached
// message ref no longer resolves and should be removed from the local cache.
func IsStaleMessageNotFoundError(err error) bool {
	if err == nil {
		return false
	}
	var apiErr gmailAPIHTTPError
	if errors.As(err, &apiErr) {
		return apiErr.StatusCode == http.StatusNotFound || apiErr.StatusCode == http.StatusGone
	}
	lower := strings.ToLower(err.Error())
	return strings.Contains(lower, "gmail api ") &&
		(strings.Contains(lower, "status 404") || strings.Contains(lower, "status 410")) &&
		(strings.Contains(lower, "not found") || strings.Contains(lower, "requested entity was not found"))
}

func isGmailAPIHistoryExpired(err error) bool {
	apiErr, ok := err.(gmailAPIHTTPError)
	if !ok {
		return false
	}
	return apiErr.StatusCode == http.StatusNotFound || apiErr.StatusCode == http.StatusGone || apiErr.StatusCode == http.StatusBadRequest
}

type gmailAPILabelList struct {
	Labels        []gmailAPILabel `json:"labels"`
	NextPageToken string          `json:"nextPageToken"`
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
	NextPageToken string `json:"nextPageToken"`
}

type gmailAPIHistoryList struct {
	History       []gmailAPIHistoryRecord `json:"history"`
	HistoryID     string                  `json:"historyId"`
	NextPageToken string                  `json:"nextPageToken"`
}

type gmailAPIHistoryRecord struct {
	ID              string                  `json:"id"`
	Messages        []gmailAPIHistoryMsg    `json:"messages"`
	MessagesAdded   []gmailAPIHistoryChange `json:"messagesAdded"`
	MessagesDeleted []gmailAPIHistoryChange `json:"messagesDeleted"`
	LabelsAdded     []gmailAPIHistoryChange `json:"labelsAdded"`
	LabelsRemoved   []gmailAPIHistoryChange `json:"labelsRemoved"`
}

type gmailAPIHistoryMsg struct {
	ID       string `json:"id"`
	ThreadID string `json:"threadId"`
}

type gmailAPIHistoryChange struct {
	Message  gmailAPIHistoryMsg `json:"message"`
	LabelIDs []string           `json:"labelIds"`
}

type gmailAPIDraftList struct {
	Drafts        []gmailAPIDraft `json:"drafts"`
	NextPageToken string          `json:"nextPageToken"`
}

type gmailAPIDraft struct {
	ID      string          `json:"id"`
	Message gmailAPIMessage `json:"message"`
}

type gmailAPIMessage struct {
	ID           string                 `json:"id"`
	ThreadID     string                 `json:"threadId"`
	HistoryID    string                 `json:"historyId"`
	LabelIDs     []string               `json:"labelIds"`
	InternalDate any                    `json:"internalDate"`
	SizeEstimate int                    `json:"sizeEstimate"`
	Raw          string                 `json:"raw"`
	Payload      gmailAPIMessagePayload `json:"payload"`
}

type gmailAPIMessagePayload struct {
	MIMEType string                   `json:"mimeType"`
	Filename string                   `json:"filename"`
	Headers  []gmailAPIMessageHeader  `json:"headers"`
	Body     gmailAPIMessagePartBody  `json:"body"`
	Parts    []gmailAPIMessagePayload `json:"parts"`
}

type gmailAPIMessageHeader struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type gmailAPIMessagePartBody struct {
	AttachmentID string `json:"attachmentId"`
	Size         int    `json:"size"`
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

func gmailAPIMessageMetadataBody(payload gmailAPIMessage) (*models.EmailBody, error) {
	if strings.TrimSpace(payload.Raw) != "" {
		return gmailAPIMessageBody(payload)
	}
	body := &models.EmailBody{}
	for _, header := range payload.Payload.Headers {
		value := strings.TrimSpace(header.Value)
		switch strings.ToLower(strings.TrimSpace(header.Name)) {
		case "from":
			body.From = value
		case "to":
			body.To = value
		case "cc":
			body.CC = value
		case "bcc":
			body.BCC = value
		case "subject":
			body.Subject = value
		case "message-id":
			body.MessageID = value
		case "list-unsubscribe":
			body.ListUnsubscribe = value
		case "list-unsubscribe-post":
			body.ListUnsubscribePost = value
		case "in-reply-to":
			body.InReplyTo = value
		case "references":
			body.References = value
		}
	}
	if body.MessageID == "" && strings.TrimSpace(payload.ID) != "" {
		body.MessageID = "gmail:" + strings.TrimSpace(payload.ID)
	}
	return body, nil
}

func gmailAPIMessageHasAttachments(part gmailAPIMessagePayload) bool {
	if strings.TrimSpace(part.Filename) != "" && (strings.TrimSpace(part.Body.AttachmentID) != "" || part.Body.Size > 0) {
		return true
	}
	for _, child := range part.Parts {
		if gmailAPIMessageHasAttachments(child) {
			return true
		}
	}
	return false
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

func containsAnyLabel(labels []string, wants ...string) bool {
	for _, label := range labels {
		for _, want := range wants {
			if strings.EqualFold(label, want) {
				return true
			}
		}
	}
	return false
}

func maxGmailAPIHistoryID(current, candidate string) string {
	current = strings.TrimSpace(current)
	candidate = strings.TrimSpace(candidate)
	if candidate == "" {
		return current
	}
	if current == "" {
		return candidate
	}
	currentNum, currentErr := strconv.ParseUint(current, 10, 64)
	candidateNum, candidateErr := strconv.ParseUint(candidate, 10, 64)
	if currentErr == nil && candidateErr == nil {
		if candidateNum > currentNum {
			return candidate
		}
		return current
	}
	if candidate > current {
		return candidate
	}
	return current
}

func buildGmailAPIRawMessage(from, to, cc, bcc, subject, body string) string {
	var b strings.Builder
	b.WriteString("From: " + from + "\r\n")
	b.WriteString("To: " + to + "\r\n")
	if strings.TrimSpace(cc) != "" {
		b.WriteString("Cc: " + cc + "\r\n")
	}
	if strings.TrimSpace(bcc) != "" {
		b.WriteString("Bcc: " + bcc + "\r\n")
	}
	b.WriteString("Subject: " + subject + "\r\n")
	b.WriteString("MIME-Version: 1.0\r\n")
	b.WriteString("Content-Type: text/plain; charset=utf-8\r\n")
	b.WriteString("\r\n")
	b.WriteString(body)
	return b.String()
}

func buildGmailAPIComposeRaw(req ComposeSendRequest) (string, error) {
	htmlBody, inlines, inlineErr := appsmtp.BuildInlineImages(req.MarkdownBody)
	if inlineErr != nil {
		htmlBody, _ = appsmtp.MarkdownToHTMLAndPlain(req.MarkdownBody)
		inlines = nil
	}
	_, plainText := appsmtp.MarkdownToHTMLAndPlain(req.MarkdownBody)
	if htmlBody == "" && len(req.Attachments) == 0 && len(inlines) == 0 {
		return buildGmailAPIRawMessage(req.From, req.To, req.CC, req.BCC, req.Subject, plainText), nil
	}

	outerBoundary := fmt.Sprintf("gmail_outer_%d", time.Now().UnixNano())
	relatedBoundary := fmt.Sprintf("gmail_related_%d", time.Now().UnixNano()+1)
	altBoundary := fmt.Sprintf("gmail_alt_%d", time.Now().UnixNano()+2)

	var msg strings.Builder
	msg.WriteString("From: " + req.From + "\r\n")
	msg.WriteString("To: " + req.To + "\r\n")
	if strings.TrimSpace(req.CC) != "" {
		msg.WriteString("Cc: " + req.CC + "\r\n")
	}
	if strings.TrimSpace(req.BCC) != "" {
		msg.WriteString("Bcc: " + req.BCC + "\r\n")
	}
	msg.WriteString("Subject: " + req.Subject + "\r\n")
	msg.WriteString("MIME-Version: 1.0\r\n")
	msg.WriteString(fmt.Sprintf("Content-Type: multipart/mixed; boundary=%q\r\n\r\n", outerBoundary))

	msg.WriteString(fmt.Sprintf("--%s\r\n", outerBoundary))
	msg.WriteString(fmt.Sprintf("Content-Type: multipart/related; boundary=%q\r\n\r\n", relatedBoundary))
	msg.WriteString(fmt.Sprintf("--%s\r\n", relatedBoundary))
	msg.WriteString(fmt.Sprintf("Content-Type: multipart/alternative; boundary=%q\r\n\r\n", altBoundary))
	msg.WriteString(fmt.Sprintf("--%s\r\nContent-Type: text/plain; charset=utf-8\r\n\r\n%s\r\n", altBoundary, plainText))
	if strings.TrimSpace(htmlBody) != "" {
		msg.WriteString(fmt.Sprintf("--%s\r\nContent-Type: text/html; charset=utf-8\r\n\r\n%s\r\n", altBoundary, htmlBody))
	}
	msg.WriteString(fmt.Sprintf("--%s--\r\n", altBoundary))

	for i, img := range inlines {
		filename := fmt.Sprintf("inline%03d%s", i+1, gmailAPIExtFromMIME(img.MIMEType))
		writeGmailAPIBinaryPart(&msg, relatedBoundary, img.MIMEType, "inline", filename, img.ContentID, img.Data)
	}
	msg.WriteString(fmt.Sprintf("--%s--\r\n", relatedBoundary))

	for _, att := range req.Attachments {
		if att.Filename == "" || len(att.Data) == 0 {
			continue
		}
		writeGmailAPIBinaryPart(&msg, outerBoundary, "application/octet-stream", "attachment", att.Filename, "", att.Data)
	}
	msg.WriteString(fmt.Sprintf("--%s--\r\n", outerBoundary))
	return msg.String(), nil
}

func writeGmailAPIBinaryPart(msg *strings.Builder, boundary, mimeType, disposition, filename, contentID string, data []byte) {
	if len(data) == 0 {
		return
	}
	if strings.TrimSpace(mimeType) == "" {
		mimeType = "application/octet-stream"
	}
	msg.WriteString(fmt.Sprintf("--%s\r\n", boundary))
	msg.WriteString(fmt.Sprintf("Content-Type: %s\r\n", mimeType))
	msg.WriteString("Content-Transfer-Encoding: base64\r\n")
	if strings.TrimSpace(contentID) != "" {
		msg.WriteString(fmt.Sprintf("Content-ID: <%s>\r\n", contentID))
	}
	msg.WriteString(fmt.Sprintf("Content-Disposition: %s; filename=%q\r\n\r\n", disposition, filename))
	encoded := base64.StdEncoding.EncodeToString(data)
	for len(encoded) > 76 {
		msg.WriteString(encoded[:76] + "\r\n")
		encoded = encoded[76:]
	}
	if encoded != "" {
		msg.WriteString(encoded + "\r\n")
	}
}

func insertGmailAPIBccHeader(raw, bcc string) string {
	bcc = strings.TrimSpace(bcc)
	if bcc == "" || strings.Contains(strings.ToLower(raw), "\r\nbcc:") {
		return raw
	}
	if idx := strings.Index(raw, "\r\nSubject:"); idx >= 0 {
		return raw[:idx] + "\r\nBcc: " + bcc + raw[idx:]
	}
	if idx := strings.Index(raw, "\r\nMIME-Version:"); idx >= 0 {
		return raw[:idx] + "\r\nBcc: " + bcc + raw[idx:]
	}
	return "Bcc: " + bcc + "\r\n" + raw
}

func gmailAPIExtFromMIME(mimeType string) string {
	switch strings.ToLower(strings.TrimSpace(mimeType)) {
	case "image/jpeg":
		return ".jpg"
	case "image/png":
		return ".png"
	case "image/gif":
		return ".gif"
	case "image/webp":
		return ".webp"
	default:
		return ".bin"
	}
}

func (s *GmailAPIMailSource) fetchDraft(ctx context.Context, draftID string) (gmailAPIDraft, error) {
	req, err := s.newRequest(ctx, http.MethodGet, s.baseURL+"/users/me/drafts/"+url.PathEscape(draftID)+"?format=raw", nil)
	if err != nil {
		return gmailAPIDraft{}, err
	}
	var draft gmailAPIDraft
	if err := s.doJSON(req, &draft); err != nil {
		return gmailAPIDraft{}, err
	}
	if draft.ID == "" {
		draft.ID = draftID
	}
	return draft, nil
}

func (s *GmailAPIMailSource) rememberDraft(uid uint32, draftID string) {
	if s.drafts == nil {
		s.drafts = make(map[uint32]string)
	}
	if draftID != "" {
		s.drafts[uid] = draftID
	}
}

func (s *GmailAPIMailSource) draftIDForUID(ctx context.Context, uid uint32, folder string) (string, error) {
	if s.drafts != nil {
		if draftID := strings.TrimSpace(s.drafts[uid]); draftID != "" {
			return draftID, nil
		}
	}
	messageID := ""
	if s.cache != nil {
		if email, err := s.cache.GetEmailByFolderUID(folder, uid); err == nil && email != nil {
			messageID = gmailIDFromLocalID(email.LocalID)
		}
	}
	if messageID == "" {
		return "", fmt.Errorf("Gmail API draft id unavailable for uid %d in %s", uid, folder)
	}
	draftID, err := s.draftIDForMessageID(ctx, messageID)
	if err != nil {
		return "", err
	}
	s.rememberDraft(uid, draftID)
	return draftID, nil
}

func (s *GmailAPIMailSource) draftIDForMessageID(ctx context.Context, messageID string) (string, error) {
	req, err := s.newRequest(ctx, http.MethodGet, s.baseURL+"/users/me/drafts", nil)
	if err != nil {
		return "", err
	}
	var list gmailAPIDraftList
	if err := s.doJSON(req, &list); err != nil {
		return "", err
	}
	for _, item := range list.Drafts {
		draftID := strings.TrimSpace(item.ID)
		if draftID == "" {
			continue
		}
		msgID := strings.TrimSpace(item.Message.ID)
		if msgID == "" {
			draft, err := s.fetchDraft(ctx, draftID)
			if err != nil {
				return "", err
			}
			msgID = strings.TrimSpace(draft.Message.ID)
		}
		if msgID == messageID {
			return draftID, nil
		}
	}
	return "", fmt.Errorf("Gmail API draft for message %s not found", messageID)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
