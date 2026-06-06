package app

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/herald-email/herald-mail-app/internal/memory"
)

type memoryStubBackend struct {
	stubBackend
	prep      memory.ReplyPrep
	lastQuery memory.ReplyPrepQuery
	calls     int
}

func (b *memoryStubBackend) BuildReplyMemoryContext(_ context.Context, query memory.ReplyPrepQuery) (memory.ReplyPrep, error) {
	b.calls++
	b.lastQuery = query
	return b.prep, nil
}

func TestComposeMemoryRadarLoadsWithoutMutatingDraft(t *testing.T) {
	prep := memory.ReplyPrep{
		Nudges: []memory.Nudge{{
			Type:       "open_loop",
			Message:    "Open question: Sergey asked for availability.",
			Confidence: 0.91,
			Evidence: []memory.Evidence{{
				SourceType: memory.SourceEmail,
				MessageID:  "msg-sergey",
				Folder:     "INBOX",
			}},
		}},
	}
	backend := &memoryStubBackend{prep: prep}
	m := New(backend, nil, "", nil, false)
	m.loading = false
	m.activeTab = tabCompose
	m.composeTo.SetValue("sergey@example.com")
	m.composeSubject.SetValue("Senior engineer interview")
	m.composeBody.SetValue("Thanks Sergey,\n\nI can send this today.")
	m.replyContextEmail = mockEmails()[0]

	cmd := m.startComposeMemoryRadar()
	if cmd == nil {
		t.Fatal("expected Compose Radar command")
	}
	beforeTo := m.composeTo.Value()
	beforeSubject := m.composeSubject.Value()
	beforeBody := m.composeBody.Value()

	raw := cmd()
	msg, ok := raw.(ComposeMemoryRadarMsg)
	if !ok {
		t.Fatalf("radar command returned %T", raw)
	}
	updatedModel, _ := m.Update(msg)
	updated := updatedModel.(*Model)

	if updated.composeTo.Value() != beforeTo || updated.composeSubject.Value() != beforeSubject || updated.composeBody.Value() != beforeBody {
		t.Fatalf("Compose Radar mutated draft fields")
	}
	if len(updated.composeMemoryPrep.Nudges) != 1 || updated.composeMemoryLoading {
		t.Fatalf("radar state = loading %v prep %#v", updated.composeMemoryLoading, updated.composeMemoryPrep)
	}
	if backend.lastQuery.Recipient != "sergey@example.com" || backend.lastQuery.Subject != "Senior engineer interview" {
		t.Fatalf("radar query = %#v", backend.lastQuery)
	}
}

func TestComposeMemoryRadarDebounceIgnoresStaleDraftContext(t *testing.T) {
	backend := &memoryStubBackend{}
	m := New(backend, nil, "", nil, false)
	m.loading = false
	m.activeTab = tabCompose
	m.composeTo.SetValue("sergey@example.com")
	m.composeSubject.SetValue("Senior engineer interview")
	m.composeBody.SetValue("old body")
	m.replyContextEmail = mockEmails()[0]

	m.composeMemoryDebounceToken++
	stale := ComposeMemoryRadarDebounceMsg{
		Token:     m.composeMemoryDebounceToken,
		Signature: m.composeMemoryRadarSignature(),
	}
	m.composeBody.SetValue("new body")

	updatedModel, cmd := m.Update(stale)
	updated := updatedModel.(*Model)
	if cmd != nil {
		t.Fatal("stale debounce should not start a memory refresh command")
	}
	if backend.calls != 0 || updated.composeMemoryLoading {
		t.Fatalf("stale debounce started radar: calls=%d loading=%v", backend.calls, updated.composeMemoryLoading)
	}

	cmd = updated.scheduleComposeMemoryRadarRefresh()
	if cmd == nil {
		t.Fatal("expected current debounce command for reply compose")
	}
	current := ComposeMemoryRadarDebounceMsg{
		Token:     updated.composeMemoryDebounceToken,
		Signature: updated.composeMemoryRadarSignature(),
	}
	updatedModel, cmd = updated.Update(current)
	updated = updatedModel.(*Model)
	if cmd == nil {
		t.Fatal("current debounce should start Compose Radar refresh")
	}
	raw := cmd()
	msg, ok := raw.(ComposeMemoryRadarMsg)
	if !ok {
		t.Fatalf("radar command returned %T", raw)
	}
	updatedModel, _ = updated.Update(msg)
	updated = updatedModel.(*Model)

	if backend.calls != 1 {
		t.Fatalf("BuildReplyMemoryContext calls = %d, want 1", backend.calls)
	}
	if backend.lastQuery.DraftExcerpt != "new body" {
		t.Fatalf("draft excerpt = %q, want latest body", backend.lastQuery.DraftExcerpt)
	}
	if updated.composeTo.Value() != "sergey@example.com" || updated.composeSubject.Value() != "Senior engineer interview" || updated.composeBody.Value() != "new body" {
		t.Fatalf("debounced radar mutated draft fields")
	}
}

func TestComposeBodyEditSchedulesMemoryRadarRefreshForRepliesOnly(t *testing.T) {
	backend := &memoryStubBackend{}
	m := New(backend, nil, "", nil, false)
	m.loading = false
	m.activeTab = tabCompose
	m.composeField = composeFieldBody
	m.composeBody.Focus()
	m.composeTo.SetValue("sergey@example.com")
	m.composeSubject.SetValue("Senior engineer interview")
	m.composeBody.SetValue("Thanks")
	m.composeBody.CursorEnd()
	m.replyContextEmail = mockEmails()[0]

	beforeToken := m.composeMemoryDebounceToken
	model, cmd := m.handleComposeKey(keyRunes("!"))
	updated := model.(*Model)
	if cmd == nil {
		t.Fatal("body edit should return a batched command with radar debounce")
	}
	if updated.composeMemoryDebounceToken == beforeToken {
		t.Fatal("body edit did not schedule Compose Radar debounce")
	}
	if updated.composeBody.Value() != "Thanks!" {
		t.Fatalf("body edit value = %q", updated.composeBody.Value())
	}

	updated.replyContextEmail = nil
	beforeToken = updated.composeMemoryDebounceToken
	model, _ = updated.handleComposeKey(keyRunes("?"))
	updated = model.(*Model)
	if updated.composeMemoryDebounceToken != beforeToken {
		t.Fatal("new compose without reply context should not schedule Compose Radar debounce")
	}
}

func TestRenderComposeMemoryRadarShowsBoundedSourceBackedLine(t *testing.T) {
	m := &Model{theme: defaultTheme, windowWidth: 100, windowHeight: 40}
	m.composeMemoryPrep = memory.ReplyPrep{
		GeneratedAt: time.Now(),
		Nudges: []memory.Nudge{{
			Type:       "open_loop",
			Message:    strings.Repeat("Sergey asked for availability. ", 10),
			Confidence: 0.91,
			Evidence: []memory.Evidence{{
				SourceType: memory.SourceEmail,
				MessageID:  "msg-sergey",
			}},
		}},
	}

	rendered := m.renderComposeMemoryRadar(60)

	if !strings.Contains(rendered, "Radar") || !strings.Contains(rendered, "msg-sergey") {
		t.Fatalf("rendered radar missing label/source:\n%s", rendered)
	}
	for _, line := range strings.Split(rendered, "\n") {
		if len([]rune(line)) > 120 {
			t.Fatalf("radar line appears unbounded: %q", line)
		}
	}
}
