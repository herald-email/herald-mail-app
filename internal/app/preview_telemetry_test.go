package app

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	backendpkg "github.com/herald-email/herald-mail-app/internal/backend"
	"github.com/herald-email/herald-mail-app/internal/models"
)

type previewTelemetryBackend struct {
	stubBackend
	body            *models.EmailBody
	cachedPreview   *models.EmailBody
	err             error
	delay           time.Duration
	cacheLookupID   string
	cacheWriteID    string
	cacheWriteBody  *models.EmailBody
	cacheLookupCall int
	previewBody     *models.EmailBody
	previewFetchID  string
	previewFetchUID uint32
}

func (b *previewTelemetryBackend) FetchEmailBody(_ string, _ uint32) (*models.EmailBody, error) {
	b.fetchBodyCalls++
	if b.delay > 0 {
		time.Sleep(b.delay)
	}
	return b.body, b.err
}

func (b *previewTelemetryBackend) GetCachedPreviewBody(messageID string) (*models.EmailBody, error) {
	b.cacheLookupID = messageID
	b.cacheLookupCall++
	return b.cachedPreview, nil
}

func (b *previewTelemetryBackend) CachePreviewBody(messageID string, body *models.EmailBody) error {
	b.cacheWriteID = messageID
	b.cacheWriteBody = body
	return nil
}

func (b *previewTelemetryBackend) FetchPreviewBody(messageID, _ string, uid uint32) (*models.EmailBody, error) {
	b.previewFetchID = messageID
	b.previewFetchUID = uid
	return b.previewBody, b.err
}

type previewTelemetryServiceBackend struct {
	previewTelemetryBackend
	servicePreviewResult backendpkg.MessageReadResult
	servicePreviewErr    error
	servicePreviewCalls  int
	servicePreviewRef    models.MessageRef
	servicePreviewIntent backendpkg.MessageReadIntent
}

func (b *previewTelemetryServiceBackend) GetMessagePreview(_ context.Context, ref models.MessageRef, intent backendpkg.MessageReadIntent) (backendpkg.MessageReadResult, error) {
	b.servicePreviewCalls++
	b.servicePreviewRef = ref
	b.servicePreviewIntent = intent
	return b.servicePreviewResult, b.servicePreviewErr
}

func TestLoadEmailBodyCmdPopulatesTelemetryOnSuccess(t *testing.T) {
	backend := &previewTelemetryBackend{
		body:  &models.EmailBody{TextPlain: "cached by imap"},
		delay: 2 * time.Millisecond,
	}
	m := New(backend, nil, "", nil, false)

	msg := m.loadEmailBodyCmd("msg-telemetry", "INBOX", 42)().(EmailBodyMsg)

	if msg.MessageID != "msg-telemetry" {
		t.Fatalf("MessageID = %q, want msg-telemetry", msg.MessageID)
	}
	if msg.Folder != "INBOX" || msg.UID != 42 {
		t.Fatalf("folder/uid = %q/%d, want INBOX/42", msg.Folder, msg.UID)
	}
	if msg.LoadSource != previewLoadSourceIMAP {
		t.Fatalf("LoadSource = %q, want %q", msg.LoadSource, previewLoadSourceIMAP)
	}
	if msg.LoadStartedAt.IsZero() || msg.LoadFinishedAt.IsZero() {
		t.Fatalf("expected non-zero load timestamps, got start=%v finish=%v", msg.LoadStartedAt, msg.LoadFinishedAt)
	}
	if msg.LoadFinishedAt.Before(msg.LoadStartedAt) {
		t.Fatalf("finished before started: start=%v finish=%v", msg.LoadStartedAt, msg.LoadFinishedAt)
	}
	if msg.LoadDuration <= 0 {
		t.Fatalf("LoadDuration = %s, want positive duration", msg.LoadDuration)
	}
}

