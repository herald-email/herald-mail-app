package smtp

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/pem"
	"math/big"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/herald-email/herald-mail-app/internal/config"
)

func TestCheckUsesPlainAuthForPasswordConfig(t *testing.T) {
	server := startAuthCaptureSMTPServer(t)
	cfg := smtpCheckConfig(server.addr)
	cfg.Credentials.Username = "user@example.test"
	cfg.Credentials.Password = "app-password"

	if err := New(cfg).Check(context.Background()); err != nil {
		t.Fatalf("Check returned error: %v", err)
	}

	if server.authLine == "" || !strings.HasPrefix(server.authLine, "AUTH PLAIN ") {
		t.Fatalf("expected AUTH PLAIN, got %q", server.authLine)
	}
	decoded, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(server.authLine, "AUTH PLAIN "))
	if err != nil {
		t.Fatalf("decode AUTH PLAIN: %v", err)
	}
	if got, want := string(decoded), "\x00user@example.test\x00app-password"; got != want {
		t.Fatalf("AUTH PLAIN payload = %q, want %q", got, want)
	}
}

func TestCheckUsesXOAUTH2ForGmailOAuthConfig(t *testing.T) {
	server := startAuthCaptureSMTPServer(t)
	cfg := smtpCheckConfig(server.addr)
	cfg.Gmail.Email = "oauth@example.test"
	cfg.Gmail.AccessToken = "access-token"
	cfg.Gmail.RefreshToken = "refresh-token"
	cfg.Gmail.TokenExpiry = time.Now().Add(time.Hour).UTC().Format(time.RFC3339)

	if err := New(cfg).Check(context.Background()); err != nil {
		t.Fatalf("Check returned error: %v", err)
	}

	if server.authLine == "" || !strings.HasPrefix(server.authLine, "AUTH XOAUTH2 ") {
		t.Fatalf("expected AUTH XOAUTH2, got %q", server.authLine)
	}
	decoded, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(server.authLine, "AUTH XOAUTH2 "))
	if err != nil {
		t.Fatalf("decode AUTH XOAUTH2: %v", err)
	}
	payload := string(decoded)
	if !strings.Contains(payload, "user=oauth@example.test\x01") {
		t.Fatalf("XOAUTH2 payload missing user: %q", payload)
	}
	if !strings.Contains(payload, "auth=Bearer access-token\x01") {
		t.Fatalf("XOAUTH2 payload missing bearer token: %q", payload)
	}
}

func TestCheckSupportsImplicitTLSOnLocalBridgePort(t *testing.T) {
	server := startImplicitTLSAuthCaptureSMTPServer(t)
	cfg := smtpCheckConfig(server.addr)
	cfg.Vendor = "protonmail"
	cfg.Credentials.Username = "bridge@example.test"
	cfg.Credentials.Password = "bridge-password"

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := New(cfg).Check(ctx); err != nil {
		t.Fatalf("Check returned error: %v", err)
	}

	if server.authLine == "" || !strings.HasPrefix(server.authLine, "AUTH PLAIN ") {
		t.Fatalf("expected AUTH PLAIN over implicit TLS, got %q", server.authLine)
	}
}

func TestSendPlainUsesXOAUTH2ForGmailOAuthConfig(t *testing.T) {
	server := startAuthCaptureSMTPServer(t)
	cfg := smtpCheckConfig(server.addr)
	cfg.Gmail.Email = "oauth@example.test"
	cfg.Gmail.AccessToken = "access-token"
	cfg.Gmail.RefreshToken = "refresh-token"
	cfg.Gmail.TokenExpiry = time.Now().Add(time.Hour).UTC().Format(time.RFC3339)

	err := New(cfg).sendPlain(server.addr, "oauth@example.test", []string{"friend@example.test"}, "Subject: hello\r\n\r\nBody")
	if err != nil {
		t.Fatalf("sendPlain returned error: %v", err)
	}
	if server.authLine == "" || !strings.HasPrefix(server.authLine, "AUTH XOAUTH2 ") {
		t.Fatalf("expected compose send path to use AUTH XOAUTH2, got %q", server.authLine)
	}
}

func startImplicitTLSAuthCaptureSMTPServer(t *testing.T) *authCaptureSMTPServer {
	t.Helper()
	cert := newTestCertificate(t)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	tlsLn := tls.NewListener(ln, &tls.Config{Certificates: []tls.Certificate{cert}})
	server := &authCaptureSMTPServer{addr: ln.Addr().String()}
	t.Cleanup(func() { _ = tlsLn.Close() })

	done := make(chan struct{})
	go serveAuthCaptureSMTP(t, tlsLn, server, done)
	t.Cleanup(func() {
		select {
		case <-done:
		case <-time.After(time.Second):
			t.Fatal("implicit TLS SMTP test server did not finish")
		}
	})
	return server
}

func smtpCheckConfig(addr string) *config.Config {
	host, portText, _ := strings.Cut(addr, ":")
	port := 0
	for _, ch := range portText {
		port = port*10 + int(ch-'0')
	}
	cfg := &config.Config{}
	cfg.SMTP.Host = host
	cfg.SMTP.Port = port
	return cfg
}

type authCaptureSMTPServer struct {
	addr     string
	authLine string
}

func startAuthCaptureSMTPServer(t *testing.T) *authCaptureSMTPServer {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	server := &authCaptureSMTPServer{addr: ln.Addr().String()}
	t.Cleanup(func() { _ = ln.Close() })

	done := make(chan struct{})
	go serveAuthCaptureSMTP(t, ln, server, done)
	t.Cleanup(func() {
		select {
		case <-done:
		case <-time.After(time.Second):
			t.Fatal("SMTP test server did not finish")
		}
	})
	return server
}

func serveAuthCaptureSMTP(_ *testing.T, ln net.Listener, server *authCaptureSMTPServer, done chan<- struct{}) {
	defer close(done)
	conn, err := ln.Accept()
	if err != nil {
		return
	}
	defer conn.Close()
	writeLine(conn, "220 test.smtp ESMTP")
	for {
		line, ok := readLine(conn)
		if !ok {
			return
		}
		switch {
		case strings.HasPrefix(line, "EHLO"):
			writeLine(conn, "250-test.smtp")
			writeLine(conn, "250 AUTH PLAIN XOAUTH2")
		case strings.HasPrefix(line, "AUTH "):
			server.authLine = line
			writeLine(conn, "235 2.7.0 Authentication successful")
		case strings.HasPrefix(line, "DATA"):
			writeLine(conn, "354 End data with <CR><LF>.<CR><LF>")
			for {
				dataLine, ok := readLine(conn)
				if !ok {
					return
				}
				if dataLine == "." {
					break
				}
			}
			writeLine(conn, "250 2.0.0 queued")
		case strings.HasPrefix(line, "QUIT"):
			writeLine(conn, "221 2.0.0 Bye")
			return
		default:
			writeLine(conn, "250 OK")
		}
	}
}

func newTestCertificate(t *testing.T) tls.Certificate {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "127.0.0.1"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
	}
	certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("create cert: %v", err)
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
	cert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		t.Fatalf("parse cert: %v", err)
	}
	return cert
}

func writeLine(conn net.Conn, line string) {
	_, _ = conn.Write([]byte(line + "\r\n"))
}

func readLine(conn net.Conn) (string, bool) {
	var b strings.Builder
	buf := make([]byte, 1)
	for {
		n, err := conn.Read(buf)
		if err != nil || n == 0 {
			return "", false
		}
		if buf[0] == '\n' {
			return strings.TrimRight(b.String(), "\r"), true
		}
		b.WriteByte(buf[0])
	}
}
