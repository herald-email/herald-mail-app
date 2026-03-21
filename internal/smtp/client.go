package smtp

import (
	"crypto/tls"
	"fmt"
	"net/smtp"
	"strings"

	"mail-processor/internal/config"
)

// Client sends email via SMTP (ProtonMail Bridge or compatible server)
type Client struct {
	cfg *config.Config
}

// New creates a new SMTP client
func New(cfg *config.Config) *Client {
	return &Client{cfg: cfg}
}

// Send sends an email message
func (c *Client) Send(from, to, subject, body string) error {
	host := c.cfg.SMTP.Host
	port := c.cfg.SMTP.Port
	if host == "" {
		return fmt.Errorf("smtp.host not configured in proton.yaml")
	}
	if port == 0 {
		port = 1025 // ProtonMail Bridge default SMTP port
	}

	addr := fmt.Sprintf("%s:%d", host, port)

	// Build raw message
	var msg strings.Builder
	msg.WriteString(fmt.Sprintf("From: %s\r\n", from))
	msg.WriteString(fmt.Sprintf("To: %s\r\n", to))
	msg.WriteString(fmt.Sprintf("Subject: %s\r\n", subject))
	msg.WriteString("MIME-Version: 1.0\r\n")
	msg.WriteString("Content-Type: text/plain; charset=utf-8\r\n")
	msg.WriteString("\r\n")
	msg.WriteString(body)

	// Connect with TLS (required for ProtonMail Bridge)
	tlsCfg := &tls.Config{
		InsecureSkipVerify: true,
		ServerName:         host,
	}

	conn, err := tls.Dial("tcp", addr, tlsCfg)
	if err != nil {
		// Fall back to plain SMTP with STARTTLS
		return c.sendPlain(addr, from, to, subject, body)
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
	_, err = w.Write([]byte(msg.String()))
	return err
}

func (c *Client) sendPlain(addr, from, to, subject, body string) error {
	host := c.cfg.SMTP.Host
	auth := smtp.PlainAuth("", c.cfg.Credentials.Username, c.cfg.Credentials.Password, host)

	var msg strings.Builder
	msg.WriteString(fmt.Sprintf("From: %s\r\n", from))
	msg.WriteString(fmt.Sprintf("To: %s\r\n", to))
	msg.WriteString(fmt.Sprintf("Subject: %s\r\n", subject))
	msg.WriteString("MIME-Version: 1.0\r\n")
	msg.WriteString("Content-Type: text/plain; charset=utf-8\r\n")
	msg.WriteString("\r\n")
	msg.WriteString(body)

	return smtp.SendMail(addr, auth, from, []string{to}, []byte(msg.String()))
}
