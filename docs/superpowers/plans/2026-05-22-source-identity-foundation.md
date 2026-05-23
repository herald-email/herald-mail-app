# Source Identity Foundation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add source/account identity and scoped mail references while preserving current single-account behavior.

**Architecture:** This slice introduces durable identity types, default legacy normalization, and source-scoped cache columns without changing the visible TUI. Existing folder/message-ID calls continue to work, but new code can start passing `MessageRef`, `CollectionRef`, `SourceID`, and `AccountID` through models, sync events, deletion requests, and cache writes.

**Tech Stack:** Go models, config loader tests, SQLite migration tests, existing cache APIs, package-level unit tests.

---

## File Map

These are the tracked files expected for the first identity slice. Keep this slice focused on identity plumbing; plugin extraction, multi-account UI, and calendar provider code belong to later plans.

- Create: `internal/models/identity.go` - source/account/collection/message reference types and default normalization helpers.
- Create: `internal/models/identity_test.go` - tests for default refs, local IDs, and cache keys.
- Modify: `internal/models/email.go` - add source/account/local identity fields to `EmailData`, `FolderSyncEvent`, `DeletionRequest`, `DeletionResult`, and `NewEmailsNotification`.
- Modify: `internal/config/config.go` - add additive `sources:` config structs and normalize legacy config into one default mail source.
- Modify: `internal/config/config_test.go` - verify legacy and new source config normalization.
- Modify: `internal/cache/cache.go` - add source/account/local columns and keep legacy message-ID primary key behavior.
- Modify: `internal/cache/cache_test.go` - verify migrations, default source values, and legacy lookups.
- Create: `reports/TEST_REPORT_2026-05-23_source-identity-foundation.md` during verification. The `reports/` directory is gitignored and the report is not committed.

## Task 1: Specify Scoped Identity Models

This task defines the identity language before touching config or cache code. The tests should prove that legacy single-account emails produce deterministic scoped refs without requiring existing callers to know about multi-account concepts.

**Files:**
- Create: `internal/models/identity_test.go`

- [x] **Step 1: Write failing tests for default identity helpers**

Create `internal/models/identity_test.go` with:

```go
package models

import "testing"

func TestMessageRefFromLegacyEmailUsesDefaultScope(t *testing.T) {
	email := EmailData{
		MessageID: "<abc@example.com>",
		UID:       42,
		Folder:    "INBOX",
	}

	ref := email.MessageRef()

	if ref.SourceID != DefaultMailSourceID {
		t.Fatalf("SourceID = %q, want %q", ref.SourceID, DefaultMailSourceID)
	}
	if ref.AccountID != DefaultAccountID {
		t.Fatalf("AccountID = %q, want %q", ref.AccountID, DefaultAccountID)
	}
	if ref.Folder != "INBOX" || ref.UID != 42 || ref.MessageID != "<abc@example.com>" {
		t.Fatalf("ref = %#v, want legacy folder/uid/message id preserved", ref)
	}
	if ref.LocalID == "" {
		t.Fatal("LocalID should be populated for scoped cache lookups")
	}
}

func TestMessageRefUsesExplicitEmailScope(t *testing.T) {
	email := EmailData{
		SourceID:  SourceID("work-mail"),
		AccountID: AccountID("work"),
		LocalID:   "mail:work-mail:work:INBOX:<abc@example.com>",
		MessageID: "<abc@example.com>",
		Folder:    "INBOX",
	}

	ref := email.MessageRef()

	if ref.SourceID != SourceID("work-mail") || ref.AccountID != AccountID("work") {
		t.Fatalf("ref scope = %q/%q, want work-mail/work", ref.SourceID, ref.AccountID)
	}
	if ref.LocalID != email.LocalID {
		t.Fatalf("LocalID = %q, want %q", ref.LocalID, email.LocalID)
	}
}

func TestCollectionRefCacheKeyIncludesScope(t *testing.T) {
	ref := CollectionRef{
		SourceID:     SourceID("work-mail"),
		AccountID:    AccountID("work"),
		Kind:         SourceKindMail,
		CollectionID: "INBOX",
	}

	if got, want := ref.CacheKey(), "mail:work-mail:work:INBOX"; got != want {
		t.Fatalf("CacheKey = %q, want %q", got, want)
	}
}
```

