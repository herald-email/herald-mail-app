package app

import (
	"testing"
	"time"

	"github.com/herald-email/herald-mail-app/internal/models"
)

func TestSyncAccumulator_MicrobatchesByCountThreshold(t *testing.T) {
	acc := newSyncAccumulator(100, 500*time.Millisecond)

	action := acc.observe(models.FolderSyncEvent{
		Folder:     "INBOX",
		Generation: 1,
		Phase:      models.SyncPhaseRowsCached,
		EventCount: 99,
	})
	if action.FlushNow {
		t.Fatal("expected fewer than 100 events to stay buffered")
	}
	if !action.ArmTimer {
		t.Fatal("expected timer to arm on first buffered event")
	}

	action = acc.observe(models.FolderSyncEvent{
		Folder:     "INBOX",
		Generation: 1,
		Phase:      models.SyncPhaseRowsCached,
		EventCount: 1,
	})
	if !action.FlushNow {
		t.Fatal("expected threshold hit at 100 events")
	}
	if acc.pendingCount != 0 {
		t.Fatalf("expected pending count to reset after flush, got %d", acc.pendingCount)
	}
}

func TestSyncAccumulator_CompleteForcesImmediateFlush(t *testing.T) {
	acc := newSyncAccumulator(100, 500*time.Millisecond)

	action := acc.observe(models.FolderSyncEvent{
		Folder:     "INBOX",
		Generation: 7,
		Phase:      models.SyncPhaseRowsCached,
		EventCount: 12,
	})
	if !action.ArmTimer {
		t.Fatal("expected buffered rows to arm a timer")
	}

	action = acc.observe(models.FolderSyncEvent{
		Folder:     "INBOX",
		Generation: 7,
		Phase:      models.SyncPhaseComplete,
	})
	if !action.FlushNow {
		t.Fatal("expected sync_complete to flush immediately")
	}
	if acc.pendingCount != 0 {
		t.Fatalf("expected pending count to reset after completion flush, got %d", acc.pendingCount)
	}
}

func TestSyncAccumulator_InvalidatesPriorGeneration(t *testing.T) {
	acc := newSyncAccumulator(100, 500*time.Millisecond)

	acc.observe(models.FolderSyncEvent{
		Folder:     "INBOX",
		Generation: 3,
		Phase:      models.SyncPhaseRowsCached,
		EventCount: 10,
	})

	action := acc.observe(models.FolderSyncEvent{
		Folder:     "INBOX",
		Generation: 4,
		Phase:      models.SyncPhaseSyncStarted,
	})
	if action.FlushNow {
		t.Fatal("did not expect generation reset itself to flush")
	}
	if acc.generation != 4 {
		t.Fatalf("expected accumulator generation 4, got %d", acc.generation)
	}
	if acc.pendingCount != 0 {
		t.Fatalf("expected generation reset to drop buffered work, got %d pending", acc.pendingCount)
	}
}
