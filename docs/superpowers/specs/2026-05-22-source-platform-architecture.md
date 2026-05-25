# Source Platform Architecture

This spec defines the refactoring direction for multi-account mail and future calendar integration. It matters because Herald should grow from a single IMAP mailbox client into a source platform without losing the current responsiveness rules that make the TUI feel immediate.

## Goals

The first goal is to introduce durable source identity and work coordination before adding visible multi-account or calendar UI. This keeps the migration safe because every later feature can depend on scoped references, cache-first reads, and explicit queue policy.

- [ ] Add source and account identity to mail data, sync events, background work, daemon APIs, and MCP-facing references without changing single-account behavior.
- [x] Add active-account mail switching in the TUI without enabling cross-account writes or changing single-account behavior.
- [ ] Preserve the current latest-user-intent rule from active folder loading and extend it to preview/detail/search work.
- [x] Make mail body/detail reads cache-first, persistent-cache-aware, and in-flight-coalesced so callers use one method whether data comes from cache or a remote source.
- [x] Extend the same cache-first service boundary to calendar event reads after `EventRef` and calendar cache tables exist.
- [ ] Keep provider plugins simple: plugins expose cancellable blocking operations, while Herald owns queue policy, caching, coalescing, stale-result filtering, and UI priority.
- [x] Introduce calendar as a source capability so Google Calendar and CalDAV share Herald-facing models even though their transports differ.

## Non-Goals

This foundation does not require a full multi-account UI or a calendar UI in the first implementation slice. Those features should arrive only after source identity and work coordination are boring and test-covered.

- [ ] Do not replace the Bubble Tea app with a daemon-only client as part of the first refactor.
- [ ] Do not build calendar mutation UI before read-only calendar sync/detail/search exists.
- [ ] Do not make one generic queue that handles every operation identically.
- [ ] Do not move provider-specific sync token, ETag, OAuth, or IMAP command details into the TUI.

## Identity Model

Every item shown by Herald needs an identity that says which source owns it, which account or credential set produced it, which collection it belongs to, and which provider version makes cached data fresh. Legacy single-account configs map to default IDs so existing users keep the same behavior.

- [x] `SourceID` identifies one configured source instance, such as `default-mail`, `work-mail`, `personal-google-calendar`, or `family-caldav`.
- [x] `AccountID` identifies the credential/account owner under a source. For most first-party sources, source and account are one-to-one, but keeping both lets shared calendars and future delegated access stay precise.
- [x] `CollectionRef` identifies a provider collection: IMAP folder, Google calendar, CalDAV calendar, or future contact/address-book collection.
- [x] `MessageRef` identifies mail by source, account, folder, UID, UIDVALIDITY, message ID, and a Herald local ID.
- [x] `EventRef` identifies calendar events by source, account, calendar ID, provider event ID, recurrence instance ID when present, ETag/revision, and a Herald local ID.
- [x] Legacy message IDs remain displayable and MCP-friendly, but internal writes and cache lookups prefer scoped local IDs.

Example type shape:

```go
type SourceID string
type AccountID string

type SourceKind string

const (
	SourceKindMail     SourceKind = "mail"
	SourceKindCalendar SourceKind = "calendar"
)

type CollectionRef struct {
	SourceID     SourceID
	AccountID    AccountID
	Kind         SourceKind
	CollectionID string
	DisplayName  string
}

type MessageRef struct {
	SourceID    SourceID
	AccountID   AccountID
	Folder      string
	UID         uint32
	UIDValidity uint32
	MessageID   string
	LocalID     string
}

type EventRef struct {
	SourceID     SourceID
	AccountID    AccountID
	CalendarID   string
	EventID      string
	InstanceID   string
	ETag         string
	LocalID      string
}
```

## Source Plugins

Sources are provider adapters, not mini-apps. They should know how to talk to IMAP, Google Calendar, or CalDAV, but they should not decide UI priority, cache eviction, background fairness, or stale-result behavior.

- [ ] `SourcePlugin` opens configured sources and reports source kind plus capabilities.
- [x] `MailSource` covers current mail-specific collections, sync, body fetch, mutation, drafts, folder status, and search behind `IMAPMailSource`.
- [x] `CalendarSource` covers read-only calendars, event listing/sync, and event detail fetch; `MutationSource` covers selected edit/RSVP writes with explicit recurrence-scope and provider-conflict guards.
- [x] Mail source methods accept `context.Context` so future HTTP-based providers can cancel requests and IMAP providers can at least check cancellation before starting and before returning.
- [x] Google Calendar source refreshes source-scoped OAuth tokens, sends cached provider sync tokens on incremental event sync, and returns next sync tokens to Herald-owned cache services.
- [ ] Plugins return provider metadata needed for freshness, such as UIDVALIDITY, MODSEQ, ETag, sync token, or revision.

Example capability shape:

