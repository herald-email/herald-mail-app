//go:build !race

// Package testutil provides shared test helpers, including an in-memory IMAP
// server for integration tests.
package testutil

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"strconv"
	"testing"
	"time"

	goImap "github.com/emersion/go-imap"
	"github.com/emersion/go-imap/backend"
	imapmemory "github.com/emersion/go-imap/backend/memory"
	"github.com/emersion/go-imap/server"
	"github.com/herald-email/herald-mail-app/internal/config"
)

// bytesLiteral wraps a []byte so it satisfies the imap.Literal interface.
type bytesLiteral struct {
	*bytes.Reader
}

func newBytesLiteral(b []byte) *bytesLiteral {
	return &bytesLiteral{bytes.NewReader(b)}
}

func (l *bytesLiteral) Len() int { return l.Reader.Len() }

// credentialWrapper wraps the memory backend and remaps credentials so tests
// can use arbitrary usernames/passwords.
type credentialWrapper struct {
	inner    *imapmemory.Backend
	username string
	password string
}

func (b *credentialWrapper) Login(connInfo *goImap.ConnInfo, username, password string) (backend.User, error) {
	if username != b.username || password != b.password {
		return nil, backend.ErrInvalidCredentials
	}
	// The memory backend was initialised with "username"/"password"; forward
	// with those default credentials so the user object is valid.
	return b.inner.Login(connInfo, "username", "password")
}

// StartMockIMAPServer starts an in-memory IMAP server on a random loopback
// port.  It pre-seeds INBOX with 5 messages and returns:
//   - addr   – "host:port" of the listening server
//   - cfg    – a *config.Config already pointed at the server
//   - stop   – call to shut the server down
//
// The server uses a self-signed TLS certificate so that the production
// Connect() code (which calls StartTLS with InsecureSkipVerify) works
// without modification.
func StartMockIMAPServer(t testing.TB) (addr string, cfg *config.Config, stop func()) {
	t.Helper()

	// Build the memory backend (default user "username"/"password", 1 message).
	memBe := imapmemory.New()

	// Seed 4 more messages so INBOX has 5 total.
	u, err := memBe.Login(nil, "username", "password")
	if err != nil {
		t.Fatalf("testutil: cannot obtain test user: %v", err)
	}
	mb, err := u.GetMailbox("INBOX")
	if err != nil {
		t.Fatalf("testutil: cannot get INBOX: %v", err)
	}

	for i := 2; i <= 5; i++ {
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
		if err := mb.CreateMessage(
			[]string{},
			time.Now().Add(time.Duration(i)*time.Minute),
			newBytesLiteral([]byte(rawMsg)),
		); err != nil {
			t.Fatalf("testutil: seed message %d: %v", i, err)
		}
	}

	// Create "Archive" mailbox so MoveEmail tests have somewhere to move to.
	if err := u.CreateMailbox("Archive"); err != nil {
		t.Fatalf("testutil: create Archive mailbox: %v", err)
	}

	// Wrap the backend so tests can use "test@example.com"/"password".
	be := &credentialWrapper{
		inner:    memBe,
		username: "test@example.com",
		password: "password",
	}

	// Generate a self-signed TLS certificate so the production Connect() path
	// (Dial → StartTLS with InsecureSkipVerify) works without modification.
	tlsCert, err := generateSelfSignedCert()
	if err != nil {
		t.Fatalf("testutil: generate TLS cert: %v", err)
	}

	// Start the IMAP server.
	s := server.New(be)
	s.AllowInsecureAuth = true
	s.TLSConfig = &tls.Config{Certificates: []tls.Certificate{tlsCert}}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("testutil: listen: %v", err)
	}

	go func() { _ = s.Serve(ln) }()

	addr = ln.Addr().String()
	host, portStr, _ := net.SplitHostPort(addr)
	port, _ := strconv.Atoi(portStr)

	cfg = &config.Config{}
	cfg.Server.Host = host // "127.0.0.1" triggers the local-bridge code path
	cfg.Server.Port = port
	cfg.Credentials.Username = "test@example.com"
	cfg.Credentials.Password = "password"

	stop = func() {
		_ = s.Close()
		_ = ln.Close()
	}
	return addr, cfg, stop
}

// generateSelfSignedCert returns a TLS certificate with a self-signed cert
// valid for "127.0.0.1" and "localhost".
func generateSelfSignedCert() (tls.Certificate, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return tls.Certificate{}, err
	}

	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "localhost"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(24 * time.Hour),
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
		DNSNames:     []string{"localhost"},
	}

	certDER, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		return tls.Certificate{}, err
	}

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return tls.Certificate{}, err
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})

	return tls.X509KeyPair(certPEM, keyPEM)
}
