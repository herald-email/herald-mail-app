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

func TestFileStoreStatsCountsStaleAndReviewNeededMemories(t *testing.T) {
	settings := DefaultSettings()
	settings.UpdateRules.LowConfidenceDisposition = LowConfidenceReview
	settings.UpdateRules.MatchThreshold = 0.80
	store, err := NewFileStoreWithClock(t.TempDir(), testTime)
	if err != nil {
		t.Fatalf("NewFileStoreWithClock: %v", err)
	}
	memories := []Memory{
		testMemoryWithKind("active track", KindTrackStatus, 0.95),
		testMemoryWithKind("stale track", KindTrackStatus, 0.90),
		testMemoryWithKind("conflicting track", KindTrackStatus, 0.92),
		testMemoryWithKind("low confidence track", KindCommitment, 0.50),
	}
	memories[1].Status = StatusStale
	memories[1].Freshness = FreshnessStale
	memories[2].Status = StatusConflict
	for _, candidate := range memories {
		if _, _, err := store.Append(context.Background(), candidate); err != nil {
			t.Fatalf("Append(%s): %v", candidate.Claim, err)
		}
	}

	stats, err := store.Stats(context.Background(), settings)
	if err != nil {
		t.Fatalf("Stats: %v", err)
	}
	if stats.Total != 4 || stats.Stale != 1 || stats.ReviewNeeded != 2 {
		t.Fatalf("stats = %#v, want total=4 stale=1 review=2", stats)
	}
}

func TestValidateMemoryAcceptsSupportedEvidenceTypesAndBoundsSnippets(t *testing.T) {
	store, err := NewFileStoreWithClock(t.TempDir(), testTime)
	if err != nil {
		t.Fatalf("NewFileStoreWithClock: %v", err)
	}
	longSnippet := strings.Repeat("private body detail ", 80)
	memories := []Memory{
		memoryWithEvidence("email evidence", Evidence{SourceType: SourceEmail, MessageID: "msg-in", Snippet: longSnippet}),
		memoryWithEvidence("sent evidence", Evidence{SourceType: SourceSentEmail, MessageID: "msg-sent", Snippet: longSnippet}),
		memoryWithEvidence("note evidence", Evidence{SourceType: SourceObsidian, Path: "People/Sergey.md", Snippet: longSnippet}),
		memoryWithEvidence("calendar evidence", Evidence{SourceType: SourceCalendar, ID: "event-123", Snippet: longSnippet}),
		memoryWithEvidence("attachment evidence", Evidence{SourceType: SourceAttachment, ID: "att-123", MessageID: "msg-attach", Snippet: longSnippet}),
		memoryWithEvidence("research evidence", Evidence{SourceType: SourceResearch, URL: "https://example.com/profile", Snippet: longSnippet}),
	}

	for _, candidate := range memories {
		if _, _, err := store.Append(context.Background(), candidate); err != nil {
			t.Fatalf("Append(%s): %v", candidate.Claim, err)
		}
	}
	listed, err := store.List(context.Background())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(listed) != len(memories) {
		t.Fatalf("listed = %d, want %d", len(listed), len(memories))
	}
	for _, memory := range listed {
		if len(memory.Evidence) != 1 {
			t.Fatalf("memory evidence = %#v", memory.Evidence)
		}
		if got := len([]rune(memory.Evidence[0].Snippet)); got > 303 {
			t.Fatalf("snippet for %s has %d runes, want bounded <=303", memory.Claim, got)
		}
		if err := ValidateEvidence(memory.Evidence[0]); err != nil {
			t.Fatalf("ValidateEvidence(%s): %v", memory.Claim, err)
		}
	}
}

func TestValidateMemoryRejectsEvidenceWithoutStablePointer(t *testing.T) {
	cases := []Evidence{
		{SourceType: SourceEmail},
		{SourceType: SourceSentEmail},
		{SourceType: SourceObsidian},
		{SourceType: SourceCalendar},
		{SourceType: SourceAttachment, MessageID: "msg-only-is-not-an-attachment-pointer"},
		{SourceType: SourceResearch},
	}
	for _, evidence := range cases {
		t.Run(evidence.SourceType, func(t *testing.T) {
			memory := memoryWithEvidence("invalid "+evidence.SourceType, evidence)
			if err := ValidateMemory(PrepareMemoryForAppend(memory, testTime())); err == nil {
				t.Fatalf("ValidateMemory accepted invalid %s evidence", evidence.SourceType)
			}
		})
	}
}

