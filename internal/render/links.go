package render

import (
	"net/url"
	"regexp"
	"strings"

	"github.com/charmbracelet/x/ansi"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	goldtext "github.com/yuin/goldmark/text"
)

// URLRe matches http/https URLs.
var URLRe = regexp.MustCompile(`https?://[^\s<>\[\](){}"'` + "`" + `]+`)
var bracketedURLRe = regexp.MustCompile(`\[(https?://[^\]\s]+)\]`)

// LinkifyWrappedLines applies LinkifyURLs to each pre-wrapped line individually.
// This ensures OSC 8 escape sequences never span a line break, which would leave
// the terminal in a broken hyperlink state and corrupt adjacent panel rendering.
func LinkifyWrappedLines(lines []string) []string {
	out := make([]string, len(lines))
	for i, line := range lines {
		out[i] = LinkifyURLs(line)
	}
	return out
}

// LinkifyURLs replaces raw URLs with OSC 8 terminal hyperlinks.
// The visible text is a shortened version (domain + truncated path);
// the full URL is embedded in the escape sequence so terminals can open it on click.
func LinkifyURLs(text string) string {
	return URLRe.ReplaceAllStringFunc(text, func(raw string) string {
		trimmed := strings.TrimRight(raw, ".,;:!?)")
		// Strip tracking params so the visible label shows the meaningful destination
		// instead of 200+ chars of encoded tracker gibberish.
		cleaned := StripTrackers(trimmed)
		label := ShortenURL(cleaned)
		// OSC 8: \033]8;;URL\033\\ LABEL \033]8;;\033\\
		// Wrap label in blue color so links are readable on dark backgrounds.
		coloredLabel := "\033[38;5;75m" + label + "\033[39m"
		return "\033]8;;" + trimmed + "\033\\" + coloredLabel + "\033]8;;\033\\"
	})
}

type emailLinkToken struct {
	Text string
	URL  string
}

// RenderEmailBodyLines converts an email body into wrapped terminal lines with
// OSC 8 hyperlinks. Markdown/HTML-derived links keep their anchor text visible,
// while naked URLs use shortened labels and keep the full URL hidden in the
// hyperlink target.
func RenderEmailBodyLines(body string, width int) []string {
	if width <= 0 {
		width = 80
	}
	body = collapseLabelURLReferenceLines(body)
	body = unwrapInlineBracketedURLs(body)
	body = protectBareURLsForMarkdown(body)
	tokens := splitNakedURLTokens(markdownEmailTokens(body))
	return wrapEmailLinkTokens(tokens, width)
}

func collapseLabelURLReferenceLines(body string) string {
	body = strings.ReplaceAll(body, "\r\n", "\n")
	lines := strings.Split(body, "\n")
	out := make([]string, 0, len(lines))
	for i := 0; i < len(lines); i++ {
		label := strings.TrimSpace(lines[i])
		if i+1 < len(lines) {
			if rawURL, ok := bracketedURL(lines[i+1]); ok && usableReferenceLabel(label) {
				out = append(out, "["+escapeMarkdownLabel(label)+"](<"+rawURL+">)")
				i++
				continue
			}
		}
		if rawURL, ok := bracketedURL(lines[i]); ok {
			out = append(out, rawURL)
			continue
		}
		out = append(out, lines[i])
	}
	return strings.Join(out, "\n")
}

func bracketedURL(line string) (string, bool) {
	trimmed := strings.TrimSpace(line)
	if !strings.HasPrefix(trimmed, "[") || !strings.HasSuffix(trimmed, "]") {
		return "", false
	}
	rawURL := strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(trimmed, "["), "]"))
	lower := strings.ToLower(rawURL)
	if strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://") {
		return rawURL, true
	}
	return "", false
}

func usableReferenceLabel(label string) bool {
	if label == "" {
		return false
	}
	if strings.HasPrefix(label, "[") || strings.HasPrefix(label, "!") {
		return false
	}
	return !URLRe.MatchString(label)
}

