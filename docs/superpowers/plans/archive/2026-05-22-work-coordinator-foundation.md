# Work Coordinator Foundation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a tested `internal/work` foundation that preserves latest UI intent, coalesces duplicate resource fetches, serializes mutations, and leaves current app behavior unchanged.

**Architecture:** The new package owns queue policy primitives but does not replace existing app queues in this slice. It provides source-scoped keys, intent/resource dedupe semantics, and deterministic tests for the `email1 -> email2 -> email1` case. Later plans will migrate `latestWinsLoadCoordinator`, preview fetches, source sync, and mutation queues onto these primitives.

**Tech Stack:** Go standard library, `context`, goroutines, channels, mutexes, package-level unit tests.

---

## File Map

These are the only tracked source files this foundation slice should touch. Keeping the first implementation isolated makes it possible to test queue semantics before wiring them into mail, daemon, or calendar flows.

- Create: `internal/work/work.go` - shared key, policy, priority, result, and status types.
- Create: `internal/work/coordinator.go` - coordinator implementation for take-latest intent work, resource coalescing, serial source work, and bounded fair queue scaffolding.
- Create: `internal/work/coordinator_test.go` - deterministic tests for duplicate resource coalescing, cached replay, stale intent suppression, and serial mutation ordering.
- Modify: `internal/backend/sync_coordinator_test.go` - add one regression assertion documenting that the legacy coordinator remains unchanged until migration.
- Create: `reports/TEST_REPORT_2026-05-22_work-coordinator-foundation.md` during verification. The `reports/` directory is gitignored and the report is not committed.

## Task 1: Specify Interactive Dedupe Behavior

This task defines the behavior before implementation. The tests lock down the important UI invariant: duplicate resource requests are joined, and only the resource matching the latest visible intent may repaint.

**Files:**
- Create: `internal/work/coordinator_test.go`

- [x] **Step 1: Write failing tests for latest intent and coalesced resources**

Create `internal/work/coordinator_test.go` with:

