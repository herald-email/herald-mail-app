package imap

import (
	"strings"
	"testing"
)

func TestFetchAllServerUIDFlagStatesReturnsErrorWhenClientClosed(t *testing.T) {
	c := &Client{}

	_, _, err := c.fetchAllServerUIDFlagStates("INBOX")
	if err == nil {
		t.Fatal("fetchAllServerUIDFlagStates returned nil error for closed client")
	}
	if !strings.Contains(err.Error(), "IMAP client unavailable") {
		t.Fatalf("error = %v, want IMAP client unavailable", err)
	}
}
