package imap

import (
	"errors"
	"testing"

	goimap "github.com/emersion/go-imap"
)

// --- decideSyncStrategy ---

func TestDecideSyncStrategy_NoNewMail(t *testing.T) {
	// Same UIDNEXT → no fetch needed
	got := decideSyncStrategy(12345, 500, 12345, 500)
	if got != syncStrategyNone {
		t.Errorf("expected syncStrategyNone, got %v", got)
	}
}

func TestDecideSyncStrategy_NewEmails(t *testing.T) {
	// UIDNEXT increased → incremental fetch
	got := decideSyncStrategy(12345, 500, 12345, 510)
	if got != syncStrategyIncremental {
		t.Errorf("expected syncStrategyIncremental, got %v", got)
	}
}

func TestDecideSyncStrategy_UIDValidityChanged(t *testing.T) {
	// UIDVALIDITY mismatch → full resync
	got := decideSyncStrategy(12345, 500, 99999, 500)
	if got != syncStrategyFull {
		t.Errorf("expected syncStrategyFull, got %v", got)
	}
}

func TestDecideSyncStrategy_FirstRun(t *testing.T) {
	// storedValidity==0 means no prior sync → treat as full resync
	got := decideSyncStrategy(0, 0, 12345, 500)
	if got != syncStrategyFull {
		t.Errorf("expected syncStrategyFull on first run, got %v", got)
	}
}

func TestAdjustSyncStrategyForCacheRecovery_ForcesFullResyncWhenCacheIsIncomplete(t *testing.T) {
	got := adjustSyncStrategyForCacheRecovery(syncStrategyNone, 12345, 12345, 25, 1317)
	if got != syncStrategyFull {
		t.Fatalf("expected incomplete cache to force full sync, got %v", got)
	}
}

func TestAdjustSyncStrategyForCacheRecovery_PreservesNoneWhenCacheLooksHealthy(t *testing.T) {
	got := adjustSyncStrategyForCacheRecovery(syncStrategyNone, 12345, 12345, 1317, 1317)
	if got != syncStrategyNone {
		t.Fatalf("expected healthy cache to preserve syncStrategyNone, got %v", got)
	}
}

func TestMessageFlagStateFromIMAPDetectsDraftFlagAndCanonicalDraftFolder(t *testing.T) {
	tests := []struct {
		name      string
		flags     []string
		folder    string
		labels    []string
		wantRead  bool
		wantStar  bool
		wantDraft bool
	}{
		{
			name:      "explicit draft flag",
			flags:     []string{goimap.SeenFlag, goimap.FlaggedFlag, "\\Draft"},
			folder:    "INBOX",
			wantRead:  true,
			wantStar:  true,
			wantDraft: true,
		},
		{
			name:      "gmail drafts folder",
			flags:     []string{goimap.SeenFlag},
			folder:    "[Gmail]/Drafts",
			wantRead:  true,
			wantDraft: true,
		},
		{
			name:      "inbox drafts folder",
			flags:     nil,
			folder:    "INBOX.Drafts",
			wantDraft: true,
		},
		{
			name:     "regular inbox",
			flags:    []string{goimap.SeenFlag},
			folder:   "INBOX",
			wantRead: true,
		},
		{
			name:      "gmail draft label on inbox graft",
			flags:     []string{goimap.SeenFlag},
			folder:    "INBOX",
			wantRead:  true,
			wantDraft: true,
			labels:    []string{"\\Inbox", "\\Draft"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := messageFlagStateFromIMAP(77, tt.flags, tt.folder, tt.labels...)
			if got.UID != 77 {
				t.Fatalf("UID = %d, want 77", got.UID)
			}
			if got.IsRead != tt.wantRead || got.IsStarred != tt.wantStar || got.IsDraft != tt.wantDraft {
				t.Fatalf("flags = read:%v starred:%v draft:%v, want read:%v starred:%v draft:%v",
					got.IsRead, got.IsStarred, got.IsDraft, tt.wantRead, tt.wantStar, tt.wantDraft)
			}
		})
	}
}

// --- buildValidIDSet ---

func TestBuildValidIDSet_AllValid(t *testing.T) {
	cached := []uidMsgPair{
		{MessageID: "<a@x.com>", UID: 1},
		{MessageID: "<b@x.com>", UID: 2},
	}
	serverUIDs := map[uint32]bool{1: true, 2: true}

	valid, staleUIDs, staleMessageIDs := buildValidIDSet(cached, serverUIDs)

	if !valid["<a@x.com>"] || !valid["<b@x.com>"] {
		t.Error("expected both IDs to be valid")
	}
	if len(staleUIDs) != 0 {
		t.Errorf("expected no stale UIDs, got %v", staleUIDs)
	}
	if len(staleMessageIDs) != 0 {
		t.Errorf("expected no stale message IDs, got %v", staleMessageIDs)
	}
}

