package backend

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/mail"
	"net/url"
	"path"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/herald-email/herald-mail-app/internal/cache"
	"github.com/herald-email/herald-mail-app/internal/config"
	"github.com/herald-email/herald-mail-app/internal/models"
	appsmtp "github.com/herald-email/herald-mail-app/internal/smtp"
)

func TestSourceRegistryOpensGmailAPIMailSource(t *testing.T) {
	c, err := cache.New(path.Join(t.TempDir(), "gmail-api-source.db"))
	if err != nil {
		t.Fatalf("cache.New: %v", err)
	}
	defer c.Close()

	registry := DefaultSourceRegistry()
	opened, err := registry.Open(context.Background(), config.SourceConfig{
		ID:          "work-gmail",
		Kind:        string(models.SourceKindMail),
		Provider:    "gmail",
		DisplayName: "Work Gmail",
		AccountID:   "work",
		Google:      config.GoogleConfig{Email: "work@example.com", AccessToken: "token"},
	}, SourceDeps{Cache: c})
	if err != nil {
		t.Fatalf("Open Gmail API source: %v", err)
	}
	defer opened.Close()

	if opened.Provider != "gmail" || opened.Mail == nil || !opened.Caps.MailSync || !opened.Caps.MailMutations || !opened.Caps.CacheBypassReads {
		t.Fatalf("opened source = %#v, want Gmail API mail source with sync/mutation/cache-bypass capabilities", opened)
	}
}

func TestSourceRegistryKeepsGmailAPIAlias(t *testing.T) {
	c, err := cache.New(path.Join(t.TempDir(), "gmail-api-alias.db"))
	if err != nil {
		t.Fatalf("cache.New: %v", err)
	}
	defer c.Close()

	opened, err := DefaultSourceRegistry().Open(context.Background(), config.SourceConfig{
		ID:        "legacy-gmail-api",
		Kind:      string(models.SourceKindMail),
		Provider:  "gmail_api",
		AccountID: "legacy",
		Google:    config.GoogleConfig{Email: "legacy@example.com", AccessToken: "token"},
	}, SourceDeps{Cache: c})
	if err != nil {
		t.Fatalf("Open gmail_api alias: %v", err)
	}
	defer opened.Close()

	if opened.Provider != "gmail" || opened.Mail == nil {
		t.Fatalf("opened alias source = %#v, want canonical Gmail API provider", opened)
	}
}

func TestIsStaleMessageNotFoundErrorRecognizesGmailHTTPNotFound(t *testing.T) {
	notFound := gmailAPIHTTPError{
		Method:     http.MethodGet,
		Path:       "/gmail/v1/users/me/messages/missing",
		StatusCode: http.StatusNotFound,
		Body:       `{ "error": { "message": "Requested entity was not found." } }`,
	}
	if !IsStaleMessageNotFoundError(notFound) {
		t.Fatalf("expected Gmail API 404 to be treated as stale message ref")
	}

	rateLimited := gmailAPIHTTPError{
		Method:     http.MethodGet,
		Path:       "/gmail/v1/users/me/messages/slow",
		StatusCode: http.StatusTooManyRequests,
		Body:       `{ "error": { "message": "Rate limited." } }`,
	}
	if IsStaleMessageNotFoundError(rateLimited) {
		t.Fatalf("rate limit should not prune cached email")
	}
}

func TestGmailAPIMailSourceEmailDataFromMessageCarriesThreadMetadata(t *testing.T) {
	source := &GmailAPIMailSource{
		id:        models.SourceID("gmail-api"),
		accountID: models.AccountID("work"),
	}
	internalDate := "1770000000000"

	email, err := source.emailDataFromMessage("INBOX", gmailAPIMessage{
		ID:           "msg-1",
		ThreadID:     "thread-123",
		InternalDate: &internalDate,
		Payload: gmailAPIMessagePayload{
			Headers: []gmailAPIMessageHeader{
				{Name: "Message-ID", Value: "<reply@example.test>"},
				{Name: "In-Reply-To", Value: "<original@example.test>"},
				{Name: "References", Value: "<root@example.test> <original@example.test>"},
				{Name: "From", Value: "me@example.test"},
				{Name: "Subject", Value: "Re: Gmail API thread"},
			},
		},
	})
	if err != nil {
		t.Fatalf("emailDataFromMessage: %v", err)
	}
	if email.ProviderThreadID != "thread-123" {
		t.Fatalf("ProviderThreadID = %q, want Gmail thread id", email.ProviderThreadID)
	}
	if email.InReplyTo != "<original@example.test>" || email.References != "<root@example.test> <original@example.test>" {
		t.Fatalf("thread headers = (%q, %q), want Gmail metadata headers", email.InReplyTo, email.References)
	}
}