```go
package work

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

func TestInteractiveWorkCoalescesDuplicateResourceAndKeepsLatestIntent(t *testing.T) {
	c := NewCoordinator()
	defer c.Close()

	ctx := context.Background()
	var email1Runs int32
	started := make(chan struct{})
	release := make(chan struct{})
	secondRelease := make(chan struct{})

	first := c.Submit(ctx, Spec{
		IntentKey:   IntentKey{ViewID: "timeline-preview"},
		ResourceKey: ResourceKey{SourceID: "default-mail", CollectionID: "INBOX", ItemID: "email-1", Operation: "fetch-preview"},
		Policy:      PolicyTakeLatestByIntent | PolicyCoalesceByResource,
		Priority:    PriorityInteractive,
		Run: func(ctx context.Context) (any, error) {
			atomic.AddInt32(&email1Runs, 1)
			close(started)
			select {
			case <-release:
				return "body-1", nil
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		},
	})

	<-started
	second := c.Submit(ctx, Spec{
		IntentKey:   IntentKey{ViewID: "timeline-preview"},
		ResourceKey: ResourceKey{SourceID: "default-mail", CollectionID: "INBOX", ItemID: "email-2", Operation: "fetch-preview"},
		Policy:      PolicyTakeLatestByIntent | PolicyCoalesceByResource,
		Priority:    PriorityInteractive,
		Run: func(ctx context.Context) (any, error) {
			select {
			case <-secondRelease:
			case <-ctx.Done():
				return nil, ctx.Err()
			}
			return "body-2", nil
		},
	})

	third := c.Submit(ctx, Spec{
		IntentKey:   IntentKey{ViewID: "timeline-preview"},
		ResourceKey: ResourceKey{SourceID: "default-mail", CollectionID: "INBOX", ItemID: "email-1", Operation: "fetch-preview"},
		Policy:      PolicyTakeLatestByIntent | PolicyCoalesceByResource,
		Priority:    PriorityInteractive,
		Run: func(ctx context.Context) (any, error) {
			t.Fatal("duplicate email-1 resource should join the in-flight work")
			return nil, nil
		},
	})

	close(release)
	close(secondRelease)

	if got, err := first.Await(ctx); err != nil || got != "body-1" {
		t.Fatalf("first Await = (%v, %v), want body-1 nil", got, err)
	}
	if got, err := third.Await(ctx); err != nil || got != "body-1" {
		t.Fatalf("third Await = (%v, %v), want coalesced body-1 nil", got, err)
	}
	if got, err := second.Await(ctx); !errors.Is(err, ErrStaleIntent) || got != nil {
		t.Fatalf("second Await = (%v, %v), want nil ErrStaleIntent", got, err)
	}
	if got := atomic.LoadInt32(&email1Runs); got != 1 {
		t.Fatalf("email1Runs = %d, want 1", got)
	}
}

func TestCompletedResourceCanReplayFromCoordinatorCache(t *testing.T) {
	c := NewCoordinator()
	defer c.Close()

	ctx := context.Background()
	var runs int32
	key := ResourceKey{SourceID: "default-mail", CollectionID: "INBOX", ItemID: "email-1", Operation: "fetch-preview"}

	first := c.Submit(ctx, Spec{
		IntentKey:   IntentKey{ViewID: "timeline-preview"},
		ResourceKey: key,
		Policy:      PolicyTakeLatestByIntent | PolicyCoalesceByResource | PolicyReplayCompletedResource,
		Priority:    PriorityInteractive,
		Run: func(ctx context.Context) (any, error) {
			atomic.AddInt32(&runs, 1)
			return "body-1", nil
		},
	})
	if got, err := first.Await(ctx); err != nil || got != "body-1" {
		t.Fatalf("first Await = (%v, %v), want body-1 nil", got, err)
	}

	second := c.Submit(ctx, Spec{
		IntentKey:   IntentKey{ViewID: "timeline-preview"},
		ResourceKey: key,
		Policy:      PolicyTakeLatestByIntent | PolicyCoalesceByResource | PolicyReplayCompletedResource,
		Priority:    PriorityInteractive,
		Run: func(ctx context.Context) (any, error) {
			t.Fatal("completed resource should replay without rerunning")
			return nil, nil
		},
	})
	if got, err := second.Await(ctx); err != nil || got != "body-1" {
		t.Fatalf("second Await = (%v, %v), want cached body-1 nil", got, err)
	}
	if got := atomic.LoadInt32(&runs); got != 1 {
		t.Fatalf("runs = %d, want 1", got)
	}
}

func TestSerialBySourcePreservesMutationOrder(t *testing.T) {
	c := NewCoordinator()
	defer c.Close()

	ctx := context.Background()
	var order []string

	first := c.Submit(ctx, Spec{
		SourceID: SourceID("default-mail"),
		Policy:   PolicySerialBySource,
		Priority: PriorityUserAction,
		Run: func(ctx context.Context) (any, error) {
			order = append(order, "delete-1")
			time.Sleep(10 * time.Millisecond)
			return "ok-1", nil
		},
	})
	second := c.Submit(ctx, Spec{
		SourceID: SourceID("default-mail"),
		Policy:   PolicySerialBySource,
		Priority: PriorityUserAction,
		Run: func(ctx context.Context) (any, error) {
			order = append(order, "delete-2")
			return "ok-2", nil
		},
	})

	if got, err := first.Await(ctx); err != nil || got != "ok-1" {
		t.Fatalf("first Await = (%v, %v), want ok-1 nil", got, err)
	}
	if got, err := second.Await(ctx); err != nil || got != "ok-2" {
		t.Fatalf("second Await = (%v, %v), want ok-2 nil", got, err)
	}
	if len(order) != 2 || order[0] != "delete-1" || order[1] != "delete-2" {
		t.Fatalf("order = %#v, want delete-1 then delete-2", order)
	}
}
```

- [x] **Step 2: Run tests and verify they fail**

Run:

```bash
go test ./internal/work -count=1
```

Expected: FAIL because `internal/work` has no implementation yet.

- [x] **Step 3: Commit the failing tests**

Run:

```bash
git add internal/work/coordinator_test.go
git commit -m "test: specify work coordinator intent dedupe"
```

## Task 2: Add Work Types And Coordinator

This task implements the smallest reusable coordination package that satisfies the tests. It intentionally does not migrate existing app queues yet; that happens after the semantics are proved in isolation.

**Files:**
- Create: `internal/work/work.go`
- Create: `internal/work/coordinator.go`

- [x] **Step 1: Add shared work types**

Create `internal/work/work.go` with:

