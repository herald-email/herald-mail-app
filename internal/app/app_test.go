package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/herald-email/herald-mail-app/internal/accountcheck"
	"github.com/herald-email/herald-mail-app/internal/ai"
	"github.com/herald-email/herald-mail-app/internal/aicheck"
	"github.com/herald-email/herald-mail-app/internal/backend"
	"github.com/herald-email/herald-mail-app/internal/config"
	"github.com/herald-email/herald-mail-app/internal/models"
)

// TestValidIDsMsg_HandlerExists verifies that ValidIDsMsg is a defined type and
// that the Model has a validIDsCh field (compilation check).
// Full handler behaviour is covered by integration; here we ensure the types exist.
func TestValidIDsMsg_TypeExists(t *testing.T) {
	msg := ValidIDsMsg{ValidIDs: map[string]bool{"<a@x.com>": true}}
	if !msg.ValidIDs["<a@x.com>"] {
		t.Error("ValidIDsMsg.ValidIDs not accessible")
	}
}

// stubClassifier is a minimal ai.AIClient for testing reclassification.
type stubClassifier struct {
	category           ai.Category
	err                error
	chatResponse       string
	chatErr            error
	chatMessages       []ai.ChatMessage
	lastEmbeddingModel string
	embedCalls         int
}

func (s *stubClassifier) Classify(_, _ string) (ai.Category, error) { return s.category, s.err }
func (s *stubClassifier) Chat(messages []ai.ChatMessage) (string, error) {
	s.chatMessages = append([]ai.ChatMessage(nil), messages...)
	return s.chatResponse, s.chatErr
}
func (s *stubClassifier) ChatWithTools(_ []ai.ChatMessage, _ []ai.Tool) (string, []ai.ToolCall, error) {
	return "", nil, ai.ErrToolsNotSupported
}
func (s *stubClassifier) Embed(text string) ([]float32, error) {
	s.embedCalls++
	if strings.Contains(text, "ctx-overflow") && len([]rune(text)) > 240 {
		return nil, fmt.Errorf("ollama /api/embeddings returned 500: {\"error\":\"the input length exceeds the context length\"}")
	}
	return []float32{1, 2, 3}, nil
}
func (s *stubClassifier) SetEmbeddingModel(model string)                        { s.lastEmbeddingModel = model }
func (s *stubClassifier) GenerateQuickReplies(_, _, _ string) ([]string, error) { return nil, nil }
func (s *stubClassifier) EnrichContact(_ string, _ []string) (string, []string, error) {
	return "", nil, nil
}
func (s *stubClassifier) HasVisionModel() bool { return false }
func (s *stubClassifier) DescribeImage(_ context.Context, _ []byte, _ string) (string, error) {
	return "", nil
}
func (s *stubClassifier) Ping() error { return nil }

func keyRunes(s string) tea.KeyPressMsg {
	runes := []rune(s)
	msg := tea.KeyPressMsg{Text: s}
	if len(runes) == 1 {
		msg.Code = runes[0]
	}
	return msg
}

func keyRune(r rune) tea.KeyPressMsg {
	return keyRunes(string(r))
}

func keyCode(code rune) tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: code}
}

func keyCtrl(r rune) tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: r, Mod: tea.ModCtrl}
}

func keyPhysical(text string, base rune) tea.KeyPressMsg {
	msg := keyRunes(text)
	msg.BaseCode = base
	return msg
}

// makeReclassifyModel builds the minimal Model state required to test reclassification.
func makeReclassifyModel(classifier ai.AIClient) *Model {
	m := &Model{
		timeline:        TimelineState{expandedThreads: make(map[string]bool)},
		classifications: make(map[string]string),
		backend:         &stubBackend{},
		classifier:      classifier,
	}
	m.timeline.emails = []*models.EmailData{
		{MessageID: "msg-1", Subject: "Hello", Sender: "alice@example.com", Date: time.Now()},
	}
	return m
}

// TestReclassifyResult verifies that ReclassifyResultMsg updates classifications
// and sets a success status message.
func TestReclassifyResult(t *testing.T) {
	m := makeReclassifyModel(&stubClassifier{category: "news"})

	result, _ := m.Update(ReclassifyResultMsg{MessageID: "msg-1", Category: "news"})
	updated := result.(*Model)

	if updated.classifications["msg-1"] != "news" {
		t.Errorf("classifications[msg-1] = %q, want %q", updated.classifications["msg-1"], "news")
	}
	if updated.statusMessage != "Reclassified: news" {
		t.Errorf("statusMessage = %q, want %q", updated.statusMessage, "Reclassified: news")
	}
}

