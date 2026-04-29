package imap

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"net/mail"
	"net/textproto"
	"strings"

	"github.com/emersion/go-imap"
	"mail-processor/internal/logger"
	"mail-processor/internal/models"
	emailrender "mail-processor/internal/render"
)

// FetchEmailBody retrieves the full MIME body of the email identified by uid
// in the given folder. It returns parsed plain text and any inline images.
// On connection errors (broken pipe, EOF), it reconnects once and retries.
func (c *Client) FetchEmailBody(uid uint32, folder string) (*models.EmailBody, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	body, err := c.fetchEmailBodyLocked(uid, folder)
	if err != nil && isConnectionError(err) {
		logger.Info("FetchEmailBody: connection error, reconnecting: %v", err)
		if reconErr := c.Reconnect(); reconErr != nil {
			return nil, fmt.Errorf("reconnect failed: %w (original: %v)", reconErr, err)
		}
		body, err = c.fetchEmailBodyLocked(uid, folder)
	}
	return body, err
}

// fetchEmailBodyLocked performs the actual IMAP fetch. Must be called with c.mu held.
func (c *Client) fetchEmailBodyLocked(uid uint32, folder string) (*models.EmailBody, error) {
	if c.client == nil {
		return nil, fmt.Errorf("not connected")
	}

	// Select folder read-only; subsequent write ops will re-select read-write.
	if _, err := c.client.Select(folder, true); err != nil {
		return nil, fmt.Errorf("select folder %s: %w", folder, err)
	}

	seqset := new(imap.SeqSet)
	seqset.AddNum(uid)

	section := &imap.BodySectionName{} // BODY[] — entire RFC 2822 message
	items := []imap.FetchItem{section.FetchItem()}

	messages := make(chan *imap.Message, 1)
	done := make(chan error, 1)
	go func() {
		done <- c.client.UidFetch(seqset, items, messages)
	}()

	msg := <-messages
	if err := <-done; err != nil {
		return nil, fmt.Errorf("uid fetch: %w", err)
	}
	if msg == nil {
		return nil, fmt.Errorf("message uid %d not found in %s", uid, folder)
	}

	bodyReader := msg.GetBody(section)
	if bodyReader == nil {
		return nil, fmt.Errorf("empty body for uid %d", uid)
	}

	raw, err := io.ReadAll(bodyReader)
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}

	return parseMIMEBody(raw)
}

func parseMIMEBody(raw []byte) (*models.EmailBody, error) {
	mailMsg, err := mail.ReadMessage(bytes.NewReader(raw))
	if err != nil {
		// Fallback: treat the whole thing as plain text
		return &models.EmailBody{TextPlain: string(raw)}, nil
	}
	result := &models.EmailBody{}
	result.From = mailMsg.Header.Get("From")
	result.To = mailMsg.Header.Get("To")
	result.CC = mailMsg.Header.Get("Cc")
	result.BCC = mailMsg.Header.Get("Bcc")
	result.Subject = mailMsg.Header.Get("Subject")
	result.MessageID = mailMsg.Header.Get("Message-Id")
	result.InReplyTo = mailMsg.Header.Get("In-Reply-To")
	result.References = mailMsg.Header.Get("References")
	result.ListUnsubscribe = mailMsg.Header.Get("List-Unsubscribe")
	result.ListUnsubscribePost = mailMsg.Header.Get("List-Unsubscribe-Post")
	parseMIMEPart(textproto.MIMEHeader(mailMsg.Header), mailMsg.Body, result, "")
	// If there is no plain-text part, convert HTML to markdown
	if result.TextPlain == "" && result.TextHTML != "" {
		result.TextPlain = emailrender.HTMLToMarkdown(result.TextHTML)
		result.IsFromHTML = true
	}
	return result, nil
}

// htmlToMarkdown is retained for package-local tests and delegates to the
// shared preview converter used by the TUI.
func htmlToMarkdown(htmlStr string) string {
	return emailrender.HTMLToMarkdown(htmlStr)
}

