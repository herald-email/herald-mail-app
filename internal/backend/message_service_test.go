package backend

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/herald-email/herald-mail-app/internal/config"
	"github.com/herald-email/herald-mail-app/internal/models"
	"github.com/herald-email/herald-mail-app/internal/work"
)

func TestGetMessageCacheFirstUsesCachedBodyBeforeNoCache(t *testing.T) {
	cache := newFakeMessageCache()
	ref := testMessageRef("msg-cache")
	cache.bodies[testRefKey(ref)] = &models.EmailBody{TextPlain: "cached full body"}
	source := newFakeMessageSource()
	source.body[testRefKey(ref)] = &models.EmailBody{TextPlain: "provider should not run"}
	service := NewMessageService(MessageServiceOptions{Cache: cache, Source: source})

	got, err := service.GetMessage(context.Background(), ref)
	if err != nil {
		t.Fatalf("GetMessage: %v", err)
	}
	if got.Source != MessageReadSourceCache {
		t.Fatalf("source = %q, want %q", got.Source, MessageReadSourceCache)
	}
	if got.Body == nil || got.Body.TextPlain != "cached full body" {
		t.Fatalf("body = %#v, want cached full body", got.Body)
	}
	if calls := source.bodyCallCount(ref); calls != 0 {
		t.Fatalf("provider body calls = %d, want 0", calls)
	}
}

func TestGetMessageNoCacheBypassesCacheAndWritesThrough(t *testing.T) {
	cache := newFakeMessageCache()
	ref := testMessageRef("msg-nocache")
	cache.bodies[testRefKey(ref)] = &models.EmailBody{TextPlain: "stale cache"}
	source := newFakeMessageSource()
	source.body[testRefKey(ref)] = &models.EmailBody{TextPlain: "fresh provider body"}
	service := NewMessageService(MessageServiceOptions{
		Cache:         cache,
		Source:        source,
		StoragePolicy: func() string { return config.CacheStoragePolicyPreserveAll },
	})

	got, err := service.GetMessageNoCache(context.Background(), ref)
	if err != nil {
		t.Fatalf("GetMessageNoCache: %v", err)
	}
	if got.Source != MessageReadSourceProvider {
		t.Fatalf("source = %q, want %q", got.Source, MessageReadSourceProvider)
	}
	if got.Body == nil || got.Body.TextPlain != "fresh provider body" {
		t.Fatalf("body = %#v, want provider body", got.Body)
	}
	if calls := source.bodyCallCount(ref); calls != 1 {
		t.Fatalf("provider body calls = %d, want 1", calls)
	}
	if cached := cache.bodies[testRefKey(ref)]; cached == nil || cached.TextPlain != "fresh provider body" {
		t.Fatalf("write-through body = %#v, want fresh provider body", cached)
	}
	if policy := cache.bodyPolicies[testRefKey(ref)]; policy != config.CacheStoragePolicyPreserveAll {
		t.Fatalf("write-through policy = %q, want preserve_all", policy)
	}
}

func TestGetMessageCoalescesDuplicateInFlightProviderFetch(t *testing.T) {
	cache := newFakeMessageCache()
	ref := testMessageRef("msg-coalesce")
	source := newFakeMessageSource()
	source.blockBody(ref)
	service := NewMessageService(MessageServiceOptions{Cache: cache, Source: source})

	results := make(chan messageServiceResult, 2)
	go func() {
		result, err := service.GetMessage(context.Background(), ref)
		results <- messageServiceResult{result: result, err: err}
	}()
	source.waitForBodyStart(t, ref)

	go func() {
		result, err := service.GetMessage(context.Background(), ref)
		results <- messageServiceResult{result: result, err: err}
	}()
	time.Sleep(20 * time.Millisecond)
	if calls := source.bodyCallCount(ref); calls != 1 {
		t.Fatalf("provider body calls while coalesced = %d, want 1", calls)
	}

	source.releaseBody(ref, &models.EmailBody{TextPlain: "coalesced body"}, nil)
	first := <-results
	second := <-results
	for i, got := range []messageServiceResult{first, second} {
		if got.err != nil {
			t.Fatalf("result %d err = %v", i, got.err)
		}
		if got.result.Body == nil || got.result.Body.TextPlain != "coalesced body" {
			t.Fatalf("result %d body = %#v, want coalesced body", i, got.result.Body)
		}
	}
}

