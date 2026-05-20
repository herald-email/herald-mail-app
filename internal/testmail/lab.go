// Package testmail provides deterministic IMAP/SMTP servers for integration
// tests that need realistic mail transport without touching private accounts.
package testmail

import (
	"bufio"
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/mail"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	goImap "github.com/emersion/go-imap"
	"github.com/emersion/go-imap/backend"
	imapmemory "github.com/emersion/go-imap/backend/memory"
	"github.com/emersion/go-imap/server"
	"github.com/herald-email/herald-mail-app/internal/config"
)

const (
	DefaultAliceAddress = "alice@herald.test"
	DefaultBobAddress   = "bob@herald.test"
	DefaultPassword     = "password"
)

// Option customizes a virtual mail lab.
type Option func(*options)

type options struct {
	accounts []accountSpec
	seeds    []seedSpec
	timeout  time.Duration
}

type accountSpec struct {
	address  string
	password string
}

type seedSpec struct {
	address string
	folder  string
	raw     []byte
	flags   []string
}

// WithAccount adds an account to the lab. If no accounts are specified, the
// lab starts alice@herald.test and bob@herald.test.
func WithAccount(address, password string) Option {
	return func(o *options) {
		o.accounts = append(o.accounts, accountSpec{address: address, password: password})
	}
}

// WithEML seeds a raw RFC 5322 message into an account folder after startup.
func WithEML(address, folder string, raw []byte, flags ...string) Option {
	return func(o *options) {
		o.seeds = append(o.seeds, seedSpec{
			address: address,
			folder:  folder,
			raw:     append([]byte(nil), raw...),
			flags:   append([]string(nil), flags...),
		})
	}
}

// WithWaitTimeout changes how long WaitForSubject polls before failing.
func WithWaitTimeout(timeout time.Duration) Option {
	return func(o *options) {
		o.timeout = timeout
	}
}

// Lab owns a set of virtual IMAP accounts and one SMTP router that can deliver
// messages between them.
type Lab struct {
	t          testing.TB
	accounts   map[string]*Account
	smtpAddr   string
	smtpLn     net.Listener
	tlsCert    tls.Certificate
	timeout    time.Duration
	capturedMu sync.Mutex
	captured   []CapturedMessage
}

// Account is one virtual mailbox account.
type Account struct {
	Address  string
	Password string

	t        testing.TB
	lab      *Lab
	imapAddr string
	mem      *imapmemory.Backend
	user     backend.User
	server   *server.Server
	listener net.Listener
	mu       sync.Mutex
}

// MessageRef identifies a message inside the virtual lab.
type MessageRef struct {
	Account   string
	Folder    string
	MessageID string
	UID       uint32
}

// CapturedMessage records one SMTP DATA transaction.
type CapturedMessage struct {
	From string
	To   []string
	Data []byte
}

// Start creates a virtual mail lab with per-account IMAP servers and one SMTP
// router. The caller's test automatically cleans it up.
func Start(t testing.TB, opts ...Option) *Lab {
	t.Helper()

	cfg := options{timeout: 2 * time.Second}
	for _, opt := range opts {
		opt(&cfg)
	}
	if len(cfg.accounts) == 0 {
		cfg.accounts = []accountSpec{
			{address: DefaultAliceAddress, password: DefaultPassword},
			{address: DefaultBobAddress, password: DefaultPassword},
		}
	}

	cert, err := generateSelfSignedCert()
	if err != nil {
		t.Fatalf("testmail: generate TLS cert: %v", err)
	}
	lab := &Lab{
		t:        t,
		accounts: make(map[string]*Account),
		tlsCert:  cert,
		timeout:  cfg.timeout,
	}

	for _, spec := range cfg.accounts {
		account := lab.startAccount(t, spec.address, spec.password)
		lab.accounts[normalizeAddress(account.Address)] = account
	}
	lab.startSMTP(t)

	for _, seed := range cfg.seeds {
		account := lab.Account(seed.address)
		if account == nil {
			t.Fatalf("testmail: seed account %q not found", seed.address)
		}
		account.AppendEML(seed.folder, seed.raw, seed.flags...)
	}

	t.Cleanup(func() { lab.Close() })
	return lab
}

// Account returns a virtual account by email address.
func (l *Lab) Account(address string) *Account {
	if l == nil {
		return nil
	}
	return l.accounts[normalizeAddress(address)]
}

// CapturedSMTP returns a deep copy of all SMTP transactions accepted by the lab.
func (l *Lab) CapturedSMTP() []CapturedMessage {
	l.capturedMu.Lock()
	defer l.capturedMu.Unlock()

	out := make([]CapturedMessage, len(l.captured))
	for i, msg := range l.captured {
		out[i] = CapturedMessage{
			From: msg.From,
			To:   append([]string(nil), msg.To...),
			Data: append([]byte(nil), msg.Data...),
		}
	}
	return out
}