- [x] **Step 2: Run the model tests and verify they fail**

Run:

```bash
go test ./internal/models -run 'MessageRef|CollectionRef' -count=1
```

Expected: FAIL because the identity types and helper methods do not exist yet.

- [x] **Step 3: Commit the failing tests**

Run:

```bash
git add internal/models/identity_test.go
git commit -m "test: specify scoped source identity"
```

## Task 2: Add Identity Types And Model Fields

This task adds the stable data structures that later source plugins, cache services, daemon APIs, and MCP tools will share. Keep helper behavior conservative: empty IDs normalize to default single-account IDs.

**Files:**
- Create: `internal/models/identity.go`
- Modify: `internal/models/email.go`

- [x] **Step 1: Add shared identity types**

Create `internal/models/identity.go` with:

```go
package models

import "strings"

type SourceID string
type AccountID string

type SourceKind string

const (
	DefaultMailSourceID SourceID  = "default-mail"
	DefaultAccountID    AccountID = "default"

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

func NormalizeSourceID(id SourceID, fallback SourceID) SourceID {
	if strings.TrimSpace(string(id)) != "" {
		return id
	}
	if fallback != "" {
		return fallback
	}
	return DefaultMailSourceID
}

func NormalizeAccountID(id AccountID) AccountID {
	if strings.TrimSpace(string(id)) != "" {
		return id
	}
	return DefaultAccountID
}

func (r CollectionRef) CacheKey() string {
	return strings.Join([]string{
		string(r.Kind),
		string(NormalizeSourceID(r.SourceID, DefaultMailSourceID)),
		string(NormalizeAccountID(r.AccountID)),
		r.CollectionID,
	}, ":")
}

func (r MessageRef) WithDefaults() MessageRef {
	r.SourceID = NormalizeSourceID(r.SourceID, DefaultMailSourceID)
	r.AccountID = NormalizeAccountID(r.AccountID)
	if r.LocalID == "" {
		r.LocalID = strings.Join([]string{
			"mail",
			string(r.SourceID),
			string(r.AccountID),
			r.Folder,
			r.MessageID,
		}, ":")
	}
	return r
}
```

- [x] **Step 2: Add scoped fields to mail models**

Modify `internal/models/email.go`:

```go
type EmailData struct {
	SourceID    SourceID  `db:"source_id"`
	AccountID   AccountID `db:"account_id"`
	LocalID     string    `db:"local_id"`
	UIDValidity uint32    `db:"uid_validity"`

	MessageID      string    `db:"message_id"`
	UID            uint32    `db:"uid"`
	// existing fields stay unchanged
}

func (e EmailData) MessageRef() MessageRef {
	return MessageRef{
		SourceID:    e.SourceID,
		AccountID:   e.AccountID,
		Folder:      e.Folder,
		UID:         e.UID,
		UIDValidity: e.UIDValidity,
		MessageID:   e.MessageID,
		LocalID:     e.LocalID,
	}.WithDefaults()
}
```

Add the same optional scope fields to event/request types without changing existing fields:

```go
type FolderSyncEvent struct {
	SourceID  SourceID
	AccountID AccountID
	// existing fields stay unchanged
}

type DeletionRequest struct {
	SourceID  SourceID  `json:"source_id,omitempty"`
	AccountID AccountID `json:"account_id,omitempty"`
	LocalID   string    `json:"local_id,omitempty"`
	// existing fields stay unchanged
}

type DeletionResult struct {
	SourceID  SourceID  `json:"source_id,omitempty"`
	AccountID AccountID `json:"account_id,omitempty"`
	LocalID   string    `json:"local_id,omitempty"`
	// existing fields stay unchanged
}

type NewEmailsNotification struct {
	SourceID  SourceID
	AccountID AccountID
	// existing fields stay unchanged
}
```

