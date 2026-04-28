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

	fromHeader, fromEnvelope, err := normalizeMailbox("From", from)
	if err != nil {
		return err
	}
	rcpts, err := normalizeRecipientFields(to, "", "")
	if err != nil {
		return err
	}

	addr := fmt.Sprintf("%s:%d", host, port)
	rawMsg := buildMIMEMessage(fromHeader, rcpts.ToHeader, subject, plainText, htmlBody, "")

	// Connect with TLS (required for ProtonMail Bridge)
	tlsCfg := &tls.Config{
		InsecureSkipVerify: true,
		ServerName:         host,
	}

	conn, err := tls.Dial("tcp", addr, tlsCfg)
	if err != nil {
		// Fall back to plain SMTP with STARTTLS
		return c.sendPlain(addr, fromEnvelope, rcpts.AllEnvelope, rawMsg)
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

	if err := client.Mail(fromEnvelope); err != nil {
		return fmt.Errorf("smtp MAIL FROM: %w", err)
	}
	for _, rcpt := range rcpts.AllEnvelope {
		if err := client.Rcpt(rcpt); err != nil {
			return fmt.Errorf("smtp RCPT TO %s: %w", rcpt, err)
		}
	}

	w, err := client.Data()
	if err != nil {
		return fmt.Errorf("smtp DATA: %w", err)
	}
	defer w.Close()
	_, err = w.Write([]byte(rawMsg))
	return err
}

// buildMIMEMessage assembles the raw RFC 2822 message. cc is written as a
// Cc: header when non-empty. BCC recipients must be handled by the caller
// via RCPT TO commands — they must NOT appear in message headers per RFC 5321.
func buildMIMEMessage(from, to, subject, plainText, htmlBody, cc string) string {
	var msg strings.Builder
	msg.WriteString(fmt.Sprintf("From: %s\r\n", from))
	msg.WriteString(fmt.Sprintf("To: %s\r\n", to))
	if cc != "" {
		msg.WriteString(fmt.Sprintf("Cc: %s\r\n", cc))
	}
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
	fromHeader, fromEnvelope, err := normalizeMailbox("From", from)
	if err != nil {
		return err
	}
	rcpts, err := normalizeRecipientFields(to, "", "")
	if err != nil {
		return err
	}

	var msg strings.Builder
	msg.WriteString(fmt.Sprintf("From: %s\r\n", fromHeader))
	msg.WriteString(fmt.Sprintf("To: %s\r\n", rcpts.ToHeader))
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
		return c.sendPlain(addr, fromEnvelope, rcpts.AllEnvelope, rawMsg)
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
	if err := client.Mail(fromEnvelope); err != nil {
		return fmt.Errorf("smtp MAIL FROM: %w", err)
	}
	for _, rcpt := range rcpts.AllEnvelope {
		if err := client.Rcpt(rcpt); err != nil {
			return fmt.Errorf("smtp RCPT TO %s: %w", rcpt, err)
		}
	}
	w, err := client.Data()
	if err != nil {
		return fmt.Errorf("smtp DATA: %w", err)
	}
	defer w.Close()
	_, err = w.Write([]byte(rawMsg))
	return err
}