```go
package work

import (
	"context"
	"errors"
)

var (
	ErrStaleIntent = errors.New("work result superseded by newer intent")
	ErrClosed      = errors.New("work coordinator closed")
)

type Policy uint32

const (
	PolicyTakeLatestByIntent Policy = 1 << iota
	PolicyCoalesceByResource
	PolicyReplayCompletedResource
	PolicySerialBySource
	PolicyFairBySource
	PolicyGlobalBudget
)

type Priority int

const (
	PriorityBackground Priority = iota
	PriorityUserAction
	PriorityInteractive
)

type SourceID string

type IntentKey struct {
	ViewID string
}

type ResourceKey struct {
	SourceID     string
	AccountID    string
	CollectionID string
	ItemID       string
	Operation    string
	Freshness    string
}

type Spec struct {
	SourceID    SourceID
	IntentKey   IntentKey
	ResourceKey ResourceKey
	Policy      Policy
	Priority    Priority
	Run         func(context.Context) (any, error)
}

type Result struct {
	ready <-chan struct{}
	state *resultState
	filter func(resultState) resultState
}

type resultState struct {
	value any
	err   error
}

func (r Result) Await(ctx context.Context) (any, error) {
	select {
	case <-r.ready:
		state := *r.state
		if r.filter != nil {
			state = r.filter(state)
		}
		return state.value, state.err
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func completedResult(state resultState) Result {
	ready := make(chan struct{})
	close(ready)
	return Result{ready: ready, state: &state}
}
```

- [x] **Step 2: Add the coordinator implementation**

Create `internal/work/coordinator.go` with:

```go
package work

import (
	"context"
	"sync"
)

type Coordinator struct {
	mu              sync.Mutex
	closed          bool
	intents         map[IntentKey]intentState
	inflight        map[ResourceKey]*call
	completed       map[ResourceKey]resultState
	sourceLocks     map[SourceID]*sync.Mutex
}

type intentState struct {
	generation int64
	resource   ResourceKey
}

type call struct {
	ready chan struct{}
	state resultState
	done  bool
}

func NewCoordinator() *Coordinator {
	return &Coordinator{
		intents:     make(map[IntentKey]intentState),
		inflight:    make(map[ResourceKey]*call),
		completed:   make(map[ResourceKey]resultState),
		sourceLocks: make(map[SourceID]*sync.Mutex),
	}
}

func (c *Coordinator) Close() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.closed = true
	for key, active := range c.inflight {
		delete(c.inflight, key)
		c.finishCallLocked(active, resultState{err: ErrClosed})
	}
}

func (c *Coordinator) Submit(ctx context.Context, spec Spec) Result {
	if spec.Run == nil {
		return completedResult(resultState{err: ErrClosed})
	}

	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return completedResult(resultState{err: ErrClosed})
	}

	generation := int64(0)
	if spec.Policy&PolicyTakeLatestByIntent != 0 {
		state := c.intents[spec.IntentKey]
		state.generation++
		state.resource = spec.ResourceKey
		c.intents[spec.IntentKey] = state
		generation = state.generation
	}
	filter := c.intentFilter(spec, generation)

	if spec.Policy&PolicyReplayCompletedResource != 0 {
		if completed, ok := c.completed[spec.ResourceKey]; ok {
			c.mu.Unlock()
			result := completedResult(completed)
			result.filter = filter
			return result
		}
	}

	if spec.Policy&PolicyCoalesceByResource != 0 {
		if active, ok := c.inflight[spec.ResourceKey]; ok {
			c.mu.Unlock()
			return Result{ready: active.ready, state: &active.state, filter: filter}
		}
	}

	active := &call{ready: make(chan struct{})}
	if spec.Policy&PolicyCoalesceByResource != 0 {
		c.inflight[spec.ResourceKey] = active
	}
	sourceID := spec.SourceID
	if sourceID == "" {
		sourceID = SourceID(spec.ResourceKey.SourceID)
	}
	sourceLock := c.sourceLockLocked(sourceID)
	c.mu.Unlock()

	go c.run(ctx, spec, sourceLock, active)
	return Result{ready: active.ready, state: &active.state, filter: filter}
}

func (c *Coordinator) sourceLockLocked(sourceID SourceID) *sync.Mutex {
	if sourceID == "" {
		sourceID = "__default__"
	}
	lock := c.sourceLocks[sourceID]
	if lock == nil {
		lock = &sync.Mutex{}
		c.sourceLocks[sourceID] = lock
	}
	return lock
}

func (c *Coordinator) run(ctx context.Context, spec Spec, sourceLock *sync.Mutex, active *call) {
	if spec.Policy&PolicySerialBySource != 0 {
		sourceLock.Lock()
		defer sourceLock.Unlock()
	}

	value, err := spec.Run(ctx)
	state := resultState{value: value, err: err}

	c.mu.Lock()
	if spec.Policy&PolicyCoalesceByResource != 0 {
		delete(c.inflight, spec.ResourceKey)
	}
	if err == nil && spec.Policy&PolicyReplayCompletedResource != 0 {
		c.completed[spec.ResourceKey] = state
	}
	c.finishCallLocked(active, state)
	c.mu.Unlock()
}

func (c *Coordinator) finishCallLocked(active *call, state resultState) {
	if active.done {
		return
	}
	active.state = state
	active.done = true
	close(active.ready)
}

func (c *Coordinator) intentFilter(spec Spec, generation int64) func(resultState) resultState {
	if spec.Policy&PolicyTakeLatestByIntent == 0 {
		return nil
	}
	return func(state resultState) resultState {
		c.mu.Lock()
		latest := c.intents[spec.IntentKey]
		current := latest.generation == generation || latest.resource == spec.ResourceKey
		c.mu.Unlock()
		if !current {
			return resultState{err: ErrStaleIntent}
		}
		return state
	}
}
```