```go
type SourcePlugin interface {
	Kind() SourceKind
	Open(ctx context.Context, cfg SourceConfig, deps SourceDeps) (Source, error)
}

type Source interface {
	ID() SourceID
	AccountID() AccountID
	DisplayName() string
	Capabilities() SourceCapabilities
	Close() error
}

type MailSource interface {
	ListMailboxes(ctx context.Context) ([]CollectionRef, error)
	SyncMailbox(ctx context.Context, ref CollectionRef) error
	FetchMessageBody(ctx context.Context, ref MessageRef) (*EmailBody, error)
	DeleteMessage(ctx context.Context, ref MessageRef) error
	SendMessage(ctx context.Context, draft OutgoingMessage) error
}

type CalendarSource interface {
	ListCalendars(ctx context.Context) ([]CollectionRef, error)
	ListEvents(ctx context.Context, ref CollectionRef) ([]CalendarEvent, error)
	FetchEvent(ctx context.Context, ref EventRef) (*CalendarEvent, error)
}

type CalendarMutationSource interface {
	UpdateEvent(ctx context.Context, event CalendarEvent, opts CalendarMutationOptions) (*CalendarEvent, error)
	RespondToEvent(ctx context.Context, ref EventRef, status string, opts CalendarMutationOptions) (*CalendarEvent, error)
}
```

## Work Coordination

The queue abstraction should preserve the product rule that UI intent is more important than background work. Queue policy should be explicit per operation rather than hidden in ad hoc channels.

- [x] `TakeLatestByIntent` keeps only the newest request for a visible UI intent, such as timeline preview or active search.
- [x] `CoalesceByResource` joins duplicate in-flight work for the same provider resource, such as fetching the same email body twice.
- [x] `SerialBySource` preserves mutation order per source and never drops confirmed user actions.
- [x] `FairBySource` prevents one account or calendar from monopolizing background sync or indexing.
- [ ] `GlobalBudget` preserves the current AI scheduler idea: interactive AI beats background AI, and local Ollama capacity is shared across all sources.

The canonical preview sequence is `email1 -> email2 -> email1`. If the first `email1` body completed, the second `email1` returns from cache. If it is in flight, the second request joins it. If `email2` finishes after the user returned to `email1`, its result is stale and cannot repaint the preview.

## Cache-First Services

Callers should not decide whether a body, event, or preview comes from cache or provider fetch. Herald-owned services sit between the TUI/backend and raw source plugins.

- [x] `MessageService.GetMessage(ctx, ref)` checks persistent cache, in-flight work/completed replay, then source fetch.
- [x] `MessageService.GetMessagePreview(ctx, ref, intent)` follows the same pattern, honors offline cache policy, and applies latest-intent filtering.
- [x] `MessageService.GetMessageNoCache(ctx, ref)` and `GetMessagePreviewNoCache(ctx, ref)` make deliberate provider bypasses explicit and write through to cache.
- [x] `CalendarEventService.GetEvent(ctx, ref)` checks event cache and provider freshness metadata before fetching.
- [x] Cache keys include source/account/collection identity and freshness metadata where available for mail reads.
- [x] Context cancellation is useful but not required for correctness; cache hits, in-flight joining, completed replay, and stale-result tokens provide the correctness guarantee.

## Queue Impact On Current Work

Existing background lanes should migrate gradually. Each lane keeps the semantics that currently make it safe, but the work keys become source-scoped.

- [ ] `latestWinsLoadCoordinator` migrates first into a reusable `internal/work` policy for active collection sync.
- [ ] `deletionRequestCh` becomes a source mutation lane carrying `MessageRef` and using `SerialBySource`.
- [x] `ruleRequestCh` becomes an automation event lane that can later handle `MailMessageReceived` and `CalendarEventChanged`.
- [ ] `classifyCh` remains mail-only at first but stores results under scoped message identity.
- [ ] Embedding, contact enrichment, and future event indexing use global AI budget plus fair source/account tagging.
- [x] Preview prewarming remains active-view-scoped mail work and uses cache-first preview services.
- [ ] Cleanup scheduling becomes source-aware, but destructive execution remains serialized per source.
- [ ] Daemon SSE events carry source/account/collection/item references so TUI, SSH, and MCP can filter or route safely.

## Storage Direction

The preferred long-term storage model is one profile database with source-scoped rows. That model supports unified search, contacts, calendar agenda, automation, and MCP better than one database per account.

- [x] Existing `cache.database_path` remains valid and points to the profile database.
- [x] Initial migration adds nullable or defaulted `source_id`, `account_id`, and `local_id` columns while keeping legacy primary keys operational.
- [ ] Later migration can move from `message_id` primary keys to scoped local IDs once all callers use refs.
- [x] Calendar tables use source-scoped primary keys from the start.
- [ ] FTS and embedding tables include source/account scope before unified cross-source search ships.

## Config Direction

Config should preserve existing single-account YAML and introduce an additive `sources:` or `accounts:` structure. The loader normalizes both into the same internal source graph.

- [x] Existing single-account config normalizes to one mail source named `default-mail`.
- [x] Multi-account mail config can add multiple mail sources without requiring users to split config files.
- [x] Calendar config adds calendar sources with provider-specific auth blocks hidden behind source config.
- [ ] Shared preferences such as theme, keyboard, AI provider, daemon settings, and cache policy stay profile-level unless a future user-visible need requires overrides.
- [ ] Compose signature can start as account-level for mail sources and keep the current legacy `compose.signature.text` as the default account signature.

