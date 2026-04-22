package imap

import (
	"testing"
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
