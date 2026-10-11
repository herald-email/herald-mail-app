package backend

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net"
	"sync"
	"testing"

	"github.com/herald-email/herald-mail-app/internal/config"
	"github.com/herald-email/herald-mail-app/internal/models"
	"github.com/herald-email/herald-mail-app/internal/testmail"
)

type failingBodyMailSource struct{ *fakeMailSource }

func (s *failingBodyMailSource) FetchMessageNoCache(context.Context, models.MessageRef) (*models.EmailBody, error) {
	return nil, errors.New("select folder INBOX: configuration not found")
}

func TestFetchEmailBodyPreservesCacheOnUnrelatedProviderError(t *testing.T) {
	c := newMemoryCache(t)
	email := makeEmail("review@example.com")
	email.UID = 42
	if err := c.CacheEmail(email); err != nil {
		t.Fatal(err)
	}
	b := &LocalBackend{cache: c, mailSource: &failingBodyMailSource{newFakeMailSource()}}
	defer b.ensureMessageService().Close()
	_, err := b.FetchEmailBody("INBOX", 42)
	if err == nil {
		t.Fatal("expected provider failure")
	}
	if _, err := c.GetEmailByFolderUID("INBOX", 42); err != nil {
		t.Fatalf("unrelated provider error deleted valid cache row: %v", err)
	}
}

func TestIMAPMailSourceCloseDoesNotDial(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	dialed := make(chan bool, 1)
	go func() {
		c, e := ln.Accept()
		if e == nil {
			dialed <- true
			fmt.Fprint(c, "* BYE unavailable\r\n")
			c.Close()
		}
	}()
	cfg := &config.Config{}
	cfg.Server.Host = "127.0.0.1"
	cfg.Server.Port = ln.Addr().(*net.TCPAddr).Port
	s := NewIMAPMailSource(cfg, "", nil, nil)
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-dialed:
		t.Fatal("Close dialed a disconnected IMAP server")
	default:
	}
}

func TestIMAPMailSourceConcurrentColdReads(t *testing.T) {
	lab := testmail.Start(t, testmail.WithAccount("test@example.com", "password"))
	defer lab.Close()
	s := NewIMAPMailSource(lab.Account("test@example.com").Config(""), "", nil, nil)
	defer s.Close()
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			if _, err := s.ListFolders(context.Background()); err != nil {
				t.Errorf("ListFolders: %v", err)
			}
		}()
	}
	close(start)
	wg.Wait()
}

func TestFetchEmailBodyRemovesConfirmedMissingUID(t *testing.T) {
	lab := testmail.Start(t, testmail.WithAccount("test@example.com", "password"))
	source := NewIMAPMailSource(lab.Account("test@example.com").Config(""), "", nil, nil)
	defer source.Close()
	c := newMemoryCache(t)
	email := makeEmail("missing@example.com")
	email.UID = 42
	if err := c.CacheEmail(email); err != nil {
		t.Fatal(err)
	}
	b := &LocalBackend{cache: c, mailSource: source}
	defer b.ensureMessageService().Close()
	if _, err := b.FetchEmailBody("INBOX", 42); err == nil {
		t.Fatal("expected missing message error")
	}
	if _, err := c.GetEmailByFolderUID("INBOX", 42); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("cache row remains: %v", err)
	}
	// Also cover the fallback path when no cache row exists.
	if _, err := b.FetchEmailBody("INBOX", 43); err == nil {
		t.Fatal("expected missing uncached message error")
	}
}
