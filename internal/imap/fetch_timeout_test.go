package imap

import (
	"errors"
	"testing"
	"time"

	"github.com/emersion/go-imap"
)

func TestRunIMAPStreamCommandRetriesAfterIdleTimeout(t *testing.T) {
	attempts := 0
	reconnects := 0
	abortCh := make(chan struct{})
	handled := make([]uint32, 0, 1)

	err := runIMAPStreamCommand(imapStreamCommandOptions{
		Name:        "test fetch",
		Folder:      "INBOX",
		Phase:       "fetching",
		RangeLabel:  "1:2",
		IdleTimeout: 10 * time.Millisecond,
		AbortWait:   100 * time.Millisecond,
		MaxAttempts: 2,
		Reconnect: func() error {
			reconnects++
			return nil
		},
		Abort: func() {
			if attempts == 1 {
				close(abortCh)
			}
		},
	}, func(messages chan *imap.Message) error {
		attempts++
		defer close(messages)
		if attempts == 1 {
			messages <- &imap.Message{SeqNum: 1}
			<-abortCh
			return errors.New("use of closed network connection")
		}
		messages <- &imap.Message{SeqNum: 2}
		return nil
	}, func(msg *imap.Message) error {
		handled = append(handled, msg.SeqNum)
		return nil
	})

	if err != nil {
		t.Fatalf("runIMAPStreamCommand returned error: %v", err)
	}
	if attempts != 2 {
		t.Fatalf("attempts = %d, want 2", attempts)
	}
	if reconnects != 1 {
		t.Fatalf("reconnects = %d, want 1", reconnects)
	}
	if len(handled) != 1 || handled[0] != 2 {
		t.Fatalf("handled messages = %v, want only successful retry message 2", handled)
	}
}

func TestRunIMAPStreamCommandReturnsTimeoutAfterRetryExhausted(t *testing.T) {
	attempts := 0
	abortChans := []chan struct{}{make(chan struct{}), make(chan struct{})}

	err := runIMAPStreamCommand(imapStreamCommandOptions{
		Name:        "test fetch",
		Folder:      "INBOX",
		Phase:       "fetching",
		RangeLabel:  "1:2",
		IdleTimeout: 10 * time.Millisecond,
		AbortWait:   100 * time.Millisecond,
		MaxAttempts: 2,
		Reconnect:   func() error { return nil },
		Abort: func() {
			if attempts >= 1 && attempts <= len(abortChans) {
				close(abortChans[attempts-1])
			}
		},
	}, func(messages chan *imap.Message) error {
		attempts++
		defer close(messages)
		messages <- &imap.Message{SeqNum: uint32(attempts)}
		<-abortChans[attempts-1]
		return errors.New("use of closed network connection")
	}, func(*imap.Message) error {
		return nil
	})

	if !errors.Is(err, ErrIMAPCommandTimeout) {
		t.Fatalf("error = %v, want ErrIMAPCommandTimeout", err)
	}
	if attempts != 2 {
		t.Fatalf("attempts = %d, want 2", attempts)
	}
}