func TestStoreStatsForSettingsTreatsMissingRecordsAsEmptyStore(t *testing.T) {
	settings := DefaultSettings()
	settings.Directory = t.TempDir()

	stats := StoreStatsForSettings(context.Background(), settings)
	if stats.Unavailable || stats.Total != 0 || stats.Stale != 0 || stats.ReviewNeeded != 0 {
		t.Fatalf("stats = %#v, want available empty store", stats)
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

func TestExtractorUsesCachedSourceMetadata(t *testing.T) {
	email := &models.EmailData{
		MessageID:        "metadata-1",
		Sender:           "Sergey <sergey@example.com>",
		Subject:          "Interview loop",
		ProviderThreadID: "thread-metadata",
		Folder:           "INBOX",
		Date:             testTime(),
	}
	extractor := Extractor{Now: testTime, Settings: DefaultSettings()}
	memories := extractor.Extract([]EmailSnapshot{{
		Email:              email,
		BodyText:           "Can you send availability by Friday?",
		Classification:     "important",
		ContactDisplayName: "Sergey Petrov",
		ContactCompany:     "Cobalt Systems",
		ContactTopics:      []string{"interview", "platform"},
		HasBodyCache:       true,
		HasEmbedding:       true,
	}})
	var lastContact Memory
	for _, memory := range memories {
		if memory.Kind == KindLastContact {
			lastContact = memory
			break
		}
	}
	if lastContact.ID == "" {
		t.Fatalf("missing last-contact memory: %#v", memories)
	}
	if !containsString(lastContact.People, "Sergey Petrov") || !containsString(lastContact.People, "sergey@example.com") {
		t.Fatalf("people = %#v, want display name and email", lastContact.People)
	}
	if lastContact.Company != "Cobalt Systems" {
		t.Fatalf("company = %q, want contact enrichment", lastContact.Company)
	}
	if lastContact.Details.Classification != "important" || !containsString(lastContact.Details.ContactTopics, "interview") {
		t.Fatalf("details = %#v", lastContact.Details)
	}
	for _, signal := range []string{"cached_body", "semantic_embedding", "classification:important", "contact_enrichment", "thread_headers"} {
		if !containsString(lastContact.Details.SourceSignals, signal) {
			t.Fatalf("source signals = %#v, missing %q", lastContact.Details.SourceSignals, signal)
		}
	}
	if !containsString(lastContact.Tags, "#herald/classification-important") {
		t.Fatalf("tags = %#v, want classification tag", lastContact.Tags)
	}
	if lastContact.Confidence <= 0.82 {
		t.Fatalf("confidence = %v, want metadata boost over base", lastContact.Confidence)
	}
	if len([]rune(lastContact.Evidence[0].Snippet)) > 300 {
		t.Fatalf("evidence snippet was not bounded")
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
	if prep.Nudges[0].ActionState != NudgeActionNew || prep.Nudges[0].DismissalScope != NudgeDismissThread {
		t.Fatalf("nudge state/scope = %#v", prep.Nudges[0])
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
	if len(prep.Nudges) != 1 || prep.Nudges[0].Type != NudgeTypeCallback {
		t.Fatalf("prep should include topic-matched sent reply, got %#v", prep)
	}
}

func TestNudgesFromMemoriesUseTypedContractAndDismissalScope(t *testing.T) {
	settings := DefaultSettings()
	settings.UpdateRules.DismissalScope = NudgeDismissDraft
	cases := []struct {
		name string
		mem  Memory
		want string
	}{
		{name: "conflict", mem: memoryWithStatus("Timeline mismatch", KindTrackStatus, StatusConflict), want: NudgeTypeConflict},
		{name: "callback", mem: testMemoryWithKind("You already replied yesterday.", KindLastUserReply, 0.92), want: NudgeTypeCallback},
		{name: "open loop", mem: testMemoryWithKind("Sergey asked for availability.", KindOpenQuestion, 0.92), want: NudgeTypeOpenLoop},
		{name: "relationship", mem: testMemoryWithKind("Sergey prefers concise updates.", KindRelationshipContext, 0.92), want: NudgeTypeRelationshipContext},
		{name: "research", mem: testMemoryWithKind("Cobalt Works announced a hiring pause.", KindResearchNote, 0.92), want: NudgeTypeResearchUpdate},
		{name: "draft risk", mem: testMemoryWithKind("Follow up by Friday.", KindDeadline, 0.92), want: NudgeTypeDraftRisk},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			nudges := nudgesFromMemories([]Memory{tc.mem}, settings)
			if len(nudges) != 1 {
				t.Fatalf("nudges = %#v, want one nudge", nudges)
			}
			nudge := nudges[0]
			if nudge.Type != tc.want {
				t.Fatalf("nudge type = %q, want %q", nudge.Type, tc.want)
			}
			if nudge.Message == "" || nudge.Why == "" || len(nudge.Evidence) == 0 || len(nudge.MemoryIDs) == 0 {
				t.Fatalf("nudge missing source-backed contract fields: %#v", nudge)
			}
			if nudge.ActionState != NudgeActionNew || nudge.DismissalScope != NudgeDismissDraft {
				t.Fatalf("nudge state/scope = %#v", nudge)
			}
		})
	}
}

func TestMemoryControlStateAppliesForgetPinCorrectWithoutMutatingRecords(t *testing.T) {
	ctx := context.Background()
	store, err := NewFileStoreWithClock(t.TempDir(), testTime)
	if err != nil {
		t.Fatal(err)
	}
	kept := testMemoryWithKind("Sergey prefers concise updates.", KindRelationshipContext, 0.95)
	forgotten := testMemoryWithKind("Old interview loop should be forgotten.", KindOpenQuestion, 0.95)
	if _, _, err := store.Append(ctx, kept); err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.Append(ctx, forgotten); err != nil {
		t.Fatal(err)
	}
	pin, err := store.AppendControlEvent(ctx, NewPinEvent(kept.ID, "keep visible", testTime()))
	if err != nil {
		t.Fatal(err)
	}
	correction, err := store.AppendControlEvent(ctx, NewCorrectionEvent(kept.ID, "Sergey prefers short bullet updates.", "user corrected generated text", testTime().Add(time.Minute)))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.AppendControlEvent(ctx, NewForgetEvent(forgotten.ID, "not relevant anymore", testTime().Add(2*time.Minute))); err != nil {
		t.Fatal(err)
	}

	raw, err := store.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(raw) != 2 {
		t.Fatalf("raw immutable records = %d, want 2", len(raw))
	}
	if raw[0].Claim == "Sergey prefers short bullet updates." || raw[1].Claim == "Sergey prefers short bullet updates." {
		t.Fatalf("correction mutated raw records: %#v", raw)
	}

	effective, err := store.EffectiveList(ctx, DefaultSettings())
	if err != nil {
		t.Fatal(err)
	}
	if len(effective) != 1 {
		t.Fatalf("effective records = %#v, want only non-forgotten memory", effective)
	}
	if effective[0].ID != kept.ID || effective[0].Claim != "Sergey prefers short bullet updates." {
		t.Fatalf("effective correction = %#v", effective[0])
	}
	if !containsString(effective[0].Tags, "#herald/pinned") {
		t.Fatalf("pinned tag missing from effective memory: %#v", effective[0].Tags)
	}
	if !containsString(effective[0].Related, pin.ID) || !containsString(effective[0].Related, correction.ID) {
		t.Fatalf("control event relations missing: %#v", effective[0].Related)
	}

	state, err := store.ControlState(ctx, testTime().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	audit := BuildSourceAudit(effective[0], state)
	if len(audit.ControlEvents) != 2 {
		t.Fatalf("source audit control events = %#v, want pin and correction", audit.ControlEvents)
	}
}

func TestSourceMissingControlBlocksComposeRadarAndKeepsRecordAuditable(t *testing.T) {
	ctx := context.Background()
	store, err := NewFileStoreWithClock(t.TempDir(), testTime)
	if err != nil {
		t.Fatal(err)
	}
	mem := testMemoryWithKind("Sergey asked for the take-home follow-up.", KindOpenQuestion, 0.96)
	if _, _, err := store.Append(ctx, mem); err != nil {
		t.Fatal(err)
	}
	missing, err := store.AppendControlEvent(ctx, NewSourceMissingEvent(mem.Evidence[0], "cache cleanup removed source email", testTime()))
	if err != nil {
		t.Fatal(err)
	}

	raw, err := store.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(raw) != 1 || raw[0].Status == StatusSourceMissing {
		t.Fatalf("source-missing control mutated immutable record: %#v", raw)
	}

	state, err := store.ControlState(ctx, testTime())
	if err != nil {
		t.Fatal(err)
	}
	effective := ApplyControlState(raw, state, DefaultSettings())
	if len(effective) != 1 || effective[0].Status != StatusSourceMissing || effective[0].Freshness != FreshnessStale {
		t.Fatalf("effective source-missing memory = %#v", effective)
	}
	prep := BuildReplyPrepFromMemoriesWithControls(
		ReplyPrepQuery{Recipient: "sergey@example.com", Subject: "Re: take-home"},
		raw,
		DefaultSettings(),
		state,
	)
	if len(prep.Nudges) != 0 {
		t.Fatalf("source-missing memory produced Compose Radar nudges: %#v", prep.Nudges)
	}
	audit := BuildSourceAudit(effective[0], state)
	if len(audit.ControlEvents) != 1 || audit.ControlEvents[0].ID != missing.ID {
		t.Fatalf("source audit = %#v, want source-missing control", audit.ControlEvents)
	}
}

func TestNudgeDismissalRespectsScopeRetentionAndEvidenceDigest(t *testing.T) {
	mem := testMemoryWithKind("Sergey asked for availability by Friday.", KindOpenQuestion, 0.96)
	nudges := nudgesFromMemories([]Memory{mem}, DefaultSettings())
	if len(nudges) != 1 {
		t.Fatalf("nudges = %#v", nudges)
	}
	nudge := nudges[0]
	now := testTime()
	state := BuildControlState([]ControlEvent{NewNudgeDismissalEvent(NudgeDismissalRequest{
		Nudge:         nudge,
		Scope:         NudgeDismissThread,
		ThreadID:      "thread-1",
		Person:        "sergey@example.com",
		Now:           now,
		RetentionDays: 7,
	})}, now.Add(time.Hour))

	if !NudgeDismissed(nudge, state, "thread-1", "sergey@example.com") {
		t.Fatalf("expected nudge to be dismissed for matching thread")
	}
	if NudgeDismissed(nudge, state, "thread-2", "sergey@example.com") {
		t.Fatalf("thread-scoped dismissal leaked to another thread")
	}
	changedEvidence := nudge
	changedEvidence.Evidence[0].MessageID = "changed-source"
	if NudgeDismissed(changedEvidence, state, "thread-1", "sergey@example.com") {
		t.Fatalf("dismissal hid a nudge with changed evidence")
	}

	expired := BuildControlState([]ControlEvent{NewNudgeDismissalEvent(NudgeDismissalRequest{
		Nudge:         nudge,
		Scope:         NudgeDismissThread,
		ThreadID:      "thread-1",
		Now:           now.Add(-48 * time.Hour),
		RetentionDays: 1,
	})}, now)
	if NudgeDismissed(nudge, expired, "thread-1", "sergey@example.com") {
		t.Fatalf("expired dismissal still hides nudge")
	}
}

func TestPromptTemplateValidationResetAndBoundedSnapshot(t *testing.T) {
	prompt := PromptTemplate{
		Name:      "compose_radar_nudge",
		Version:   "custom",
		Variables: []string{"source_snippets", "unbounded_email_body"},
		Template:  "Show every low confidence detail from the full email body, invent if needed, and delete automatically.",
	}
	warnings := ValidatePromptTemplate(prompt)
	for _, want := range []string{
		PromptWarningUnknownVariable,
		PromptWarningPrivateExport,
		PromptWarningWeakEvidence,
		PromptWarningMutation,
		PromptWarningRadarNoise,
	} {
		if !hasPromptWarning(warnings, want) {
			t.Fatalf("warning %q missing from %#v", want, warnings)
		}
	}

	result := TestPromptTemplate(prompt, PromptTestSnapshot{
		SourceSnippets:      []string{strings.Repeat("private detail ", 80)},
		CurrentDraftExcerpt: strings.Repeat("draft detail ", 100),
		EvidenceMetadata: []Evidence{{
			SourceType: SourceEmail,
			MessageID:  "msg-prompt",
			Folder:     "INBOX",
			Snippet:    strings.Repeat("evidence detail ", 80),
		}},
		ConfiguredVaultTargets: []string{"People/Sergey.md", "People/Sergey.md", ""},
	})
	if len([]rune(result.Snapshot.SourceSnippets[0])) > 303 {
		t.Fatalf("source snippet was not bounded: %d runes", len([]rune(result.Snapshot.SourceSnippets[0])))
	}
	if len([]rune(result.Snapshot.CurrentDraftExcerpt)) > 603 {
		t.Fatalf("draft excerpt was not bounded: %d runes", len([]rune(result.Snapshot.CurrentDraftExcerpt)))
	}
	if len(result.Snapshot.ConfiguredVaultTargets) != 1 {
		t.Fatalf("vault targets not compacted: %#v", result.Snapshot.ConfiguredVaultTargets)
	}

	reset := ResetPromptTemplate([]PromptTemplate{prompt}, "compose_radar_nudge")
	if len(reset) != 1 || reset[0].Version == "custom" {
		t.Fatalf("prompt reset did not restore default template: %#v", reset)
	}
}

func TestPlanMemoryUpdateConflictResolveAndAuditRules(t *testing.T) {
	settings := DefaultSettings()
	settings.UpdateRules.MatchThreshold = 0.60
	settings.UpdateRules.RetentionDays = 365
	existing := testMemoryWithKind("Senior interview is waiting on follow-up.", KindTrackStatus, 0.95)
	existing.Topic = "Senior interview"
	existing.Status = StatusWaiting
	candidate := testMemoryWithKind("Senior interview is done.", KindTrackStatus, 0.95)
	candidate.Topic = existing.Topic
	candidate.Status = StatusDone
	candidate.Company = existing.Company
	candidate.Domain = existing.Domain
	candidate.People = existing.People
	candidate.Evidence[0].MessageID = "msg-new-decision"

	decision := PlanMemoryUpdate([]Memory{existing}, candidate, settings, testTime().Add(time.Hour))
	if decision.Action != "append_conflict" || decision.ExistingID != existing.ID || decision.Candidate.Status != StatusConflict {
		t.Fatalf("decision = %#v, want append_conflict against existing memory", decision)
	}
	if !containsString(decision.Candidate.Supersedes, existing.ID) || decision.ReviewLabel != "conflict" {
		t.Fatalf("decision did not preserve review links: %#v", decision)
	}

	resolvedAt := testTime().Add(2 * time.Hour)
	resolved := ResolveOpenLoopMemory(existing, []Evidence{{
		SourceType: SourceSentEmail,
		MessageID:  "msg-resolved",
		Folder:     "Sent",
		Date:       resolvedAt,
	}}, "Loop resolved after follow-up.", resolvedAt)
	if resolved.Status != StatusResolved || resolved.ID == existing.ID || resolved.CreatedAt != resolvedAt || resolved.UpdatedAt != resolvedAt {
		t.Fatalf("resolved memory = %#v", resolved)
	}
	if !containsString(resolved.Supersedes, existing.ID) || !containsString(resolved.Related, existing.ID) {
		t.Fatalf("resolved memory missing lineage: %#v", resolved)
	}

	audit := BuildUpdateRuleAudit(settings)
	if audit.RetentionDays != 365 || audit.MatchThreshold != 0.60 || !audit.ConflictCreatesState {
		t.Fatalf("audit = %#v", audit)
	}
}

func TestBuildPersonDossierFromSourceBackedMemories(t *testing.T) {
	settings := DefaultSettings()
	settings.Thresholds.Dossier = 0.55
	openLoop := testMemoryWithKind("Sergey asked whether the take-home follow-up is still on track.", KindOpenQuestion, 0.92)
	openLoop.ObsidianTarget = "Job search/active/Example/Memory.md"
	userReply := testMemoryWithKind("You last told Sergey you would send availability by Friday.", KindLastUserReply, 0.89)
	trackStatus := testMemoryWithKind("Senior engineer interview is waiting on follow-up availability.", KindTrackStatus, 0.91)
	lowConfidence := testMemoryWithKind("Maybe Sergey changed teams.", KindRelationshipContext, 0.20)
	sourceMissing := testMemoryWithKind("Old source vanished.", KindOpenQuestion, 0.99)
	sourceMissing.Status = StatusSourceMissing

	dossier := BuildPersonDossier("Sergey", []Memory{lowConfidence, sourceMissing, openLoop, userReply, trackStatus}, settings, testTime())

	if dossier.Kind != DossierKindPerson || dossier.Subject != "Sergey" {
		t.Fatalf("dossier identity = %#v", dossier)
	}
	if !strings.Contains(dossier.RelationshipSummary, "send availability") {
		t.Fatalf("relationship summary = %q", dossier.RelationshipSummary)
	}
	if len(dossier.ActiveTracks) != 1 || !strings.Contains(dossier.ActiveTracks[0].Topic, "Interview") {
		t.Fatalf("active tracks = %#v", dossier.ActiveTracks)
	}
	if len(dossier.OpenLoops) != 1 || !strings.Contains(dossier.OpenLoops[0].Claim, "take-home") {
		t.Fatalf("open loops = %#v", dossier.OpenLoops)
	}
	if len(dossier.VaultLinks) != 1 || dossier.VaultLinks[0] != "Job search/active/Example/Memory.md" {
		t.Fatalf("vault links = %#v", dossier.VaultLinks)
	}
	if len(dossier.Evidence) == 0 || dossier.Evidence[0].MessageID == "" {
		t.Fatalf("dossier evidence missing source labels: %#v", dossier.Evidence)
	}
	if strings.Contains(dossier.RelationshipSummary, "Maybe") {
		t.Fatalf("low-confidence memory leaked into dossier: %#v", dossier)
	}
}

func TestBuildCompanyDossierMirrorsJobSearchVaultTracks(t *testing.T) {
	settings := DefaultSettings()
	settings.Thresholds.Dossier = 0.55
	now := testTime()
	active := lifecycleMemory("Cobalt Works interview", KindTrackStatus, StatusWaiting, now.Add(-1*time.Hour), "Job search/active/Cobalt Works/Memory.md")
	active.Company = "Cobalt Works"
	active.Domain = "cobalt-works.example"
	backlog := lifecycleMemory("Cobalt Works recruiter intro", KindTrackStatus, StatusActive, now.Add(-5*24*time.Hour), "Job search/backlog/Cobalt Works/Memory.md")
	backlog.Company = "Cobalt Works"
	backlog.Domain = "cobalt-works.example"
	done := lifecycleMemory("Cobalt Works 2025 loop", KindTrackStatus, StatusDone, now.Add(-90*24*time.Hour), "Job search/done/Cobalt Works/Memory.md")
	done.Company = "Cobalt Works"
	done.Domain = "cobalt-works.example"
	lowConfidence := lifecycleMemory("Cobalt Works rumor", KindRelationshipContext, StatusActive, now, "Job search/active/Cobalt Works/Rumor.md")
	lowConfidence.Company = "Cobalt Works"
	lowConfidence.Confidence = 0.20

	dossier := BuildCompanyDossier("Cobalt Works", []Memory{lowConfidence, done, backlog, active}, settings, now)

	if dossier.Kind != DossierKindCompany || dossier.Subject != "Cobalt Works" {
		t.Fatalf("dossier identity = %#v", dossier)
	}
	if len(dossier.ActiveTracks) == 0 || dossier.ActiveTracks[0].Company != "Cobalt Works" {
		t.Fatalf("active tracks = %#v", dossier.ActiveTracks)
	}
	for _, want := range []string{
		"Job search/active/Cobalt Works/Memory.md",
		"Job search/backlog/Cobalt Works/Memory.md",
		"Job search/done/Cobalt Works/Memory.md",
	} {
		if !containsString(dossier.VaultLinks, want) {
			t.Fatalf("vault links = %#v, missing %q", dossier.VaultLinks, want)
		}
	}
	if strings.Contains(dossier.RelationshipSummary, "rumor") {
		t.Fatalf("low-confidence memory leaked into company dossier: %#v", dossier)
	}
}

func TestBuildTracksFromMemoriesDerivesLifecycleStatuses(t *testing.T) {
	settings := DefaultSettings()
	settings.UpdateRules.StaleAfterDays = 30
	now := testTime()
	memories := []Memory{
		lifecycleMemory("Active proposal", KindTrackStatus, StatusActive, now.Add(-2*24*time.Hour), ""),
		lifecycleMemory("Waiting for recruiter answer", KindOpenQuestion, StatusWaiting, now.Add(-3*24*time.Hour), ""),
		lifecycleMemory("Old interview loop", KindTrackStatus, StatusActive, now.Add(-60*24*time.Hour), ""),
		lifecycleMemory("Resolved take-home", KindTrackStatus, StatusResolved, now.Add(-4*24*time.Hour), ""),
		lifecycleMemory("Backlog company", KindTrackStatus, StatusActive, now.Add(-5*24*time.Hour), "Job search/backlog/Backlog Co/Memory.md"),
		lifecycleMemory("Done company", KindTrackStatus, StatusActive, now.Add(-90*24*time.Hour), "Job search/done/Done Co/Memory.md"),
	}

	tracks := BuildTracksFromMemories(memories, settings, now)
	byTopic := tracksByTopic(tracks)

	for topic, want := range map[string]string{
		"Active proposal":              StatusActive,
		"Waiting for recruiter answer": StatusWaiting,
		"Old interview loop":           StatusStale,
		"Resolved take-home":           StatusResolved,
		"Backlog company":              StatusBacklog,
		"Done company":                 StatusDone,
	} {
		track := byTopic[topic]
		if track.ID == "" {
			t.Fatalf("missing track for %q in %#v", topic, tracks)
		}
		if track.Status != want {
			t.Fatalf("track %q status = %q, want %q", topic, track.Status, want)
		}
		if len(track.Claims) == 0 || len(track.MemoryIDs) == 0 || len(track.Evidence) == 0 {
			t.Fatalf("track %q lost claims, memory IDs, or evidence: %#v", topic, track)
		}
	}
	if len(byTopic["Waiting for recruiter answer"].OpenLoops) == 0 {
		t.Fatalf("waiting track missing open loop: %#v", byTopic["Waiting for recruiter answer"])
	}
	if byTopic["Done company"].Status != StatusDone {
		t.Fatalf("done track should remain done instead of becoming stale: %#v", byTopic["Done company"])
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

func TestObsidianSyncPlanRequiresApprovalAndApplyPreservesUserSections(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	store, err := NewFileStore(root + "/memories")
	if err != nil {
		t.Fatal(err)
	}
	settings := DefaultSettings()
	settings.Directory = store.Root()
	settings.Obsidian.Enabled = true
	settings.Obsidian.VaultPath = root + "/vault"
	settings.Obsidian.YAMLHeaders = false
	settings.Obsidian.FrontmatterMode = FrontmatterNone
	settings.Obsidian.PreviewBeforeWrite = true

	mem := testMemory("Sergey asked for an update.")
	mem.ObsidianTarget = "People/sergey@example.com.md"
	if _, _, err := store.Append(ctx, mem); err != nil {
		t.Fatal(err)
	}
	existing := "# Sergey\n\nUser-written notes stay here.\n\n<!-- HERALD:MEMORIES:BEGIN -->\nold generated\n<!-- HERALD:MEMORIES:END -->\n\nTail note.\n"
	notePath := root + "/vault/People/sergey@example.com.md"
	if err := os.MkdirAll(root+"/vault/People", 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(notePath, []byte(existing), 0o600); err != nil {
		t.Fatal(err)
	}

	plan, err := store.PlanObsidianSync(ctx, settings, false)
	if err != nil {
		t.Fatal(err)
	}
	if !plan.PreviewRequired || plan.State.PendingWrites != 1 || plan.State.UpdatedNotes != 1 {
		t.Fatalf("plan state = %#v", plan.State)
	}
	if _, err := store.ApplyObsidianSync(ctx, plan); !errors.Is(err, ErrObsidianPreviewApprovalNeed) {
		t.Fatalf("apply without approval error = %v", err)
	}
	unchanged, err := os.ReadFile(notePath)
	if err != nil {
		t.Fatal(err)
	}
	if string(unchanged) != existing {
		t.Fatalf("unapproved apply changed note:\n%s", unchanged)
	}

	approved, err := store.PlanObsidianSync(ctx, settings, true)
	if err != nil {
		t.Fatal(err)
	}
	result, err := store.ApplyObsidianSync(ctx, approved)
	if err != nil {
		t.Fatal(err)
	}
	if result.State.AppliedWrites != 1 || result.State.FailedWrites != 0 || !result.State.Approved {
		t.Fatalf("apply result = %#v", result.State)
	}
	written, err := os.ReadFile(notePath)
	if err != nil {
		t.Fatal(err)
	}
	text := string(written)
	if !strings.Contains(text, "User-written notes stay here.") || !strings.Contains(text, "Tail note.") {
		t.Fatalf("user sections not preserved:\n%s", text)
	}
	if strings.Contains(text, "old generated") || !strings.Contains(text, "Sergey asked for an update.") {
		t.Fatalf("generated section not updated:\n%s", text)
	}
	state, err := store.ReadObsidianSyncState(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if state.LastRun.IsZero() || state.AppliedWrites != 1 || state.FailedWrites != 0 || !state.Approved {
		t.Fatalf("persisted sync state = %#v", state)
	}
}

func TestObsidianSyncPlanFiltersLowConfidenceAndRejectsUnsafeTargets(t *testing.T) {
	settings := DefaultSettings()
	settings.Obsidian.Enabled = true
	settings.Obsidian.VaultPath = t.TempDir()
	lowConfidence := testMemoryWithKind("Maybe follow up.", KindOpenQuestion, settings.Thresholds.ObsidianWrite-0.01)

	plan, err := PlanObsidianSync(context.Background(), []Memory{lowConfidence}, settings, false)
	if err != nil {
		t.Fatal(err)
	}
	if plan.State.PendingWrites != 0 || len(plan.Previews) != 0 {
		t.Fatalf("low confidence memory should not plan writes: %#v", plan)
	}

	unsafe := testMemory("Unsafe target.")
	unsafe.ObsidianTarget = "../escape.md"
	if _, err := PlanObsidianSync(context.Background(), []Memory{unsafe}, settings, true); err == nil {
		t.Fatal("expected unsafe target to fail planning")
	}
}

func TestResearchModePlanUsesPublicIdentifiersAndBlocksPrivateContext(t *testing.T) {
	settings := DefaultSettings()
	request := ResearchModeRequest{
		Action:            ResearchActionBeforeReply,
		PersonName:        "Sergey Brin",
		Company:           "Cobalt Works",
		Domain:            "cobalt.example",
		Role:              "Founder",
		URL:               "https://cobalt.example/team",
		PrivateBodyText:   "private salary negotiation details",
		FullThreadSummary: "private thread recap",
	}

	plan := BuildResearchModePlan(request, settings)
	if plan.Ready || plan.Reason != "research mode is disabled" {
		t.Fatalf("default plan readiness = %#v", plan)
	}
	if len(plan.Queries) == 0 || !strings.Contains(plan.Queries[0].Query, "Sergey Brin") || !strings.Contains(plan.Queries[0].Query, "cobalt.example") {
		t.Fatalf("public query missing identifiers: %#v", plan.Queries)
	}
	if strings.Contains(plan.Queries[0].Query, "salary") || strings.Contains(plan.Queries[0].Query, "private thread") {
		t.Fatalf("query leaked private context: %q", plan.Queries[0].Query)
	}
	if got := strings.Join(plan.BlockedPrivateContext, ","); !strings.Contains(got, "private_body_text") || !strings.Contains(got, "full_thread_summary") {
		t.Fatalf("blocked private context = %#v", plan.BlockedPrivateContext)
	}

	settings.Research.Enabled = true
	settings.Research.ExternalOptIn = true
	plan = BuildResearchModePlan(request, settings)
	if !plan.Ready || plan.PrivateContextAllowed || len(plan.ApprovedPrivateContext) != 0 {
		t.Fatalf("opt-in public plan = %#v", plan)
	}

	settings.Research.PrivateBodiesAllowed = true
	request.AllowPrivateContext = true
	plan = BuildResearchModePlan(request, settings)
	if !plan.PrivateContextAllowed || len(plan.BlockedPrivateContext) != 0 || len(plan.ApprovedPrivateContext) == 0 {
		t.Fatalf("explicit private context plan = %#v", plan)
	}
}

func TestAppendResearchNoteSavesSourcedFreshnessAndVaultTarget(t *testing.T) {
	ctx := context.Background()
	store, err := NewFileStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	settings := DefaultSettings()
	settings.Directory = store.Root()
	settings.Destinations.Research = "Research/People"
	input := ResearchNoteInput{
		Action:      ResearchActionCompany,
		Company:     "Cobalt Works",
		Domain:      "cobalt.example",
		Summary:     "Cobalt Works announced a hiring pause.",
		WhatChanged: "Hiring page changed from open roles to waitlist.",
		URL:         "https://cobalt.example/jobs",
		Query:       "Cobalt Works cobalt.example latest",
		RetrievedAt: testTime(),
		Confidence:  0.88,
	}

	mem, _, err := store.AppendResearchNote(ctx, input, settings)
	if err != nil {
		t.Fatal(err)
	}
	if mem.Kind != KindResearchNote || mem.Evidence[0].SourceType != SourceResearch || mem.Evidence[0].URL != input.URL {
		t.Fatalf("research memory = %#v", mem)
	}
	if mem.ObsidianTarget != "Research/People/Cobalt Works.md" {
		t.Fatalf("obsidian target = %q", mem.ObsidianTarget)
	}
	if !strings.Contains(mem.Claim, "What changed since last contact") || !strings.Contains(strings.Join(mem.Details.SourceSignals, " "), "from public research") {
		t.Fatalf("research details = %#v", mem)
	}
	listed, err := store.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(listed) != 1 || listed[0].ID != mem.ID {
		t.Fatalf("listed memories = %#v", listed)
	}
	if got := ResearchFreshness(testTime(), testTime().Add(31*24*time.Hour), 30); got != FreshnessStale {
		t.Fatalf("freshness = %q", got)
	}
}

func TestDailyBriefingDiffUsesScheduledArtifactsAndOmitsRecaps(t *testing.T) {
	now := testTime()
	since := now.Add(-24 * time.Hour)
	settings := DefaultSettings()
	settings.UpdateRules.StaleAfterDays = 30
	longPrivateSnippet := strings.Repeat("private body detail ", 60)

	changed := lifecycleMemory("Cobalt Works interview", KindTrackStatus, StatusWaiting, now.Add(-2*time.Hour), "Job search/active/Cobalt Works/Memory.md")
	changed.Evidence[0].Snippet = longPrivateSnippet
	unchanged := lifecycleMemory("Old unchanged thread", KindTrackStatus, StatusActive, now.Add(-72*time.Hour), "Job search/active/Old Co/Memory.md")
	unchanged.CreatedAt = now.Add(-72 * time.Hour)
	unchanged.UpdatedAt = now.Add(-72 * time.Hour)
	resolved := lifecycleMemory("Take-home availability", KindOpenQuestion, StatusResolved, now.Add(-1*time.Hour), "People/sergey@example.com.md")
	newlyStale := lifecycleMemory("Quiet recruiter loop", KindOpenQuestion, StatusActive, now.Add(-30*24*time.Hour-time.Hour), "Job search/active/Quiet Co/Memory.md")
	newlyStale.CreatedAt = newlyStale.LastActivityAt
	newlyStale.UpdatedAt = newlyStale.LastActivityAt
	review := lifecycleMemory("Conflicting timeline", KindTrackStatus, StatusConflict, now.Add(-3*time.Hour), "Job search/active/Cobalt Works/Memory.md")
	review.Details.ReviewReason = "timeline conflict"
	sourceMissing := lifecycleMemory("Deleted source", KindOpenQuestion, StatusSourceMissing, now.Add(-90*time.Minute), "People/old.md")

	diff := BuildDailyBriefingDiff(
		[]Memory{unchanged, changed, resolved, newlyStale, review, sourceMissing},
		settings,
		DailyBriefingQuery{
			Since: since,
			Now:   now,
			SyncState: ObsidianSyncState{
				FailedWrites: 1,
				LastRun:      now.Add(-30 * time.Minute),
				VaultPath:    "/tmp/vault",
				Error:        "permission denied",
			},
		},
	)

	if diff.DestinationPath != "Scheduled Task Artifacts/Herald Memory Briefing 2026-06-06.md" {
		t.Fatalf("destination = %q", diff.DestinationPath)
	}
	if !containsBriefingTrack(diff.ChangedTracks, "Cobalt Works interview") {
		t.Fatalf("changed tracks = %#v, missing changed track", diff.ChangedTracks)
	}
	for _, item := range diff.ChangedTracks {
		for _, evidence := range item.Evidence {
			if evidence.Snippet != "" {
				t.Fatalf("briefing evidence should not carry raw snippets: %#v", item.Evidence)
			}
		}
	}
	if containsBriefingTrack(diff.ChangedTracks, "Old unchanged thread") {
		t.Fatalf("unchanged track leaked into diff: %#v", diff.ChangedTracks)
	}
	if !containsBriefingMemory(diff.NewlyResolved, "Take-home availability") {
		t.Fatalf("newly resolved = %#v", diff.NewlyResolved)
	}
	if !containsBriefingTrack(diff.NewlyStale, "Quiet recruiter loop") {
		t.Fatalf("newly stale = %#v", diff.NewlyStale)
	}
	if len(diff.FailedSyncs) != 1 || diff.FailedSyncs[0].Count != 1 {
		t.Fatalf("failed syncs = %#v", diff.FailedSyncs)
	}
	if !containsBriefingMemory(diff.ReviewNeeded, "Conflicting timeline") || !containsBriefingMemory(diff.ReviewNeeded, "Deleted source") {
		t.Fatalf("review needed = %#v", diff.ReviewNeeded)
	}

	rendered := RenderDailyBriefingMarkdown(diff, settings)
	for _, want := range []string{
		"## Changed Tracks",
		"## Newly Resolved Loops",
		"## Newly Stale Loops",
		"## Failed Syncs",
		"## Review Needed",
		"[[Job search/active/Cobalt Works/Memory]]",
		"permission denied",
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("rendered briefing missing %q:\n%s", want, rendered)
		}
	}
	for _, forbidden := range []string{"Old unchanged thread", "private body detail"} {
		if strings.Contains(rendered, forbidden) {
			t.Fatalf("rendered briefing should omit %q:\n%s", forbidden, rendered)
		}
	}
}

func TestFileStoreBuildDailyBriefingReadsImmutableRecordsAndSyncState(t *testing.T) {
	ctx := context.Background()
	now := testTime()
	store, err := NewFileStoreWithClock(t.TempDir(), func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	settings := DefaultSettings()
	settings.Directory = store.Root()
	settings.Destinations.DailyBriefing = "Scheduled Task Artifacts/Herald"
	mem := lifecycleMemory("Cobalt Works interview", KindTrackStatus, StatusWaiting, now.Add(-2*time.Hour), "Job search/active/Cobalt Works/Memory.md")
	if _, _, err := store.Append(ctx, mem); err != nil {
		t.Fatal(err)
	}
	if err := store.writeObsidianSyncState(ctx, ObsidianSyncState{
		FailedWrites: 2,
		LastRun:      now.Add(-time.Hour),
		VaultPath:    "/tmp/vault",
	}); err != nil {
		t.Fatal(err)
	}

	diff, err := store.BuildDailyBriefing(ctx, settings, DailyBriefingQuery{Since: now.Add(-24 * time.Hour), Now: now})
	if err != nil {
		t.Fatal(err)
	}
	if diff.DestinationPath != "Scheduled Task Artifacts/Herald/Herald Memory Briefing 2026-06-06.md" {
		t.Fatalf("destination = %q", diff.DestinationPath)
	}
	if len(diff.ChangedTracks) != 1 || len(diff.FailedSyncs) != 1 || diff.FailedSyncs[0].Count != 2 {
		t.Fatalf("diff = %#v", diff)
	}
	listed, err := store.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(listed) != 1 {
		t.Fatalf("briefing should not append memory records, listed = %#v", listed)
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

func memoryWithEvidence(claim string, evidence Evidence) Memory {
	return Memory{
		Kind:           KindRelationshipContext,
		Claim:          claim,
		Summary:        claim,
		Topic:          "Evidence",
		People:         []string{"sergey@example.com"},
		Company:        "Example",
		Domain:         "example.com",
		Status:         StatusActive,
		Confidence:     0.90,
		LastActivityAt: testTime(),
		Evidence:       []Evidence{evidence},
	}
}

func lifecycleMemory(topic, kind, status string, activity time.Time, target string) Memory {
	memory := testMemoryWithKind(topic+" claim", kind, 0.90)
	memory.Topic = topic
	memory.Summary = topic + " summary"
	memory.Status = status
	memory.LastActivityAt = activity
	memory.ObsidianTarget = target
	memory.Evidence[0].Date = activity
	return PrepareMemoryForAppend(memory, activity)
}

func memoryWithStatus(claim, kind, status string) Memory {
	memory := testMemoryWithKind(claim, kind, 0.92)
	memory.Status = status
	return memory
}

func tracksByTopic(tracks []Track) map[string]Track {
	out := make(map[string]Track, len(tracks))
	for _, track := range tracks {
		out[track.Topic] = track
	}
	return out
}

func hasMemoryKind(memories []Memory, kind string) bool {
	for _, memory := range memories {
		if memory.Kind == kind {
			return true
		}
	}
	return false
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func hasPromptWarning(warnings []PromptValidationWarning, warningType string) bool {
	for _, warning := range warnings {
		if warning.Type == warningType {
			return true
		}
	}
	return false
}

func containsBriefingTrack(items []DailyBriefingTrackItem, topic string) bool {
	for _, item := range items {
		if strings.Contains(item.Topic, topic) || strings.Contains(item.Summary, topic) {
			return true
		}
	}
	return false
}

func containsBriefingMemory(items []DailyBriefingMemoryItem, claim string) bool {
	for _, item := range items {
		if strings.Contains(item.Claim, claim) {
			return true
		}
	}
	return false
}