func TestGetMessagePreviewEmail1Email2Email1LatestIntent(t *testing.T) {
	cache := newFakeMessageCache()
	email1 := testMessageRef("email1")
	email2 := testMessageRef("email2")
	source := newFakeMessageSource()
	source.blockPreview(email1)
	source.blockPreview(email2)
	service := NewMessageService(MessageServiceOptions{Cache: cache, Source: source})
	intent := MessageReadIntent{ViewID: "timeline-preview"}

	results := make(chan messageServiceResult, 3)
	go func() {
		result, err := service.GetMessagePreview(context.Background(), email1, intent)
		results <- messageServiceResult{result: result, err: err}
	}()
	source.waitForPreviewStart(t, email1)

	go func() {
		result, err := service.GetMessagePreview(context.Background(), email2, intent)
		results <- messageServiceResult{result: result, err: err}
	}()
	source.waitForPreviewStart(t, email2)

	go func() {
		result, err := service.GetMessagePreview(context.Background(), email1, intent)
		results <- messageServiceResult{result: result, err: err}
	}()
	time.Sleep(20 * time.Millisecond)
	if calls := source.previewCallCount(email1); calls != 1 {
		t.Fatalf("email1 provider calls = %d, want duplicate intent to coalesce with first call", calls)
	}

	source.releasePreview(email1, &models.EmailBody{TextPlain: "email1 body"}, nil)
	email1First := <-results
	email1Replay := <-results
	for i, got := range []messageServiceResult{email1First, email1Replay} {
		if got.err != nil {
			t.Fatalf("email1 result %d err = %v", i, got.err)
		}
		if got.result.Body == nil || got.result.Body.TextPlain != "email1 body" {
			t.Fatalf("email1 result %d body = %#v, want email1 body", i, got.result.Body)
		}
	}

	source.releasePreview(email2, &models.EmailBody{TextPlain: "stale email2 body"}, nil)
	stale := <-results
	if !errors.Is(stale.err, work.ErrStaleIntent) {
		t.Fatalf("email2 err = %v, want ErrStaleIntent", stale.err)
	}
}

func TestGetMessagePreviewReplaysCompletedResourceAfterInterveningStaleIntent(t *testing.T) {
	cache := newFakeMessageCache()
	cache.disablePreviewReads = true
	email1 := testMessageRef("email1-replay")
	email2 := testMessageRef("email2-replay")
	source := newFakeMessageSource()
	source.preview[testRefKey(email1)] = &models.EmailBody{TextPlain: "completed email1"}
	source.blockPreview(email2)
	service := NewMessageService(MessageServiceOptions{Cache: cache, Source: source})
	intent := MessageReadIntent{ViewID: "timeline-preview"}

	first, err := service.GetMessagePreview(context.Background(), email1, intent)
	if err != nil {
		t.Fatalf("first email1 GetMessagePreview: %v", err)
	}
	if first.Body == nil || first.Body.TextPlain != "completed email1" {
		t.Fatalf("first email1 body = %#v", first.Body)
	}

	results := make(chan messageServiceResult, 1)
	go func() {
		result, err := service.GetMessagePreview(context.Background(), email2, intent)
		results <- messageServiceResult{result: result, err: err}
	}()
	source.waitForPreviewStart(t, email2)

	replay, err := service.GetMessagePreview(context.Background(), email1, intent)
	if err != nil {
		t.Fatalf("replayed email1 GetMessagePreview: %v", err)
	}
	if replay.Source != MessageReadSourceProvider {
		t.Fatalf("replayed source = %q, want provider replay", replay.Source)
	}
	if replay.Body == nil || replay.Body.TextPlain != "completed email1" {
		t.Fatalf("replayed email1 body = %#v", replay.Body)
	}
	if calls := source.previewCallCount(email1); calls != 1 {
		t.Fatalf("email1 provider calls = %d, want completed replay without refetch", calls)
	}

	source.releasePreview(email2, &models.EmailBody{TextPlain: "stale email2"}, nil)
	stale := <-results
	if !errors.Is(stale.err, work.ErrStaleIntent) {
		t.Fatalf("email2 err = %v, want ErrStaleIntent", stale.err)
	}
}