- [x] **Step 3: Run focused model tests**

Run:

```bash
go test ./internal/models -run 'MessageRef|CollectionRef' -count=1
```

Expected: PASS.

- [x] **Step 4: Commit the model implementation**

Run:

```bash
git add internal/models/identity.go internal/models/email.go internal/models/identity_test.go
git commit -m "feat: add source identity model types"
```

## Task 3: Normalize Legacy And Source Config

This task makes config loading additive. Existing YAML must normalize to the same single default mail source, while new YAML can declare multiple sources without being consumed by the UI yet.

**Files:**
- Modify: `internal/config/config.go`
- Modify: `internal/config/config_test.go`

- [x] **Step 1: Write config normalization tests**

Add tests that load both legacy config and a new `sources:` config:

```go
func TestConfigNormalizedSourcesFromLegacyMailConfig(t *testing.T) {
	var cfg Config
	cfg.Credentials.Username = "user@example.com"
	cfg.Credentials.Password = "secret"
	cfg.Server.Host = "127.0.0.1"
	cfg.Server.Port = 1143
	cfg.SMTP.Host = "127.0.0.1"
	cfg.SMTP.Port = 1025

	sources := cfg.NormalizedSources()
	if len(sources) != 1 {
		t.Fatalf("len(sources) = %d, want 1", len(sources))
	}
	if sources[0].ID != "default-mail" || sources[0].Kind != "mail" || sources[0].AccountID != "default" {
		t.Fatalf("source = %#v, want default mail source", sources[0])
	}
}

func TestConfigNormalizedSourcesKeepsExplicitSources(t *testing.T) {
	cfg := Config{
		Sources: []SourceConfig{
			{ID: "work-mail", Kind: "mail", Provider: "imap", AccountID: "work"},
			{ID: "work-calendar", Kind: "calendar", Provider: "google_calendar", AccountID: "work"},
		},
	}

	sources := cfg.NormalizedSources()
	if len(sources) != 2 {
		t.Fatalf("len(sources) = %d, want 2", len(sources))
	}
}
```

- [x] **Step 2: Add source config structs and normalization**

Add source config structs that do not disturb existing top-level fields:

```go
type SourceConfig struct {
	ID          string `yaml:"id"`
	Kind        string `yaml:"kind"`     // mail | calendar
	Provider    string `yaml:"provider"` // imap | gmail | google_calendar | caldav
	DisplayName string `yaml:"display_name,omitempty"`
	AccountID   string `yaml:"account_id,omitempty"`

	Credentials CredentialsConfig `yaml:"credentials,omitempty"`
	IMAP        ServerConfig      `yaml:"imap,omitempty"`
	SMTP        ServerConfig      `yaml:"smtp,omitempty"`
	Google      GoogleConfig      `yaml:"google,omitempty"`
	CalDAV      CalDAVConfig      `yaml:"caldav,omitempty"`
}
```

If these named structs do not already exist, extract them from anonymous top-level config blocks so both legacy and new config can reuse the same shape. Then add:

```go
func (c Config) NormalizedSources() []SourceConfig {
	if len(c.Sources) > 0 {
		return normalizeExplicitSources(c.Sources)
	}
	return []SourceConfig{legacyDefaultMailSource(c)}
}
```

- [x] **Step 3: Run config tests**

Run:

```bash
go test ./internal/config -run 'NormalizedSources|Load' -count=1
```

Expected: PASS.

- [x] **Step 4: Commit the config implementation**

Run:

```bash
git add internal/config/config.go internal/config/config_test.go
git commit -m "feat: normalize legacy config into sources"
```

## Task 4: Add Source Scope To Cache Rows

This task adds defaulted source columns to the profile database while keeping legacy lookups intact. Do not switch primary keys yet; the goal is a reversible, low-risk migration that lets future code start writing scoped rows.

