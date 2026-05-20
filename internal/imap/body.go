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
	"github.com/herald-email/herald-mail-app/internal/logger"
	"github.com/herald-email/herald-mail-app/internal/models"
	emailrender "github.com/herald-email/herald-mail-app/internal/render"
	htmlcharset "golang.org/x/net/html/charset"
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

// FetchEmailPreviewBody retrieves only lightweight preview content: useful
// headers, text/plain and text/html parts, plus attachment metadata. It avoids
// downloading attachment and inline-image bytes.
func (c *Client) FetchEmailPreviewBody(uid uint32, folder string) (*models.EmailBody, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	body, err := c.fetchEmailPreviewBodyLocked(uid, folder)
	if err != nil && isConnectionError(err) {
		logger.Info("FetchEmailPreviewBody: connection error, reconnecting: %v", err)
		if reconErr := c.Reconnect(); reconErr != nil {
			return nil, fmt.Errorf("reconnect failed: %w (original: %v)", reconErr, err)
		}
		body, err = c.fetchEmailPreviewBodyLocked(uid, folder)
	}
	return body, err
}

func (c *Client) fetchEmailPreviewBodyLocked(uid uint32, folder string) (*models.EmailBody, error) {
	if c.client == nil {
		return nil, fmt.Errorf("not connected")
	}
	if _, err := retryAfterReconnect(func() (*imap.MailboxStatus, error) {
		return c.client.Select(folder, true)
	}, c.Reconnect); err != nil {
		return nil, fmt.Errorf("select folder %s: %w", folder, err)
	}

	seqset := new(imap.SeqSet)
	seqset.AddNum(uid)

	headerSection := &imap.BodySectionName{
		BodyPartName: imap.BodyPartName{
			Specifier: imap.HeaderSpecifier,
			Fields: []string{
				"From", "To", "Cc", "Bcc", "Subject", "Message-Id",
				"In-Reply-To", "References", "List-Unsubscribe", "List-Unsubscribe-Post",
			},
		},
		Peek: true,
	}
	items := []imap.FetchItem{imap.FetchBodyStructure, headerSection.FetchItem()}

	var msg *imap.Message
	if err := c.runFetchStreamLocked(imapStreamCommandOptions{
		Name:          "uid fetch preview structure",
		Folder:        folder,
		Phase:         "preview",
		RangeLabel:    fmt.Sprintf("uid=%d", uid),
		MessageBuffer: 1,
	}, func(messages chan *imap.Message) error {
		return c.client.UidFetch(seqset, items, messages)
	}, func(fetched *imap.Message) error {
		if msg == nil {
			msg = fetched
		}
		return nil
	}); err != nil {
		return nil, fmt.Errorf("uid fetch preview structure: %w", err)
	}
	if msg == nil {
		return nil, fmt.Errorf("message uid %d not found in %s", uid, folder)
	}
	if msg.BodyStructure == nil {
		logger.Warn("FetchEmailPreviewBody: no BODYSTRUCTURE for uid %d in %s, falling back to full body", uid, folder)
		return c.fetchEmailBodyLocked(uid, folder)
	}

	result := &models.EmailBody{}
	if headerReader := msg.GetBody(headerSection); headerReader != nil {
		if rawHeader, err := io.ReadAll(headerReader); err == nil {
			applyPreviewHeaders(rawHeader, result)
		}
	}
	result.Attachments = previewAttachmentMetadata(msg.BodyStructure)

	plainPath, plainPart, htmlPath, htmlPart := previewTextParts(msg.BodyStructure)
	var textSections []*imap.BodySectionName
	if len(plainPath) > 0 {
		textSections = append(textSections, &imap.BodySectionName{BodyPartName: imap.BodyPartName{Path: plainPath}, Peek: true})
	}
	if len(htmlPath) > 0 {
		textSections = append(textSections, &imap.BodySectionName{BodyPartName: imap.BodyPartName{Path: htmlPath}, Peek: true})
	}
	if len(textSections) == 0 {
		return result, nil
	}

	textItems := make([]imap.FetchItem, 0, len(textSections))
	for _, section := range textSections {
		textItems = append(textItems, section.FetchItem())
	}
	msg = nil
	if err := c.runFetchStreamLocked(imapStreamCommandOptions{
		Name:          "uid fetch preview text",
		Folder:        folder,
		Phase:         "preview",
		RangeLabel:    fmt.Sprintf("uid=%d", uid),
		MessageBuffer: 1,
	}, func(messages chan *imap.Message) error {
		return c.client.UidFetch(seqset, textItems, messages)
	}, func(fetched *imap.Message) error {
		if msg == nil {
			msg = fetched
		}
		return nil
	}); err != nil {
		return nil, fmt.Errorf("uid fetch preview text: %w", err)
	}
	if msg == nil {
		return result, nil
	}
	if len(plainPath) > 0 {
		if reader := msg.GetBody(textSections[0]); reader != nil {
			if text := decodePreviewTextPart(plainPart, reader); text != "" {
				result.TextPlain = text
			}
		}
	}
	if len(htmlPath) > 0 {
		idx := 0
		if len(plainPath) > 0 {
			idx = 1
		}
		if reader := msg.GetBody(textSections[idx]); reader != nil {
			if text := decodePreviewTextPart(htmlPart, reader); text != "" {
				result.TextHTML = text
			}
		}
	}
	if result.TextPlain == "" && result.TextHTML != "" {
		result.TextPlain = emailrender.HTMLToMarkdown(result.TextHTML)
		result.IsFromHTML = true
	}
	return result, nil
}