// WaitForSubject waits until a message with subject exists in the given account
// folder, then returns its reference. It fails the owning test on timeout.
func (l *Lab) WaitForSubject(address, folder, subject string) MessageRef {
	l.t.Helper()
	account := l.Account(address)
	if account == nil {
		l.t.Fatalf("testmail: account %q not found", address)
	}
	deadline := time.Now().Add(l.timeout)
	for time.Now().Before(deadline) {
		if ref, ok := account.findSubject(folder, subject); ok {
			return ref
		}
		time.Sleep(10 * time.Millisecond)
	}
	l.t.Fatalf("testmail: subject %q not found in %s/%s", subject, address, folder)
	return MessageRef{}
}

// Close shuts down every server owned by the lab.
func (l *Lab) Close() {
	if l == nil {
		return
	}
	if l.smtpLn != nil {
		_ = l.smtpLn.Close()
	}
	for _, account := range l.accounts {
		if account.server != nil {
			_ = account.server.Close()
		}
		if account.listener != nil {
			_ = account.listener.Close()
		}
	}
}

// Config returns a Herald config pointed at this account's IMAP server and the
// lab SMTP router. cachePath should usually be inside t.TempDir().
func (a *Account) Config(cachePath string) *config.Config {
	host, port := splitHostPort(a.t, a.imapAddr)
	smtpHost, smtpPort := splitHostPort(a.t, a.lab.smtpAddr)
	cfg := &config.Config{}
	cfg.Vendor = "protonmail"
	cfg.Credentials.Username = a.Address
	cfg.Credentials.Password = a.Password
	cfg.Server.Host = host
	cfg.Server.Port = port
	cfg.SMTP.Host = smtpHost
	cfg.SMTP.Port = smtpPort
	cfg.Cache.DatabasePath = cachePath
	cfg.Cache.StoragePolicy = config.CacheStoragePolicyNoAttachments
	cfg.AI.Provider = "disabled"
	return cfg
}

// AppendEML appends a raw RFC 5322 message into a folder and returns its ref.
func (a *Account) AppendEML(folder string, raw []byte, flags ...string) MessageRef {
	a.t.Helper()
	if strings.TrimSpace(folder) == "" {
		folder = "INBOX"
	}
	a.mu.Lock()
	defer a.mu.Unlock()

	mbox := a.mustMailbox(folder)
	msgDate := messageDate(raw)
	if err := mbox.CreateMessage(flags, msgDate, newBytesLiteral(raw)); err != nil {
		a.t.Fatalf("testmail: append %s/%s: %v", a.Address, folder, err)
	}
	last := mbox.Messages[len(mbox.Messages)-1]
	return MessageRef{
		Account:   a.Address,
		Folder:    folder,
		MessageID: messageID(raw, last.Uid),
		UID:       last.Uid,
	}
}

func (l *Lab) startAccount(t testing.TB, address, password string) *Account {
	t.Helper()
	address = strings.TrimSpace(address)
	if address == "" {
		t.Fatal("testmail: account address cannot be empty")
	}
	if password == "" {
		password = DefaultPassword
	}

	mem := imapmemory.New()
	user, err := mem.Login(nil, "username", "password")
	if err != nil {
		t.Fatalf("testmail: default memory login: %v", err)
	}
	clearDefaultInbox(t, user)
	for _, folder := range []string{"Sent", "Drafts", "Archive", "Trash"} {
		if err := user.CreateMailbox(folder); err != nil {
			t.Fatalf("testmail: create %s mailbox for %s: %v", folder, address, err)
		}
	}

	account := &Account{
		Address:  address,
		Password: password,
		t:        t,
		lab:      l,
		mem:      mem,
		user:     user,
	}
	wrapped := &credentialWrapper{inner: mem, username: address, password: password}
	srv := server.New(wrapped)
	srv.AllowInsecureAuth = true
	srv.TLSConfig = &tls.Config{Certificates: []tls.Certificate{l.tlsCert}}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("testmail: listen IMAP for %s: %v", address, err)
	}
	account.server = srv
	account.listener = ln
	account.imapAddr = ln.Addr().String()
	go func() { _ = srv.Serve(ln) }()
	return account
}

func (l *Lab) startSMTP(t testing.TB) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("testmail: listen SMTP: %v", err)
	}
	l.smtpAddr = ln.Addr().String()
	l.smtpLn = tls.NewListener(ln, &tls.Config{Certificates: []tls.Certificate{l.tlsCert}})
	go l.serveSMTP()
}

