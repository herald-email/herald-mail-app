package memory

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/herald-email/herald-mail-app/internal/models"
	"gopkg.in/yaml.v3"
)

func testTime() time.Time {
	return time.Date(2026, 6, 6, 12, 0, 0, 0, time.UTC)
}

func TestSettingsDefaultsAreObsidianFriendlyAndImmutable(t *testing.T) {
	settings := DefaultSettings()

	if !settings.Enabled {
		t.Fatal("memories should be enabled by default for cached local mail")
	}
	if settings.Directory != DefaultDirectory {
		t.Fatalf("Directory = %q, want %q", settings.Directory, DefaultDirectory)
	}
	if !settings.Immutable {
		t.Fatal("memories should be immutable by default")
	}
	if got := strings.Join(settings.Sources.Folders, ","); got != "INBOX,Sent" {
		t.Fatalf("folders = %q, want INBOX,Sent", got)
	}
	if settings.Destinations.People != "People" || settings.Destinations.DailyBriefing != "Scheduled Task Artifacts" {
		t.Fatalf("destinations = %#v", settings.Destinations)
	}
	if settings.Obsidian.FrontmatterMode != FrontmatterMinimal || !settings.Obsidian.YAMLHeaders {
		t.Fatalf("obsidian frontmatter defaults = %#v", settings.Obsidian)
	}
	if settings.Obsidian.LinkMode != LinkModeWiki || settings.Obsidian.TagMode != TagModeConservative {
		t.Fatalf("obsidian profile defaults = %#v", settings.Obsidian)
	}
	if len(settings.Prompts) == 0 {
		t.Fatal("expected default prompt templates")
	}
}

func TestSettingsCanDisableContactsAndHideYAMLHeaders(t *testing.T) {
	var settings Settings
	data := []byte(`
enabled: false
directory: /tmp/memory
sources:
  folders: [INBOX]
  contacts: false
obsidian:
  yaml_headers: false
  link_mode: markdown
  tag_mode: workflow
`)
	if err := yaml.Unmarshal(data, &settings); err != nil {
		t.Fatalf("yaml.Unmarshal: %v", err)
	}
	settings.ApplyDefaults()

	if settings.Enabled {
		t.Fatal("explicit enabled: false should be preserved")
	}
	if settings.Sources.Contacts {
		t.Fatal("explicit sources.contacts: false should be preserved")
	}
	if settings.Obsidian.FrontmatterMode != FrontmatterNone || settings.Obsidian.YAMLHeaders {
		t.Fatalf("YAML header toggle not applied: %#v", settings.Obsidian)
	}
	if settings.Obsidian.LinkMode != LinkModeMarkdown || settings.Obsidian.TagMode != TagModeWorkflow {
		t.Fatalf("profile modes not normalized: %#v", settings.Obsidian)
	}
}

func TestSettingsCanPreserveExplicitUpdateRuleFalse(t *testing.T) {
	var settings Settings
	data := []byte(`
update_rules:
  conflict_creates_state: false
`)
	if err := yaml.Unmarshal(data, &settings); err != nil {
		t.Fatalf("yaml.Unmarshal: %v", err)
	}
	settings.ApplyDefaults()

	if settings.UpdateRules.ConflictCreatesState {
		t.Fatal("explicit update_rules.conflict_creates_state: false should be preserved")
	}
}

func TestFileStoreAppendIsImmutable(t *testing.T) {
	store, err := NewFileStoreWithClock(t.TempDir(), testTime)
	if err != nil {
		t.Fatalf("NewFileStoreWithClock: %v", err)
	}
	memory := testMemory("memory one")

	written, path, err := store.Append(context.Background(), memory)
	if err != nil {
		t.Fatalf("Append first: %v", err)
	}
	if written.ID == "" || !strings.Contains(path, written.ID+".json") {
		t.Fatalf("written/path = %#v %q", written, path)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat memory file: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("memory file mode = %v, want 0600", got)
	}
	_, _, err = store.Append(context.Background(), memory)
	if !errors.Is(err, ErrMemoryExists) {
		t.Fatalf("second append err = %v, want ErrMemoryExists", err)
	}

	listed, err := store.List(context.Background())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(listed) != 1 || listed[0].ID != written.ID {
		t.Fatalf("listed = %#v", listed)
	}
}

func TestExtractorBuildsJobSearchMemoriesFromInboxAndSent(t *testing.T) {
	inbound := &models.EmailData{
		MessageID: "msg-in",
		Sender:    "Sergey <sergey@example.com>",
		Subject:   "Re: Senior engineer interview",
		Folder:    "INBOX",
		Date:      testTime().Add(-2 * time.Hour),
	}
	sent := &models.EmailData{
		MessageID: "msg-sent",
		Sender:    "me@example.com",
		Subject:   "Re: Senior engineer interview",
		Folder:    "Sent",
		Date:      testTime().Add(-1 * time.Hour),
	}
	extractor := Extractor{Now: testTime, Settings: DefaultSettings(), UserAddresses: []string{"me@example.com"}}

	memories := extractor.Extract([]EmailSnapshot{
		{Email: inbound, BodyText: "Can you send your availability by Friday? We would like to schedule the next interview."},
		{Email: sent, BodyText: "I will send my availability today and follow up by Friday."},
	})

	if !hasMemoryKind(memories, KindLastContact) || !hasMemoryKind(memories, KindLastUserReply) || !hasMemoryKind(memories, KindOpenQuestion) || !hasMemoryKind(memories, KindTrackStatus) {
		t.Fatalf("expected last contact, user reply, open question, and track status memories: %#v", memories)
	}
	for _, memory := range memories {
		if memory.ID == "" || len(memory.Evidence) == 0 {
			t.Fatalf("memory missing immutable id or evidence: %#v", memory)
		}
		if strings.Contains(memory.Details.SourceQuote, "availability by Friday") && len([]rune(memory.Details.SourceQuote)) > 300 {
			t.Fatalf("source quote not bounded: %q", memory.Details.SourceQuote)
		}
	}
}