func TestGmailAPIMailSourceSyncFetchMutateAndSend(t *testing.T) {
	fake := newFakeGmailAPIServer(t)
	defer fake.Close()

	c, err := cache.New(path.Join(t.TempDir(), "gmail-api-core.db"))
	if err != nil {
		t.Fatalf("cache.New: %v", err)
	}
	defer c.Close()

	source := newTestGmailAPIMailSource(t, fake.URL(), c)
	if err := source.Connect(context.Background()); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer source.Close()

	folders, err := source.ListFolders(context.Background())
	if err != nil {
		t.Fatalf("ListFolders: %v", err)
	}
	for _, want := range []string{"INBOX", "STARRED", "All Mail"} {
		if !containsString(folders, want) {
			t.Fatalf("folders = %#v, missing %q", folders, want)
		}
	}

	if err := source.ProcessEmailsIncremental(context.Background(), "INBOX"); err != nil {
		t.Fatalf("ProcessEmailsIncremental: %v", err)
	}
	emails, err := c.GetEmailsSortedByDate("INBOX")
	if err != nil {
		t.Fatalf("GetEmailsSortedByDate: %v", err)
	}
	if len(emails) != 1 {
		t.Fatalf("cached emails = %d, want 1", len(emails))
	}
	email := emails[0]
	if email.SourceID != "gmail-api" || email.AccountID != "work" || email.MessageID != "<api-1@example.com>" || email.IsRead || !email.IsStarred || email.LocalID == "" {
		t.Fatalf("cached email = %#v, want scoped unread/starred Gmail API row", email)
	}

	body, err := source.FetchMessageNoCache(context.Background(), email.MessageRef())
	if err != nil {
		t.Fatalf("FetchMessageNoCache: %v", err)
	}
	if body.MessageID != "<api-1@example.com>" || !strings.Contains(body.TextPlain, "hello from gmail api") {
		t.Fatalf("body = %#v, want parsed raw MIME body", body)
	}

	if _, err := source.SearchIMAP(context.Background(), "INBOX", "roadmap"); err != nil {
		t.Fatalf("SearchIMAP: %v", err)
	}
	if err := source.MarkRead(context.Background(), email.UID, "INBOX"); err != nil {
		t.Fatalf("MarkRead: %v", err)
	}
	if err := source.MarkUnread(context.Background(), email.UID, "INBOX"); err != nil {
		t.Fatalf("MarkUnread: %v", err)
	}
	if err := source.UnmarkStarred(context.Background(), email.UID, "INBOX"); err != nil {
		t.Fatalf("UnmarkStarred: %v", err)
	}
	if err := source.MarkStarred(context.Background(), email.UID, "INBOX"); err != nil {
		t.Fatalf("MarkStarred: %v", err)
	}
	if err := source.ArchiveEmail(context.Background(), email.MessageID, "INBOX"); err != nil {
		t.Fatalf("ArchiveEmail: %v", err)
	}
	if err := source.MoveEmail(context.Background(), email.MessageID, "INBOX", "Later"); err != nil {
		t.Fatalf("MoveEmail: %v", err)
	}
	if err := source.DeleteEmail(context.Background(), email.MessageID, "INBOX"); err != nil {
		t.Fatalf("DeleteEmail: %v", err)
	}
	if err := source.SendEmail(context.Background(), "me@example.com", "you@example.com", "Gmail API send", "hello"); err != nil {
		t.Fatalf("SendEmail: %v", err)
	}

	fake.assertModify(t, "msg-1", []string(nil), []string{"UNREAD"})
	fake.assertModify(t, "msg-1", []string{"UNREAD"}, []string(nil))
	fake.assertModify(t, "msg-1", []string(nil), []string{"STARRED"})
	fake.assertModify(t, "msg-1", []string{"STARRED"}, []string(nil))
	fake.assertModify(t, "msg-1", []string(nil), []string{"INBOX"})
	fake.assertModify(t, "msg-1", []string{"Label_42"}, []string{"INBOX"})
	if !fake.sawTrash("msg-1") {
		t.Fatalf("expected DeleteEmail to call Gmail trash endpoint, got calls %#v", fake.calls)
	}
	if fake.sentRaw == "" {
		t.Fatal("expected SendEmail to post raw RFC 2822 message")
	}
	decoded, err := base64.RawURLEncoding.DecodeString(fake.sentRaw)
	if err != nil {
		t.Fatalf("decode sent raw: %v", err)
	}
	if sent := string(decoded); !strings.Contains(sent, "Subject: Gmail API send") || !strings.Contains(sent, "hello") {
		t.Fatalf("sent raw message = %q, want subject and body", sent)
	}
}

func TestGmailAPIMailSourceDraftCreateListDeleteAndSend(t *testing.T) {
	fake := newFakeGmailAPIServer(t)
	defer fake.Close()

	c, err := cache.New(path.Join(t.TempDir(), "gmail-api-drafts.db"))
	if err != nil {
		t.Fatalf("cache.New: %v", err)
	}
	defer c.Close()

	source := newTestGmailAPIMailSource(t, fake.URL(), c)
	raw := []byte("From: Me <me@example.com>\r\nTo: You <you@example.com>\r\nCc: Copy <copy@example.com>\r\nSubject: Draft API\r\nContent-Type: text/plain; charset=utf-8\r\n\r\ndraft body")
	uid, folder, err := source.AppendDraft(context.Background(), raw)
	if err != nil {
		t.Fatalf("AppendDraft: %v", err)
	}
	if uid == 0 || folder != "DRAFT" {
		t.Fatalf("AppendDraft = (%d, %q), want nonzero DRAFT", uid, folder)
	}
	if fake.draftRaw == "" {
		t.Fatal("AppendDraft did not post raw draft MIME")
	}

	drafts, err := source.ListDrafts(context.Background())
	if err != nil {
		t.Fatalf("ListDrafts: %v", err)
	}
	if len(drafts) != 1 || drafts[0].UID != uid || drafts[0].Subject != "Draft API" || drafts[0].To == "" || drafts[0].CC == "" || !strings.Contains(drafts[0].Body, "draft body") {
		t.Fatalf("drafts = %#v, want parsed Gmail API draft", drafts)
	}

	if err := source.DeleteDraft(context.Background(), uid, folder); err != nil {
		t.Fatalf("DeleteDraft: %v", err)
	}
	if fake.deletedDraft != "draft-1" {
		t.Fatalf("deleted draft = %q, want draft-1", fake.deletedDraft)
	}

	uid, folder, err = source.AppendDraft(context.Background(), raw)
	if err != nil {
		t.Fatalf("AppendDraft for send: %v", err)
	}
	if err := source.SendDraft(context.Background(), uid, folder); err != nil {
		t.Fatalf("SendDraft: %v", err)
	}
	if fake.sentDraft != "draft-1" {
		t.Fatalf("sent draft = %q, want draft-1", fake.sentDraft)
	}
}