// fetchEmailBodyLocked performs the actual IMAP fetch. Must be called with c.mu held.
func (c *Client) fetchEmailBodyLocked(uid uint32, folder string) (*models.EmailBody, error) {
	if c.client == nil {
		return nil, fmt.Errorf("not connected")
	}

	// Select folder read-only; subsequent write ops will re-select read-write.
	if _, err := retryAfterReconnect(func() (*imap.MailboxStatus, error) {
		return c.client.Select(folder, true)
	}, c.Reconnect); err != nil {
		return nil, fmt.Errorf("select folder %s: %w", folder, err)
	}

	seqset := new(imap.SeqSet)
	seqset.AddNum(uid)

	section := &imap.BodySectionName{} // BODY[] — entire RFC 2822 message
	items := []imap.FetchItem{section.FetchItem()}

	var msg *imap.Message
	if err := c.runFetchStreamLocked(imapStreamCommandOptions{
		Name:          "uid fetch body",
		Folder:        folder,
		Phase:         "body",
		RangeLabel:    fmt.Sprintf("uid=%d", uid),
		MessageBuffer: 1,
	}, func(messages chan *imap.Message) error {
		return c.client.UidFetch(seqset, items, messages)
	}, func(fetched *imap.Message) error {
		if msg == nil {
			msg = fetched
		}
		return nil
	}); err != nil {
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
	result.From = decodeHeaderValue(mailMsg.Header.Get("From"))
	result.To = decodeHeaderValue(mailMsg.Header.Get("To"))
	result.CC = decodeHeaderValue(mailMsg.Header.Get("Cc"))
	result.BCC = decodeHeaderValue(mailMsg.Header.Get("Bcc"))
	result.Subject = decodeHeaderValue(mailMsg.Header.Get("Subject"))
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

func decodeHeaderValue(value string) string {
	if value == "" {
		return ""
	}
	decoded, err := (&mime.WordDecoder{}).DecodeHeader(value)
	if err != nil {
		return value
	}
	return decoded
}

func applyPreviewHeaders(raw []byte, result *models.EmailBody) {
	if result == nil || len(raw) == 0 {
		return
	}
	msg, err := mail.ReadMessage(bytes.NewReader(append(raw, []byte("\r\n")...)))
	if err != nil {
		return
	}
	result.From = decodeHeaderValue(msg.Header.Get("From"))
	result.To = decodeHeaderValue(msg.Header.Get("To"))
	result.CC = decodeHeaderValue(msg.Header.Get("Cc"))
	result.BCC = decodeHeaderValue(msg.Header.Get("Bcc"))
	result.Subject = decodeHeaderValue(msg.Header.Get("Subject"))
	result.MessageID = msg.Header.Get("Message-Id")
	result.InReplyTo = msg.Header.Get("In-Reply-To")
	result.References = msg.Header.Get("References")
	result.ListUnsubscribe = msg.Header.Get("List-Unsubscribe")
	result.ListUnsubscribePost = msg.Header.Get("List-Unsubscribe-Post")
}

func previewTextParts(bs *imap.BodyStructure) (plainPath []int, plain *imap.BodyStructure, htmlPath []int, html *imap.BodyStructure) {
	if bs == nil {
		return nil, nil, nil, nil
	}
	bs.Walk(func(path []int, part *imap.BodyStructure) bool {
		if part == nil || len(part.Parts) > 0 || isAttachmentPart(part) {
			return true
		}
		mimeType := strings.ToLower(part.MIMEType)
		mimeSubType := strings.ToLower(part.MIMESubType)
		switch {
		case mimeType == "text" && mimeSubType == "plain" && plain == nil:
			plainPath = append([]int(nil), path...)
			plain = part
		case mimeType == "text" && mimeSubType == "html" && html == nil:
			htmlPath = append([]int(nil), path...)
			html = part
		}
		return true
	})
	return plainPath, plain, htmlPath, html
}

func previewAttachmentMetadata(bs *imap.BodyStructure) []models.Attachment {
	if bs == nil {
		return nil
	}
	var attachments []models.Attachment
	bs.Walk(func(path []int, part *imap.BodyStructure) bool {
		if part == nil || len(part.Parts) > 0 {
			return true
		}
		if !isAttachmentPart(part) {
			return true
		}
		filename := bodyStructureFilename(part)
		if filename == "" {
			filename = "attachment_" + previewPartPath(path)
		}
		attachments = append(attachments, models.Attachment{
			Filename: filename,
			MIMEType: strings.ToLower(part.MIMEType + "/" + part.MIMESubType),
			Size:     int(part.Size),
			PartPath: previewPartPath(path),
		})
		return true
	})
	return attachments
}

func isAttachmentPart(part *imap.BodyStructure) bool {
	disp := strings.ToLower(strings.TrimSpace(part.Disposition))
	if strings.HasPrefix(disp, "attachment") {
		return true
	}
	mimeType := strings.ToLower(part.MIMEType)
	return mimeType != "" && mimeType != "text" && mimeType != "multipart" && mimeType != "image"
}

func bodyStructureFilename(part *imap.BodyStructure) string {
	for _, params := range []map[string]string{part.DispositionParams, part.Params} {
		for _, key := range []string{"filename", "name"} {
			if params != nil {
				if value := strings.TrimSpace(params[key]); value != "" {
					return decodeHeaderValue(value)
				}
			}
		}
	}
	return ""
}

func previewPartPath(path []int) string {
	if len(path) == 0 {
		return ""
	}
	parts := make([]string, len(path))
	for i, n := range path {
		parts[i] = fmt.Sprintf("%d", n)
	}
	return strings.Join(parts, ".")
}

func decodePreviewTextPart(part *imap.BodyStructure, body io.Reader) string {
	if part == nil || body == nil {
		return ""
	}
	data, err := decodeMIMEContent(part.Encoding, body)
	if err != nil {
		logger.Warn("Failed to decode preview MIME part %s/%s: %v", part.MIMEType, part.MIMESubType, err)
		return ""
	}
	return decodeTextPart(data, part.Params)
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

	disp := strings.ToLower(header.Get("Content-Disposition"))
	if strings.HasPrefix(disp, "attachment") {
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
		return
	}

	switch mediaType {
	case "text/plain":
		if result.TextPlain == "" {
			result.TextPlain = decodeTextPart(data, params)
		}
	case "text/html":
		if result.TextHTML == "" {
			result.TextHTML = decodeTextPart(data, params)
		}
	default:
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

func decodeTextPart(data []byte, params map[string]string) string {
	charsetName := strings.TrimSpace(params["charset"])
	if charsetName != "" {
		reader, err := htmlcharset.NewReaderLabel(charsetName, bytes.NewReader(data))
		if err != nil {
			logger.Warn("Failed to create charset reader for %s: %v", charsetName, err)
		} else if decoded, err := io.ReadAll(reader); err != nil {
			logger.Warn("Failed to decode text part charset %s: %v", charsetName, err)
		} else {
			data = decoded
		}
	}
	return emailrender.NormalizeEmailTextArtifacts(string(data))
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
