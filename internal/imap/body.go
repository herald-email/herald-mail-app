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
	"golang.org/x/net/html"
	"mail-processor/internal/logger"
	"mail-processor/internal/models"
)

// FetchEmailBody retrieves the full MIME body of the email identified by uid
// in the given folder. It returns parsed plain text and any inline images.
func (c *Client) FetchEmailBody(uid uint32, folder string) (*models.EmailBody, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
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
	// If there is no plain-text part, convert HTML to readable text
	if result.TextPlain == "" && result.TextHTML != "" {
		result.TextPlain = htmlToText(result.TextHTML)
	}
	return result, nil
}

// htmlToText converts an HTML string to readable plain text using a lazy-newline
// accumulator. Newlines are never emitted immediately after a block element;
// instead they are queued and only flushed immediately before the next real text.
// This means any number of nested empty divs produces at most one blank line,
// and trailing newlines at end-of-body are suppressed entirely.
func htmlToText(htmlStr string) string {
	doc, err := html.Parse(strings.NewReader(htmlStr))
	if err != nil {
		return html.UnescapeString(stripTags(htmlStr))
	}

	var sb strings.Builder
	pendingNL := 0 // queued newlines (0-2); flushed before the next real text

	addNL := func(n int) {
		if n > pendingNL {
			pendingNL = n
		}
		if pendingNL > 2 {
			pendingNL = 2
		}
	}

	writeText := func(s string) {
		if s == "" {
			return
		}
		if sb.Len() > 0 {
			for i := 0; i < pendingNL; i++ {
				sb.WriteByte('\n')
			}
		}
		pendingNL = 0
		sb.WriteString(s)
	}

	var walk func(*html.Node)
	walk = func(n *html.Node) {
		switch n.Type {
		case html.TextNode:
			writeText(strings.Join(strings.Fields(n.Data), " "))
			return
		case html.ElementNode:
			tag := strings.ToLower(n.Data)
			if tag == "style" || tag == "script" || tag == "head" {
				return
			}
			if tag == "br" {
				addNL(1)
				return
			}
			if tag == "td" || tag == "th" {
				writeText(" ")
				for c := n.FirstChild; c != nil; c = c.NextSibling {
					walk(c)
				}
				writeText(" ")
				return
			}
			for c := n.FirstChild; c != nil; c = c.NextSibling {
				walk(c)
			}
			switch tag {
			case "p", "h1", "h2", "h3", "h4", "h5", "h6":
				addNL(2) // blank line after headings/paragraphs
			case "div", "section", "article", "blockquote", "li", "tr":
				addNL(1) // newline after other block elements
			}
			return
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)
	return strings.TrimSpace(sb.String())
}

// stripTags is a naive fallback that removes all < > delimited tags.
func stripTags(s string) string {
	var sb strings.Builder
	inTag := false
	for _, r := range s {
		switch {
		case r == '<':
			inTag = true
		case r == '>':
			inTag = false
		case !inTag:
			sb.WriteRune(r)
		}
	}
	return sb.String()
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