func TestGmailAPIMailSourceSendComposePreserved(t *testing.T) {
	fake := newFakeGmailAPIServer(t)
	defer fake.Close()

	c, err := cache.New(path.Join(t.TempDir(), "gmail-api-preserved.db"))
	if err != nil {
		t.Fatalf("cache.New: %v", err)
	}
	defer c.Close()

	source := newTestGmailAPIMailSource(t, fake.URL(), c)
	err = source.SendCompose(context.Background(), ComposeSendRequest{
		From:    "me@example.com",
		To:      "you@example.com",
		Subject: "Re: Preserved API",
		Preserved: &appsmtp.PreservedMessageRequest{
			Kind:            models.PreservedMessageKindReply,
			From:            "me@example.com",
			To:              "you@example.com",
			Subject:         "Re: Preserved API",
			TopNoteMarkdown: "Thanks for the note.",
			Original: models.PreservedMessageOriginal{
				MessageID: "<original@example.com>",
				Sender:    "you@example.com",
				Subject:   "Preserved API",
				TextPlain: "original body",
			},
		},
	})
	if err != nil {
		t.Fatalf("SendCompose preserved: %v", err)
	}
	if fake.sentRaw == "" {
		t.Fatal("expected Gmail API preserved send to post raw MIME")
	}
	decoded, err := base64.RawURLEncoding.DecodeString(fake.sentRaw)
	if err != nil {
		t.Fatalf("decode sent raw: %v", err)
	}
	sent := string(decoded)
	for _, want := range []string{
		"From: me@example.com",
		"To: you@example.com",
		"Subject: Re: Preserved API",
		"In-Reply-To: <original@example.com>",
		"Thanks for the note.",
		"original body",
	} {
		if !strings.Contains(sent, want) {
			t.Fatalf("sent raw missing %q:\n%s", want, sent)
		}
	}
}

func TestGmailAPIMailSourceHistoryDeltaAndExpiredCursorFallback(t *testing.T) {
	fake := newFakeGmailAPIServer(t)
	defer fake.Close()

	c, err := cache.New(path.Join(t.TempDir(), "gmail-api-history.db"))
	if err != nil {
		t.Fatalf("cache.New: %v", err)
	}
	defer c.Close()

	source := newTestGmailAPIMailSource(t, fake.URL(), c)
	if err := source.ProcessEmailsIncremental(context.Background(), "INBOX"); err != nil {
		t.Fatalf("initial ProcessEmailsIncremental: %v", err)
	}
	cursor, found, err := c.GetMetadata(source.historyCursorKey("INBOX"))
	if err != nil {
		t.Fatalf("GetMetadata initial cursor: %v", err)
	}
	if !found || cursor != "101" {
		t.Fatalf("initial cursor = (%q, %v), want 101 true", cursor, found)
	}

	fake.resetCalls()
	fake.listIDs = []string{"msg-1"}
	fake.messagePayloads["msg-2"] = gmailMessagePayloadFor("msg-2", "<api-2@example.com>", []string{"INBOX", "UNREAD"}, "102")
	fake.messagePayloads["msg-3"] = gmailMessagePayloadFor("msg-3", "<api-3@example.com>", []string{"INBOX", "STARRED"}, "103")
	fake.historyResponse = map[string]any{
		"history": []map[string]any{
			{"messagesAdded": []map[string]any{{"message": map[string]string{"id": "msg-2"}}}},
			{"labelsRemoved": []map[string]any{{"message": map[string]string{"id": "msg-1"}, "labelIds": []string{"INBOX"}}}},
			{"labelsAdded": []map[string]any{{"message": map[string]string{"id": "msg-3"}, "labelIds": []string{"STARRED"}}}},
		},
		"historyId": "105",
	}
	if err := source.ProcessEmailsIncremental(context.Background(), "INBOX"); err != nil {
		t.Fatalf("history ProcessEmailsIncremental: %v", err)
	}
	if !fake.sawPath("/gmail/v1/users/me/history") {
		t.Fatalf("expected history endpoint, got calls %#v", fake.calls)
	}
	emails, err := c.GetEmailsSortedByDate("INBOX")
	if err != nil {
		t.Fatalf("GetEmailsSortedByDate after history: %v", err)
	}
	if got := messageIDSet(emails); !got["<api-2@example.com>"] || !got["<api-3@example.com>"] || got["<api-1@example.com>"] {
		t.Fatalf("history cache message set = %#v, want msg-2/msg-3 only", got)
	}
	cursor, _, err = c.GetMetadata(source.historyCursorKey("INBOX"))
	if err != nil {
		t.Fatalf("GetMetadata updated cursor: %v", err)
	}
	if cursor != "105" {
		t.Fatalf("updated cursor = %q, want 105", cursor)
	}

	fake.resetCalls()
	fake.historyResponse = map[string]any{
		"history": []map[string]any{
			{"messagesDeleted": []map[string]any{{"message": map[string]string{"id": "msg-2"}}}},
			{"labelsAdded": []map[string]any{{"message": map[string]string{"id": "msg-3"}, "labelIds": []string{"TRASH"}}}},
		},
		"historyId": "106",
	}
	if err := source.ProcessEmailsIncremental(context.Background(), "INBOX"); err != nil {
		t.Fatalf("history delete/trash sync: %v", err)
	}
	emails, err = c.GetEmailsSortedByDate("INBOX")
	if err != nil {
		t.Fatalf("GetEmailsSortedByDate after delete/trash: %v", err)
	}
	if len(emails) != 0 {
		t.Fatalf("emails after delete/trash = %#v, want empty", emails)
	}

	if err := c.SetMetadata(source.historyCursorKey("INBOX"), "1"); err != nil {
		t.Fatalf("SetMetadata expired cursor: %v", err)
	}
	fake.resetCalls()
	fake.historyStatus = http.StatusNotFound
	fake.listIDs = []string{"msg-4"}
	fake.messagePayloads["msg-4"] = gmailMessagePayloadFor("msg-4", "<api-4@example.com>", []string{"INBOX"}, "200")
	if err := source.ProcessEmailsIncremental(context.Background(), "INBOX"); err != nil {
		t.Fatalf("expired cursor fallback sync: %v", err)
	}
	if !fake.sawPath("/gmail/v1/users/me/history") || !fake.sawPath("/gmail/v1/users/me/messages") {
		t.Fatalf("expected history attempt then messages fallback, got calls %#v", fake.calls)
	}
	emails, err = c.GetEmailsSortedByDate("INBOX")
	if err != nil {
		t.Fatalf("GetEmailsSortedByDate after fallback: %v", err)
	}
	if len(emails) != 1 || emails[0].MessageID != "<api-4@example.com>" {
		t.Fatalf("fallback emails = %#v, want msg-4", emails)
	}
	cursor, _, err = c.GetMetadata(source.historyCursorKey("INBOX"))
	if err != nil {
		t.Fatalf("GetMetadata fallback cursor: %v", err)
	}
	if cursor != "200" {
		t.Fatalf("fallback cursor = %q, want 200", cursor)
	}
}