func TestReplyPrepPromotesOnlyHighConfidenceNudges(t *testing.T) {
	settings := DefaultSettings()
	memories := []Memory{
		testMemoryWithKind("open", KindOpenQuestion, 0.90),
		testMemoryWithKind("low", KindCommitment, 0.40),
		testMemoryWithKind("reply", KindLastUserReply, 0.88),
	}

	prep := BuildReplyPrepFromMemories(ReplyPrepQuery{Recipient: "sergey@example.com", Subject: "Interview"}, memories, settings)

	if len(prep.Memories) != 3 {
		t.Fatalf("prep memories = %d, want 3", len(prep.Memories))
	}
	if len(prep.Nudges) != 2 {
		t.Fatalf("nudges = %#v, want only high-confidence nudges", prep.Nudges)
	}
	if prep.Nudges[0].Evidence[0].MessageID == "" {
		t.Fatalf("nudge missing source evidence: %#v", prep.Nudges[0])
	}
}

func TestReplyPrepIncludesTopicFallbackForSentReplies(t *testing.T) {
	store, err := NewFileStoreWithClock(t.TempDir(), testTime)
	if err != nil {
		t.Fatalf("NewFileStoreWithClock: %v", err)
	}
	sentReply := testMemoryWithKind("You last replied about the interview schedule.", KindLastUserReply, 0.88)
	sentReply.Topic = "Senior engineer interview"
	sentReply.People = []string{"me@example.com"}
	if _, _, err := store.Append(context.Background(), sentReply); err != nil {
		t.Fatalf("Append sent reply memory: %v", err)
	}
	service := NewServiceWithStore(DefaultSettings(), store, nil)

	prep, err := service.BuildReplyPrep(context.Background(), ReplyPrepQuery{
		Recipient: "sergey@example.com",
		Subject:   "Re: Senior engineer interview",
	})
	if err != nil {
		t.Fatalf("BuildReplyPrep: %v", err)
	}
	if len(prep.Nudges) != 1 || prep.Nudges[0].Type != "related_reply" {
		t.Fatalf("prep should include topic-matched sent reply, got %#v", prep)
	}
}

func TestObsidianPreviewPreservesUserSectionsAndCanHideYAML(t *testing.T) {
	settings := DefaultSettings()
	settings.Obsidian.YAMLHeaders = false
	settings.Obsidian.FrontmatterMode = FrontmatterNone
	memory := testMemory("Sergey asked for an update.")
	memory.ObsidianTarget = "People/sergey@example.com.md"
	existing := "# Sergey\n\nUser-written notes stay here.\n\n<!-- HERALD:MEMORIES:BEGIN -->\nold generated\n<!-- HERALD:MEMORIES:END -->\n\nTail note.\n"

	previews := PreviewObsidianSync([]Memory{memory}, settings, map[string]string{memory.ObsidianTarget: existing})

	if len(previews) != 1 {
		t.Fatalf("previews = %d, want 1", len(previews))
	}
	preview := previews[0]
	if strings.Contains(preview.Generated, "---") {
		t.Fatalf("generated note should hide YAML headers:\n%s", preview.Generated)
	}
	if !strings.Contains(preview.Merged, "User-written notes stay here.") || !strings.Contains(preview.Merged, "Tail note.") {
		t.Fatalf("merged note did not preserve user sections:\n%s", preview.Merged)
	}
	if strings.Contains(preview.Merged, "old generated") {
		t.Fatalf("old generated section was not replaced:\n%s", preview.Merged)
	}
	if !preview.WouldUpdate || preview.WouldCreate {
		t.Fatalf("preview flags = %#v", preview)
	}
}

func testMemory(claim string) Memory {
	return testMemoryWithKind(claim, KindOpenQuestion, 0.90)
}

func testMemoryWithKind(claim, kind string, confidence float64) Memory {
	return PrepareMemoryForAppend(Memory{
		Kind:           kind,
		Claim:          claim,
		Summary:        claim,
		Topic:          "Interview",
		People:         []string{"sergey@example.com"},
		Company:        "Example",
		Domain:         "example.com",
		Status:         StatusWaiting,
		Confidence:     confidence,
		LastActivityAt: testTime(),
		Evidence: []Evidence{{
			SourceType: SourceEmail,
			MessageID:  "msg-" + strings.ReplaceAll(claim, " ", "-"),
			Folder:     "INBOX",
			Date:       testTime(),
			Snippet:    claim,
		}},
	}, testTime())
}

func hasMemoryKind(memories []Memory, kind string) bool {
	for _, memory := range memories {
		if memory.Kind == kind {
			return true
		}
	}
	return false
}