func (l *Lab) serveSMTP() {
	for {
		conn, err := l.smtpLn.Accept()
		if err != nil {
			return
		}
		go l.handleSMTPConn(conn)
	}
}

func (l *Lab) handleSMTPConn(conn net.Conn) {
	defer conn.Close()
	reader := bufio.NewReader(conn)
	writer := bufio.NewWriter(conn)
	writeSMTPLine(writer, "220 testmail.herald.test ESMTP")

	var authUser string
	var from string
	var rcpts []string
	for {
		line, ok := readSMTPLine(reader)
		if !ok {
			return
		}
		upper := strings.ToUpper(line)
		switch {
		case strings.HasPrefix(upper, "EHLO") || strings.HasPrefix(upper, "HELO"):
			writeSMTPLine(writer, "250-testmail.herald.test")
			writeSMTPLine(writer, "250 AUTH PLAIN")
		case strings.HasPrefix(upper, "AUTH PLAIN"):
			user, pass, err := parsePlainAuth(line, reader, writer)
			if err != nil || !l.validCredentials(user, pass) {
				writeSMTPLine(writer, "535 5.7.8 Authentication credentials invalid")
				return
			}
			authUser = user
			writeSMTPLine(writer, "235 2.7.0 Authentication successful")
		case strings.HasPrefix(upper, "MAIL FROM:"):
			from = extractSMTPPath(line)
			if from == "" {
				from = authUser
			}
			writeSMTPLine(writer, "250 2.1.0 OK")
		case strings.HasPrefix(upper, "RCPT TO:"):
			rcpt := extractSMTPPath(line)
			if rcpt != "" {
				rcpts = append(rcpts, rcpt)
			}
			writeSMTPLine(writer, "250 2.1.5 OK")
		case strings.HasPrefix(upper, "DATA"):
			writeSMTPLine(writer, "354 End data with <CR><LF>.<CR><LF>")
			raw, err := readSMTPData(reader)
			if err != nil {
				writeSMTPLine(writer, "451 4.3.0 Failed reading message")
				return
			}
			l.routeMessage(from, rcpts, raw)
			writeSMTPLine(writer, "250 2.0.0 queued")
			rcpts = nil
		case strings.HasPrefix(upper, "RSET"):
			from = ""
			rcpts = nil
			writeSMTPLine(writer, "250 2.0.0 OK")
		case strings.HasPrefix(upper, "NOOP"):
			writeSMTPLine(writer, "250 2.0.0 OK")
		case strings.HasPrefix(upper, "QUIT"):
			writeSMTPLine(writer, "221 2.0.0 Bye")
			return
		default:
			writeSMTPLine(writer, "250 2.0.0 OK")
		}
	}
}

func (l *Lab) routeMessage(from string, rcpts []string, raw []byte) {
	from = normalizeAddress(from)
	normalizedRcpts := make([]string, 0, len(rcpts))
	for _, rcpt := range rcpts {
		normalizedRcpts = append(normalizedRcpts, normalizeAddress(rcpt))
	}

	l.capturedMu.Lock()
	l.captured = append(l.captured, CapturedMessage{
		From: from,
		To:   append([]string(nil), normalizedRcpts...),
		Data: append([]byte(nil), raw...),
	})
	l.capturedMu.Unlock()

	if sender := l.Account(from); sender != nil {
		sender.AppendEML("Sent", raw, goImap.SeenFlag)
	}
	for _, rcpt := range normalizedRcpts {
		if account := l.Account(rcpt); account != nil {
			account.AppendEML("INBOX", raw)
		}
	}
}

func (l *Lab) validCredentials(username, password string) bool {
	account := l.Account(username)
	return account != nil && account.Password == password
}

func (a *Account) findSubject(folder, subject string) (MessageRef, bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	mbox, ok := a.mailbox(folder)
	if !ok {
		return MessageRef{}, false
	}
	for _, msg := range mbox.Messages {
		gotSubject, gotMessageID := headersFromRaw(msg.Body)
		if gotSubject == subject {
			if gotMessageID == "" {
				gotMessageID = fmt.Sprintf("uid-%d", msg.Uid)
			}
			return MessageRef{Account: a.Address, Folder: folder, MessageID: gotMessageID, UID: msg.Uid}, true
		}
	}
	return MessageRef{}, false
}

func (a *Account) mustMailbox(folder string) *imapmemory.Mailbox {
	if mbox, ok := a.mailbox(folder); ok {
		return mbox
	}
	if err := a.user.CreateMailbox(folder); err != nil {
		a.t.Fatalf("testmail: create mailbox %s/%s: %v", a.Address, folder, err)
	}
	mbox, ok := a.mailbox(folder)
	if !ok {
		a.t.Fatalf("testmail: mailbox %s/%s was not created", a.Address, folder)
	}
	return mbox
}