func TestGmailAPIMailSourceColdSyncUsesBoundedMetadataFetches(t *testing.T) {
	fake := newFakeGmailAPIServer(t)
	defer fake.Close()
	fake.listIDs = []string{"msg-1", "msg-2"}
	fake.messagePayloads["msg-2"] = gmailMessagePayloadFor("msg-2", "<api-2@example.com>", []string{"INBOX"}, "102")

	c, err := cache.New(path.Join(t.TempDir(), "gmail-api-bounded-metadata.db"))
	if err != nil {
		t.Fatalf("cache.New: %v", err)
	}
	defer c.Close()
	source := newTestGmailAPIMailSource(t, fake.URL(), c)

	if err := source.ProcessEmailsIncremental(context.Background(), "INBOX"); err != nil {
		t.Fatalf("ProcessEmailsIncremental: %v", err)
	}

	listCalls := fake.callsForPath("/gmail/v1/users/me/messages")
	if len(listCalls) != 1 {
		t.Fatalf("message list calls = %d, want one first-page bounded list call: %#v", len(listCalls), fake.calls)
	}
	if got := listCalls[0].Query.Get("maxResults"); got == "" {
		t.Fatalf("message list maxResults was empty, want bounded cold-sync page size")
	} else if n, err := strconv.Atoi(got); err != nil || n <= 0 || n > 100 {
		t.Fatalf("message list maxResults = %q, want 1..100", got)
	}

	for _, call := range fake.calls {
		if call.Method != http.MethodGet || !strings.HasPrefix(call.Path, "/gmail/v1/users/me/messages/") {
			continue
		}
		if got := call.Query.Get("format"); got == "raw" {
			t.Fatalf("cold sync fetched raw body for %s; want metadata/header fetch only", call.Path)
		}
		if got := call.Query.Get("format"); got != "metadata" {
			t.Fatalf("cold sync format for %s = %q, want metadata", call.Path, got)
		}
	}

	emails, err := c.GetEmailsSortedByDate("INBOX")
	if err != nil {
		t.Fatalf("GetEmailsSortedByDate: %v", err)
	}
	if len(emails) != 2 {
		t.Fatalf("cached emails = %d, want 2", len(emails))
	}
	if emails[0].Subject == "" || emails[0].Sender == "" || emails[0].MessageID == "" {
		t.Fatalf("metadata cached email missing sender/subject/message id: %#v", emails[0])
	}
}

