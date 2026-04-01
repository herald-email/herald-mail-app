package cache

import (
	"os"
	"testing"
)

func newTestCacheForUnsub(t *testing.T) (*Cache, func()) {
	t.Helper()
	f, err := os.CreateTemp("", "unsubscribed_test_*.db")
	if err != nil {
		t.Fatalf("create temp db: %v", err)
	}
	f.Close()
	c, err := New(f.Name())
	if err != nil {
		os.Remove(f.Name())
		t.Fatalf("new cache: %v", err)
	}
	return c, func() {
		c.Close()
		os.Remove(f.Name())
	}
}

func TestRecordUnsubscribe(t *testing.T) {
	c, cleanup := newTestCacheForUnsub(t)
	defer cleanup()

	if err := c.RecordUnsubscribe("newsletter@example.com", "one-click", "https://example.com/unsub"); err != nil {
		t.Fatalf("RecordUnsubscribe: %v", err)
	}

	ok, err := c.IsUnsubscribedSender("newsletter@example.com")
	if err != nil {
		t.Fatalf("IsUnsubscribedSender: %v", err)
	}
	if !ok {
		t.Error("expected sender to be recorded as unsubscribed")
	}
}

func TestRecordUnsubscribe_Idempotent(t *testing.T) {
	c, cleanup := newTestCacheForUnsub(t)
	defer cleanup()

	sender := "promo@store.com"
	for i := 0; i < 3; i++ {
		if err := c.RecordUnsubscribe(sender, "url-copied", "https://store.com/unsub"); err != nil {
			t.Fatalf("RecordUnsubscribe attempt %d: %v", i, err)
		}
	}

	var count int
	if err := c.db.QueryRow(`SELECT COUNT(*) FROM unsubscribed_senders WHERE sender = ?`, sender).Scan(&count); err != nil {
		t.Fatalf("count query: %v", err)
	}
	if count != 1 {
		t.Errorf("expected exactly 1 record, got %d", count)
	}
}

func TestIsUnsubscribedSender_False(t *testing.T) {
	c, cleanup := newTestCacheForUnsub(t)
	defer cleanup()

	ok, err := c.IsUnsubscribedSender("unknown@nowhere.com")
	if err != nil {
		t.Fatalf("IsUnsubscribedSender: %v", err)
	}
	if ok {
		t.Error("expected unknown sender to return false")
	}
}
