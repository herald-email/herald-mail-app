package work

import "testing"

func TestLatestIntentQueueReplacesPendingAndAdvancesGeneration(t *testing.T) {
	q := NewLatestIntentQueue[string]()
	intent := IntentKey{ViewID: "active-folder"}

	first := q.Submit(intent, ResourceKey{CollectionID: "INBOX", Operation: "sync"}, "INBOX")
	second := q.Submit(intent, ResourceKey{CollectionID: "Sent", Operation: "sync"}, "Sent")
	third := q.Submit(intent, ResourceKey{CollectionID: "INBOX", Operation: "sync"}, "INBOX")

	if first.Generation != 1 || second.Generation != 2 || third.Generation != 3 {
		t.Fatalf("generations = %d, %d, %d; want 1, 2, 3", first.Generation, second.Generation, third.Generation)
	}

	got, ok := q.DrainPending()
	if !ok {
		t.Fatal("expected pending item")
	}
	if got.Generation != third.Generation || got.Value != "INBOX" || got.ResourceKey.CollectionID != "INBOX" {
		t.Fatalf("pending = %#v, want latest INBOX generation %d", got, third.Generation)
	}
	if _, ok := q.DrainPending(); ok {
		t.Fatal("expected empty queue after draining latest item")
	}
}

func TestLatestIntentQueueKeepsNewestWhileWorkIsInFlight(t *testing.T) {
	q := NewLatestIntentQueue[string]()
	intent := IntentKey{ViewID: "active-folder"}

	initial := q.Submit(intent, ResourceKey{CollectionID: "INBOX", Operation: "sync"}, "INBOX")
	drained, ok := q.DrainPending()
	if !ok {
		t.Fatal("expected initial item")
	}
	if drained.Generation != initial.Generation || drained.Value != "INBOX" {
		t.Fatalf("drained = %#v, want initial generation %d", drained, initial.Generation)
	}

	sent := q.Submit(intent, ResourceKey{CollectionID: "Sent", Operation: "sync"}, "Sent")
	archive := q.Submit(intent, ResourceKey{CollectionID: "Archive", Operation: "sync"}, "Archive")
	got, ok := q.DrainPending()
	if !ok {
		t.Fatal("expected newest queued item while work is in flight")
	}
	if got.Generation != archive.Generation || got.Value != "Archive" {
		t.Fatalf("pending = %#v, want newest archive generation %d", got, archive.Generation)
	}
	if sent.Generation >= archive.Generation {
		t.Fatalf("expected generations to increase, got sent=%d archive=%d", sent.Generation, archive.Generation)
	}
}

func TestLatestIntentQueueKeepsDistinctIntentOrder(t *testing.T) {
	q := NewLatestIntentQueue[string]()
	active := IntentKey{ViewID: "active-folder"}
	search := IntentKey{ViewID: "active-search"}

	q.Submit(active, ResourceKey{CollectionID: "INBOX", Operation: "sync"}, "active-1")
	searchItem := q.Submit(search, ResourceKey{CollectionID: "all", Operation: "search"}, "search-1")
	activeItem := q.Submit(active, ResourceKey{CollectionID: "Archive", Operation: "sync"}, "active-2")

	got, ok := q.DrainPending()
	if !ok {
		t.Fatal("expected first pending item")
	}
	if got.IntentKey != active || got.Generation != activeItem.Generation || got.Value != "active-2" {
		t.Fatalf("first drain = %#v, want latest active intent item %#v", got, activeItem)
	}

	got, ok = q.DrainPending()
	if !ok {
		t.Fatal("expected second pending item")
	}
	if got.IntentKey != search || got.Generation != searchItem.Generation || got.Value != "search-1" {
		t.Fatalf("second drain = %#v, want search item %#v", got, searchItem)
	}
}
