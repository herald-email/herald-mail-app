package smtp

import (
	"crypto/tls"
	"encoding/base64"
	"fmt"
	"net/smtp"
	"strings"
	"time"

	"mail-processor/internal/config"
	"mail-processor/internal/models"
)

// Client sends email via SMTP (ProtonMail Bridge or compatible server)
type Client struct {
	cfg *config.Config
}

// New creates a new SMTP client
func New(cfg *config.Config) *Client {
	return &Client{cfg: cfg}
}

// Send sends an email. plainText is the fallback plain-text body; htmlBody is
// the HTML part for clients that support it. When htmlBody is non-empty the
// message is sent as multipart/alternative; otherwise plain text only.
func (c *Client) Send(from, to, subject, plainText, htmlBody string) error {
	host := c.cfg.SMTP.Host
	port := c.cfg.SMTP.Port
	if host == "" {
		return fmt.Errorf("smtp.host not configured in proton.yaml")
	}
	if port == 0 {
		port = 1025 // ProtonMail Bridge default SMTP port
	}

	addr := fmt.Sprintf("%s:%d", host, port)
	rawMsg := buildMIMEMessage(from, to, subject, plainText, htmlBody)

	// Connect with TLS (required for ProtonMail Bridge)
	tlsCfg := &tls.Config{
		InsecureSkipVerify: true,
		ServerName:         host,
	}

	conn, err := tls.Dial("tcp", addr, tlsCfg)
	if err != nil {
		// Fall back to plain SMTP with STARTTLS
		return c.sendPlain(addr, from, to, rawMsg)
	}
	defer conn.Close()

	client, err := smtp.NewClient(conn, host)
	if err != nil {
		return fmt.Errorf("smtp connect: %w", err)
	}
	defer client.Quit()

	auth := smtp.PlainAuth("", c.cfg.Credentials.Username, c.cfg.Credentials.Password, host)
	if err := client.Auth(auth); err != nil {
		return fmt.Errorf("smtp auth: %w", err)
	}

	if err := client.Mail(from); err != nil {
		return fmt.Errorf("smtp MAIL FROM: %w", err)
	}
	if err := client.Rcpt(to); err != nil {
		return fmt.Errorf("smtp RCPT TO: %w", err)
	}

	w, err := client.Data()
	if err != nil {
		return fmt.Errorf("smtp DATA: %w", err)
	}
	defer w.Close()
	_, err = w.Write([]byte(rawMsg))
	return err
}

// buildMIMEMessage assembles the raw RFC 2822 message. When htmlBody is
// non-empty it produces multipart/alternative with plain-text + HTML parts;
// otherwise it falls back to a simple text/plain message.
func buildMIMEMessage(from, to, subject, plainText, htmlBody string) string {
	var msg strings.Builder
	msg.WriteString(fmt.Sprintf("From: %s\r\n", from))
	msg.WriteString(fmt.Sprintf("To: %s\r\n", to))
	msg.WriteString(fmt.Sprintf("Subject: %s\r\n", subject))
	msg.WriteString("MIME-Version: 1.0\r\n")

	if htmlBody == "" {
		msg.WriteString("Content-Type: text/plain; charset=utf-8\r\n")
		msg.WriteString("\r\n")
		msg.WriteString(plainText)
		return msg.String()
	}

	boundary := fmt.Sprintf("boundary_%d", time.Now().UnixNano())
	msg.WriteString(fmt.Sprintf("Content-Type: multipart/alternative; boundary=%q\r\n", boundary))
	msg.WriteString("\r\n")

	msg.WriteString(fmt.Sprintf("--%s\r\n", boundary))
	msg.WriteString("Content-Type: text/plain; charset=utf-8\r\n")
	msg.WriteString("\r\n")
	msg.WriteString(plainText)
	msg.WriteString("\r\n")

	msg.WriteString(fmt.Sprintf("--%s\r\n", boundary))
	msg.WriteString("Content-Type: text/html; charset=utf-8\r\n")
	msg.WriteString("\r\n")
	msg.WriteString(htmlBody)
	msg.WriteString("\r\n")

	msg.WriteString(fmt.Sprintf("--%s--\r\n", boundary))
	return msg.String()
}