**Files:**
- Modify: `internal/cache/cache.go`
- Modify: `internal/cache/cache_test.go`

- [x] **Step 1: Write migration and cache-write tests**

Add cache tests that create a temp database, initialize it, and inspect schema/data:

```go
func TestCacheAddsDefaultSourceColumns(t *testing.T) {
	cache := newTestCache(t)
	defer cache.Close()

	cols := tableColumns(t, cache.db, "emails")
	for _, name := range []string{"source_id", "account_id", "local_id", "uid_validity"} {
		if !cols[name] {
			t.Fatalf("emails missing column %s", name)
		}
	}
}

func TestCacheEmailWritesDefaultScopeForLegacyEmail(t *testing.T) {
	cache := newTestCache(t)
	defer cache.Close()

	email := &models.EmailData{MessageID: "<m@example.com>", UID: 1, Folder: "INBOX"}
	if err := cache.CacheEmail(email); err != nil {
		t.Fatalf("CacheEmail: %v", err)
	}

	got, err := cache.GetEmailByID("<m@example.com>")
	if err != nil {
		t.Fatalf("GetEmailByID: %v", err)
	}
	if got.SourceID != models.DefaultMailSourceID || got.AccountID != models.DefaultAccountID || got.LocalID == "" {
		t.Fatalf("scope = %#v, want default source/account/local id", got)
	}
}
```

- [x] **Step 2: Add safe SQLite migrations**

In `initDB`, add best-effort column migrations after the `emails` table exists:

