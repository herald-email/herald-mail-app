package work

import (
	"context"
	"errors"
	"reflect"
	"sync"
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

	email1 := ResourceKey{SourceID: "default-mail", CollectionID: "INBOX", ItemID: "email-1", Operation: "fetch-preview"}
	email2 := ResourceKey{SourceID: "default-mail", CollectionID: "INBOX", ItemID: "email-2", Operation: "fetch-preview"}

	first := c.Submit(ctx, Spec{
		IntentKey:   IntentKey{ViewID: "timeline-preview"},
		ResourceKey: email1,
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
		ResourceKey: email2,
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
		ResourceKey: email1,
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

func TestCompletedReplayAfterInterveningIntentKeepsLatestResource(t *testing.T) {
	c := NewCoordinator()
	defer c.Close()

	ctx := context.Background()
	intent := IntentKey{ViewID: "timeline-preview"}
	email1 := ResourceKey{SourceID: "default-mail", CollectionID: "INBOX", ItemID: "email-1", Operation: "fetch-preview"}
	email2 := ResourceKey{SourceID: "default-mail", CollectionID: "INBOX", ItemID: "email-2", Operation: "fetch-preview"}
	email2Release := make(chan struct{})

	first := c.Submit(ctx, Spec{
		IntentKey:   intent,
		ResourceKey: email1,
		Policy:      PolicyTakeLatestByIntent | PolicyCoalesceByResource | PolicyReplayCompletedResource,
		Priority:    PriorityInteractive,
		Run: func(ctx context.Context) (any, error) {
			return "body-1", nil
		},
	})
	if got, err := first.Await(ctx); err != nil || got != "body-1" {
		t.Fatalf("first Await = (%v, %v), want body-1 nil", got, err)
	}

	second := c.Submit(ctx, Spec{
		IntentKey:   intent,
		ResourceKey: email2,
		Policy:      PolicyTakeLatestByIntent | PolicyCoalesceByResource | PolicyReplayCompletedResource,
		Priority:    PriorityInteractive,
		Run: func(ctx context.Context) (any, error) {
			select {
			case <-email2Release:
			case <-ctx.Done():
				return nil, ctx.Err()
			}
			return "body-2", nil
		},
	})
	third := c.Submit(ctx, Spec{
		IntentKey:   intent,
		ResourceKey: email1,
		Policy:      PolicyTakeLatestByIntent | PolicyCoalesceByResource | PolicyReplayCompletedResource,
		Priority:    PriorityInteractive,
		Run: func(ctx context.Context) (any, error) {
			t.Fatal("completed email-1 resource should replay without rerunning")
			return nil, nil
		},
	})

	if got, err := third.Await(ctx); err != nil || got != "body-1" {
		t.Fatalf("third Await = (%v, %v), want replayed body-1 nil", got, err)
	}
	close(email2Release)
	if got, err := second.Await(ctx); !errors.Is(err, ErrStaleIntent) || got != nil {
		t.Fatalf("second Await = (%v, %v), want nil ErrStaleIntent", got, err)
	}
}

func TestSerialBySourcePreservesMutationOrder(t *testing.T) {
	c := NewCoordinator()
	defer c.Close()

	ctx := context.Background()
	firstStarted := make(chan struct{})
	releaseFirst := make(chan struct{})
	var mu sync.Mutex
	var order []string

	first := c.Submit(ctx, Spec{
		SourceID: SourceID("default-mail"),
		Policy:   PolicySerialBySource,
		Priority: PriorityUserAction,
		Run: func(ctx context.Context) (any, error) {
			mu.Lock()
			order = append(order, "delete-1")
			mu.Unlock()
			close(firstStarted)
			select {
			case <-releaseFirst:
			case <-ctx.Done():
				return nil, ctx.Err()
			}
			return "ok-1", nil
		},
	})
	<-firstStarted
	second := c.Submit(ctx, Spec{
		SourceID: SourceID("default-mail"),
		Policy:   PolicySerialBySource,
		Priority: PriorityUserAction,
		Run: func(ctx context.Context) (any, error) {
			mu.Lock()
			order = append(order, "delete-2")
			mu.Unlock()
			return "ok-2", nil
		},
	})

	select {
	case <-second.ready:
		t.Fatal("second mutation finished before first mutation was released")
	case <-time.After(10 * time.Millisecond):
	}
	close(releaseFirst)

	if got, err := first.Await(ctx); err != nil || got != "ok-1" {
		t.Fatalf("first Await = (%v, %v), want ok-1 nil", got, err)
	}
	if got, err := second.Await(ctx); err != nil || got != "ok-2" {
		t.Fatalf("second Await = (%v, %v), want ok-2 nil", got, err)
	}
	mu.Lock()
	defer mu.Unlock()
	if !reflect.DeepEqual(order, []string{"delete-1", "delete-2"}) {
		t.Fatalf("order = %#v, want delete-1 then delete-2", order)
	}
}

func TestCoordinatorCloseRejectsNewWorkAndClosesInflightWork(t *testing.T) {
	c := NewCoordinator()
	ctx := context.Background()

	release := make(chan struct{})
	active := c.Submit(ctx, Spec{
		ResourceKey: ResourceKey{SourceID: "default-mail", CollectionID: "INBOX", ItemID: "email-1", Operation: "fetch-preview"},
		Policy:      PolicyCoalesceByResource,
		Priority:    PriorityInteractive,
		Run: func(ctx context.Context) (any, error) {
			<-release
			return "late", nil
		},
	})

	c.Close()
	if got, err := active.Await(ctx); !errors.Is(err, ErrClosed) || got != nil {
		t.Fatalf("active Await = (%v, %v), want nil ErrClosed", got, err)
	}

	later := c.Submit(ctx, Spec{
		ResourceKey: ResourceKey{SourceID: "default-mail", CollectionID: "INBOX", ItemID: "email-2", Operation: "fetch-preview"},
		Run: func(ctx context.Context) (any, error) {
			t.Fatal("closed coordinator should reject new work")
			return nil, nil
		},
	})
	if got, err := later.Await(ctx); !errors.Is(err, ErrClosed) || got != nil {
		t.Fatalf("later Await = (%v, %v), want nil ErrClosed", got, err)
	}
	close(release)
}

func TestFairQueueRoundRobinsSources(t *testing.T) {
	q := NewFairQueue[string]()

	q.Push(SourceID("work"), "work-1")
	q.Push(SourceID("work"), "work-2")
	q.Push(SourceID("personal"), "personal-1")
	q.Push(SourceID("personal"), "personal-2")
	q.Push(SourceID("work"), "work-3")

	var got []string
	for {
		item, ok := q.Pop()
		if !ok {
			break
		}
		got = append(got, item)
	}

	want := []string{"work-1", "personal-1", "work-2", "personal-2", "work-3"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("fair pop order = %#v, want %#v", got, want)
	}
}