func escapeMarkdownLabel(label string) string {
	label = strings.ReplaceAll(label, `\`, `\\`)
	label = strings.ReplaceAll(label, `[`, `\[`)
	label = strings.ReplaceAll(label, `]`, `\]`)
	return label
}

func unwrapInlineBracketedURLs(body string) string {
	return bracketedURLRe.ReplaceAllString(body, "$1")
}

func protectBareURLsForMarkdown(body string) string {
	matches := URLRe.FindAllStringIndex(body, -1)
	if len(matches) == 0 {
		return body
	}
	var out strings.Builder
	pos := 0
	for _, match := range matches {
		if match[0] < pos {
			continue
		}
		out.WriteString(body[pos:match[0]])
		raw := body[match[0]:match[1]]
		trimmed := strings.TrimRight(raw, ".,;:!?)")
		suffix := raw[len(trimmed):]
		alreadyAutolink := match[0] > 0 && body[match[0]-1] == '<' && match[1] < len(body) && body[match[1]] == '>'
		if alreadyAutolink {
			out.WriteString(raw)
		} else {
			out.WriteByte('<')
			out.WriteString(trimmed)
			out.WriteByte('>')
			out.WriteString(suffix)
		}
		pos = match[1]
	}
	out.WriteString(body[pos:])
	return out.String()
}

func markdownEmailTokens(body string) []emailLinkToken {
	source := []byte(body)
	doc := goldmark.DefaultParser().Parse(goldtext.NewReader(source))
	var tokens []emailLinkToken

	appendText := func(s string) {
		if s != "" {
			tokens = append(tokens, emailLinkToken{Text: s})
		}
	}
	appendLink := func(label, rawURL string) {
		rawURL = strings.TrimSpace(rawURL)
		if rawURL == "" || rawURL == "#" || strings.HasPrefix(strings.ToLower(rawURL), "javascript:") {
			appendText(label)
			return
		}
		label = linkLabel(label, rawURL)
		tokens = append(tokens, emailLinkToken{Text: label, URL: rawURL})
	}

	var renderInline func(ast.Node)
	renderInline = func(n ast.Node) {
		switch node := n.(type) {
		case *ast.Text:
			appendText(string(node.Text(source)))
			if node.HardLineBreak() || node.SoftLineBreak() {
				appendText("\n")
			}
			return
		case *ast.String:
			appendText(string(node.Text(source)))
			return
		case *ast.Link:
			appendLink(collectMarkdownText(node, source), string(node.Destination))
			return
		case *ast.Image:
			appendLink(collectMarkdownText(node, source), string(node.Destination))
			return
		case *ast.AutoLink:
			rawURL := string(node.URL(source))
			if strings.HasPrefix(strings.ToLower(rawURL), "http://") || strings.HasPrefix(strings.ToLower(rawURL), "https://") {
				appendLink(string(node.Label(source)), rawURL)
			} else {
				appendText(string(node.Label(source)))
			}
			return
		}
		for child := n.FirstChild(); child != nil; child = child.NextSibling() {
			renderInline(child)
		}
	}

	var renderBlock func(ast.Node)
	renderBlock = func(n ast.Node) {
		switch node := n.(type) {
		case *ast.Document:
			for child := node.FirstChild(); child != nil; child = child.NextSibling() {
				renderBlock(child)
				if child.NextSibling() != nil {
					appendText("\n\n")
				}
			}
			return
		case *ast.Paragraph:
			for child := node.FirstChild(); child != nil; child = child.NextSibling() {
				renderInline(child)
			}
			return
		case *ast.Heading:
			appendText(strings.Repeat("#", node.Level) + " ")
			for child := node.FirstChild(); child != nil; child = child.NextSibling() {
				renderInline(child)
			}
			return
		case *ast.List:
			for child := node.FirstChild(); child != nil; child = child.NextSibling() {
				renderBlock(child)
				if child.NextSibling() != nil {
					appendText("\n")
				}
			}
			return
		case *ast.ListItem:
			appendText("- ")
			for child := node.FirstChild(); child != nil; child = child.NextSibling() {
				renderBlock(child)
			}
			return
		case *ast.TextBlock:
			for child := node.FirstChild(); child != nil; child = child.NextSibling() {
				renderInline(child)
			}
			return
		case *ast.CodeBlock, *ast.FencedCodeBlock:
			lines := node.Lines()
			for i := 0; i < lines.Len(); i++ {
				segment := lines.At(i)
				appendText(string(segment.Value(source)))
			}
			return
		}
		for child := n.FirstChild(); child != nil; child = child.NextSibling() {
			if child.Type() == ast.TypeBlock {
				renderBlock(child)
			} else {
				renderInline(child)
			}
		}
	}

	renderBlock(doc)
	if len(tokens) == 0 && body != "" {
		return []emailLinkToken{{Text: body}}
	}
	return tokens
}

func collectMarkdownText(n ast.Node, source []byte) string {
	var parts []string
	var walk func(ast.Node)
	walk = func(node ast.Node) {
		switch t := node.(type) {
		case *ast.Text:
			parts = append(parts, string(t.Text(source)))
		case *ast.String:
			parts = append(parts, string(t.Text(source)))
		case *ast.AutoLink:
			parts = append(parts, string(t.Label(source)))
		default:
			for child := node.FirstChild(); child != nil; child = child.NextSibling() {
				walk(child)
			}
		}
	}
	for child := n.FirstChild(); child != nil; child = child.NextSibling() {
		walk(child)
	}
	return strings.Join(strings.Fields(strings.Join(parts, " ")), " ")
}

func linkLabel(label, rawURL string) string {
	label = strings.Join(strings.Fields(label), " ")
	if label == "" || URLRe.MatchString(label) {
		label = ShortenURL(StripTrackers(rawURL))
	}
	return label
}

func splitNakedURLTokens(tokens []emailLinkToken) []emailLinkToken {
	var out []emailLinkToken
	for _, token := range tokens {
		if token.URL != "" {
			out = append(out, token)
			continue
		}
		text := token.Text
		matches := URLRe.FindAllStringIndex(text, -1)
		if len(matches) == 0 {
			out = append(out, token)
			continue
		}
		pos := 0
		for _, match := range matches {
			if match[0] > pos {
				out = append(out, emailLinkToken{Text: text[pos:match[0]]})
			}
			raw := text[match[0]:match[1]]
			trimmed := strings.TrimRight(raw, ".,;:!?)")
			suffix := raw[len(trimmed):]
			out = append(out, emailLinkToken{Text: linkLabel("", trimmed), URL: trimmed})
			if suffix != "" {
				out = append(out, emailLinkToken{Text: suffix})
			}
			pos = match[1]
		}
		if pos < len(text) {
			out = append(out, emailLinkToken{Text: text[pos:]})
		}
	}
	return out
}

type emailWord struct {
	Label string
	URL   string
}

func wrapEmailLinkTokens(tokens []emailLinkToken, width int) []string {
	paragraphs := emailParagraphs(tokens)
	lines := make([]string, 0, len(paragraphs))
	for _, paragraph := range paragraphs {
		if len(paragraph) == 0 {
			if len(lines) == 0 || lines[len(lines)-1] != "" {
				lines = append(lines, "")
			}
			continue
		}
		var current []emailWord
		currentWidth := 0
		flush := func() {
			if len(current) == 0 {
				return
			}
			lines = append(lines, renderEmailWords(current))
			current = nil
			currentWidth = 0
		}
		for _, word := range paragraph {
			wordWidth := ansi.StringWidth(word.Label)
			if wordWidth > width {
				word.Label = ansi.Truncate(word.Label, width, "")
				wordWidth = ansi.StringWidth(word.Label)
			}
			if len(current) == 0 {
				current = append(current, word)
				currentWidth = wordWidth
				continue
			}
			if currentWidth+1+wordWidth > width {
				flush()
				current = append(current, word)
				currentWidth = wordWidth
				continue
			}
			current = append(current, word)
			currentWidth += 1 + wordWidth
		}
		flush()
	}
	return lines
}

func emailParagraphs(tokens []emailLinkToken) [][]emailWord {
	paragraphs := [][]emailWord{{}}
	addWord := func(word emailWord) {
		if word.Label == "" {
			return
		}
		paragraphs[len(paragraphs)-1] = append(paragraphs[len(paragraphs)-1], word)
	}
	addBreak := func() {
		paragraphs = append(paragraphs, []emailWord{})
	}
	for _, token := range tokens {
		if token.URL != "" {
			addWord(emailWord{Label: token.Text, URL: token.URL})
			continue
		}
		parts := strings.Split(token.Text, "\n")
		for i, part := range parts {
			if i > 0 {
				addBreak()
			}
			for _, field := range strings.Fields(part) {
				addWord(emailWord{Label: field})
			}
		}
	}
	return paragraphs
}

func renderEmailWords(words []emailWord) string {
	var b strings.Builder
	for i, word := range words {
		if i > 0 {
			b.WriteByte(' ')
		}
		if word.URL == "" {
			b.WriteString(word.Label)
		} else {
			b.WriteString(terminalHyperlink(word.Label, word.URL))
		}
	}
	return b.String()
}

func terminalHyperlink(label, rawURL string) string {
	return TerminalHyperlink(label, rawURL)
}

// TerminalHyperlink wraps a visible label in an OSC 8 hyperlink target.
func TerminalHyperlink(label, rawURL string) string {
	coloredLabel := "\033[38;5;75m" + label + "\033[39m"
	return "\033]8;;" + rawURL + "\033\\" + coloredLabel + "\033]8;;\033\\"
}

// ShortenURL produces a human-readable label like "example.com/path…"
func ShortenURL(raw string) string {
	parsed, err := url.Parse(raw)
	if err != nil {
		if len(raw) > 40 {
			return raw[:37] + "..."
		}
		return raw
	}
	host := parsed.Hostname()
	path := parsed.Path
	if q := parsed.RawQuery; q != "" {
		path += "?" + q
	}
	if path == "" || path == "/" {
		return host
	}
	full := host + path
	if len(full) > 50 {
		return full[:47] + "..."
	}
	return full
}

// --- Tracker / link sanitization ---

// knownTrackerParams lists URL query parameters commonly used for email
// tracking. StripTrackers removes these so the underlying destination is
// visible without click-tracking noise.
var knownTrackerParams = []string{
	// UTM campaign tracking
	"utm_source", "utm_medium", "utm_campaign", "utm_term", "utm_content", "utm_id",
	// General click/email trackers
	"mc_cid", "mc_eid", // Mailchimp
	"_hsenc", "_hsmi", "hsa_cam", // HubSpot
	"fbclid",          // Facebook
	"gclid", "gclsrc", // Google Ads
	"msclkid",                       // Microsoft Ads
	"dclid",                         // DoubleClick
	"twclid",                        // Twitter
	"igshid",                        // Instagram
	"s_kwcid",                       // Adobe Analytics
	"trk", "trkCampaign", "trkInfo", // LinkedIn
	"si",          // Spotify
	"ref", "ref_", // Generic referrer tags
	"oly_anon_id", "oly_enc_id", // Onlytica
	"vero_id",            // Vero
	"_bta_tid", "_bta_c", // Bronto
	"spm",      // Taobao/Alibaba
	"wickedid", // Wicked Reports
	"dm_i",     // dotdigital
}

// trackerParamSet is a fast lookup built from knownTrackerParams.
var trackerParamSet map[string]bool

func init() {
	trackerParamSet = make(map[string]bool, len(knownTrackerParams))
	for _, p := range knownTrackerParams {
		trackerParamSet[strings.ToLower(p)] = true
	}
}

// StripTrackers removes known tracking query parameters from a URL string.
// If all query parameters are trackers the query string is removed entirely.
// The URL is otherwise unchanged (scheme, host, path, fragment preserved).
func StripTrackers(rawURL string) string {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}
	q := parsed.Query()
	if len(q) == 0 {
		return rawURL
	}
	changed := false
	for key := range q {
		if trackerParamSet[strings.ToLower(key)] {
			q.Del(key)
			changed = true
		}
	}
	if !changed {
		return rawURL
	}
	parsed.RawQuery = q.Encode()
	return parsed.String()
}

// StripTrackersFromText applies StripTrackers to every URL found in text.
func StripTrackersFromText(text string) string {
	return URLRe.ReplaceAllStringFunc(text, func(raw string) string {
		trimmed := strings.TrimRight(raw, ".,;:!?)")
		suffix := raw[len(trimmed):]
		return StripTrackers(trimmed) + suffix
	})
}
