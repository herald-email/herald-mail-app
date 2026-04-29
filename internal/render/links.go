package render

import (
	"net/url"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/charmbracelet/glamour"
	glamouransi "github.com/charmbracelet/glamour/ansi"
	xansi "github.com/charmbracelet/x/ansi"
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
	body = labelBareURLsForMarkdown(body)
	if lines, err := renderGlamourEmailBodyLines(body, width); err == nil {
		return lines
	}
	tokens := splitNakedURLTokens(markdownEmailTokens(protectBareURLsForMarkdown(body)))
	return wrapEmailLinkTokens(tokens, width)
}

func renderGlamourEmailBodyLines(body string, width int) ([]string, error) {
	linkTargets := markdownLinkTargets(body)
	renderer, err := glamour.NewTermRenderer(
		glamour.WithStyles(glamourEmailStyle()),
		glamour.WithWordWrap(width),
	)
	if err != nil {
		return nil, err
	}
	rendered, err := renderer.Render(body)
	if err != nil {
		return nil, err
	}
	rendered = strings.ReplaceAll(rendered, "\r\n", "\n")
	rendered = strings.Trim(rendered, "\n")
	if rendered == "" {
		return nil, nil
	}
	renderedLines := strings.Split(rendered, "\n")
	lines := make([]string, 0, len(renderedLines))
	for i := 0; i < len(renderedLines); i++ {
		line := trimRightANSIWhitespace(renderedLines[i])
		wrapped := wrapGlamourLineToWidth(line, width)
		if len(wrapped) > 1 && i+1 < len(renderedLines) && canMergeShortWrappedTail(line, wrapped[len(wrapped)-1], renderedLines[i+1], width) {
			wrapped[len(wrapped)-1] = wrapped[len(wrapped)-1] + " " + trimRightANSIWhitespace(renderedLines[i+1])
			i++
		}
		for _, wrapped := range wrapped {
			lines = append(lines, applyTerminalHyperlinks(wrapped, linkTargets))
		}
	}
	return lines, nil
}

func wrapGlamourLineToWidth(line string, width int) []string {
	if width <= 0 || xansi.StringWidth(line) <= width {
		return []string{line}
	}
	wrapped := xansi.Wordwrap(line, width, "")
	if strings.TrimSpace(wrapped) == "" {
		return []string{line}
	}
	return strings.Split(wrapped, "\n")
}

func canMergeShortWrappedTail(originalLine, tail, nextLine string, width int) bool {
	if strings.Contains(originalLine, "│") || strings.Contains(originalLine, "─") {
		return false
	}
	tailText := strings.TrimSpace(xansi.Strip(tail))
	nextText := strings.TrimSpace(xansi.Strip(nextLine))
	if tailText == "" || nextText == "" || utf8.RuneCountInString(tailText) > 3 {
		return false
	}
	if strings.HasPrefix(nextText, "•") || strings.HasPrefix(nextText, "#") || strings.Contains(nextText, "│") {
		return false
	}
	return xansi.StringWidth(tail+" "+trimRightANSIWhitespace(nextLine)) <= width
}

func glamourEmailStyle() glamouransi.StyleConfig {
	style := glamour.DarkStyleConfig
	style.Document.BlockPrefix = ""
	style.Document.BlockSuffix = ""
	style.Document.Margin = uintPtr(0)
	style.Link.Format = `{{ "" }}`
	style.LinkText.Color = stringPtr("39")
	style.LinkText.Underline = boolPtr(true)
	style.LinkText.Bold = boolPtr(false)
	style.Image.Format = `{{ "" }}`
	style.ImageText.Format = "{{ .text }}"
	style.ImageText.Color = stringPtr("39")
	style.ImageText.Underline = boolPtr(true)
	return style
}

func stringPtr(value string) *string {
	return &value
}

func boolPtr(value bool) *bool {
	return &value
}

func uintPtr(value uint) *uint {
	return &value
}

type emailLinkTarget struct {
	Label string
	URL   string
}

func markdownLinkTargets(body string) []emailLinkTarget {
	tokens := markdownEmailTokens(body)
	targets := make([]emailLinkTarget, 0, len(tokens))
	seen := make(map[emailLinkTarget]bool)
	for _, token := range tokens {
		if token.URL == "" || token.Text == "" {
			continue
		}
		target := emailLinkTarget{Label: token.Text, URL: token.URL}
		if seen[target] {
			continue
		}
		seen[target] = true
		targets = append(targets, target)
	}
	return targets
}

func trimRightANSIWhitespace(line string) string {
	cut := 0
	includeEscapesAfterLastText := false
	for i := 0; i < len(line); {
		if line[i] == '\033' {
			j := skipEscapeSeqBytes(line, i)
			if includeEscapesAfterLastText {
				cut = j
			}
			i = j
			continue
		}
		r, size := utf8.DecodeRuneInString(line[i:])
		if r == utf8.RuneError && size == 0 {
			break
		}
		if unicode.IsSpace(r) {
			includeEscapesAfterLastText = false
		} else {
			cut = i + size
			includeEscapesAfterLastText = true
		}
		i += size
	}
	return line[:cut]
}

