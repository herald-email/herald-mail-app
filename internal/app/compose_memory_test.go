package app

import (
	"context"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
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

func composeMemoryTestNudge() memory.Nudge {
	return memory.Nudge{
		ID:          "nudge-sergey",
		Type:        memory.NudgeTypeOpenLoop,
		Message:     "Open question: Sergey asked for availability.",
		Why:         "There is an unresolved reply thread.",
		Confidence:  0.91,
		ActionState: memory.NudgeActionNew,
		Evidence: []memory.Evidence{{
			SourceType: memory.SourceEmail,
			MessageID:  "msg-sergey",
			Folder:     "INBOX",
		}},
	}
}

func keyAltRunes(s string) tea.KeyPressMsg {
	msg := keyRunes(s)
	msg.Mod = tea.ModAlt
	return msg
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

func TestComposeMemoryRadarActionsAreExplicitAndLocal(t *testing.T) {
	m := New(&memoryStubBackend{}, nil, "", nil, false)
	m.loading = false
	m.activeTab = tabCompose
	m.composeTo.SetValue("sergey@example.com")
	m.composeSubject.SetValue("Senior engineer interview")
	m.composeBody.SetValue("Thanks Sergey")
	m.composeBody.CursorEnd()
	m.replyContextEmail = mockEmails()[0]
	m.composeMemoryPrep = memory.ReplyPrep{Nudges: []memory.Nudge{composeMemoryTestNudge()}}

	bodyBefore := m.composeBody.Value()
	if cmd := m.performComposeMemoryRadarAction(composeMemoryActionSource); cmd != nil {
		t.Fatal("source inspection should be local")
	}
	if m.composeBody.Value() != bodyBefore {
		t.Fatal("source inspection mutated the draft body")
	}
	if !strings.Contains(m.composeStatus, "msg-sergey") || !strings.Contains(m.composeStatus, "INBOX") {
		t.Fatalf("source status = %q", m.composeStatus)
	}
	if m.composeMemoryPrep.Nudges[0].ActionState != memory.NudgeActionNew {
		t.Fatalf("source inspection changed action state to %q", m.composeMemoryPrep.Nudges[0].ActionState)
	}

	if cmd := m.performComposeMemoryRadarAction(composeMemoryActionInsert); cmd == nil {
		t.Fatal("insert should schedule a local Radar refresh after changing body context")
	}
	if !strings.Contains(m.composeBody.Value(), "Open question: Sergey asked for availability.") {
		t.Fatalf("inserted body = %q", m.composeBody.Value())
	}
	if m.composeMemoryPrep.Nudges[0].ActionState != memory.NudgeActionInserted {
		t.Fatalf("insert action state = %q", m.composeMemoryPrep.Nudges[0].ActionState)
	}

	bodyBefore = m.composeBody.Value()
	m.composeMemoryPrep = memory.ReplyPrep{Nudges: []memory.Nudge{composeMemoryTestNudge()}}
	if cmd := m.performComposeMemoryRadarAction(composeMemoryActionDismiss); cmd != nil {
		t.Fatal("dismiss should be local")
	}
	if len(m.composeMemoryPrep.Nudges) != 0 {
		t.Fatalf("dismiss kept nudges: %#v", m.composeMemoryPrep.Nudges)
	}
	if m.composeBody.Value() != bodyBefore {
		t.Fatal("dismiss mutated the draft body")
	}

	m.composeMemoryPrep = memory.ReplyPrep{Nudges: []memory.Nudge{composeMemoryTestNudge()}}
	if cmd := m.performComposeMemoryRadarAction(composeMemoryActionResolve); cmd != nil {
		t.Fatal("resolve should be local")
	}
	if m.composeMemoryPrep.Nudges[0].ActionState != memory.NudgeActionResolved {
		t.Fatalf("resolve action state = %q", m.composeMemoryPrep.Nudges[0].ActionState)
	}

	if cmd := m.performComposeMemoryRadarAction(composeMemoryActionSave); cmd != nil {
		t.Fatal("save should be local")
	}
	if m.composeMemoryPrep.Nudges[0].ActionState != memory.NudgeActionSaved {
		t.Fatalf("save action state = %q", m.composeMemoryPrep.Nudges[0].ActionState)
	}

	bodyBefore = m.composeBody.Value()
	if cmd := m.performComposeMemoryRadarAction(composeMemoryActionResearchPerson); cmd != nil {
		t.Fatal("person research intent should be local")
	}
	if m.composeBody.Value() != bodyBefore {
		t.Fatal("person research intent mutated the draft body")
	}
	if !strings.Contains(m.composeStatus, "Research Mode is opt-in") || !strings.Contains(m.composeStatus, "sergey@example.com") {
		t.Fatalf("person research status = %q", m.composeStatus)
	}
	if cmd := m.performComposeMemoryRadarAction(composeMemoryActionResearchCompany); cmd != nil {
		t.Fatal("company research intent should be local")
	}
	if m.composeBody.Value() != bodyBefore {
		t.Fatal("company research intent mutated the draft body")
	}
	if !strings.Contains(m.composeStatus, "example.com") {
		t.Fatalf("company research status = %q", m.composeStatus)
	}
}

func TestComposeRadarAltShortcutsDoNotStealPlainText(t *testing.T) {
	m := New(&memoryStubBackend{}, nil, "", nil, false)
	m.loading = false
	m.activeTab = tabCompose
	m.composeField = composeFieldBody
	m.composeBody.Focus()
	m.composeTo.SetValue("sergey@example.com")
	m.composeSubject.SetValue("Senior engineer interview")
	m.composeBody.SetValue("Start")
	m.composeBody.CursorEnd()
	m.replyContextEmail = mockEmails()[0]
	m.composeMemoryPrep = memory.ReplyPrep{Nudges: []memory.Nudge{composeMemoryTestNudge()}}

	model, cmd := m.handleComposeKey(keyRunes("i"))
	updated := model.(*Model)
	if cmd == nil {
		t.Fatal("plain text body edit should still schedule Radar refresh")
	}
	if updated.composeBody.Value() != "Starti" {
		t.Fatalf("plain i body = %q", updated.composeBody.Value())
	}
	if updated.composeMemoryPrep.Nudges[0].ActionState != memory.NudgeActionNew {
		t.Fatalf("plain i changed action state to %q", updated.composeMemoryPrep.Nudges[0].ActionState)
	}

	updated.composeMemoryPrep = memory.ReplyPrep{Nudges: []memory.Nudge{composeMemoryTestNudge()}}
	bodyBefore := updated.composeBody.Value()
	model, cmd = updated.handleComposeKey(keyAltRunes("i"))
	updated = model.(*Model)
	if cmd == nil {
		t.Fatal("alt+i insert should schedule Radar refresh")
	}
	if updated.composeBody.Value() == bodyBefore || !strings.Contains(updated.composeBody.Value(), "Open question: Sergey asked for availability.") {
		t.Fatalf("alt+i body = %q", updated.composeBody.Value())
	}
	if updated.composeMemoryPrep.Nudges[0].ActionState != memory.NudgeActionInserted {
		t.Fatalf("alt+i action state = %q", updated.composeMemoryPrep.Nudges[0].ActionState)
	}
}
