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
}

func (b *memoryStubBackend) BuildReplyMemoryContext(_ context.Context, query memory.ReplyPrepQuery) (memory.ReplyPrep, error) {
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