func TestLoadEmailBodyCmdPopulatesTelemetryOnError(t *testing.T) {
	backend := &previewTelemetryBackend{
		err:   errors.New("imap unavailable"),
		delay: 2 * time.Millisecond,
	}
	m := New(backend, nil, "", nil, false)

	msg := m.loadEmailBodyCmd("msg-error", "Archive", 7)().(EmailBodyMsg)

	if msg.Err == nil {
		t.Fatal("expected error")
	}
	if msg.Folder != "Archive" || msg.UID != 7 {
		t.Fatalf("folder/uid = %q/%d, want Archive/7", msg.Folder, msg.UID)
	}
	if msg.LoadSource != previewLoadSourceIMAP {
		t.Fatalf("LoadSource = %q, want %q", msg.LoadSource, previewLoadSourceIMAP)
	}
	if msg.LoadDuration <= 0 {
		t.Fatalf("LoadDuration = %s, want positive duration", msg.LoadDuration)
	}
}

func TestTimelineStaleEmailBodyMsgDoesNotOverwritePreviewTelemetry(t *testing.T) {
	m := makeSizedModel(t, 120, 40)
	current := &models.EmailData{MessageID: "msg-current", Folder: "INBOX", UID: 2}
	m.timeline.selectedEmail = current
	m.timeline.previewLoad = previewLoadTelemetry{
		MessageID: "msg-current",
		Folder:    "INBOX",
		UID:       2,
		Source:    previewLoadSourceIMAP,
		Duration:  42 * time.Millisecond,
	}

	model, _, handled := m.handleTimelineMsg(EmailBodyMsg{
		MessageID:    "msg-stale",
		Folder:       "INBOX",
		UID:          1,
		LoadSource:   previewLoadSourceIMAP,
		LoadDuration: time.Second,
	})
	if !handled {
		t.Fatal("expected stale EmailBodyMsg to be handled")
	}
	updated := model.(*Model)
	if updated.timeline.previewLoad.MessageID != "msg-current" {
		t.Fatalf("stale result overwrote preview telemetry: %#v", updated.timeline.previewLoad)
	}
	if updated.timeline.previewLoad.Duration != 42*time.Millisecond {
		t.Fatalf("preview telemetry duration = %s, want 42ms", updated.timeline.previewLoad.Duration)
	}
}

func TestTimelinePreviewShowsLoadTagWithinNarrowHeader(t *testing.T) {
	m := makeSizedModel(t, 80, 24)
	email := &models.EmailData{
		MessageID: "msg-load-tag",
		Folder:    "INBOX",
		UID:       11,
		Sender:    "sender@example.com",
		Subject:   "Preview timing",
		Date:      time.Date(2026, 5, 15, 9, 30, 0, 0, time.UTC),
	}
	m.timeline.selectedEmail = email
	m.timeline.bodyMessageID = email.MessageID
	m.timeline.body = &models.EmailBody{TextPlain: "hello"}
	m.timeline.previewLoad = previewLoadTelemetry{
		MessageID: email.MessageID,
		Folder:    email.Folder,
		UID:       email.UID,
		Source:    previewLoadSourceIMAP,
		Duration:  42 * time.Millisecond,
	}

	rendered := m.renderEmailPreview()
	stripped := stripANSI(rendered)
	if !strings.Contains(stripped, "Load: 42ms imap") {
		t.Fatalf("expected load tag, got:\n%s", stripped)
	}
	assertFitsWidth(t, 80, rendered)
}

func TestCleanupEmailBodyMsgStoresPreviewTelemetry(t *testing.T) {
	m := makeSizedModel(t, 100, 30)
	email := &models.EmailData{MessageID: "cleanup-msg", Folder: "INBOX", UID: 3}
	m.cleanupPreviewEmail = email
	m.cleanupBodyLoading = true

	model, _ := m.Update(CleanupEmailBodyMsg{
		MessageID:    email.MessageID,
		Folder:       email.Folder,
		UID:          email.UID,
		Body:         &models.EmailBody{TextPlain: "cleanup body"},
		LoadSource:   previewLoadSourceIMAP,
		LoadDuration: 85 * time.Millisecond,
	})

	updated := model.(*Model)
	if updated.cleanupPreviewLoad.MessageID != email.MessageID {
		t.Fatalf("cleanup preview telemetry = %#v, want message %q", updated.cleanupPreviewLoad, email.MessageID)
	}
	if updated.cleanupPreviewLoad.Duration != 85*time.Millisecond {
		t.Fatalf("cleanup preview duration = %s, want 85ms", updated.cleanupPreviewLoad.Duration)
	}
}