func TestGmailAPIMailSourceRetryHonorsRetryAfterHeader(t *testing.T) {
	fake := newFakeGmailAPIServer(t)
	defer fake.Close()
	fake.retryOnce = map[string]bool{"/gmail/v1/users/me/labels": true}
	fake.retryAfter = map[string]string{"/gmail/v1/users/me/labels": "1"}

	c, err := cache.New(path.Join(t.TempDir(), "gmail-api-retry-after.db"))
	if err != nil {
		t.Fatalf("cache.New: %v", err)
	}
	defer c.Close()
	source := newTestGmailAPIMailSource(t, fake.URL(), c)

	started := time.Now()
	if _, err := source.ListFolders(context.Background()); err != nil {
		t.Fatalf("ListFolders: %v", err)
	}
	if elapsed := time.Since(started); elapsed < 900*time.Millisecond {
		t.Fatalf("ListFolders retry elapsed = %s, want Retry-After delay to be honored", elapsed)
	}
}

func TestGmailAPIMailSourcePaginationRetryAndComposeMIME(t *testing.T) {
	fake := newFakeGmailAPIServer(t)
	defer fake.Close()
	fake.labelPages = []fakeGmailLabelPage{
		{Labels: []map[string]string{{"id": "INBOX", "name": "INBOX", "type": "system"}}, Next: "labels-2"},
		{Labels: []map[string]string{{"id": "Label_99", "name": "Projects", "type": "user"}}},
	}
	fake.messagePages = []fakeGmailIDPage{
		{IDs: []string{"msg-1"}, Next: "messages-2"},
		{IDs: []string{"msg-2"}},
	}
	fake.messagePayloads["msg-2"] = gmailMessagePayloadFor("msg-2", "<api-2@example.com>", []string{"INBOX"}, "102")
	fake.draftPages = []fakeGmailIDPage{
		{IDs: []string{"draft-1"}, Next: "drafts-2"},
		{IDs: []string{"draft-2"}},
	}
	fake.historyPages = []map[string]any{
		{
			"history":       []map[string]any{{"messagesDeleted": []map[string]any{{"message": map[string]string{"id": "msg-1"}}}}},
			"historyId":     "201",
			"nextPageToken": "history-2",
		},
		{
			"history":   []map[string]any{{"messagesDeleted": []map[string]any{{"message": map[string]string{"id": "msg-2"}}}}},
			"historyId": "202",
		},
	}
	fake.retryOnce = map[string]bool{"/gmail/v1/users/me/messages": true}

	c, err := cache.New(path.Join(t.TempDir(), "gmail-api-hardening.db"))
	if err != nil {
		t.Fatalf("cache.New: %v", err)
	}
	defer c.Close()
	source := newTestGmailAPIMailSource(t, fake.URL(), c)

	folders, err := source.ListFolders(context.Background())
	if err != nil {
		t.Fatalf("ListFolders: %v", err)
	}
	if !containsString(folders, "Projects") {
		t.Fatalf("folders = %#v, want paginated user label", folders)
	}
	if err := source.ProcessEmailsIncremental(context.Background(), "INBOX"); err != nil {
		t.Fatalf("ProcessEmailsIncremental paginated/retry: %v", err)
	}
	emails, err := c.GetEmailsSortedByDate("INBOX")
	if err != nil {
		t.Fatalf("GetEmailsSortedByDate: %v", err)
	}
	if got := messageIDSet(emails); !got["<api-1@example.com>"] || !got["<api-2@example.com>"] {
		t.Fatalf("cached paginated messages = %#v, want msg-1 and msg-2", got)
	}
	if fake.pathCallCount("/gmail/v1/users/me/messages") < 3 {
		t.Fatalf("message list calls = %#v, want retry plus second page", fake.calls)
	}

	drafts, err := source.ListDrafts(context.Background())
	if err != nil {
		t.Fatalf("ListDrafts paginated: %v", err)
	}
	if len(drafts) != 2 {
		t.Fatalf("drafts = %d, want 2", len(drafts))
	}
	if err := c.SetMetadata(source.historyCursorKey("INBOX"), "200"); err != nil {
		t.Fatalf("SetMetadata: %v", err)
	}
	if err := source.ProcessEmailsIncremental(context.Background(), "INBOX"); err != nil {
		t.Fatalf("ProcessEmailsIncremental history pagination: %v", err)
	}
	emails, err = c.GetEmailsSortedByDate("INBOX")
	if err != nil {
		t.Fatalf("GetEmailsSortedByDate after history pagination: %v", err)
	}
	if len(emails) != 0 {
		t.Fatalf("emails after paginated history delete = %#v, want empty", emails)
	}

	raw, err := buildGmailAPIComposeRaw(ComposeSendRequest{
		From:         "Me <me@example.com>",
		To:           "You <you@example.com>",
		CC:           "Copy <copy@example.com>",
		BCC:          "Blind <blind@example.com>",
		Subject:      "MIME parity",
		MarkdownBody: "hello",
		Attachments:  []models.ComposeAttachment{{Filename: "note.txt", Data: []byte("hello attachment")}},
	})
	if err != nil {
		t.Fatalf("buildGmailAPIComposeRaw: %v", err)
	}
	for _, want := range []string{"Cc: Copy <copy@example.com>", "Bcc: Blind <blind@example.com>", `Content-Disposition: attachment; filename="note.txt"`, base64.StdEncoding.EncodeToString([]byte("hello attachment"))} {
		if !strings.Contains(raw, want) {
			t.Fatalf("raw MIME missing %q:\n%s", want, raw)
		}
	}
}