func parseMIMEPart(header textproto.MIMEHeader, body io.Reader, result *models.EmailBody, partPath string) {
	ct := header.Get("Content-Type")
	if ct == "" {
		ct = "text/plain"
	}
	mediaType, params, err := mime.ParseMediaType(ct)
	if err != nil {
		return
	}

	if strings.HasPrefix(mediaType, "multipart/") {
		mr := multipart.NewReader(body, params["boundary"])
		partIdx := 1
		for {
			part, err := mr.NextPart()
			if err != nil {
				break
			}
			childPath := fmt.Sprintf("%d", partIdx)
			if partPath != "" {
				childPath = partPath + "." + childPath
			}
			parseMIMEPart(textproto.MIMEHeader(part.Header), part, result, childPath)
			partIdx++
		}
		return
	}

	data, err := decodeMIMEContent(header.Get("Content-Transfer-Encoding"), body)
	if err != nil {
		logger.Warn("Failed to decode MIME part %s: %v", mediaType, err)
		return
	}

	switch mediaType {
	case "text/plain":
		if result.TextPlain == "" {
			result.TextPlain = string(data)
		}
	case "text/html":
		if result.TextHTML == "" {
			result.TextHTML = string(data)
		}
	default:
		disp := strings.ToLower(header.Get("Content-Disposition"))
		if strings.HasPrefix(mediaType, "image/") {
			// Inline images (no disposition or explicitly inline) go to InlineImages
			if disp == "" || strings.HasPrefix(disp, "inline") {
				cid := strings.Trim(header.Get("Content-ID"), "<>")
				result.InlineImages = append(result.InlineImages, models.InlineImage{
					ContentID: cid,
					MIMEType:  mediaType,
					Data:      data,
				})
				return
			}
		}
		// Everything else with an explicit attachment disposition (or unknown
		// non-text/non-image types) is treated as a downloadable attachment.
		if strings.HasPrefix(disp, "attachment") || (!strings.HasPrefix(mediaType, "text/") && !strings.HasPrefix(mediaType, "multipart/")) {
			filename := extractAttachmentFilename(header)
			if filename == "" {
				filename = fmt.Sprintf("attachment_%s", partPath)
			}
			result.Attachments = append(result.Attachments, models.Attachment{
				Filename: filename,
				MIMEType: mediaType,
				Size:     len(data),
				PartPath: partPath,
				Data:     data,
			})
		}
	}
}

// extractAttachmentFilename parses the filename from Content-Disposition or
// Content-Type parameters.
func extractAttachmentFilename(header textproto.MIMEHeader) string {
	dec := mime.WordDecoder{}
	if disp := header.Get("Content-Disposition"); disp != "" {
		if _, params, err := mime.ParseMediaType(disp); err == nil {
			if fn := params["filename"]; fn != "" {
				if decoded, err := dec.DecodeHeader(fn); err == nil {
					return decoded
				}
				return fn
			}
		}
	}
	if ct := header.Get("Content-Type"); ct != "" {
		if _, params, err := mime.ParseMediaType(ct); err == nil {
			if fn := params["name"]; fn != "" {
				if decoded, err := dec.DecodeHeader(fn); err == nil {
					return decoded
				}
				return fn
			}
		}
	}
	return ""
}

// base64LineStripper wraps a reader and drops CR and LF bytes.
// RFC 2045 requires base64 body parts to have CRLF line breaks every 76
// characters; base64.StdEncoding does not tolerate whitespace, so we must
// strip it before decoding.
type base64LineStripper struct{ r io.Reader }

func (s base64LineStripper) Read(p []byte) (int, error) {
	n, err := s.r.Read(p)
	// Remove \r and \n in-place
	out := p[:0]
	for _, b := range p[:n] {
		if b != '\r' && b != '\n' {
			out = append(out, b)
		}
	}
	return len(out), err
}

func decodeMIMEContent(encoding string, r io.Reader) ([]byte, error) {
	switch strings.ToLower(strings.TrimSpace(encoding)) {
	case "base64":
		return io.ReadAll(base64.NewDecoder(base64.StdEncoding, base64LineStripper{r}))
	case "quoted-printable":
		return io.ReadAll(quotedprintable.NewReader(r))
	default:
		return io.ReadAll(r)
	}
}