func TestLoadEmailBodyCmdUsesCachedPreviewBeforeIMAP(t *testing.T) {
	backend := &previewTelemetryBackend{
		body:          &models.EmailBody{TextPlain: "imap body should not load"},
		cachedPreview: &models.EmailBody{TextPlain: "cached preview body"},
	}
	m := New(backend, nil, "", nil, false)

	msg := m.loadEmailBodyCmd("msg-cached", "INBOX", 99)().(EmailBodyMsg)

	if msg.Err != nil {
		t.Fatalf("unexpected error: %v", msg.Err)
	}
	if got := msg.Body.TextPlain; got != "cached preview body" {
		t.Fatalf("body = %q, want cached preview body", got)
	}
	if backend.cacheLookupID != "msg-cached" || backend.cacheLookupCall != 1 {
		t.Fatalf("cache lookup = id %q calls %d, want msg-cached/1", backend.cacheLookupID, backend.cacheLookupCall)
	}
	if backend.cacheWriteID != "" {
		t.Fatalf("cache write should be skipped on hit, got %q", backend.cacheWriteID)
	}
	if backend.fetchBodyCalls != 0 {
		t.Fatalf("FetchEmailBody calls = %d, want 0", backend.fetchBodyCalls)
	}
	if msg.LoadSource != previewLoadSourceCache {
		t.Fatalf("LoadSource = %q, want %q", msg.LoadSource, previewLoadSourceCache)
	}
}

func TestLoadEmailBodyCmdUsesCacheFirstServiceWhenAvailable(t *testing.T) {
	backend := &previewTelemetryServiceBackend{
		servicePreviewResult: backendpkg.MessageReadResult{
			Body:   &models.EmailBody{TextPlain: "service cached preview"},
			Source: backendpkg.MessageReadSourceCache,
		},
	}
	m := New(backend, nil, "", nil, false)

	msg := m.loadEmailBodyCmd("msg-service", "INBOX", 77)().(EmailBodyMsg)

	if msg.Err != nil {
		t.Fatalf("unexpected error: %v", msg.Err)
	}
	if got := msg.Body.TextPlain; got != "service cached preview" {
		t.Fatalf("body = %q, want service cached preview", got)
	}
	if backend.servicePreviewCalls != 1 {
		t.Fatalf("GetMessagePreview calls = %d, want 1", backend.servicePreviewCalls)
	}
	if backend.servicePreviewRef.MessageID != "msg-service" || backend.servicePreviewRef.Folder != "INBOX" || backend.servicePreviewRef.UID != 77 {
		t.Fatalf("service ref = %#v, want msg-service/INBOX/77", backend.servicePreviewRef)
	}
	if backend.servicePreviewIntent.ViewID != "timeline-preview" {
		t.Fatalf("service intent = %#v, want timeline-preview", backend.servicePreviewIntent)
	}
	if backend.cacheLookupCall != 0 || backend.previewFetchID != "" || backend.fetchBodyCalls != 0 {
		t.Fatalf("legacy preview path used: cacheLookups=%d previewFetch=%q fullFetch=%d", backend.cacheLookupCall, backend.previewFetchID, backend.fetchBodyCalls)
	}
	if msg.LoadSource != previewLoadSourceCache {
		t.Fatalf("LoadSource = %q, want cache", msg.LoadSource)
	}
}

