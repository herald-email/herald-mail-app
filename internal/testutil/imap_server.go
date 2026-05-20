//go:build !race

// Package testutil provides shared test helpers.
package testutil

import (
	"fmt"
	"testing"
	"time"

	"github.com/herald-email/herald-mail-app/internal/config"
	"github.com/herald-email/herald-mail-app/internal/testmail"
)

// StartMockIMAPServer starts an in-memory IMAP server on a random loopback
// port. It pre-seeds INBOX with 5 messages and returns:
//   - addr   - "host:port" of the listening server
//   - cfg    - a *config.Config already pointed at the server
//   - stop   - call to shut the server down
//
// New integration tests should prefer internal/testmail.Start directly; this
// wrapper keeps older tests on their original single-account fixture.
func StartMockIMAPServer(t testing.TB) (addr string, cfg *config.Config, stop func()) {
	t.Helper()

	seeds := make([]testmail.Option, 0, 6)
	seeds = append(seeds, testmail.WithAccount("test@example.com", "password"))
	for i := 1; i <= 5; i++ {
		rawMsg := fmt.Sprintf(
			"From: sender%d@example.com\r\n"+
				"To: test@example.com\r\n"+
				"Subject: Test message %d\r\n"+
				"Date: %s\r\n"+
				"Message-ID: <test-msg-%d@localhost>\r\n"+
				"Content-Type: text/plain\r\n"+
				"\r\n"+
				"Body of test message %d.\r\n",
			i, i,
			time.Now().Add(time.Duration(i)*time.Minute).Format("Mon, 02 Jan 2006 15:04:05 -0700"),
			i, i,
		)
		seeds = append(seeds, testmail.WithEML("test@example.com", "INBOX", []byte(rawMsg)))
	}

	lab := testmail.Start(t, seeds...)
	account := lab.Account("test@example.com")
	if account == nil {
		t.Fatalf("testutil: test account missing")
	}
	cfg = account.Config("")
	addr = fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)
	return addr, cfg, lab.Close
}