// SendWithAttachments sends an email with optional file attachments.
// It wraps multipart/alternative (plain+HTML) in a multipart/mixed envelope
// when attachments are present; otherwise delegates to Send.
func (c *Client) SendWithAttachments(from, to, subject, plainText, htmlBody string, attachments []models.ComposeAttachment) error {
	if len(attachments) == 0 {
		return c.Send(from, to, subject, plainText, htmlBody)
	}

	outerBoundary := fmt.Sprintf("outer_%d", time.Now().UnixNano())
	innerBoundary := fmt.Sprintf("inner_%d", time.Now().UnixNano()+1)

	var msg strings.Builder
	msg.WriteString(fmt.Sprintf("From: %s\r\n", from))
	msg.WriteString(fmt.Sprintf("To: %s\r\n", to))
	msg.WriteString(fmt.Sprintf("Subject: %s\r\n", subject))
	msg.WriteString("MIME-Version: 1.0\r\n")
	msg.WriteString(fmt.Sprintf("Content-Type: multipart/mixed; boundary=%q\r\n", outerBoundary))
	msg.WriteString("\r\n")

	// Inner multipart/alternative (text + HTML)
	msg.WriteString(fmt.Sprintf("--%s\r\n", outerBoundary))
	msg.WriteString(fmt.Sprintf("Content-Type: multipart/alternative; boundary=%q\r\n", innerBoundary))
	msg.WriteString("\r\n")

	msg.WriteString(fmt.Sprintf("--%s\r\n", innerBoundary))
	msg.WriteString("Content-Type: text/plain; charset=utf-8\r\n")
	msg.WriteString("\r\n")
	msg.WriteString(plainText)
	msg.WriteString("\r\n")

	if htmlBody != "" {
		msg.WriteString(fmt.Sprintf("--%s\r\n", innerBoundary))
		msg.WriteString("Content-Type: text/html; charset=utf-8\r\n")
		msg.WriteString("\r\n")
		msg.WriteString(htmlBody)
		msg.WriteString("\r\n")
	}
	msg.WriteString(fmt.Sprintf("--%s--\r\n", innerBoundary))

	// Attachment parts
	for _, att := range attachments {
		mimeType := att.Path // fallback
		if att.Data == nil {
			continue
		}
		mimeType = "application/octet-stream"
		msg.WriteString(fmt.Sprintf("--%s\r\n", outerBoundary))
		msg.WriteString(fmt.Sprintf("Content-Type: %s\r\n", mimeType))
		msg.WriteString(fmt.Sprintf("Content-Disposition: attachment; filename=%q\r\n", att.Filename))
		msg.WriteString("Content-Transfer-Encoding: base64\r\n")
		msg.WriteString("\r\n")
		encoded := base64.StdEncoding.EncodeToString(att.Data)
		// RFC 2045: base64 lines must be ≤76 chars
		for len(encoded) > 76 {
			msg.WriteString(encoded[:76] + "\r\n")
			encoded = encoded[76:]
		}
		if len(encoded) > 0 {
			msg.WriteString(encoded + "\r\n")
		}
	}
	msg.WriteString(fmt.Sprintf("--%s--\r\n", outerBoundary))

	rawMsg := msg.String()
	host := c.cfg.SMTP.Host
	port := c.cfg.SMTP.Port
	if port == 0 {
		port = 1025
	}
	addr := fmt.Sprintf("%s:%d", host, port)

	tlsCfg := &tls.Config{InsecureSkipVerify: true, ServerName: host}
	conn, err := tls.Dial("tcp", addr, tlsCfg)
	if err != nil {
		return c.sendPlain(addr, from, to, rawMsg)
	}
	defer conn.Close()

	client, err := smtp.NewClient(conn, host)
	if err != nil {
		return fmt.Errorf("smtp connect: %w", err)
	}
	defer client.Quit()

	auth := smtp.PlainAuth("", c.cfg.Credentials.Username, c.cfg.Credentials.Password, host)
	if err := client.Auth(auth); err != nil {
		return fmt.Errorf("smtp auth: %w", err)
	}
	if err := client.Mail(from); err != nil {
		return fmt.Errorf("smtp MAIL FROM: %w", err)
	}
	if err := client.Rcpt(to); err != nil {
		return fmt.Errorf("smtp RCPT TO: %w", err)
	}
	w, err := client.Data()
	if err != nil {
		return fmt.Errorf("smtp DATA: %w", err)
	}
	defer w.Close()
	_, err = w.Write([]byte(rawMsg))
	return err
}

func (c *Client) sendPlain(addr, from, to, rawMsg string) error {
	host := c.cfg.SMTP.Host
	auth := smtp.PlainAuth("", c.cfg.Credentials.Username, c.cfg.Credentials.Password, host)
	return smtp.SendMail(addr, auth, from, []string{to}, []byte(rawMsg))
}