// TestReclassifyResultError verifies that a failed ReclassifyResultMsg sets an error status.
func TestReclassifyResultError(t *testing.T) {
	m := makeReclassifyModel(&stubClassifier{})

	result, _ := m.Update(ReclassifyResultMsg{MessageID: "msg-1", Err: errors.New("AI offline")})
	updated := result.(*Model)

	if updated.classifications["msg-1"] != "" {
		t.Errorf("expected no classification on error, got %q", updated.classifications["msg-1"])
	}
	if updated.statusMessage != "Reclassify failed: AI offline" {
		t.Errorf("statusMessage = %q, want %q", updated.statusMessage, "Reclassify failed: AI offline")
	}
}

// TestReclassifyNilClassifier verifies that pressing A with no classifier set
// shows the "No AI configured" status message without panicking.
func TestReclassifyNilClassifier(t *testing.T) {
	m := makeReclassifyModel(nil)
	m.classifier = nil // explicitly nil

	result, cmd := m.Update(ReclassifyResultMsg{Err: errors.New("no AI classifier configured")})
	updated := result.(*Model)

	if cmd != nil {
		t.Error("expected nil cmd when classifier is nil")
	}
	if updated.statusMessage != "Reclassify failed: no AI classifier configured" {
		t.Errorf("statusMessage = %q", updated.statusMessage)
	}
}

// TestReclassifyEmailCmd verifies that reclassifyEmailCmd returns a command
// that calls Classify and returns ReclassifyResultMsg with the correct category.
func TestReclassifyEmailCmd(t *testing.T) {
	m := makeReclassifyModel(&stubClassifier{category: "imp"})
	email := &models.EmailData{
		MessageID: "msg-42",
		Sender:    "boss@corp.com",
		Subject:   "Urgent: Q4 review",
	}

	cmd := m.reclassifyEmailCmd(email)
	if cmd == nil {
		t.Fatal("expected non-nil cmd")
	}

	msg := cmd()
	rr, ok := msg.(ReclassifyResultMsg)
	if !ok {
		t.Fatalf("expected ReclassifyResultMsg, got %T", msg)
	}
	if rr.Err != nil {
		t.Fatalf("unexpected error: %v", rr.Err)
	}
	if rr.MessageID != "msg-42" {
		t.Errorf("MessageID = %q, want %q", rr.MessageID, "msg-42")
	}
	if rr.Category != "imp" {
		t.Errorf("Category = %q, want %q", rr.Category, "imp")
	}
}

func TestSettingsSaved_EmbeddingModelChangeInvalidatesCache(t *testing.T) {
	backend := &stubBackend{ensureResult: true}
	classifier := &stubClassifier{}
	m := New(backend, nil, "", classifier, false)
	m.cfg = &config.Config{}
	m.cfg.Ollama.EmbeddingModel = "nomic-embed-text"

	next := &config.Config{}
	next.Ollama.EmbeddingModel = "nomic-embed-text-v2-moe"

	updatedModel, _ := m.Update(SettingsSavedMsg{Config: next})
	updated := updatedModel.(*Model)

	if !backend.ensureCalled {
		t.Fatal("expected backend embedding invalidation to run")
	}
	if backend.ensuredModel != "nomic-embed-text-v2-moe" {
		t.Fatalf("EnsureEmbeddingModel called with %q", backend.ensuredModel)
	}
	if classifier.lastEmbeddingModel != "nomic-embed-text-v2-moe" {
		t.Fatalf("SetEmbeddingModel called with %q", classifier.lastEmbeddingModel)
	}
	if updated.statusMessage != "Settings saved. Embeddings reset for the new model." {
		t.Fatalf("statusMessage = %q", updated.statusMessage)
	}
}