func TestBuildGmailAPIComposeRaw_EncodesNonASCIIDisplayNameHeaders(t *testing.T) {
	raw, err := buildGmailAPIComposeRaw(ComposeSendRequest{
		From:         "Me <me@example.com>",
		To:           "Любимая Катюшка <evgolubtsova@gmail.com>",
		CC:           "Команда <team@example.com>",
		BCC:          "Скрытая Копия <blind@example.com>",
		Subject:      "MIME parity",
		MarkdownBody: "hello",
	})
	if err != nil {
		t.Fatalf("buildGmailAPIComposeRaw: %v", err)
	}
	msg, err := mail.ReadMessage(strings.NewReader(raw))
	if err != nil {
		t.Fatalf("ReadMessage: %v", err)
	}
	for _, field := range []string{"To", "Cc", "Bcc"} {
		header := msg.Header.Get(field)
		if strings.ContainsAny(header, "ЛюбКомандСкрыт") {
			t.Fatalf("%s header contains raw UTF-8 display name; want RFC 2047 encoded word: %q", field, header)
		}
		if !strings.Contains(strings.ToLower(header), "=?utf-8?") {
			t.Fatalf("%s header = %q, want UTF-8 encoded word", field, header)
		}
	}
	to, err := mail.ParseAddress(msg.Header.Get("To"))
	if err != nil {
		t.Fatalf("ParseAddress(To): %v", err)
	}
	if to.Name != "Любимая Катюшка" || to.Address != "evgolubtsova@gmail.com" {
		t.Fatalf("decoded To = %q <%s>, want Russian display name and address", to.Name, to.Address)
	}
}

func newTestGmailAPIMailSource(t *testing.T, baseURL string, c *cache.Cache) *GmailAPIMailSource {
	t.Helper()
	opened, err := (GmailAPISourcePlugin{}).Open(context.Background(), config.SourceConfig{
		ID:        "gmail-api",
		Kind:      string(models.SourceKindMail),
		Provider:  "gmail",
		AccountID: "work",
		Google: config.GoogleConfig{
			Email:       "me@example.com",
			AccessToken: "test-token",
			APIBaseURL:  baseURL + "/gmail/v1",
		},
	}, SourceDeps{Cache: c})
	if err != nil {
		t.Fatalf("open Gmail API source: %v", err)
	}
	source, ok := opened.Mail.(*GmailAPIMailSource)
	if !ok {
		t.Fatalf("opened Mail = %T, want *GmailAPIMailSource", opened.Mail)
	}
	return source
}

type fakeGmailAPIServer struct {
	server          *httptest.Server
	calls           []fakeGmailCall
	sentRaw         string
	draftRaw        string
	deletedDraft    string
	sentDraft       string
	listIDs         []string
	messagePayloads map[string]map[string]any
	historyResponse map[string]any
	historyStatus   int
	labelPages      []fakeGmailLabelPage
	messagePages    []fakeGmailIDPage
	draftPages      []fakeGmailIDPage
	historyPages    []map[string]any
	retryOnce       map[string]bool
	retryAfter      map[string]string
	retried         map[string]bool
}

type fakeGmailIDPage struct {
	IDs  []string
	Next string
}

type fakeGmailLabelPage struct {
	Labels []map[string]string
	Next   string
}

type fakeGmailCall struct {
	Method string
	Path   string
	Query  url.Values
	Body   map[string][]string
}

func newFakeGmailAPIServer(t *testing.T) *fakeGmailAPIServer {
	t.Helper()
	fake := &fakeGmailAPIServer{
		listIDs: []string{"msg-1"},
		messagePayloads: map[string]map[string]any{
			"msg-1": gmailMessagePayload(),
		},
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/gmail/v1/users/me/labels", fake.handleLabels)
	mux.HandleFunc("/gmail/v1/users/me/history", fake.handleHistory)
	mux.HandleFunc("/gmail/v1/users/me/messages", fake.handleMessages)
	mux.HandleFunc("/gmail/v1/users/me/messages/", fake.handleMessage)
	mux.HandleFunc("/gmail/v1/users/me/drafts", fake.handleDrafts)
	mux.HandleFunc("/gmail/v1/users/me/drafts/", fake.handleDraft)
	mux.HandleFunc("/gmail/v1/users/me/drafts/send", fake.handleDraftSend)
	fake.server = httptest.NewServer(mux)
	return fake
}

func (f *fakeGmailAPIServer) URL() string { return f.server.URL }
func (f *fakeGmailAPIServer) Close()      { f.server.Close() }

func (f *fakeGmailAPIServer) resetCalls() { f.calls = nil }

func (f *fakeGmailAPIServer) record(r *http.Request, body map[string][]string) {
	f.calls = append(f.calls, fakeGmailCall{Method: r.Method, Path: r.URL.Path, Query: r.URL.Query(), Body: body})
}

func (f *fakeGmailAPIServer) handleLabels(w http.ResponseWriter, r *http.Request) {
	f.record(r, nil)
	if f.shouldRetryOnce(w, r.URL.Path) {
		return
	}
	if len(f.labelPages) > 0 {
		page := f.labelPages[pageIndex(r)]
		writeJSON(w, map[string]any{"labels": page.Labels, "nextPageToken": page.Next})
		return
	}
	writeJSON(w, map[string]any{"labels": []map[string]string{
		{"id": "INBOX", "name": "INBOX", "type": "system"},
		{"id": "STARRED", "name": "STARRED", "type": "system"},
		{"id": "CATEGORY_PERSONAL", "name": "CATEGORY_PERSONAL", "type": "system"},
		{"id": "Label_42", "name": "Later", "type": "user"},
	}})
}

func (f *fakeGmailAPIServer) handleMessages(w http.ResponseWriter, r *http.Request) {
	body := readModifyBody(r)
	f.record(r, body)
	if f.shouldRetryOnce(w, r.URL.Path) {
		return
	}
	if r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/send") {
		f.sentRaw = firstBodyValue(body, "raw")
		writeJSON(w, map[string]string{"id": "sent-1"})
		return
	}
	if len(f.messagePages) > 0 {
		page := f.messagePages[pageIndex(r)]
		messages := make([]map[string]string, 0, len(page.IDs))
		for _, id := range page.IDs {
			messages = append(messages, map[string]string{"id": id, "threadId": "thread-" + id})
		}
		writeJSON(w, map[string]any{"messages": messages, "nextPageToken": page.Next})
		return
	}
	messages := make([]map[string]string, 0, len(f.listIDs))
	for _, id := range f.listIDs {
		messages = append(messages, map[string]string{"id": id, "threadId": "thread-" + id})
	}
	writeJSON(w, map[string]any{"messages": messages})
}