// SendWithInlineImages is like SendWithAttachments but also embeds inline
// images via multipart/related. When inlines is empty it behaves exactly like
// SendWithAttachments.
//
// MIME structure with inline images:
//
//	multipart/mixed
//	  └─ multipart/related
//	       ├─ multipart/alternative
//	       │    ├─ text/plain
//	       │    └─ text/html
//	       └─ image/png (Content-ID: <img001@herald>; Content-Disposition: inline)
//	  └─ attachment parts
func (c *Client) SendWithInlineImages(from, to, subject, plainText, htmlBody, cc, bcc string, attachments []models.ComposeAttachment, inlines []InlineImage) error {
	if len(inlines) == 0 && cc == "" && bcc == "" {
		return c.SendWithAttachments(from, to, subject, plainText, htmlBody, attachments)
	}

	outerBoundary := fmt.Sprintf("outer_%d", time.Now().UnixNano())
	relatedBoundary := fmt.Sprintf("related_%d", time.Now().UnixNano()+1)
	innerBoundary := fmt.Sprintf("inner_%d", time.Now().UnixNano()+2)
	fromHeader, fromEnvelope, err := normalizeMailbox("From", from)
	if err != nil {
		return err
	}
	rcpts, err := normalizeRecipientFields(to, cc, bcc)
	if err != nil {
		return err
	}

	var msg strings.Builder
	msg.WriteString(fmt.Sprintf("From: %s\r\n", fromHeader))
	msg.WriteString(fmt.Sprintf("To: %s\r\n", rcpts.ToHeader))
	if rcpts.CCHeader != "" {
		msg.WriteString(fmt.Sprintf("Cc: %s\r\n", rcpts.CCHeader))
	}
	msg.WriteString(fmt.Sprintf("Subject: %s\r\n", subject))
	msg.WriteString("MIME-Version: 1.0\r\n")
	msg.WriteString(fmt.Sprintf("Content-Type: multipart/mixed; boundary=%q\r\n", outerBoundary))
	msg.WriteString("\r\n")

	// multipart/related wraps alternative + inline images
	msg.WriteString(fmt.Sprintf("--%s\r\n", outerBoundary))
	msg.WriteString(fmt.Sprintf("Content-Type: multipart/related; boundary=%q\r\n", relatedBoundary))
	msg.WriteString("\r\n")

	// multipart/alternative (plain + HTML)
	msg.WriteString(fmt.Sprintf("--%s\r\n", relatedBoundary))
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

	// Inline image parts
	for i, img := range inlines {
		msg.WriteString(fmt.Sprintf("--%s\r\n", relatedBoundary))
		msg.WriteString(fmt.Sprintf("Content-Type: %s\r\n", img.MIMEType))
		msg.WriteString("Content-Transfer-Encoding: base64\r\n")
		msg.WriteString(fmt.Sprintf("Content-ID: <%s>\r\n", img.ContentID))
		ext := extFromMIME(img.MIMEType)
		msg.WriteString(fmt.Sprintf("Content-Disposition: inline; filename=%q\r\n", fmt.Sprintf("img%03d%s", i+1, ext)))
		msg.WriteString("\r\n")
		encoded := base64.StdEncoding.EncodeToString(img.Data)
		for len(encoded) > 76 {
			msg.WriteString(encoded[:76] + "\r\n")
			encoded = encoded[76:]
		}
		if len(encoded) > 0 {
			msg.WriteString(encoded + "\r\n")
		}
	}
	msg.WriteString(fmt.Sprintf("--%s--\r\n", relatedBoundary))

	// File attachment parts
	for _, att := range attachments {
		if att.Data == nil {
			continue
		}
		msg.WriteString(fmt.Sprintf("--%s\r\n", outerBoundary))
		msg.WriteString("Content-Type: application/octet-stream\r\n")
		msg.WriteString(fmt.Sprintf("Content-Disposition: attachment; filename=%q\r\n", att.Filename))
		msg.WriteString("Content-Transfer-Encoding: base64\r\n")
		msg.WriteString("\r\n")
		encoded := base64.StdEncoding.EncodeToString(att.Data)
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
		return c.sendPlain(addr, fromEnvelope, rcpts.AllEnvelope, rawMsg)
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
	if err := client.Mail(fromEnvelope); err != nil {
		return fmt.Errorf("smtp MAIL FROM: %w", err)
	}
	for _, rcpt := range rcpts.AllEnvelope {
		if err := client.Rcpt(rcpt); err != nil {
			return fmt.Errorf("smtp RCPT TO %s: %w", rcpt, err)
		}
	}
	w, err := client.Data()
	if err != nil {
		return fmt.Errorf("smtp DATA: %w", err)
	}
	defer w.Close()
	_, err = w.Write([]byte(rawMsg))
	return err
}

// buildReplyMIMEMessage assembles a raw RFC 2822 reply message with In-Reply-To
// and References threading headers. When htmlBody is non-empty the message is
// sent as multipart/alternative; otherwise plain text only.
func buildReplyMIMEMessage(from, to, subject, plainText, htmlBody, inReplyTo, references string) string {
	var msg strings.Builder
	msg.WriteString(fmt.Sprintf("From: %s\r\n", from))
	msg.WriteString(fmt.Sprintf("To: %s\r\n", to))
	msg.WriteString(fmt.Sprintf("Subject: %s\r\n", subject))
	msg.WriteString(fmt.Sprintf("In-Reply-To: %s\r\n", inReplyTo))
	if references != "" {
		msg.WriteString(fmt.Sprintf("References: %s %s\r\n", references, inReplyTo))
	} else {
		msg.WriteString(fmt.Sprintf("References: %s\r\n", inReplyTo))
	}
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

// SendReply sends an email that is a reply to an existing message.
// inReplyTo is the Message-ID of the original (e.g. "<abc@domain>").
// references is the full References chain from the original (may be empty string).
func (c *Client) SendReply(from, to, subject, plainText, htmlBody, inReplyTo, references string) error {
	host := c.cfg.SMTP.Host
	port := c.cfg.SMTP.Port
	if host == "" {
		return fmt.Errorf("smtp.host not configured in proton.yaml")
	}
	if port == 0 {
		port = 1025
	}

	addr := fmt.Sprintf("%s:%d", host, port)
	fromHeader, fromEnvelope, err := normalizeMailbox("From", from)
	if err != nil {
		return err
	}
	rcpts, err := normalizeRecipientFields(to, "", "")
	if err != nil {
		return err
	}
	rawMsg := buildReplyMIMEMessage(fromHeader, rcpts.ToHeader, subject, plainText, htmlBody, inReplyTo, references)

	tlsCfg := &tls.Config{
		InsecureSkipVerify: true,
		ServerName:         host,
	}

	conn, err := tls.Dial("tcp", addr, tlsCfg)
	if err != nil {
		return c.sendPlain(addr, fromEnvelope, rcpts.AllEnvelope, rawMsg)
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
	if err := client.Mail(fromEnvelope); err != nil {
		return fmt.Errorf("smtp MAIL FROM: %w", err)
	}
	for _, rcpt := range rcpts.AllEnvelope {
		if err := client.Rcpt(rcpt); err != nil {
			return fmt.Errorf("smtp RCPT TO %s: %w", rcpt, err)
		}
	}
	w, err := client.Data()
	if err != nil {
		return fmt.Errorf("smtp DATA: %w", err)
	}
	defer w.Close()
	_, err = w.Write([]byte(rawMsg))
	return err
}

// extFromMIME returns a file extension for common image MIME types.
func extFromMIME(mimeType string) string {
	switch mimeType {
	case "image/png":
		return ".png"
	case "image/jpeg":
		return ".jpg"
	case "image/gif":
		return ".gif"
	case "image/webp":
		return ".webp"
	default:
		return ".bin"
	}
}

func (c *Client) sendPlain(addr, from string, rcpts []string, rawMsg string) error {
	host := c.cfg.SMTP.Host
	auth := smtp.PlainAuth("", c.cfg.Credentials.Username, c.cfg.Credentials.Password, host)
	return smtp.SendMail(addr, auth, from, rcpts, []byte(rawMsg))
}

// parseAddrs returns bare envelope addresses from a comma-separated header
// address list. It is kept for older tests and helpers; send paths use
// normalizeRecipientFields directly so validation errors can be surfaced.
func parseAddrs(s string) []string {
	_, envelope, err := normalizeAddressList("recipient", s, false)
	if err != nil {
		return nil
	}
	return envelope
}
