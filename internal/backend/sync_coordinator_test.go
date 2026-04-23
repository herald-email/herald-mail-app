package backend

import "testing"

func TestLatestWinsLoadCoordinator_ReplacesPendingRequests(t *testing.T) {
	c := newLatestWinsLoadCoordinator()

	first := c.Submit("INBOX")
	second := c.Submit("Sent")
	third := c.Submit("INBOX")

	if first.Generation == second.Generation || second.Generation == third.Generation {
		t.Fatalf("expected monotonically increasing generations, got %d %d %d", first.Generation, second.Generation, third.Generation)
	}

	got, ok := c.DrainPending()
	if !ok {
		t.Fatal("expected pending request")
	}
	if got.Folder != "INBOX" || got.Generation != third.Generation {
		t.Fatalf("expected latest pending request to win, got %+v want generation %d", got, third.Generation)
	}

	if _, ok := c.DrainPending(); ok {
		t.Fatal("expected coordinator to be empty after draining latest request")
	}
}

func TestLatestWinsLoadCoordinator_KeepsNewestWhileWorkIsInFlight(t *testing.T) {
	c := newLatestWinsLoadCoordinator()

	initial := c.Submit("INBOX")
	drained, ok := c.DrainPending()
	if !ok {
		t.Fatal("expected initial pending request")
	}
	if drained.Generation != initial.Generation {
		t.Fatalf("expected to drain initial request, got generation %d want %d", drained.Generation, initial.Generation)
	}

	sent := c.Submit("Sent")
	archive := c.Submit("Archive")

	got, ok := c.DrainPending()
	if !ok {
		t.Fatal("expected newest queued request while work is in flight")
	}
	if got.Generation != archive.Generation || got.Folder != "Archive" {
		t.Fatalf("expected newest request to win, got %+v want %+v", got, archive)
	}
	if sent.Generation >= archive.Generation {
		t.Fatalf("expected generations to increase, got sent=%d archive=%d", sent.Generation, archive.Generation)
	}
}