func (f *fakeGmailAPIServer) handleMessage(w http.ResponseWriter, r *http.Request) {
	body := readModifyBody(r)
	f.record(r, body)
	switch {
	case strings.HasSuffix(r.URL.Path, "/send"):
		f.sentRaw = firstBodyValue(body, "raw")
		writeJSON(w, map[string]string{"id": "sent-1"})
	case strings.HasSuffix(r.URL.Path, "/modify"):
		writeJSON(w, map[string]string{"id": "msg-1"})
	case strings.HasSuffix(r.URL.Path, "/trash"):
		writeJSON(w, map[string]string{"id": "msg-1"})
	default:
		id := path.Base(r.URL.Path)
		if payload := f.messagePayloads[id]; payload != nil {
			if r.URL.Query().Get("format") == "metadata" {
				writeJSON(w, gmailMetadataPayloadFrom(payload))
				return
			}
			writeJSON(w, payload)
			return
		}
		if r.URL.Query().Get("format") == "metadata" {
			writeJSON(w, gmailMetadataPayloadFrom(gmailMessagePayload()))
			return
		}
		writeJSON(w, gmailMessagePayload())
	}
}

func (f *fakeGmailAPIServer) handleHistory(w http.ResponseWriter, r *http.Request) {
	f.record(r, nil)
	if f.shouldRetryOnce(w, r.URL.Path) {
		return
	}
	if f.historyStatus != 0 {
		http.Error(w, "history cursor expired", f.historyStatus)
		return
	}
	if len(f.historyPages) > 0 {
		writeJSON(w, f.historyPages[pageIndex(r)])
		return
	}
	if f.historyResponse != nil {
		writeJSON(w, f.historyResponse)
		return
	}
	writeJSON(w, map[string]string{"historyId": "101"})
}

func (f *fakeGmailAPIServer) handleDrafts(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		raw := readRawDraftBody(r)
		f.draftRaw = raw
		f.record(r, map[string][]string{"raw": {raw}})
		writeJSON(w, gmailDraftPayload())
	default:
		f.record(r, nil)
		if f.shouldRetryOnce(w, r.URL.Path) {
			return
		}
		if len(f.draftPages) > 0 {
			page := f.draftPages[pageIndex(r)]
			drafts := make([]map[string]any, 0, len(page.IDs))
			for _, id := range page.IDs {
				drafts = append(drafts, map[string]any{"id": id, "message": map[string]string{"id": id + "-msg"}})
			}
			writeJSON(w, map[string]any{"drafts": drafts, "nextPageToken": page.Next})
			return
		}
		writeJSON(w, map[string]any{"drafts": []map[string]any{{"id": "draft-1", "message": map[string]string{"id": "draft-msg-1"}}}})
	}
}

func (f *fakeGmailAPIServer) handleDraft(w http.ResponseWriter, r *http.Request) {
	f.record(r, readModifyBody(r))
	if r.Method == http.MethodDelete {
		f.deletedDraft = path.Base(r.URL.Path)
		w.WriteHeader(http.StatusNoContent)
		return
	}
	writeJSON(w, gmailDraftPayload())
}

func (f *fakeGmailAPIServer) handleDraftSend(w http.ResponseWriter, r *http.Request) {
	body := readStringBody(r)
	f.record(r, map[string][]string{"id": {body["id"]}})
	f.sentDraft = body["id"]
	writeJSON(w, map[string]any{"id": "sent-draft-msg-1"})
}

func gmailMessagePayload() map[string]any {
	return gmailMessagePayloadFor("msg-1", "<api-1@example.com>", []string{"INBOX", "UNREAD", "STARRED"}, "101")
}

func gmailMessagePayloadFor(id, messageID string, labels []string, historyID string) map[string]any {
	raw := "From: Sender <sender@example.com>\r\n" +
		"To: Me <me@example.com>\r\n" +
		"Subject: API Roadmap\r\n" +
		"Message-Id: " + messageID + "\r\n" +
		"Date: Sun, 31 May 2026 09:00:00 -0700\r\n" +
		"Content-Type: text/plain; charset=utf-8\r\n\r\n" +
		"hello from gmail api"
	return map[string]any{
		"id":           id,
		"threadId":     "thread-" + id,
		"historyId":    historyID,
		"labelIds":     labels,
		"internalDate": time.Date(2026, 5, 31, 16, 0, 0, 0, time.UTC).UnixMilli(),
		"sizeEstimate": len(raw),
		"raw":          base64.RawURLEncoding.EncodeToString([]byte(raw)),
		"payload": map[string]any{
			"mimeType": "text/plain",
			"headers": []map[string]string{
				{"name": "From", "value": "Sender <sender@example.com>"},
				{"name": "To", "value": "Me <me@example.com>"},
				{"name": "Subject", "value": "API Roadmap"},
				{"name": "Message-ID", "value": messageID},
				{"name": "Date", "value": "Sun, 31 May 2026 09:00:00 -0700"},
			},
		},
	}
}

