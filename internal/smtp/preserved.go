package smtp

import (
	"bytes"
	"crypto/tls"
	"encoding/base64"
	"fmt"
	htmlstd "html"
	"net/smtp"
	"strings"
	"time"

	"golang.org/x/net/html"
	"mail-processor/internal/models"
)

// PreservedMessageRequest describes an outgoing reply or forward where the
// user's editable note is sent above an original HTML/plain message context.
type PreservedMessageRequest struct {
	Kind                           models.PreservedMessageKind
	Mode                           models.PreservationMode
	From                           string
	To                             string
	CC                             string
	BCC                            string
	Subject                        string
	TopNoteMarkdown                string
	Original                       models.PreservedMessageOriginal
	ManualAttachments              []models.ComposeAttachment
	OmitOriginalAttachments        bool
	OmittedOriginalAttachmentNames []string
}

// SendPreserved sends a preserved reply or forward via SMTP.
func (c *Client) SendPreserved(req PreservedMessageRequest) error {
	host := c.cfg.SMTP.Host
	port := c.cfg.SMTP.Port
	if host == "" {
		return fmt.Errorf("smtp.host not configured in proton.yaml")
	}
	if port == 0 {
		port = 1025
	}
	fromHeader, fromEnvelope, err := normalizeMailbox("From", req.From)
	if err != nil {
		return err
	}
	rcpts, err := normalizeRecipientFields(req.To, req.CC, req.BCC)
	if err != nil {
		return err
	}
	req.From = fromHeader
	req.To = rcpts.ToHeader
	req.CC = rcpts.CCHeader
	rawMsg, err := BuildPreservedMIMEMessage(req)
	if err != nil {
		return err
	}
	addr := fmt.Sprintf("%s:%d", host, port)
	return c.sendRawMessage(addr, host, fromEnvelope, rcpts.AllEnvelope, rawMsg)
}

