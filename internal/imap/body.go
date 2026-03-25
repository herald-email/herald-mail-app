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
	result.ListUnsubscribe = mailMsg.Header.Get("List-Unsubscribe")
	result.ListUnsubscribePost = mailMsg.Header.Get("List-Unsubscribe-Post")
	parseMIMEPart(textproto.MIMEHeader(mailMsg.Header), mailMsg.Body, result, "")
	// If there is no plain-text part, convert HTML to markdown
	if result.TextPlain == "" && result.TextHTML != "" {
		result.TextPlain = htmlToMarkdown(result.TextHTML)
		result.IsFromHTML = true
	}
	return result, nil
}

// htmlToMarkdown converts an HTML string to GitHub-flavoured Markdown.
// Links become [text](url), bold/italic are preserved, headings use #-syntax,
// and list items use -. Non-breaking spaces are normalised to regular spaces.
// Consecutive blank lines are collapsed to at most one.
func htmlToMarkdown(htmlStr string) string {
	doc, err := html.Parse(strings.NewReader(htmlStr))
	if err != nil {
		return html.UnescapeString(stripTags(htmlStr))
	}

	var out strings.Builder
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
		if out.Len() > 0 {
			for i := 0; i < pendingNL; i++ {
				out.WriteByte('\n')
			}
			// Add a word-boundary space between adjacent inline text fragments
			// when neither side already has whitespace or punctuation.
			if pendingNL == 0 {
				str := out.String()
				last := str[len(str)-1]
				first := s[0]
				if last != ' ' && last != '\n' && last != '(' && last != '[' && last != '*' && last != '`' &&
					first != ' ' && first != '\n' && first != '.' && first != ',' &&
					first != '!' && first != '?' && first != ')' && first != ']' &&
					first != ':' && first != ';' && first != '*' && first != '`' {
					out.WriteByte(' ')
				}
			}
		}
		pendingNL = 0
		out.WriteString(s)
	}

	// collectText extracts plain text from a subtree (used for link labels etc.)
	var collectText func(*html.Node) string
	collectText = func(n *html.Node) string {
		var sb strings.Builder
		var inner func(*html.Node)
		inner = func(n *html.Node) {
			if n.Type == html.TextNode {
				t := strings.Join(strings.Fields(strings.ReplaceAll(n.Data, "\u00a0", " ")), " ")
				if t != "" {
					if sb.Len() > 0 {
						sb.WriteByte(' ')
					}
					sb.WriteString(t)
				}
			}
			for c := n.FirstChild; c != nil; c = c.NextSibling {
				inner(c)
			}
		}
		inner(n)
		return sb.String()
	}

	var walk func(*html.Node)
	walk = func(n *html.Node) {
		switch n.Type {
		case html.TextNode:
			t := strings.ReplaceAll(n.Data, "\u00a0", " ")
			writeText(strings.Join(strings.Fields(t), " "))
			return
		case html.ElementNode:
			tag := strings.ToLower(n.Data)
			switch tag {
			case "style", "script", "head", "img":
				return
			case "br":
				addNL(1)
				return
			case "a":
				var href string
				for _, attr := range n.Attr {
					if attr.Key == "href" {
						href = attr.Val
						break
					}
				}
				text := collectText(n)
				if text == "" {
					return
				}
				if href != "" && href != "#" && !strings.HasPrefix(href, "javascript") {
					writeText(fmt.Sprintf("[%s](%s)", text, href))
				} else {
					writeText(text)
				}
				return
			case "strong", "b":
				text := collectText(n)
				if text != "" {
					writeText("**" + text + "**")
				}
				return
			case "em", "i":
				text := collectText(n)
				if text != "" {
					writeText("*" + text + "*")
				}
				return
			case "code":
				text := collectText(n)
				if text != "" {
					writeText("`" + text + "`")
				}
				return
			case "h1":
				addNL(2)
				writeText("# ")
				for c := n.FirstChild; c != nil; c = c.NextSibling {
					walk(c)
				}
				addNL(2)
				return
			case "h2":
				addNL(2)
				writeText("## ")
				for c := n.FirstChild; c != nil; c = c.NextSibling {
					walk(c)
				}
				addNL(2)
				return
			case "h3", "h4", "h5", "h6":
				addNL(2)
				writeText("### ")
				for c := n.FirstChild; c != nil; c = c.NextSibling {
					walk(c)
				}
				addNL(2)
				return
			case "li":
				addNL(1)
				writeText("- ")
				for c := n.FirstChild; c != nil; c = c.NextSibling {
					walk(c)
				}
				addNL(1)
				return
			case "td", "th":
				for c := n.FirstChild; c != nil; c = c.NextSibling {
					walk(c)
				}
				writeText(" ")
				return
			case "tr":
				for c := n.FirstChild; c != nil; c = c.NextSibling {
					walk(c)
				}
				addNL(1)
				return
			default:
				for c := n.FirstChild; c != nil; c = c.NextSibling {
					walk(c)
				}
				switch tag {
				case "p", "div", "section", "article", "blockquote", "ul", "ol":
					addNL(2)
				}
				return
			}
		default:
			for c := n.FirstChild; c != nil; c = c.NextSibling {
				walk(c)
			}
		}
	}
	walk(doc)
	result := out.String()
	// Collapse 3+ consecutive newlines to 2
	for strings.Contains(result, "\n\n\n") {
		result = strings.ReplaceAll(result, "\n\n\n", "\n\n")
	}
	return strings.TrimSpace(result)
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