func TestSettingsSaved_ChatModelChangeAppliesImmediately(t *testing.T) {
	seenModels := make(chan string, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/generate" {
			http.NotFound(w, r)
			return
		}
		var req struct {
			Model string `json:"model"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("decode request: %v", err)
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		seenModels <- req.Model
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"response": "imp"})
	}))
	defer srv.Close()

	original := ollamaAppConfig(srv.URL, "old-chat-model", "embed-model")
	classifier, err := ai.NewFromConfig(original)
	if err != nil {
		t.Fatal(err)
	}
	m := New(&stubBackend{}, nil, "", classifier, false)
	m.SetConfig(original)

	next := ollamaAppConfig(srv.URL, "new-chat-model", "embed-model")
	updatedModel, _ := m.Update(SettingsSavedMsg{Config: next})
	updated := updatedModel.(*Model)

	cmd := updated.reclassifyEmailCmd(&models.EmailData{
		MessageID: "msg-1",
		Sender:    "alice@example.test",
		Subject:   "Needs review",
	})
	msg := cmd().(ReclassifyResultMsg)
	if msg.Err != nil {
		t.Fatalf("reclassify failed: %v", msg.Err)
	}

	select {
	case got := <-seenModels:
		if got != "new-chat-model" {
			t.Fatalf("classification used model %q, want %q", got, "new-chat-model")
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for classification request")
	}
}

func TestSettingsSaved_CachePolicyChangeSchedulesAsyncPrune(t *testing.T) {
	backend := &stubBackend{
		applyCachePolicyResult: models.PreviewCachePruneResult{
			RowsChanged:             2,
			AttachmentBytesRemoved:  10,
			InlineImageBytesRemoved: 5,
		},
	}
	m := New(backend, nil, "", nil, false)
	m.cfg = &config.Config{}
	m.cfg.Cache.StoragePolicy = config.CacheStoragePolicyPreserveAll

	next := &config.Config{}
	next.Cache.StoragePolicy = config.CacheStoragePolicyLightweight

	updatedModel, cmd := m.Update(SettingsSavedMsg{Config: next})
	updated := updatedModel.(*Model)
	if cmd == nil {
		t.Fatal("expected settings save to schedule cache policy pruning")
	}
	if backend.applyCachePolicyCalls != 0 {
		t.Fatal("cache policy pruning should run asynchronously, not during SettingsSavedMsg handling")
	}

	messages := settingsImmediateMessagesForTest(cmd)
	var applied CacheStoragePolicyAppliedMsg
	found := false
	for _, msg := range messages {
		if m, ok := msg.(CacheStoragePolicyAppliedMsg); ok {
			applied = m
			found = true
		}
	}
	if !found {
		t.Fatalf("expected CacheStoragePolicyAppliedMsg, got %#v", messages)
	}
	if backend.applyCachePolicyCalls != 1 || backend.appliedCachePolicy != config.CacheStoragePolicyLightweight {
		t.Fatalf("backend apply calls=%d policy=%q", backend.applyCachePolicyCalls, backend.appliedCachePolicy)
	}

	updatedModel, _ = updated.Update(applied)
	updated = updatedModel.(*Model)
	if !strings.Contains(updated.statusMessage, "Preview cache policy applied: lightweight") {
		t.Fatalf("statusMessage = %q", updated.statusMessage)
	}
	if !strings.Contains(updated.statusMessage, "2 rows pruned") || !strings.Contains(updated.statusMessage, "15 bytes removed") {
		t.Fatalf("statusMessage missing prune summary: %q", updated.statusMessage)
	}
}

func TestAccountSettingsValidationFailureKeepsPreviousConfig(t *testing.T) {
	oldValidate := validateAccountConfig
	validateAccountConfig = func(context.Context, *config.Config, string) accountcheck.Result {
		return accountcheck.Result{
			IMAP: accountcheck.Check{Surface: "IMAP", Err: errors.New("imap refused")},
			SMTP: accountcheck.Check{Surface: "SMTP"},
		}
	}
	defer func() { validateAccountConfig = oldValidate }()

	original := &config.Config{}
	original.Credentials.Username = "old@example.test"
	next := &config.Config{}
	next.Credentials.Username = "new@example.test"

	m := New(&stubBackend{}, nil, "old@example.test", nil, false)
	m.SetConfig(original)
	m.SetConfigPath(t.TempDir() + "/conf.yaml")

	updatedModel, cmd := m.Update(SettingsSavedMsg{Config: next, ValidateAccount: true})
	updated := updatedModel.(*Model)
	if cmd == nil {
		t.Fatal("expected account validation command")
	}
	if updated.cfg != original {
		t.Fatal("candidate config should not be applied before validation passes")
	}

	msg, ok := cmd().(AccountValidationMsg)
	if !ok {
		t.Fatalf("expected AccountValidationMsg, got %T", cmd())
	}
	updatedModel, _ = updated.Update(msg)
	updated = updatedModel.(*Model)

	if updated.cfg != original || updated.cfg.Credentials.Username != "old@example.test" {
		t.Fatalf("failed validation replaced config: %#v", updated.cfg)
	}
	if updated.accountValidation == nil || !strings.Contains(updated.accountValidation.Message, "Settings were not saved") {
		t.Fatalf("expected visible not-saved validation message, got %#v", updated.accountValidation)
	}
}

func TestAccountSettingsValidationSuccessSavesAndSwapsBackend(t *testing.T) {
	oldValidate := validateAccountConfig
	validateAccountConfig = func(context.Context, *config.Config, string) accountcheck.Result {
		return accountcheck.Result{
			IMAP: accountcheck.Check{Surface: "IMAP"},
			SMTP: accountcheck.Check{Surface: "SMTP"},
		}
	}
	defer func() { validateAccountConfig = oldValidate }()

	newBackend := &stubBackend{}
	oldFactory := newValidatedLocalBackend
	newValidatedLocalBackend = func(*config.Config, string, ai.AIClient) (backend.Backend, error) {
		return newBackend, nil
	}
	defer func() { newValidatedLocalBackend = oldFactory }()

	original := &config.Config{}
	original.Credentials.Username = "old@example.test"
	next := &config.Config{}
	next.Credentials.Username = "new@example.test"
	next.Credentials.Password = "secret"
	next.Server.Host = "imap.example.test"
	next.Server.Port = 993

	configPath := t.TempDir() + "/conf.yaml"
	m := New(&stubBackend{}, nil, "old@example.test", nil, false)
	m.SetConfig(original)
	m.SetConfigPath(configPath)

	updatedModel, cmd := m.Update(SettingsSavedMsg{Config: next, ValidateAccount: true})
	updated := updatedModel.(*Model)
	msg := cmd().(AccountValidationMsg)
	updatedModel, _ = updated.Update(msg)
	updated = updatedModel.(*Model)

	if updated.cfg != next {
		t.Fatal("validated config was not applied")
	}
	if updated.backend != newBackend {
		t.Fatal("validated account did not replace backend")
	}
	if updated.fromAddress != "new@example.test" {
		t.Fatalf("fromAddress = %q", updated.fromAddress)
	}
	if _, err := config.Load(configPath); err != nil {
		t.Fatalf("validated config was not saved: %v", err)
	}
}

func TestAccountSettingsValidationSuccessResetsSyncGenerationForNewBackend(t *testing.T) {
	oldValidate := validateAccountConfig
	validateAccountConfig = func(context.Context, *config.Config, string) accountcheck.Result {
		return accountcheck.Result{
			IMAP: accountcheck.Check{Surface: "IMAP"},
			SMTP: accountcheck.Check{Surface: "SMTP"},
		}
	}
	defer func() { validateAccountConfig = oldValidate }()

	oldFactory := newValidatedLocalBackend
	newValidatedLocalBackend = func(*config.Config, string, ai.AIClient) (backend.Backend, error) {
		return &stubBackend{}, nil
	}
	defer func() { newValidatedLocalBackend = oldFactory }()

	original := &config.Config{}
	original.Credentials.Username = "old@example.test"
	next := &config.Config{}
	next.Credentials.Username = "new@example.test"
	next.Credentials.Password = "secret"
	next.Server.Host = "imap.example.test"
	next.Server.Port = 993

	m := New(&stubBackend{}, nil, "old@example.test", nil, false)
	m.SetConfig(original)
	m.SetConfigPath(t.TempDir() + "/conf.yaml")
	m.currentFolder = "INBOX"
	m.syncGeneration = 42
	m.syncAccumulator.reset("INBOX", 42)

	updatedModel, cmd := m.Update(SettingsSavedMsg{Config: next, ValidateAccount: true})
	updated := updatedModel.(*Model)
	msg := cmd().(AccountValidationMsg)
	updatedModel, _ = updated.Update(msg)
	updated = updatedModel.(*Model)

	if updated.syncGeneration != 0 {
		t.Fatalf("syncGeneration after backend swap = %d, want reset to 0", updated.syncGeneration)
	}

	updatedModel, _ = updated.Update(SyncEventMsg{Event: models.FolderSyncEvent{
		Folder:     "INBOX",
		Generation: 1,
		Phase:      models.SyncPhaseComplete,
		Message:    "Found 0 senders",
	}})
	updated = updatedModel.(*Model)
	if updated.syncGeneration != 1 {
		t.Fatalf("new backend generation was not accepted; syncGeneration = %d, want 1", updated.syncGeneration)
	}
}

func TestAccountSettingsReturnToMenuContinuesStartupSync(t *testing.T) {
	oldValidate := validateAccountConfig
	validateAccountConfig = func(context.Context, *config.Config, string) accountcheck.Result {
		return accountcheck.Result{
			IMAP: accountcheck.Check{Surface: "IMAP"},
			SMTP: accountcheck.Check{Surface: "SMTP"},
		}
	}
	defer func() { validateAccountConfig = oldValidate }()

	oldFactory := newValidatedLocalBackend
	newValidatedLocalBackend = func(*config.Config, string, ai.AIClient) (backend.Backend, error) {
		return &stubBackend{}, nil
	}
	defer func() { newValidatedLocalBackend = oldFactory }()

	next := &config.Config{}
	next.Credentials.Username = "new@example.test"
	next.Credentials.Password = "secret"
	next.Server.Host = "imap.example.test"
	next.Server.Port = 993

	m := New(&stubBackend{}, nil, "old@example.test", nil, false)
	m.SetConfig(&config.Config{})
	m.SetConfigPath(t.TempDir() + "/conf.yaml")
	m.currentFolder = "INBOX"

	updatedModel, cmd := m.Update(SettingsSavedMsg{Config: next, ReturnToMenu: true, ValidateAccount: true})
	updated := updatedModel.(*Model)
	msg := cmd().(AccountValidationMsg)
	updatedModel, _ = updated.Update(msg)
	updated = updatedModel.(*Model)

	if !updated.showSettings || updated.settingsPanel == nil {
		t.Fatal("expected account settings menu to reopen after validated save")
	}

	updatedModel, _ = updated.Update(SyncEventMsg{Event: models.FolderSyncEvent{
		Folder:     "INBOX",
		Generation: 1,
		Phase:      models.SyncPhaseComplete,
		Message:    "Found 0 senders",
	}})
	updated = updatedModel.(*Model)
	if updated.syncGeneration != 1 {
		t.Fatalf("settings overlay swallowed startup sync event; syncGeneration = %d, want 1", updated.syncGeneration)
	}
}

func TestAllAccountsSyncEventAcceptsSourceScopedGeneration(t *testing.T) {
	m := New(&stubBackend{}, nil, "", nil, false)
	m.currentFolder = "INBOX"
	m.accounts = []backend.AccountInfo{
		{SourceID: models.SourceID("default-mail"), AccountID: models.AccountID("default"), DisplayName: "Default"},
		{SourceID: models.SourceID("gmail"), AccountID: models.AccountID("gmail"), DisplayName: "Gmail"},
	}
	m.activeSourceID = backend.AllAccountsSourceID
	m.activeAccountID = models.AccountID("all")
	m.syncGeneration = 2
	m.syncAccumulator.reset("INBOX", 2)

	updatedModel, _ := m.Update(SyncEventMsg{Event: models.FolderSyncEvent{
		SourceID:   models.SourceID("default-mail"),
		AccountID:  models.AccountID("default"),
		Folder:     "INBOX",
		Generation: 1,
		Phase:      models.SyncPhaseComplete,
		Message:    "Found 524 senders",
	}})
	updated := updatedModel.(*Model)

	if !updated.syncCountsSettled {
		t.Fatal("all-account sync should accept a current source event even when another source has a higher generation")
	}
	if got := updated.progressInfo.Message; got != "Found 524 senders" {
		t.Fatalf("progress message = %q, want source completion message", got)
	}
	if got := updated.syncingFolder; got != "INBOX" {
		t.Fatalf("syncingFolder = %q, want INBOX", got)
	}
}

func TestCalendarSettingsValidationFailureKeepsPreviousConfig(t *testing.T) {
	oldValidate := validateCalendarConfig
	validateCalendarConfig = func(context.Context, *config.Config, []models.SourceID) error {
		return errors.New("calendar refused")
	}
	defer func() { validateCalendarConfig = oldValidate }()

	original := &config.Config{Sources: []config.SourceConfig{
		{ID: "work-mail", Kind: "mail", AccountID: "work", Credentials: config.CredentialsConfig{Username: "old@example.test"}},
	}}
	next := &config.Config{Sources: []config.SourceConfig{
		{ID: "work-mail", Kind: "mail", AccountID: "work", Credentials: config.CredentialsConfig{Username: "new@example.test"}},
		{ID: "work-calendar", Kind: "calendar", Provider: "caldav", AccountID: "work", CalDAV: config.CalDAVConfig{URL: "https://caldav.example/work"}},
	}}

	m := New(&stubBackend{}, nil, "old@example.test", nil, false)
	m.SetConfig(original)
	m.SetConfigPath(t.TempDir() + "/conf.yaml")

	updatedModel, cmd := m.Update(SettingsSavedMsg{Config: next, ReturnToMenu: true, ValidateCalendar: true, CalendarSourceIDs: []models.SourceID{"work-calendar"}})
	updated := updatedModel.(*Model)
	if cmd == nil {
		t.Fatal("expected calendar validation command")
	}
	if updated.cfg != original {
		t.Fatal("candidate config should not be applied before calendar validation passes")
	}

	msg, ok := cmd().(CalendarValidationMsg)
	if !ok {
		t.Fatalf("expected CalendarValidationMsg, got %T", cmd())
	}
	updatedModel, _ = updated.Update(msg)
	updated = updatedModel.(*Model)

	if updated.cfg != original || len(updated.cfg.Sources) != 1 {
		t.Fatalf("failed calendar validation replaced config: %#v", updated.cfg)
	}
	if updated.accountValidation == nil || !strings.Contains(updated.accountValidation.Message, "Calendar settings were not saved") {
		t.Fatalf("expected visible calendar validation message, got %#v", updated.accountValidation)
	}
}

func TestCalendarSettingsICloudUnauthorizedShowsAppPasswordGuidance(t *testing.T) {
	original := &config.Config{Sources: []config.SourceConfig{
		{ID: "default-mail", Kind: "mail", AccountID: "default", Credentials: config.CredentialsConfig{Username: "old@example.test"}},
	}}
	next := &config.Config{Sources: []config.SourceConfig{
		{ID: "default-mail", Kind: "mail", AccountID: "default", Credentials: config.CredentialsConfig{Username: "new@example.test"}},
		{ID: "icloud-calendar", Kind: "calendar", Provider: "caldav", AccountID: "icloud", CalDAV: config.CalDAVConfig{URL: "https://caldav.icloud.com/", Username: "me@icloud.com", Password: "calendar-app-password"}},
	}}

	m := New(&stubBackend{}, nil, "old@example.test", nil, false)
	m.SetConfig(original)
	m.SetConfigPath(t.TempDir() + "/conf.yaml")

	updatedModel, cmd := m.Update(SettingsSavedMsg{Config: next, ReturnToMenu: true, ValidateCalendar: true, CalendarSourceIDs: []models.SourceID{"icloud-calendar"}})
	updated := updatedModel.(*Model)
	if cmd == nil {
		t.Fatal("expected calendar validation command")
	}
	updatedModel, _ = updated.Update(CalendarValidationMsg{
		Config:                     next,
		ReturnToMenu:               true,
		ReclaimOfflineCacheStorage: false,
		SourceIDs:                  []models.SourceID{"icloud-calendar"},
		Err:                        errors.New("calendar source icloud-calendar failed validation: caldav propfind failed: Unauthorized"),
	})
	updated = updatedModel.(*Model)

	if updated.cfg != original || len(updated.cfg.Sources) != 1 {
		t.Fatalf("failed iCloud calendar validation replaced config: %#v", updated.cfg)
	}
	rendered := stripANSI(updated.renderAccountValidationPanel())
	message := updated.accountValidation.Message
	for _, want := range []string{"Calendar settings were not saved", "Apple Account email", "Apple app-specific password", "generate a new app-specific password", "two-factor authentication"} {
		if !strings.Contains(message, want) {
			t.Fatalf("expected iCloud Unauthorized guidance to include %q, got message %q and rendered:\n%s", want, message, rendered)
		}
	}
	if strings.Contains(message, "calendar-app-password") || strings.Contains(rendered, "calendar-app-password") {
		t.Fatalf("validation overlay exposed saved password, message %q rendered:\n%s", message, rendered)
	}
}

func TestAISettingsOllamaValidationFailureKeepsPreviousConfig(t *testing.T) {
	oldValidate := validateOllamaModels
	validateOllamaModels = func(context.Context, *config.Config) aicheck.Result {
		return aicheck.Result{
			Host: "http://ollama.test",
			Missing: []aicheck.MissingModel{
				{Role: "chat/classification", Name: "missing-chat"},
			},
		}
	}
	defer func() { validateOllamaModels = oldValidate }()

	original := &config.Config{}
	original.AI.Provider = "disabled"
	next := &config.Config{}
	next.AI.Provider = "ollama"
	next.Ollama.Host = "http://ollama.test"
	next.Ollama.Model = "missing-chat"
	next.Ollama.EmbeddingModel = "nomic-embed-text"

	configPath := t.TempDir() + "/conf.yaml"
	m := New(&stubBackend{}, nil, "old@example.test", nil, false)
	m.SetConfig(original)
	m.SetConfigPath(configPath)

	updatedModel, cmd := m.Update(SettingsSavedMsg{Config: next, ReturnToMenu: true, ValidateOllamaModels: true})
	updated := updatedModel.(*Model)
	if cmd == nil {
		t.Fatal("expected Ollama model validation command")
	}
	if updated.cfg != original {
		t.Fatal("candidate config should not be applied before Ollama model validation passes")
	}

	msg := cmd().(OllamaModelValidationMsg)
	updatedModel, _ = updated.Update(msg)
	updated = updatedModel.(*Model)

	if updated.cfg != original || updated.cfg.AI.Provider != "disabled" {
		t.Fatalf("failed Ollama validation replaced config: %#v", updated.cfg)
	}
	if updated.aiModelWarning == nil {
		t.Fatal("expected persistent AI model warning after failed validation")
	}
	if !updated.showSettings || updated.settingsPanel == nil {
		t.Fatal("failed AI validation should return to Settings > AI")
	}
	if got := updated.settingsPanel.panelStatus; !strings.Contains(got, "ollama pull missing-chat") {
		t.Fatalf("expected settings panel status to include install command, got %q", got)
	}
}

func TestOllamaStartupWarningMarksAIDownAndDisablesAIFunctionsWithoutBlocking(t *testing.T) {
	m := New(&stubBackend{}, nil, "user@example.test", &stubClassifier{}, false)
	m.SetConfig(ollamaAppConfig("http://ollama.test", "missing-chat", "nomic-embed-text"))

	updatedModel, _ := m.Update(OllamaModelWarningMsg{
		Result: aicheck.Result{
			Host: "http://ollama.test",
			Missing: []aicheck.MissingModel{
				{Role: "chat/classification", Name: "missing-chat"},
			},
		},
	})
	updated := updatedModel.(*Model)

	if updated.aiModelWarning == nil {
		t.Fatal("expected startup warning to be retained")
	}
	if chip := stripANSI(updated.renderAIStatusChip()); !strings.Contains(chip, "AI down") {
		t.Fatalf("expected AI status chip to show down, got %q", chip)
	}
	if updated.classifier != nil {
		t.Fatal("startup warning should disable AI functions for the unavailable local model")
	}
}

func TestSettingsSaved_UnrelatedChangeKeepsStartupAIWarningDown(t *testing.T) {
	m := New(&stubBackend{}, nil, "user@example.test", &stubClassifier{}, false)
	original := ollamaAppConfig("http://ollama.test", "missing-chat", "nomic-embed-text")
	m.SetConfig(original)

	updatedModel, _ := m.Update(OllamaModelWarningMsg{
		Result: aicheck.Result{
			Host: "http://ollama.test",
			Missing: []aicheck.MissingModel{
				{Role: "chat/classification", Name: "missing-chat"},
			},
		},
	})
	updated := updatedModel.(*Model)
	if updated.classifier != nil {
		t.Fatal("startup warning should disable the unavailable AI client")
	}

	next := ollamaAppConfig("http://ollama.test", "missing-chat", "nomic-embed-text")
	next.Compose.Signature.Text = "Regards"
	updatedModel, _ = updated.Update(SettingsSavedMsg{Config: next})
	updated = updatedModel.(*Model)

	if updated.classifier != nil {
		t.Fatal("unrelated settings save should not revive an unavailable AI client")
	}
	if updated.aiModelWarning == nil {
		t.Fatal("expected AI model warning to remain visible until AI settings are repaired")
	}
}

func TestDisableAIFromWarningSavesDisabledConfig(t *testing.T) {
	configPath := t.TempDir() + "/conf.yaml"
	m := New(&stubBackend{}, nil, "user@example.test", nil, false)
	m.SetConfig(ollamaAppConfig("http://ollama.test", "missing-chat", "nomic-embed-text"))
	m.SetConfigPath(configPath)

	disabled := ollamaAppConfig("http://ollama.test", "missing-chat", "nomic-embed-text")
	disabled.AI.Provider = "disabled"
	updatedModel, _ := m.Update(SettingsSavedMsg{Config: disabled, ReturnToMenu: true})
	updated := updatedModel.(*Model)

	if updated.cfg.AI.Provider != "disabled" {
		t.Fatalf("AI provider = %q, want disabled", updated.cfg.AI.Provider)
	}
	loaded, err := config.Load(configPath)
	if err != nil {
		t.Fatalf("disabled config was not saved: %v", err)
	}
	if loaded.AI.Provider != "disabled" {
		t.Fatalf("saved AI provider = %q, want disabled", loaded.AI.Provider)
	}
}

func ollamaAppConfig(host, chatModel, embedModel string) *config.Config {
	cfg := &config.Config{}
	cfg.Credentials.Username = "user@example.test"
	cfg.Credentials.Password = "secret"
	cfg.Server.Host = "imap.example.test"
	cfg.Server.Port = 993
	cfg.SMTP.Host = "smtp.example.test"
	cfg.SMTP.Port = 587
	cfg.AI.Provider = "ollama"
	cfg.Ollama.Host = host
	cfg.Ollama.Model = chatModel
	cfg.Ollama.EmbeddingModel = embedModel
	cfg.Semantic.Model = embedModel
	return cfg
}

func TestCacheStoragePolicyAppliedMsgReportsFailure(t *testing.T) {
	m := New(&stubBackend{}, nil, "", nil, false)

	updatedModel, _ := m.Update(CacheStoragePolicyAppliedMsg{
		Policy: config.CacheStoragePolicyLightweight,
		Err:    errors.New("sqlite locked"),
	})
	updated := updatedModel.(*Model)

	if !strings.Contains(updated.statusMessage, "Preview cache policy update failed: sqlite locked") {
		t.Fatalf("statusMessage = %q", updated.statusMessage)
	}
}

func TestHandleTimelineMsg_DedupesBackgroundEmbeddingBatchStart(t *testing.T) {
	m := makeSizedModel(t, 120, 40)
	m.classifier = &stubClassifier{}
	m.cfg = &config.Config{}
	m.cfg.Semantic.Enabled = true

	model, cmd, handled := m.handleTimelineMsg(TimelineLoadedMsg{Emails: mockEmails()})
	if !handled {
		t.Fatal("expected TimelineLoadedMsg to be handled")
	}
	if cmd == nil {
		t.Fatal("expected first timeline load to start background AI work")
	}
	updated := model.(*Model)
	if !updated.embeddingBatchActive {
		t.Fatal("expected embedding batch to be marked active after scheduling")
	}

	model, cmd, handled = updated.handleTimelineMsg(TimelineLoadedMsg{Emails: mockEmails()})
	if !handled {
		t.Fatal("expected second TimelineLoadedMsg to be handled")
	}
	if cmd != nil {
		t.Fatal("expected duplicate timeline load to skip scheduling another embedding batch")
	}
}

func TestHandleTimelineMsg_SkipsAutomaticSemanticWorkWhenDisabled(t *testing.T) {
	m := makeSizedModel(t, 120, 40)
	m.classifier = &stubClassifier{}
	m.cfg = &config.Config{}
	m.cfg.Semantic.Enabled = false

	model, _, handled := m.handleTimelineMsg(TimelineLoadedMsg{Emails: mockEmails()})
	if !handled {
		t.Fatal("expected TimelineLoadedMsg to be handled")
	}
	updated := model.(*Model)
	if updated.embeddingBatchActive {
		t.Fatal("expected semantic-disabled config to skip background embedding")
	}
	if updated.contactEnrichmentActive {
		t.Fatal("expected semantic-disabled config to skip background contact enrichment")
	}
}

func TestHandleTimelineMsg_SkipsAutomaticSemanticWorkInDemoMode(t *testing.T) {
	m := makeSizedModel(t, 120, 40)
	m.classifier = &stubClassifier{}
	m.demoMode = true
	m.cfg = &config.Config{}
	m.cfg.Semantic.Enabled = true

	model, cmd, handled := m.handleTimelineMsg(TimelineLoadedMsg{Emails: mockEmails()})
	if !handled {
		t.Fatal("expected TimelineLoadedMsg to be handled")
	}
	updated := model.(*Model)
	if cmd != nil {
		t.Fatal("expected demo timeline load to avoid background semantic and classification commands")
	}
	if updated.embeddingBatchActive {
		t.Fatal("expected demo mode to skip background embedding")
	}
	if updated.contactEnrichmentActive {
		t.Fatal("expected demo mode to skip background contact enrichment")
	}
}

func TestEmbeddingProgressMsg_ClearsActiveFlagWhenComplete(t *testing.T) {
	m := makeSizedModel(t, 120, 40)
	m.embeddingBatchActive = true

	model, _ := m.Update(EmbeddingProgressMsg{Done: 0, Total: 0})
	updated := model.(*Model)
	if updated.embeddingBatchActive {
		t.Fatal("expected embedding batch active flag to clear when progress is complete")
	}
}

func TestEmbedChunksForEmail_FallsBackOnContextLengthError(t *testing.T) {
	classifier := &stubClassifier{}
	email := &models.EmailData{
		MessageID: "msg-1",
		Sender:    "alice@example.com",
		Subject:   "ctx-overflow subject",
		Date:      time.Now(),
	}
	body := strings.Repeat("ctx-overflow paragraph ", 100)

	chunks, err := embedChunksForEmail(email, body, classifier)
	if err != nil {
		t.Fatalf("embedChunksForEmail returned error: %v", err)
	}
	if len(chunks) == 0 {
		t.Fatal("expected at least one embedding chunk after fallback")
	}
	if classifier.embedCalls < 2 {
		t.Fatalf("expected fallback retries, got %d embed call(s)", classifier.embedCalls)
	}
}

func TestStartupHydratedMsg_ProgressivelyHydratesWhileLoading(t *testing.T) {
	m := New(&stubBackend{}, nil, "", nil, false)
	m.loading = true

	emails := []*models.EmailData{
		{MessageID: "msg-1", Sender: "alice@example.com", Subject: "cached", Date: time.Now(), Folder: "INBOX"},
	}
	updatedModel, _ := m.Update(StartupHydratedMsg{Emails: emails})
	updated := updatedModel.(*Model)

	if updated.timeline.emails == nil || len(updated.timeline.emails) != 1 {
		t.Fatalf("expected cached timeline emails to be loaded, got %#v", updated.timeline.emails)
	}
	if !updated.loading {
		t.Fatal("expected progressive startup hydrate to keep loading active")
	}
	if updated.statusMessage != "" {
		t.Fatalf("expected no explicit cached-data status message, got %q", updated.statusMessage)
	}
}

type slowCloseBackend struct {
	stubBackend
	delay time.Duration
}

func (b *slowCloseBackend) Close() error {
	time.Sleep(b.delay)
	return nil
}

func TestHandleKeyMsg_QuitDoesNotBlockOnBackendClose(t *testing.T) {
	m := New(&slowCloseBackend{delay: 300 * time.Millisecond}, nil, "", nil, false)

	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _ = m.handleKeyMsg(keyRunes("q"))
	}()

	select {
	case <-done:
	case <-time.After(100 * time.Millisecond):
		t.Fatal("expected quit handling to return without waiting for backend close")
	}
}

func TestHandleKeyMsg_QuitWorksFromTimelineSearchInput(t *testing.T) {
	m := New(&slowCloseBackend{delay: 300 * time.Millisecond}, nil, "", nil, false)
	m.activeTab = tabTimeline
	m.timeline.searchMode = true
	m.timeline.searchFocus = timelineSearchFocusInput
	m.timeline.searchInput.SetValue("distributed systems")

	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _ = m.handleKeyMsg(keyRunes("q"))
	}()

	select {
	case <-done:
	case <-time.After(100 * time.Millisecond):
		t.Fatal("expected global quit to work from timeline search input")
	}
}