func (c *Client) sendRawMessage(addr, host, from string, rcpts []string, rawMsg string) error {
	tlsCfg := &tls.Config{
		InsecureSkipVerify: true,
		ServerName:         host,
	}
	conn, err := tls.Dial("tcp", addr, tlsCfg)
	if err != nil {
		return c.sendPlain(addr, from, rcpts, rawMsg)
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
	for _, rcpt := range rcpts {
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

// BuildPreservedMIMEMessage assembles a raw RFC 2822 message that preserves the
// original message HTML below a Markdown top note.
func BuildPreservedMIMEMessage(req PreservedMessageRequest) (string, error) {
	req.Mode = models.NormalizePreservationMode(req.Mode)
	if req.Kind == "" {
		req.Kind = models.PreservedMessageKindForward
	}

	outerBoundary := fmt.Sprintf("outer_%d", time.Now().UnixNano())
	relatedBoundary := fmt.Sprintf("related_%d", time.Now().UnixNano()+1)
	altBoundary := fmt.Sprintf("alt_%d", time.Now().UnixNano()+2)

	topHTML, topPlain := MarkdownToHTMLAndPlain(req.TopNoteMarkdown)
	if strings.TrimSpace(req.TopNoteMarkdown) == "" {
		topHTML, topPlain = "", ""
	}
	plainBody := buildPreservedPlainBody(req, topPlain)
	htmlBody := buildPreservedHTMLBody(req, topHTML)

	var msg strings.Builder
	msg.WriteString(fmt.Sprintf("From: %s\r\n", req.From))
	msg.WriteString(fmt.Sprintf("To: %s\r\n", req.To))
	if strings.TrimSpace(req.CC) != "" {
		msg.WriteString(fmt.Sprintf("Cc: %s\r\n", req.CC))
	}
	msg.WriteString(fmt.Sprintf("Subject: %s\r\n", req.Subject))
	if req.Kind == models.PreservedMessageKindReply {
		if msgID := firstNonEmpty(req.Original.MessageID, req.Original.InReplyTo); msgID != "" {
			msg.WriteString(fmt.Sprintf("In-Reply-To: %s\r\n", msgID))
			if strings.TrimSpace(req.Original.References) != "" {
				msg.WriteString(fmt.Sprintf("References: %s %s\r\n", strings.TrimSpace(req.Original.References), msgID))
			} else {
				msg.WriteString(fmt.Sprintf("References: %s\r\n", msgID))
			}
		}
	}
	msg.WriteString("MIME-Version: 1.0\r\n")
	msg.WriteString(fmt.Sprintf("Content-Type: multipart/mixed; boundary=%q\r\n", outerBoundary))
	msg.WriteString("\r\n")

	msg.WriteString(fmt.Sprintf("--%s\r\n", outerBoundary))
	msg.WriteString(fmt.Sprintf("Content-Type: multipart/related; boundary=%q\r\n", relatedBoundary))
	msg.WriteString("\r\n")

	msg.WriteString(fmt.Sprintf("--%s\r\n", relatedBoundary))
	msg.WriteString(fmt.Sprintf("Content-Type: multipart/alternative; boundary=%q\r\n", altBoundary))
	msg.WriteString("\r\n")

	msg.WriteString(fmt.Sprintf("--%s\r\n", altBoundary))
	msg.WriteString("Content-Type: text/plain; charset=utf-8\r\n")
	msg.WriteString("\r\n")
	msg.WriteString(plainBody)
	msg.WriteString("\r\n")

	msg.WriteString(fmt.Sprintf("--%s\r\n", altBoundary))
	msg.WriteString("Content-Type: text/html; charset=utf-8\r\n")
	msg.WriteString("\r\n")
	msg.WriteString(htmlBody)
	msg.WriteString("\r\n")
	msg.WriteString(fmt.Sprintf("--%s--\r\n", altBoundary))

	for i, img := range req.Original.InlineImages {
		if len(img.Data) == 0 || img.ContentID == "" {
			continue
		}
		filename := fmt.Sprintf("inline%03d%s", i+1, extFromMIME(img.MIMEType))
		writeBinaryPart(&msg, relatedBoundary, img.MIMEType, "inline", filename, img.ContentID, img.Data)
	}
	msg.WriteString(fmt.Sprintf("--%s--\r\n", relatedBoundary))

	if req.Kind == models.PreservedMessageKindForward && !req.OmitOriginalAttachments {
		omitted := stringSet(req.OmittedOriginalAttachmentNames)
		for _, att := range req.Original.Attachments {
			if att.Filename == "" || len(att.Data) == 0 || omitted[att.Filename] {
				continue
			}
			mimeType := att.MIMEType
			if mimeType == "" {
				mimeType = "application/octet-stream"
			}
			writeBinaryPart(&msg, outerBoundary, mimeType, "attachment", att.Filename, "", att.Data)
		}
	}
	for _, att := range req.ManualAttachments {
		if att.Filename == "" || len(att.Data) == 0 {
			continue
		}
		writeBinaryPart(&msg, outerBoundary, "application/octet-stream", "attachment", att.Filename, "", att.Data)
	}

	msg.WriteString(fmt.Sprintf("--%s--\r\n", outerBoundary))
	return msg.String(), nil
}

func buildPreservedPlainBody(req PreservedMessageRequest, topPlain string) string {
	var sb strings.Builder
	if strings.TrimSpace(topPlain) != "" {
		sb.WriteString(strings.TrimSpace(topPlain))
		sb.WriteString("\r\n\r\n")
	}
	sb.WriteString(originalPlainHeader(req))
	if strings.TrimSpace(req.Original.TextPlain) != "" {
		sb.WriteString(strings.TrimSpace(req.Original.TextPlain))
	} else {
		sb.WriteString(stripHTMLTags(req.Original.TextHTML))
	}
	return sb.String()
}

func buildPreservedHTMLBody(req PreservedMessageRequest, topHTML string) string {
	var sb strings.Builder
	sb.WriteString("<!doctype html><html><body>")
	if strings.TrimSpace(topHTML) != "" {
		sb.WriteString(`<div class="herald-top-note">`)
		sb.WriteString(topHTML)
		sb.WriteString("</div>")
	}
	sb.WriteString(`<div class="herald-preserved-header" style="margin-top:16px;color:#666;font-size:13px;">`)
	sb.WriteString(htmlstd.EscapeString(strings.TrimSpace(originalPlainHeader(req))))
	sb.WriteString("</div>")
	sb.WriteString(`<blockquote class="herald-preserved-message" style="margin:12px 0 0 0;padding-left:12px;border-left:2px solid #d0d7de;">`)
	preserved := PreparePreservedHTML(req.Original.TextHTML, req.Original.TextPlain, req.Mode)
	sb.WriteString(preserved)
	sb.WriteString("</blockquote></body></html>")
	return sb.String()
}

func originalPlainHeader(req PreservedMessageRequest) string {
	date := ""
	if !req.Original.Date.IsZero() {
		date = req.Original.Date.Format(time.RFC1123Z)
	}
	if req.Kind == models.PreservedMessageKindReply {
		if date != "" {
			return fmt.Sprintf("On %s, %s wrote:\r\n", date, req.Original.Sender)
		}
		return fmt.Sprintf("On a previous message, %s wrote:\r\n", req.Original.Sender)
	}
	var lines []string
	lines = append(lines, "---------- Forwarded message ----------")
	if req.Original.Sender != "" {
		lines = append(lines, "From: "+req.Original.Sender)
	}
	if date != "" {
		lines = append(lines, "Date: "+date)
	}
	if req.Original.Subject != "" {
		lines = append(lines, "Subject: "+req.Original.Subject)
	}
	return strings.Join(lines, "\r\n") + "\r\n\r\n"
}

// PreparePreservedHTML returns sanitized original HTML, or an escaped plain-text
// quote when the original message does not contain usable HTML.
func PreparePreservedHTML(htmlBody, plainBody string, mode models.PreservationMode) string {
	htmlBody = strings.TrimSpace(htmlBody)
	if htmlBody == "" {
		plainBody = strings.TrimSpace(plainBody)
		if plainBody == "" {
			return `<pre style="white-space:pre-wrap">(No original body)</pre>`
		}
		return `<pre style="white-space:pre-wrap">` + htmlstd.EscapeString(plainBody) + `</pre>`
	}
	doc, err := html.Parse(strings.NewReader(htmlBody))
	if err != nil {
		return `<pre style="white-space:pre-wrap">` + htmlstd.EscapeString(firstNonEmpty(plainBody, htmlBody)) + `</pre>`
	}
	sanitizeHTMLNode(doc, models.NormalizePreservationMode(mode))
	rendered := renderBodyContent(doc)
	if strings.TrimSpace(rendered) == "" {
		return `<pre style="white-space:pre-wrap">` + htmlstd.EscapeString(strings.TrimSpace(firstNonEmpty(plainBody, stripHTMLTags(htmlBody)))) + `</pre>`
	}
	return rendered
}

func sanitizeHTMLNode(n *html.Node, mode models.PreservationMode) {
	for c := n.FirstChild; c != nil; {
		next := c.NextSibling
		if shouldDropNode(c, mode) {
			n.RemoveChild(c)
			c = next
			continue
		}
		if c.Type == html.ElementNode {
			c.Attr = sanitizeHTMLAttrs(c.Data, c.Attr, mode)
		}
		sanitizeHTMLNode(c, mode)
		c = next
	}
}

func shouldDropNode(n *html.Node, mode models.PreservationMode) bool {
	if n.Type == html.CommentNode {
		return true
	}
	if n.Type != html.ElementNode {
		return false
	}
	tag := strings.ToLower(n.Data)
	switch tag {
	case "script", "iframe", "object", "embed":
		return true
	}
	if mode != models.PreservationModeFidelity {
		switch tag {
		case "form", "input", "button", "textarea", "select", "option", "meta", "base":
			return true
		}
	}
	return false
}

func sanitizeHTMLAttrs(tag string, attrs []html.Attribute, mode models.PreservationMode) []html.Attribute {
	tag = strings.ToLower(tag)
	out := attrs[:0]
	for _, attr := range attrs {
		key := strings.ToLower(attr.Key)
		val := strings.TrimSpace(attr.Val)
		if mode != models.PreservationModeFidelity && strings.HasPrefix(key, "on") {
			continue
		}
		if (key == "href" || key == "src" || key == "background") && strings.HasPrefix(strings.ToLower(val), "javascript:") {
			continue
		}
		if mode == models.PreservationModePrivacy {
			if key == "src" && tag == "img" && isRemoteURL(val) {
				continue
			}
			if key == "background" && isRemoteURL(val) {
				continue
			}
			if key == "style" && strings.Contains(strings.ToLower(val), "url(") {
				continue
			}
		}
		out = append(out, attr)
	}
	return out
}

func renderBodyContent(doc *html.Node) string {
	var body *html.Node
	var find func(*html.Node)
	find = func(n *html.Node) {
		if body != nil {
			return
		}
		if n.Type == html.ElementNode && strings.EqualFold(n.Data, "body") {
			body = n
			return
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			find(c)
		}
	}
	find(doc)
	if body == nil {
		body = doc
	}
	var buf bytes.Buffer
	for c := body.FirstChild; c != nil; c = c.NextSibling {
		_ = html.Render(&buf, c)
	}
	return strings.TrimSpace(buf.String())
}

func writeBinaryPart(sb *strings.Builder, boundary, mimeType, disposition, filename, cid string, data []byte) {
	if mimeType == "" {
		mimeType = "application/octet-stream"
	}
	sb.WriteString(fmt.Sprintf("--%s\r\n", boundary))
	sb.WriteString(fmt.Sprintf("Content-Type: %s\r\n", mimeType))
	sb.WriteString("Content-Transfer-Encoding: base64\r\n")
	if cid != "" {
		sb.WriteString(fmt.Sprintf("Content-ID: <%s>\r\n", cid))
	}
	sb.WriteString(fmt.Sprintf("Content-Disposition: %s; filename=%q\r\n", disposition, filename))
	sb.WriteString("\r\n")
	encoded := base64.StdEncoding.EncodeToString(data)
	for len(encoded) > 76 {
		sb.WriteString(encoded[:76] + "\r\n")
		encoded = encoded[76:]
	}
	if len(encoded) > 0 {
		sb.WriteString(encoded + "\r\n")
	}
}

func stringSet(values []string) map[string]bool {
	set := make(map[string]bool, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			set[value] = true
		}
	}
	return set
}

func isRemoteURL(s string) bool {
	lower := strings.ToLower(strings.TrimSpace(s))
	return strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://")
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