func gmailMetadataPayloadFrom(payload map[string]any) map[string]any {
	out := make(map[string]any, len(payload))
	for key, value := range payload {
		if key == "raw" {
			continue
		}
		out[key] = value
	}
	return out
}

func gmailDraftPayload() map[string]any {
	raw := "From: Me <me@example.com>\r\n" +
		"To: You <you@example.com>\r\n" +
		"Cc: Copy <copy@example.com>\r\n" +
		"Subject: Draft API\r\n" +
		"Message-Id: <draft-api@example.com>\r\n" +
		"Date: Sun, 31 May 2026 09:05:00 -0700\r\n" +
		"Content-Type: text/plain; charset=utf-8\r\n\r\n" +
		"draft body"
	return map[string]any{
		"id": "draft-1",
		"message": map[string]any{
			"id":           "draft-msg-1",
			"threadId":     "draft-thread-1",
			"labelIds":     []string{"DRAFT"},
			"internalDate": time.Date(2026, 5, 31, 16, 5, 0, 0, time.UTC).UnixMilli(),
			"sizeEstimate": len(raw),
			"raw":          base64.RawURLEncoding.EncodeToString([]byte(raw)),
		},
	}
}

func readModifyBody(r *http.Request) map[string][]string {
	if r.Body == nil {
		return nil
	}
	defer r.Body.Close()
	data, _ := io.ReadAll(r.Body)
	if len(data) == 0 {
		return nil
	}
	var payload map[string][]string
	_ = json.Unmarshal(data, &payload)
	return payload
}

func readStringBody(r *http.Request) map[string]string {
	if r.Body == nil {
		return nil
	}
	defer r.Body.Close()
	data, _ := io.ReadAll(r.Body)
	if len(data) == 0 {
		return nil
	}
	var payload map[string]string
	_ = json.Unmarshal(data, &payload)
	return payload
}

func readRawDraftBody(r *http.Request) string {
	if r.Body == nil {
		return ""
	}
	defer r.Body.Close()
	data, _ := io.ReadAll(r.Body)
	var payload struct {
		Message struct {
			Raw string `json:"raw"`
		} `json:"message"`
	}
	_ = json.Unmarshal(data, &payload)
	return payload.Message.Raw
}

func writeJSON(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(value)
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func equalStringSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func firstBodyValue(body map[string][]string, key string) string {
	if len(body[key]) == 0 {
		return ""
	}
	return body[key][0]
}

func (f *fakeGmailAPIServer) assertModify(t *testing.T, id string, add, remove []string) {
	t.Helper()
	for _, call := range f.calls {
		if call.Method != http.MethodPost || call.Path != "/gmail/v1/users/me/messages/"+id+"/modify" {
			continue
		}
		if equalStringSlices(call.Body["addLabelIds"], add) && equalStringSlices(call.Body["removeLabelIds"], remove) {
			return
		}
	}
	t.Fatalf("missing modify call for %s add=%v remove=%v in %#v", id, add, remove, f.calls)
}

func (f *fakeGmailAPIServer) sawTrash(id string) bool {
	for _, call := range f.calls {
		if call.Method == http.MethodPost && call.Path == "/gmail/v1/users/me/messages/"+id+"/trash" {
			return true
		}
	}
	return false
}

func (f *fakeGmailAPIServer) sawPath(path string) bool {
	for _, call := range f.calls {
		if call.Path == path {
			return true
		}
	}
	return false
}

func (f *fakeGmailAPIServer) pathCallCount(path string) int {
	count := 0
	for _, call := range f.calls {
		if call.Path == path {
			count++
		}
	}
	return count
}

func (f *fakeGmailAPIServer) callsForPath(path string) []fakeGmailCall {
	var calls []fakeGmailCall
	for _, call := range f.calls {
		if call.Path == path {
			calls = append(calls, call)
		}
	}
	return calls
}

func (f *fakeGmailAPIServer) shouldRetryOnce(w http.ResponseWriter, requestPath string) bool {
	if !f.retryOnce[requestPath] || f.retried[requestPath] {
		return false
	}
	if f.retried == nil {
		f.retried = map[string]bool{}
	}
	f.retried[requestPath] = true
	if retryAfter := strings.TrimSpace(f.retryAfter[requestPath]); retryAfter != "" {
		w.Header().Set("Retry-After", retryAfter)
	}
	http.Error(w, "rate limited", http.StatusTooManyRequests)
	return true
}

func pageIndex(r *http.Request) int {
	if strings.TrimSpace(r.URL.Query().Get("pageToken")) == "" {
		return 0
	}
	return 1
}

func messageIDSet(emails []*models.EmailData) map[string]bool {
	out := make(map[string]bool, len(emails))
	for _, email := range emails {
		out[email.MessageID] = true
	}
	return out
}
