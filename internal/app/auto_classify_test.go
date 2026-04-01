package app

import (
	"errors"
	"testing"
	"time"

	"mail-processor/internal/ai"
	"mail-processor/internal/models"
)

// makeAutoClassifyModel builds a minimal Model suitable for auto-classify tests.
// It sets up the ruleRequestCh so that non-blocking sends can be observed.
func makeAutoClassifyModel(classifier ai.AIClient) *Model {
	m := &Model{
		expandedThreads: make(map[string]bool),
		classifications: make(map[string]string),
		backend:         &stubBackend{},
		classifier:      classifier,
		ruleRequestCh:   make(chan models.RuleRequest, 20),
		ruleResultCh:    make(chan models.RuleResult, 50),
	}
	m.timelineEmails = []*models.EmailData{
		{
			MessageID: "msg-new",
			Subject:   "New arrival",
			Sender:    "sender@example.com",
			Folder:    "INBOX",
			Date:      time.Now(),
		},
	}
	return m
}

// TestAutoClassifyOnArrival verifies that NewEmailsMsg with an unclassified email
// and a configured classifier dispatches autoClassifyEmailCmd (non-nil cmd) and
// that running it returns AutoClassifyResultMsg with the expected category.
func TestAutoClassifyOnArrival(t *testing.T) {
	classifier := &stubClassifier{category: "imp"}
	m := makeAutoClassifyModel(classifier)
	// Remove the email from timeline so it only exists in the incoming message.
	m.timelineEmails = nil

	email := &models.EmailData{
		MessageID: "msg-arrival",
		Subject:   "Hello",
		Sender:    "boss@corp.com",
		Folder:    "INBOX",
		Date:      time.Now(),
	}

	_, cmd := m.Update(NewEmailsMsg{Emails: []*models.EmailData{email}, Folder: ""})
	if cmd == nil {
		t.Fatal("expected a non-nil cmd after NewEmailsMsg with unclassified email")
	}

	// Running the batch cmd should eventually produce an AutoClassifyResultMsg.
	// We call the individual autoClassifyEmailCmd directly to verify correctness.
	autoCmd := m.autoClassifyEmailCmd(email)
	if autoCmd == nil {
		t.Fatal("autoClassifyEmailCmd returned nil")
	}
	msg := autoCmd()
	result, ok := msg.(AutoClassifyResultMsg)
	if !ok {
		t.Fatalf("expected AutoClassifyResultMsg, got %T", msg)
	}
	if result.Err != nil {
		t.Fatalf("unexpected error: %v", result.Err)
	}
	if result.MessageID != "msg-arrival" {
		t.Errorf("MessageID = %q, want %q", result.MessageID, "msg-arrival")
	}
	if result.Category != "imp" {
		t.Errorf("Category = %q, want %q", result.Category, "imp")
	}
}

// TestAutoClassifyTriggersRule verifies that handling AutoClassifyResultMsg
// stores the classification and sends the email to the rule engine.
func TestAutoClassifyTriggersRule(t *testing.T) {
	m := makeAutoClassifyModel(nil) // classifier not needed for this path

	// The email must be in timelineEmails so the handler can look it up.
	// makeAutoClassifyModel already set msg-new in timelineEmails.

	_, _ = m.Update(AutoClassifyResultMsg{
		MessageID: "msg-new",
		Category:  "news",
	})

	if m.classifications["msg-new"] != "news" {
		t.Errorf("classifications[msg-new] = %q, want %q", m.classifications["msg-new"], "news")
	}

	select {
	case req := <-m.ruleRequestCh:
		if req.Email.MessageID != "msg-new" {
			t.Errorf("rule request email ID = %q, want %q", req.Email.MessageID, "msg-new")
		}
		if req.Category != "news" {
			t.Errorf("rule request category = %q, want %q", req.Category, "news")
		}
	default:
		t.Error("expected a rule request to be sent to ruleRequestCh")
	}
}

// TestAutoClassifyTriggersRule_Error verifies that an error result still attempts
// to forward to the rule engine with an empty category (best-effort).
func TestAutoClassifyTriggersRule_Error(t *testing.T) {
	m := makeAutoClassifyModel(nil)

	_, _ = m.Update(AutoClassifyResultMsg{
		MessageID: "msg-new",
		Err:       errors.New("AI offline"),
	})

	// Classification must NOT be stored on error.
	if m.classifications["msg-new"] != "" {
		t.Errorf("expected no classification on error, got %q", m.classifications["msg-new"])
	}

	// Rule engine should still receive the email with empty category.
	select {
	case req := <-m.ruleRequestCh:
		if req.Email.MessageID != "msg-new" {
			t.Errorf("rule request email ID = %q, want %q", req.Email.MessageID, "msg-new")
		}
		if req.Category != "" {
			t.Errorf("rule request category = %q, want empty on error", req.Category)
		}
	default:
		t.Error("expected a rule request even on classification error")
	}
}

// TestAutoClassifyNilClassifier verifies that when classifier is nil, NewEmailsMsg
// does not dispatch an auto-classify command but still forwards already-classified
// emails to the rule engine.
func TestAutoClassifyNilClassifier(t *testing.T) {
	m := makeAutoClassifyModel(nil) // nil classifier

	// Pre-populate a classification for the email.
	m.classifications["msg-pre"] = "sub"
	preEmail := &models.EmailData{
		MessageID: "msg-pre",
		Subject:   "Pre-classified",
		Sender:    "promo@shop.com",
		Folder:    "INBOX",
		Date:      time.Now(),
	}
	// Add it to timeline so the rule-send path in NewEmailsMsg can find it (not
	// strictly required for the direct send in the handler, but mirrors real state).
	m.timelineEmails = append(m.timelineEmails, preEmail)

	unclassifiedEmail := &models.EmailData{
		MessageID: "msg-unclassified",
		Subject:   "Fresh",
		Sender:    "new@example.com",
		Folder:    "INBOX",
		Date:      time.Now(),
	}

	_, cmd := m.Update(NewEmailsMsg{
		Emails: []*models.EmailData{preEmail, unclassifiedEmail},
		Folder: "",
	})

	// cmd should be non-nil (at least listenForNewEmails re-subscription), but
	// must NOT include an auto-classify cmd for the unclassified email.
	// We verify this indirectly: autoClassifyEmailCmd must NOT be callable with a nil classifier
	// without panicking, and no AutoClassifyResultMsg should be produced.
	if cmd == nil {
		t.Error("expected at least listenForNewEmails re-subscription cmd")
	}

	// The pre-classified email should have been sent to the rule engine directly.
	select {
	case req := <-m.ruleRequestCh:
		if req.Email.MessageID != "msg-pre" {
			t.Errorf("rule request email ID = %q, want %q", req.Email.MessageID, "msg-pre")
		}
		if req.Category != "sub" {
			t.Errorf("rule request category = %q, want %q", req.Category, "sub")
		}
	default:
		t.Error("expected pre-classified email to be forwarded to ruleRequestCh")
	}

	// No second request should be queued (unclassified email with nil classifier).
	select {
	case extra := <-m.ruleRequestCh:
		t.Errorf("unexpected extra rule request for %q", extra.Email.MessageID)
	default:
		// correct — nothing queued for the unclassified email
	}
}