```go
_, _ = c.db.Exec(`ALTER TABLE emails ADD COLUMN source_id TEXT NOT NULL DEFAULT 'default-mail'`)
_, _ = c.db.Exec(`ALTER TABLE emails ADD COLUMN account_id TEXT NOT NULL DEFAULT 'default'`)
_, _ = c.db.Exec(`ALTER TABLE emails ADD COLUMN local_id TEXT`)
_, _ = c.db.Exec(`ALTER TABLE emails ADD COLUMN uid_validity INTEGER NOT NULL DEFAULT 0`)
_, _ = c.db.Exec(`CREATE INDEX IF NOT EXISTS idx_emails_source_folder_date ON emails(source_id, account_id, folder, date DESC)`)
_, _ = c.db.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS idx_emails_local_id ON emails(local_id) WHERE local_id IS NOT NULL`)
```

After the existing `folder_sync_state` table creation block, add defaulted source columns without changing its legacy primary key in this slice:

```go
_, _ = c.db.Exec(`ALTER TABLE folder_sync_state ADD COLUMN source_id TEXT NOT NULL DEFAULT 'default-mail'`)
_, _ = c.db.Exec(`ALTER TABLE folder_sync_state ADD COLUMN account_id TEXT NOT NULL DEFAULT 'default'`)
_, _ = c.db.Exec(`CREATE INDEX IF NOT EXISTS idx_folder_sync_state_source ON folder_sync_state(source_id, account_id, folder)`)
```

- [x] **Step 3: Update email inserts and scans**

Update `CacheEmail` and `BatchCacheEmails` so they write normalized source/account/local values:

```go
ref := email.MessageRef()
sourceID := ref.SourceID
accountID := ref.AccountID
localID := ref.LocalID
```

Update row scans such as `GetEmailByID`, `GetEmailsSortedByDate`, `SearchEmails`, `SearchEmailsFTS`, and helper scanners so returned `EmailData` carries the scoped fields. Use `COALESCE(source_id, 'default-mail')`, `COALESCE(account_id, 'default')`, and `COALESCE(local_id, '')` in SELECTs during the migration.

- [x] **Step 4: Keep legacy APIs intact**

Do not remove or rename existing methods such as `GetEmailByID(messageID string)`, `DeleteEmail(messageID string)`, or `GetCachedUIDsAndMessageIDs(folder string)`. Add scoped alternatives only when a caller in this slice needs them, for example:

```go
func (c *Cache) GetEmailByRef(ref models.MessageRef) (*models.EmailData, error)
```

The default implementation should prefer `local_id` when present and fall back to `message_id` for legacy rows.

- [x] **Step 5: Run cache tests**

Run:

```bash
go test ./internal/cache -run 'Source|Scope|CacheEmail|GetEmailByID' -count=1
```

Expected: PASS.

- [x] **Step 6: Commit the cache migration**

Run:

```bash
git add internal/cache/cache.go internal/cache/cache_test.go
git commit -m "feat: add source scope to cache rows"
```

## Task 5: Thread Scope Through Existing Events

This task makes source identity visible to asynchronous lanes without changing queue behavior yet. The default source/account values should be attached at backend boundaries so app code can remain mostly unchanged until multi-account UI arrives.

**Files:**
- Modify: `internal/backend/local.go`
- Modify: `internal/backend/demo.go`
- Modify: `internal/backend/remote.go`
- Modify: focused backend/app tests as needed.

- [x] **Step 1: Add default scope at backend emission points**

When `LocalBackend` emits `FolderSyncEvent` or `NewEmailsNotification`, set `SourceID: models.DefaultMailSourceID` and `AccountID: models.DefaultAccountID` unless the backend has explicit source identity. Do the same in `DemoBackend` and `RemoteBackend` so tests stay deterministic.

- [x] **Step 2: Preserve app behavior while storing scope**

When app handlers receive scoped events, use the existing folder/message-ID behavior for rendering and state updates. Store source/account fields on message state where the model already carries full `EmailData`, but avoid UI changes in this slice.

- [x] **Step 3: Add focused event tests**

Add or update tests so they assert default scope is carried through sync events and new-email notifications without changing existing UI state transitions.

- [x] **Step 4: Run focused backend/app tests**

Run:

```bash
go test ./internal/backend ./internal/app -run 'SyncEvent|NewEmails|Deletion|Source' -count=1
```

Expected: PASS.

- [x] **Step 5: Commit event plumbing**

Run:

```bash
git add internal/backend/local.go internal/backend/demo.go internal/backend/remote.go internal/app internal/backend
git commit -m "feat: carry source scope through async events"
```

## Task 6: Verify Identity Foundation Surface

This task proves the source identity foundation without claiming multi-account support. Since this slice should not change visible TUI behavior, tmux evidence is optional unless implementation touches rendering or key routing.

**Files:**
- Create: `reports/TEST_REPORT_2026-05-23_source-identity-foundation.md`

- [x] **Step 1: Run focused tests**

Run:

```bash
go test ./internal/models ./internal/config ./internal/cache -run 'MessageRef|CollectionRef|NormalizedSources|Source|Scope|CacheEmail|GetEmailByID' -count=1
```

Expected: PASS.

- [x] **Step 2: Run package tests for touched areas**

Run:

```bash
go test ./internal/models ./internal/config ./internal/cache ./internal/backend ./internal/app -count=1
```

Expected: PASS.

- [x] **Step 3: Save a local verification report**

Create `reports/TEST_REPORT_2026-05-23_source-identity-foundation.md` with:

```markdown
# Source Identity Foundation Test Report

Date: 2026-05-22
Surface: focused Go package tests; no visible TUI behavior changed

## Commands

- `go test ./internal/models ./internal/config ./internal/cache -run 'MessageRef|CollectionRef|NormalizedSources|Source|Scope|CacheEmail|GetEmailByID' -count=1`
- `go test ./internal/models ./internal/config ./internal/cache ./internal/backend ./internal/app -count=1`

## Result

Both commands passed. Legacy config normalizes to the default mail source, cache rows carry default source/account/local identity, and existing folder/message-ID APIs remain available.
```

- [x] **Step 4: Commit tracked source changes**

Run:

```bash
git status --short
git add internal/models internal/config internal/cache internal/backend internal/app
git commit -m "feat: add source identity foundation"
```

Expected: report remains untracked because `reports/` is gitignored; source changes are committed.