- [x] **Step 3: Run the work package tests**

Run:

```bash
go test ./internal/work -count=1
```

Expected: PASS.

- [x] **Step 4: Commit the implementation**

Run:

```bash
git add internal/work/work.go internal/work/coordinator.go internal/work/coordinator_test.go
git commit -m "feat: add work coordinator foundation"
```

## Task 3: Protect Legacy Load Coordinator Behavior

This task protects the existing folder load behavior while the reusable coordinator is introduced. It documents that repeated resources still advance intent generation and that the latest pending folder wins.

**Files:**
- Modify: `internal/backend/sync_coordinator_test.go`

- [x] **Step 1: Add a regression assertion for existing latest-wins load behavior**

Append this test to `internal/backend/sync_coordinator_test.go`:

```go
func TestLatestWinsLoadCoordinator_RepeatedFolderStillAdvancesGeneration(t *testing.T) {
	c := newLatestWinsLoadCoordinator()

	first := c.Submit("INBOX")
	second := c.Submit("Archive")
	third := c.Submit("INBOX")

	if first.Generation != 1 || second.Generation != 2 || third.Generation != 3 {
		t.Fatalf("generations = %d, %d, %d; want 1, 2, 3", first.Generation, second.Generation, third.Generation)
	}

	got, ok := c.DrainPending()
	if !ok {
		t.Fatal("expected pending request")
	}
	if got.Folder != "INBOX" || got.Generation != third.Generation {
		t.Fatalf("pending = %#v, want latest INBOX generation %d", got, third.Generation)
	}
}
```

- [x] **Step 2: Run backend coordinator tests**

Run:

```bash
go test ./internal/backend -run LatestWinsLoadCoordinator -count=1
```

Expected: PASS.

- [x] **Step 3: Commit the regression test**

Run:

```bash
git add internal/backend/sync_coordinator_test.go
git commit -m "test: document legacy latest-wins load behavior"
```

## Task 4: Verify Foundation Surface

This task verifies the implementation through focused package tests and records the evidence locally. There is no tmux requirement for this slice because it adds no visible TUI behavior.

**Files:**
- Create: `reports/TEST_REPORT_2026-05-22_work-coordinator-foundation.md`

- [x] **Step 1: Run focused verification**

Run:

```bash
go test ./internal/work ./internal/backend -run 'Work|LatestWinsLoadCoordinator' -count=1
```

Expected: PASS.

- [x] **Step 2: Run package tests touched by the new package**

Run:

```bash
go test ./internal/work ./internal/backend -count=1
```

Expected: PASS.

- [x] **Step 3: Save a local verification report**

Create `reports/TEST_REPORT_2026-05-22_work-coordinator-foundation.md` with:

```markdown
# Work Coordinator Foundation Test Report

Date: 2026-05-22
Surface: virtual lab not required; focused Go package tests only

## Commands

- `go test ./internal/work ./internal/backend -run 'Work|LatestWinsLoadCoordinator' -count=1`
- `go test ./internal/work ./internal/backend -count=1`

## Result

Both commands passed. The new `internal/work` package is covered by unit tests for latest UI intent, resource coalescing, completed resource replay, and serial source mutation ordering. The legacy backend load coordinator remains unchanged.
```

- [x] **Step 4: Commit tracked source changes**

Run:

```bash
git status --short
git add internal/work/work.go internal/work/coordinator.go internal/work/coordinator_test.go internal/backend/sync_coordinator_test.go
git commit -m "test: verify work coordinator foundation"
```

Expected: report remains untracked because `reports/` is gitignored; source changes are committed.