func TestGetMessageCompletedReplayReturnsIsolatedBodyCopies(t *testing.T) {
	cache := newFakeMessageCache()
	cache.disableBodyReads = true
	ref := testMessageRef("msg-replay-copy")
	source := newFakeMessageSource()
	source.body[testRefKey(ref)] = &models.EmailBody{
		TextPlain: "body with attachment",
		Attachments: []models.Attachment{
			{Filename: "report.txt", Data: []byte("payload")},
		},
	}
	service := NewMessageService(MessageServiceOptions{Cache: cache, Source: source})

	first, err := service.GetMessage(context.Background(), ref)
	if err != nil {
		t.Fatalf("first GetMessage: %v", err)
	}
	if len(first.Body.Attachments) != 1 || string(first.Body.Attachments[0].Data) != "payload" {
		t.Fatalf("first attachment = %#v, want payload", first.Body.Attachments)
	}
	first.Body.Attachments[0].Data = nil

	replayed, err := service.GetMessage(context.Background(), ref)
	if err != nil {
		t.Fatalf("replayed GetMessage: %v", err)
	}
	if calls := source.bodyCallCount(ref); calls != 1 {
		t.Fatalf("provider body calls = %d, want completed replay", calls)
	}
	if len(replayed.Body.Attachments) != 1 || string(replayed.Body.Attachments[0].Data) != "payload" {
		t.Fatalf("replayed attachment = %#v, want isolated payload copy", replayed.Body.Attachments)
	}
}

type messageServiceResult struct {
	result MessageReadResult
	err    error
}

type fakeMessageCache struct {
	mu                  sync.Mutex
	bodies              map[string]*models.EmailBody
	previews            map[string]*models.EmailBody
	bodyPolicies        map[string]string
	previewPolicies     map[string]string
	invalidatedMessages map[string]bool
	invalidatedPreview  map[string]bool
	disableBodyReads    bool
	disablePreviewReads bool
}

func newFakeMessageCache() *fakeMessageCache {
	return &fakeMessageCache{
		bodies:              make(map[string]*models.EmailBody),
		previews:            make(map[string]*models.EmailBody),
		bodyPolicies:        make(map[string]string),
		previewPolicies:     make(map[string]string),
		invalidatedMessages: make(map[string]bool),
		invalidatedPreview:  make(map[string]bool),
	}
}

func (c *fakeMessageCache) GetMessageBodyByRef(ref models.MessageRef) (*models.EmailBody, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.disableBodyReads {
		return nil, nil
	}
	key := testRefKey(ref)
	if c.invalidatedMessages[key] {
		return nil, nil
	}
	return cloneEmailBodyForTest(c.bodies[key]), nil
}

func (c *fakeMessageCache) PutMessageBodyByRef(ref models.MessageRef, body *models.EmailBody, policy string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	key := testRefKey(ref)
	c.bodies[key] = cloneEmailBodyForTest(body)
	c.previews[key] = cloneEmailBodyForTest(body)
	c.bodyPolicies[key] = policy
	c.previewPolicies[key] = policy
	c.invalidatedMessages[key] = false
	c.invalidatedPreview[key] = false
	return nil
}

func (c *fakeMessageCache) GetPreviewBodyByRef(ref models.MessageRef) (*models.EmailBody, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.disablePreviewReads {
		return nil, nil
	}
	key := testRefKey(ref)
	if c.invalidatedPreview[key] {
		return nil, nil
	}
	return cloneEmailBodyForTest(c.previews[key]), nil
}

func (c *fakeMessageCache) CachePreviewBodyByRef(ref models.MessageRef, body *models.EmailBody, policy string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	key := testRefKey(ref)
	c.previews[key] = cloneEmailBodyForTest(body)
	c.previewPolicies[key] = policy
	c.invalidatedPreview[key] = false
	return nil
}

func (c *fakeMessageCache) InvalidateMessageByRef(ref models.MessageRef) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	key := testRefKey(ref)
	c.invalidatedMessages[key] = true
	c.invalidatedPreview[key] = true
	return nil
}

func (c *fakeMessageCache) InvalidatePreviewByRef(ref models.MessageRef) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.invalidatedPreview[testRefKey(ref)] = true
	return nil
}

type fakeMessageSource struct {
	mu             sync.Mutex
	body           map[string]*models.EmailBody
	preview        map[string]*models.EmailBody
	bodyCalls      map[string]int
	previewCalls   map[string]int
	bodyBlocked    map[string]chan messageSourceOutcome
	previewBlocked map[string]chan messageSourceOutcome
	bodyStarted    chan models.MessageRef
	previewStarted chan models.MessageRef
}

type messageSourceOutcome struct {
	body *models.EmailBody
	err  error
}