func TestBuildValidIDSet_SomeStale(t *testing.T) {
	cached := []uidMsgPair{
		{MessageID: "<a@x.com>", UID: 1},
		{MessageID: "<b@x.com>", UID: 2}, // stale
		{MessageID: "<c@x.com>", UID: 3},
	}
	serverUIDs := map[uint32]bool{1: true, 3: true}

	valid, staleUIDs, staleMessageIDs := buildValidIDSet(cached, serverUIDs)

	if !valid["<a@x.com>"] || !valid["<c@x.com>"] {
		t.Error("expected a and c to be valid")
	}
	if valid["<b@x.com>"] {
		t.Error("b should not be valid")
	}
	if len(staleUIDs) != 1 || staleUIDs[0] != 2 {
		t.Errorf("expected staleUIDs=[2], got %v", staleUIDs)
	}
	if len(staleMessageIDs) != 0 {
		t.Errorf("expected no stale message IDs, got %v", staleMessageIDs)
	}
}

func TestBuildValidIDSet_LegacyZeroUID(t *testing.T) {
	// Emails cached before UID tracking (uid=0) should be invalidated and deleted.
	cached := []uidMsgPair{
		{MessageID: "<legacy@x.com>", UID: 0},
		{MessageID: "<stale@x.com>", UID: 5}, // not on server
	}
	serverUIDs := map[uint32]bool{1: true}

	valid, staleUIDs, staleMessageIDs := buildValidIDSet(cached, serverUIDs)

	if valid["<legacy@x.com>"] {
		t.Error("legacy zero-UID entry should not remain valid")
	}
	if valid["<stale@x.com>"] {
		t.Error("stale non-zero UID entry must not be valid")
	}
	if len(staleUIDs) != 1 || staleUIDs[0] != 5 {
		t.Errorf("expected staleUIDs=[5], got %v", staleUIDs)
	}
	if len(staleMessageIDs) != 1 || staleMessageIDs[0] != "<legacy@x.com>" {
		t.Errorf("expected staleMessageIDs=[<legacy@x.com>], got %v", staleMessageIDs)
	}
}

func TestBuildValidIDSet_StaleOrderedDescending(t *testing.T) {
	// Stale UIDs must be sorted highest-first (newest deleted first)
	cached := []uidMsgPair{
		{MessageID: "<x1@x.com>", UID: 10},
		{MessageID: "<x2@x.com>", UID: 30},
		{MessageID: "<x3@x.com>", UID: 20},
	}
	serverUIDs := map[uint32]bool{} // all stale

	_, staleUIDs, staleMessageIDs := buildValidIDSet(cached, serverUIDs)

	if len(staleUIDs) != 3 {
		t.Fatalf("expected 3 stale UIDs, got %d", len(staleUIDs))
	}
	if staleUIDs[0] != 30 || staleUIDs[1] != 20 || staleUIDs[2] != 10 {
		t.Errorf("expected descending order [30 20 10], got %v", staleUIDs)
	}
	if len(staleMessageIDs) != 0 {
		t.Errorf("expected no stale message IDs, got %v", staleMessageIDs)
	}
}

func TestRetryAfterReconnect_RetriesConnectionErrors(t *testing.T) {
	attempts := 0
	reconnects := 0

	value, err := retryAfterReconnect(func() (int, error) {
		attempts++
		if attempts == 1 {
			return 0, errors.New("EOF")
		}
		return 42, nil
	}, func() error {
		reconnects++
		return nil
	})

	if err != nil {
		t.Fatalf("retryAfterReconnect returned error: %v", err)
	}
	if value != 42 {
		t.Fatalf("expected value 42, got %d", value)
	}
	if attempts != 2 {
		t.Fatalf("expected 2 attempts, got %d", attempts)
	}
	if reconnects != 1 {
		t.Fatalf("expected 1 reconnect, got %d", reconnects)
	}
}

func TestRetryAfterReconnect_DoesNotRetryNonConnectionErrors(t *testing.T) {
	attempts := 0
	reconnects := 0
	wantErr := errors.New("mailbox missing")

	_, err := retryAfterReconnect(func() (int, error) {
		attempts++
		return 0, wantErr
	}, func() error {
		reconnects++
		return nil
	})

	if !errors.Is(err, wantErr) {
		t.Fatalf("expected original error, got %v", err)
	}
	if attempts != 1 {
		t.Fatalf("expected 1 attempt, got %d", attempts)
	}
	if reconnects != 0 {
		t.Fatalf("expected 0 reconnects, got %d", reconnects)
	}
}

func TestChunkUint32s_SplitsIntoStableBatches(t *testing.T) {
	chunks := chunkUint32s([]uint32{1, 2, 3, 4, 5}, 2)
	if len(chunks) != 3 {
		t.Fatalf("expected 3 chunks, got %d", len(chunks))
	}
	want := [][]uint32{{1, 2}, {3, 4}, {5}}
	for i := range want {
		if len(chunks[i]) != len(want[i]) {
			t.Fatalf("chunk %d length = %d, want %d", i, len(chunks[i]), len(want[i]))
		}
		for j := range want[i] {
			if chunks[i][j] != want[i][j] {
				t.Fatalf("chunk %d item %d = %d, want %d", i, j, chunks[i][j], want[i][j])
			}
		}
	}
}
