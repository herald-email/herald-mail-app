package app

import (
	"strings"
	"testing"

	"github.com/herald-email/herald-mail-app/internal/models"
)

type hideFutureBackend struct {
	stubBackend
	sender   string
	toFolder string
}

func (b *hideFutureBackend) SoftUnsubscribeSender(sender, toFolder string) error {
	b.sender = sender
	b.toFolder = toFolder
	return nil
}

func TestRenderEmailPreview_ShowsTagsAndActionsRows(t *testing.T) {
	m := makeSizedModel(t, 120, 40)
	m.activeTab = tabTimeline
	m.focusedPanel = panelPreview
	m.timeline.previewWidth = 60
	m.timeline.selectedEmail = &models.EmailData{
		MessageID: "msg-1",
		Sender:    "Tech Weekly <newsletter@techweekly.example>",
		Subject:   "This week in tech",
	}
	m.classifications = map[string]string{"msg-1": "news"}
	m.timeline.bodyMessageID = "msg-1"
	m.timeline.body = &models.EmailBody{
		TextPlain:       "hello",
		ListUnsubscribe: "<mailto:leave@example.com>",
	}

	rendered := stripANSI(m.renderEmailPreview())
	if !strings.Contains(rendered, "Tags: news") {
		t.Fatalf("expected preview to show tag row, got:\n%s", rendered)
	}
	if !strings.Contains(rendered, "Actions:") {
		t.Fatalf("expected preview to show actions row, got:\n%s", rendered)
	}
	if !strings.Contains(rendered, "u unsubscribe") {
		t.Fatalf("expected preview to advertise unsubscribe action, got:\n%s", rendered)
	}
	if !strings.Contains(rendered, "H hide future mail") {
		t.Fatalf("expected preview to advertise hide-future action, got:\n%s", rendered)
	}
}

func TestRenderEmailPreview_HidesUnsubscribeActionWhenHeaderMissing(t *testing.T) {
	m := makeSizedModel(t, 120, 40)
	m.activeTab = tabTimeline
	m.focusedPanel = panelPreview
	m.timeline.previewWidth = 60
	m.timeline.selectedEmail = &models.EmailData{
		MessageID: "msg-2",
		Sender:    "Updates <updates@example.com>",
		Subject:   "Product update",
	}
	m.timeline.bodyMessageID = "msg-2"
	m.timeline.body = &models.EmailBody{TextPlain: "hello"}

	rendered := stripANSI(m.renderEmailPreview())
	if !strings.Contains(rendered, "Actions:") {
		t.Fatalf("expected preview to show actions row, got:\n%s", rendered)
	}
	if strings.Contains(rendered, "u unsubscribe") {
		t.Fatalf("expected preview to hide unsubscribe when header is missing, got:\n%s", rendered)
	}
	if !strings.Contains(rendered, "H hide future mail") {
		t.Fatalf("expected preview to keep hide-future action, got:\n%s", rendered)
	}
}

func TestRenderKeyHints_TimelinePreviewShowsHideFutureMailAndConditionalUnsubscribe(t *testing.T) {
	t.Run("with list unsubscribe", func(t *testing.T) {
		m := makeSizedModel(t, 120, 40)
		m.activeTab = tabTimeline
		m.focusedPanel = panelPreview
		m.timeline.selectedEmail = &models.EmailData{MessageID: "msg-1", Sender: "sender@example.com"}
		m.timeline.bodyMessageID = "msg-1"
		m.timeline.body = &models.EmailBody{TextPlain: "hello", ListUnsubscribe: "<mailto:leave@example.com>"}

		hints := stripANSI(m.renderKeyHints())
		if !strings.Contains(hints, "u: unsubscribe") {
			t.Fatalf("expected preview hints to advertise unsubscribe, got %q", hints)
		}
		if !strings.Contains(hints, "H: hide future mail") {
			t.Fatalf("expected preview hints to advertise hide-future action, got %q", hints)
		}
	})

	t.Run("without list unsubscribe", func(t *testing.T) {
		m := makeSizedModel(t, 120, 40)
		m.activeTab = tabTimeline
		m.focusedPanel = panelPreview
		m.timeline.selectedEmail = &models.EmailData{MessageID: "msg-2", Sender: "sender@example.com"}
		m.timeline.bodyMessageID = "msg-2"
		m.timeline.body = &models.EmailBody{TextPlain: "hello"}

		hints := stripANSI(m.renderKeyHints())
		if strings.Contains(hints, "u: unsubscribe") {
			t.Fatalf("expected preview hints to hide unsubscribe when header is missing, got %q", hints)
		}
		if !strings.Contains(hints, "H: hide future mail") {
			t.Fatalf("expected preview hints to keep hide-future action, got %q", hints)
		}
	})
}

func TestHandleTimelineKey_HCreatesHideFutureMailRule(t *testing.T) {
	backend := &hideFutureBackend{}
	m := New(backend, nil, "", nil, false)
	m.activeTab = tabTimeline
	m.loading = false
	m.timeline.selectedEmail = &models.EmailData{
		MessageID: "msg-1",
		Sender:    "Tech Weekly <newsletter@techweekly.example>",
	}

	_, cmd, handled := m.handleTimelineKey(keyRune('H'))
	if !handled {
		t.Fatal("expected H to be handled in timeline preview")
	}
	if cmd == nil {
		t.Fatal("expected h to return a hide-future-mail command")
	}

	msg := cmd()
	result, ok := msg.(SoftUnsubResultMsg)
	if !ok {
		t.Fatalf("expected SoftUnsubResultMsg, got %T", msg)
	}
	if result.Sender != "Tech Weekly <newsletter@techweekly.example>" {
		t.Fatalf("expected sender to propagate through result, got %q", result.Sender)
	}
	if backend.sender != "Tech Weekly <newsletter@techweekly.example>" {
		t.Fatalf("expected backend to receive sender, got %q", backend.sender)
	}
}