func TestLoadEmailBodyCmdCachesPreviewAfterIMAPMiss(t *testing.T) {
	backend := &previewTelemetryBackend{
		body: &models.EmailBody{TextPlain: "fresh imap body"},
	}
	m := New(backend, nil, "", nil, false)

	msg := m.loadEmailBodyCmd("msg-miss", "INBOX", 100)().(EmailBodyMsg)

	if msg.Err != nil {
		t.Fatalf("unexpected error: %v", msg.Err)
	}
	if backend.fetchBodyCalls != 1 {
		t.Fatalf("FetchEmailBody calls = %d, want 1", backend.fetchBodyCalls)
	}
	if backend.cacheWriteID != "msg-miss" {
		t.Fatalf("cache write id = %q, want msg-miss", backend.cacheWriteID)
	}
	if backend.cacheWriteBody == nil || backend.cacheWriteBody.TextPlain != "fresh imap body" {
		t.Fatalf("cache write body = %#v, want fetched body", backend.cacheWriteBody)
	}
	if msg.LoadSource != previewLoadSourceIMAP {
		t.Fatalf("LoadSource = %q, want %q", msg.LoadSource, previewLoadSourceIMAP)
	}
}

func TestLoadEmailBodyCmdUsesPreviewFetcherOnCacheMiss(t *testing.T) {
	backend := &previewTelemetryBackend{
		body:        &models.EmailBody{TextPlain: "full body should not load"},
		previewBody: &models.EmailBody{TextPlain: "lightweight preview body"},
	}
	m := New(backend, nil, "", nil, false)

	msg := m.loadEmailBodyCmd("msg-preview-fetch", "INBOX", 101)().(EmailBodyMsg)

	if msg.Err != nil {
		t.Fatalf("unexpected error: %v", msg.Err)
	}
	if got := msg.Body.TextPlain; got != "lightweight preview body" {
		t.Fatalf("body = %q, want lightweight preview body", got)
	}
	if backend.previewFetchID != "msg-preview-fetch" || backend.previewFetchUID != 101 {
		t.Fatalf("preview fetch = %q/%d, want msg-preview-fetch/101", backend.previewFetchID, backend.previewFetchUID)
	}
	if backend.fetchBodyCalls != 0 {
		t.Fatalf("FetchEmailBody calls = %d, want 0", backend.fetchBodyCalls)
	}
}

func TestFetchCleanupBodyCmdUsesCacheFirstServiceWhenAvailable(t *testing.T) {
	email := &models.EmailData{MessageID: "cleanup-service", Folder: "INBOX", UID: 88}
	backend := &previewTelemetryServiceBackend{
		servicePreviewResult: backendpkg.MessageReadResult{
			Body:   &models.EmailBody{TextPlain: "cleanup service preview"},
			Source: backendpkg.MessageReadSourceCache,
		},
	}

	msg := fetchCleanupBodyCmd(backend, email)().(CleanupEmailBodyMsg)

	if msg.Err != nil {
		t.Fatalf("unexpected error: %v", msg.Err)
	}
	if got := msg.Body.TextPlain; got != "cleanup service preview" {
		t.Fatalf("cleanup body = %q, want service preview", got)
	}
	if backend.servicePreviewCalls != 1 {
		t.Fatalf("GetMessagePreview calls = %d, want 1", backend.servicePreviewCalls)
	}
	if backend.servicePreviewIntent.ViewID != "cleanup-preview" {
		t.Fatalf("service intent = %#v, want cleanup-preview", backend.servicePreviewIntent)
	}
	if backend.cacheLookupCall != 0 || backend.previewFetchID != "" || backend.fetchBodyCalls != 0 {
		t.Fatalf("legacy cleanup preview path used: cacheLookups=%d previewFetch=%q fullFetch=%d", backend.cacheLookupCall, backend.previewFetchID, backend.fetchBodyCalls)
	}
	if msg.LoadSource != previewLoadSourceCache {
		t.Fatalf("LoadSource = %q, want cache", msg.LoadSource)
	}
}
