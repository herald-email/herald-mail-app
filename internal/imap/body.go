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
)

// FetchEmailBody retrieves the full MIME body of the email identified by uid
// in the given folder. It returns parsed plain text and any inline images.
func (c *Client) FetchEmailBody(uid uint32, folder string) (*models.EmailBody, error) {
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
	parseMIMEPart(textproto.MIMEHeader(mailMsg.Header), mailMsg.Body, result)
	return result, nil
}

func parseMIMEPart(header textproto.MIMEHeader, body io.Reader, result *models.EmailBody) {
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
		for {
			part, err := mr.NextPart()
			if err != nil {
				break
			}
			parseMIMEPart(textproto.MIMEHeader(part.Header), part, result)
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
		if strings.HasPrefix(mediaType, "image/") {
			disp := strings.ToLower(header.Get("Content-Disposition"))
			// Include inline images and images with no explicit disposition
			if disp == "" || strings.HasPrefix(disp, "inline") {
				cid := strings.Trim(header.Get("Content-ID"), "<>")
				result.InlineImages = append(result.InlineImages, models.InlineImage{
					ContentID: cid,
					MIMEType:  mediaType,
					Data:      data,
				})
			}
		}
	}
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