func (a *Account) mailbox(folder string) (*imapmemory.Mailbox, bool) {
	mbox, err := a.user.GetMailbox(folder)
	if err != nil {
		return nil, false
	}
	memBox, ok := mbox.(*imapmemory.Mailbox)
	return memBox, ok
}

func clearDefaultInbox(t testing.TB, user backend.User) {
	t.Helper()
	mbox, err := user.GetMailbox("INBOX")
	if err != nil {
		t.Fatalf("testmail: get INBOX: %v", err)
	}
	memBox, ok := mbox.(*imapmemory.Mailbox)
	if !ok {
		t.Fatalf("testmail: unexpected INBOX type %T", mbox)
	}
	memBox.Messages = nil
}

type bytesLiteral struct {
	*bytes.Reader
}

func newBytesLiteral(b []byte) *bytesLiteral {
	return &bytesLiteral{bytes.NewReader(b)}
}

func (l *bytesLiteral) Len() int {
	return l.Reader.Len()
}

type credentialWrapper struct {
	inner    *imapmemory.Backend
	username string
	password string
}

func (b *credentialWrapper) Login(connInfo *goImap.ConnInfo, username, password string) (backend.User, error) {
	if username != b.username || password != b.password {
		return nil, backend.ErrInvalidCredentials
	}
	return b.inner.Login(connInfo, "username", "password")
}

func splitHostPort(t testing.TB, addr string) (string, int) {
	t.Helper()
	host, portText, err := net.SplitHostPort(addr)
	if err != nil {
		t.Fatalf("testmail: split host/port %q: %v", addr, err)
	}
	port, err := strconv.Atoi(portText)
	if err != nil {
		t.Fatalf("testmail: parse port %q: %v", portText, err)
	}
	return host, port
}

func writeSMTPLine(w *bufio.Writer, line string) {
	_, _ = w.WriteString(line + "\r\n")
	_ = w.Flush()
}

func readSMTPLine(r *bufio.Reader) (string, bool) {
	line, err := r.ReadString('\n')
	if err != nil {
		return "", false
	}
	return strings.TrimRight(line, "\r\n"), true
}

func parsePlainAuth(line string, r *bufio.Reader, w *bufio.Writer) (username, password string, err error) {
	payload := strings.TrimSpace(strings.TrimPrefix(line, line[:len("AUTH PLAIN")]))
	if payload == "" {
		writeSMTPLine(w, "334 ")
		next, ok := readSMTPLine(r)
		if !ok {
			return "", "", io.ErrUnexpectedEOF
		}
		payload = strings.TrimSpace(next)
	}
	decoded, err := base64.StdEncoding.DecodeString(payload)
	if err != nil {
		return "", "", err
	}
	parts := strings.Split(string(decoded), "\x00")
	if len(parts) < 3 {
		return "", "", fmt.Errorf("invalid AUTH PLAIN payload")
	}
	return normalizeAddress(parts[1]), parts[2], nil
}

func extractSMTPPath(line string) string {
	start := strings.IndexByte(line, '<')
	end := strings.LastIndexByte(line, '>')
	if start >= 0 && end > start {
		return normalizeAddress(line[start+1 : end])
	}
	_, rest, ok := strings.Cut(line, ":")
	if !ok {
		return ""
	}
	return normalizeAddress(strings.Fields(rest)[0])
}

func readSMTPData(r *bufio.Reader) ([]byte, error) {
	var data bytes.Buffer
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return nil, err
		}
		trimmed := strings.TrimRight(line, "\r\n")
		if trimmed == "." {
			return data.Bytes(), nil
		}
		if strings.HasPrefix(trimmed, "..") {
			trimmed = trimmed[1:]
		}
		data.WriteString(trimmed)
		data.WriteString("\r\n")
	}
}

func normalizeAddress(address string) string {
	return strings.ToLower(strings.TrimSpace(address))
}

func messageDate(raw []byte) time.Time {
	msg, err := mail.ReadMessage(bytes.NewReader(raw))
	if err != nil {
		return time.Now()
	}
	if date, err := msg.Header.Date(); err == nil {
		return date
	}
	return time.Now()
}

func messageID(raw []byte, uid uint32) string {
	_, id := headersFromRaw(raw)
	if id == "" {
		return fmt.Sprintf("uid-%d", uid)
	}
	return id
}

func headersFromRaw(raw []byte) (subject, messageID string) {
	msg, err := mail.ReadMessage(bytes.NewReader(raw))
	if err != nil {
		return "", ""
	}
	return msg.Header.Get("Subject"), msg.Header.Get("Message-ID")
}

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