func newFakeMessageSource() *fakeMessageSource {
	return &fakeMessageSource{
		body:           make(map[string]*models.EmailBody),
		preview:        make(map[string]*models.EmailBody),
		bodyCalls:      make(map[string]int),
		previewCalls:   make(map[string]int),
		bodyBlocked:    make(map[string]chan messageSourceOutcome),
		previewBlocked: make(map[string]chan messageSourceOutcome),
		bodyStarted:    make(chan models.MessageRef, 16),
		previewStarted: make(chan models.MessageRef, 16),
	}
}

func (s *fakeMessageSource) FetchMessageNoCache(ctx context.Context, ref models.MessageRef) (*models.EmailBody, error) {
	key := testRefKey(ref)
	s.mu.Lock()
	s.bodyCalls[key]++
	blocked := s.bodyBlocked[key]
	body := cloneEmailBodyForTest(s.body[key])
	s.mu.Unlock()

	s.bodyStarted <- ref.WithDefaults()
	if blocked != nil {
		select {
		case outcome := <-blocked:
			return cloneEmailBodyForTest(outcome.body), outcome.err
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	return body, nil
}

func (s *fakeMessageSource) FetchMessagePreviewNoCache(ctx context.Context, ref models.MessageRef) (*models.EmailBody, error) {
	key := testRefKey(ref)
	s.mu.Lock()
	s.previewCalls[key]++
	blocked := s.previewBlocked[key]
	body := cloneEmailBodyForTest(s.preview[key])
	s.mu.Unlock()

	s.previewStarted <- ref.WithDefaults()
	if blocked != nil {
		select {
		case outcome := <-blocked:
			return cloneEmailBodyForTest(outcome.body), outcome.err
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	return body, nil
}

func (s *fakeMessageSource) blockBody(ref models.MessageRef) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.bodyBlocked[testRefKey(ref)] = make(chan messageSourceOutcome, 1)
}

func (s *fakeMessageSource) blockPreview(ref models.MessageRef) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.previewBlocked[testRefKey(ref)] = make(chan messageSourceOutcome, 1)
}

func (s *fakeMessageSource) releaseBody(ref models.MessageRef, body *models.EmailBody, err error) {
	s.mu.Lock()
	ch := s.bodyBlocked[testRefKey(ref)]
	s.mu.Unlock()
	ch <- messageSourceOutcome{body: body, err: err}
}

func (s *fakeMessageSource) releasePreview(ref models.MessageRef, body *models.EmailBody, err error) {
	s.mu.Lock()
	ch := s.previewBlocked[testRefKey(ref)]
	s.mu.Unlock()
	ch <- messageSourceOutcome{body: body, err: err}
}

func (s *fakeMessageSource) bodyCallCount(ref models.MessageRef) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.bodyCalls[testRefKey(ref)]
}

func (s *fakeMessageSource) previewCallCount(ref models.MessageRef) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.previewCalls[testRefKey(ref)]
}

func (s *fakeMessageSource) waitForBodyStart(t *testing.T, ref models.MessageRef) {
	t.Helper()
	waitForSourceStart(t, s.bodyStarted, ref)
}

func (s *fakeMessageSource) waitForPreviewStart(t *testing.T, ref models.MessageRef) {
	t.Helper()
	waitForSourceStart(t, s.previewStarted, ref)
}

func waitForSourceStart(t *testing.T, ch <-chan models.MessageRef, want models.MessageRef) {
	t.Helper()
	deadline := time.After(time.Second)
	for {
		select {
		case got := <-ch:
			if testRefKey(got) == testRefKey(want) {
				return
			}
		case <-deadline:
			t.Fatalf("timed out waiting for provider start for %s", testRefKey(want))
		}
	}
}

func testMessageRef(messageID string) models.MessageRef {
	return models.MessageRef{
		SourceID:    models.SourceID("work-mail"),
		AccountID:   models.AccountID("work"),
		Folder:      "INBOX",
		UID:         42,
		UIDValidity: 777,
		MessageID:   messageID,
	}.WithDefaults()
}

func testRefKey(ref models.MessageRef) string {
	ref = ref.WithDefaults()
	if ref.LocalID != "" {
		return ref.LocalID
	}
	return ref.MessageID
}

func cloneEmailBodyForTest(body *models.EmailBody) *models.EmailBody {
	if body == nil {
		return nil
	}
	cloned := *body
	return &cloned
}