Sketch:

```yaml
sources:
  - id: work-mail
    kind: mail
    provider: imap
    display_name: Work Mail
    account_id: work
    credentials:
      username: user@example.com
      password: ref-or-secret
    imap:
      host: imap.example.com
      port: 993
    smtp:
      host: smtp.example.com
      port: 587
  - id: work-calendar
    kind: calendar
    provider: google_calendar
    display_name: Work Calendar
    account_id: work
    google:
      refresh_token: ref-or-secret
```

## Daemon And MCP Direction

The daemon and MCP layers must stop treating `folder` plus `message_id` as globally unique. API paths can remain friendly, but request bodies and responses need scoped references.

- [ ] Read endpoints accept optional `source_id` and `account_id`; default values preserve legacy clients.
- [ ] Mutating endpoints require scoped refs once multi-account writes are enabled.
- [ ] MCP listing outputs include both human-readable message IDs and scoped refs suitable for follow-up calls.
- [ ] Daemon event streams include source/account/collection fields on progress, new item, valid ID, sync, and mutation events.
- [ ] Calendar MCP tools are read-only at first and use the same scoped reference style as mail.

## Phased Roadmap

The work should land in small slices that each preserve current behavior. Multi-account UI and calendar UI should not start until the foundation slices have focused tests.

- [x] Phase 0: Write and review this architecture spec plus the first two implementation plans.
- [x] Phase 1: Add `internal/work` with take-latest, coalescing, serial, fair, and status primitives. Prove duplicate/stale UI cases with tests.
- [x] Phase 2: Add source identity models and default legacy normalization. Thread IDs through models/events/cache APIs while keeping single-account behavior unchanged.
- [x] Phase 3: Add cache-first message body and preview services with persistent cache, completed replay, explicit `NoCache` bypasses, and in-flight coalescing.
- [x] Phase 4: Extract `IMAPMailSource` from `LocalBackend` behind mail capability interfaces.
- [x] Phase 5: Add multi-account mail config and active-account switching UI.
  - [x] Phase 5A: Add active-account backend switching, account rail/sidebar, account switcher overlay, and account-aware status chrome.
  - [x] Phase 5B: Add opt-in unified inbox/search and account badges after scoped list/write paths are complete.
  - [x] Phase 5C: Add account-aware Compose `From` selection and route sends/drafts through the selected mail source.
- [x] Phase 6: Add calendar source abstraction plus read-only Google Calendar and CalDAV source implementations.
  - [x] Phase 6A: Add Google Calendar OAuth refresh and provider sync-token persistence for cache-backed source reads.
- [ ] Phase 7: Add unified timeline/agenda, cross-source search, source-aware automation, and selected calendar mutations.
  - [x] Phase 7A: Add an optional read-only Calendar Agenda and Event Detail TUI foundation backed by demo/cache rows.
  - [x] Phase 7B: Add a read-only Day Agenda + Drawer view that reuses cache-backed agenda rows and preserves full Event Detail.
  - [x] Phase 7C: Add a read-only Week Time-Grid view that reuses cache-backed agenda rows and preserves full Event Detail.
  - [x] Phase 7D: Add a read-only 3-Day Command view that reuses cache-backed agenda rows and preserves full Event Detail.
  - [x] Phase 7E: Add full read-only Event Detail metadata and timezone rendering so attendees, RSVP state, recurrence, attachments, local time, event timezone, and an alternate timezone are proven before mutations.
  - [x] Phase 7F: Add read-only cache-backed Calendar Search over scoped event rows before mutation UI.
  - [x] Phase 7G: Add cross-source search over mail plus events.
  - [x] Add source-aware automation lanes for mail and calendar events.
  - [x] Add a local/cache-backed Calendar Event Edit form with timezone preview and explicit save/cancel state.
  - [x] Add provider-backed Calendar Event Edit save-through with explicit provider failure and cache update semantics.
  - [x] Add provider-backed RSVP response changes with scoped attendee updates.
  - [x] Add typed provider conflict handling and explicit recurrence-scope validation for selected calendar mutations.
  - [ ] Add selected calendar mutations only after read-only detail, timezone display, and recurrence display are proven.

## Acceptance Criteria

This refactor is accepted only if it preserves current behavior while making future source work mechanically safer. Each phase should ship with focused Go tests and only broaden to tmux/daemon/MCP evidence when user-visible behavior changes.

- [x] Existing single-account TUI, SSH, daemon, and MCP flows continue to pass their current focused tests.
- [x] Work coordinator tests prove latest UI intent, resource coalescing, cached replay, mutation serialization, and source fairness.
- [x] Source identity tests prove legacy config normalization and scoped cache key generation.
- [x] Cache-first service tests prove cache hit, in-flight join, completed replay, source fetch/store, stale-result filtering, and replay result isolation.
- [x] IMAP mail source tests prove `LocalBackend` routes sync, folder, search, mutation, draft, body, and virtual-folder provider calls through `MailSource`.
- [x] Calendar abstraction tests use fake Google Calendar and CalDAV sources before live provider tests.