func applyTerminalHyperlinks(line string, targets []emailLinkTarget) string {
	for _, target := range targets {
		line = wrapVisibleLabelWithTerminalHyperlink(line, target.Label, target.URL)
	}
	return line
}

func wrapVisibleLabelWithTerminalHyperlink(line, label, rawURL string) string {
	if line == "" || label == "" || rawURL == "" || strings.Contains(line, "\033]8;;"+rawURL+"\033\\") {
		return line
	}
	visibleRunes := []rune(xansi.Strip(line))
	labelRunes := []rune(label)
	visibleRanges := visibleLabelRanges(visibleRunes, labelRunes)
	if len(visibleRanges) == 0 {
		return line
	}
	rawRanges := make([][2]int, 0, len(visibleRanges))
	for _, visibleRange := range visibleRanges {
		rawStart, rawEnd, ok := rawRangeForVisibleRuneRange(line, visibleRange[0], visibleRange[1])
		if ok {
			rawRanges = append(rawRanges, [2]int{rawStart, rawEnd})
		}
	}
	for i := len(rawRanges) - 1; i >= 0; i-- {
		rawStart, rawEnd := rawRanges[i][0], rawRanges[i][1]
		line = line[:rawStart] + "\033]8;;" + rawURL + "\033\\" + line[rawStart:rawEnd] + "\033]8;;\033\\" + line[rawEnd:]
	}
	return line
}

func visibleLabelRanges(haystack, needle []rune) [][2]int {
	if len(needle) == 0 || len(needle) > len(haystack) {
		return nil
	}
	var ranges [][2]int
	for i := 0; i <= len(haystack)-len(needle); {
		matched := true
		for j := range needle {
			if haystack[i+j] != needle[j] {
				matched = false
				break
			}
		}
		if matched {
			ranges = append(ranges, [2]int{i, i + len(needle)})
			i += len(needle)
			continue
		}
		i++
	}
	return ranges
}

func rawRangeForVisibleRuneRange(line string, startRune, endRune int) (int, int, bool) {
	rawStart, rawEnd := -1, -1
	visible := 0
	for i := 0; i < len(line); {
		if line[i] == '\033' {
			i = skipEscapeSeqBytes(line, i)
			continue
		}
		_, size := utf8.DecodeRuneInString(line[i:])
		if visible == startRune {
			rawStart = i
		}
		visible++
		i += size
		if visible == endRune {
			rawEnd = i
			for rawEnd < len(line) && line[rawEnd] == '\033' {
				rawEnd = skipEscapeSeqBytes(line, rawEnd)
			}
			break
		}
	}
	return rawStart, rawEnd, rawStart >= 0 && rawEnd >= rawStart
}

func skipEscapeSeqBytes(s string, start int) int {
	if start >= len(s) || s[start] != '\033' {
		return start + 1
	}
	if start+1 >= len(s) {
		return len(s)
	}
	switch s[start+1] {
	case ']':
		for i := start + 2; i < len(s); i++ {
			if s[i] == '\a' {
				return i + 1
			}
			if s[i] == '\033' && i+1 < len(s) && s[i+1] == '\\' {
				return i + 2
			}
		}
		return len(s)
	case '[':
		for i := start + 2; i < len(s); i++ {
			if s[i] >= 0x40 && s[i] <= 0x7e {
				return i + 1
			}
		}
		return len(s)
	default:
		return start + 2
	}
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

func labelBareURLsForMarkdown(body string) string {
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
		if isMarkdownURLDestination(body, match[0], match[1]) {
			out.WriteString(raw)
		} else {
			out.WriteString("[")
			out.WriteString(escapeMarkdownLabel(ShortenURL(StripTrackers(trimmed))))
			out.WriteString("](")
			out.WriteString(trimmed)
			out.WriteString(")")
			out.WriteString(suffix)
		}
		pos = match[1]
	}
	out.WriteString(body[pos:])
	return out.String()
}

func isMarkdownURLDestination(body string, start, end int) bool {
	if start > 0 && body[start-1] == '(' {
		return true
	}
	if start > 1 && body[start-1] == '<' && body[start-2] == '(' && end < len(body) && body[end] == '>' {
		return true
	}
	return start > 0 && body[start-1] == '<' && end < len(body) && body[end] == '>'
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
			wordWidth := xansi.StringWidth(word.Label)
			if wordWidth > width {
				word.Label = xansi.Truncate(word.Label, width, "")
				wordWidth = xansi.StringWidth(word.Label)
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
		if i > 0 && !emailWordJoinsPrevious(word.Label) {
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

func emailWordJoinsPrevious(label string) bool {
	if label == "" {
		return false
	}
	switch []rune(label)[0] {
	case '.', ',', '!', '?', ':', ';', ')', ']', '}':
		return true
	default:
		return false
	}
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
