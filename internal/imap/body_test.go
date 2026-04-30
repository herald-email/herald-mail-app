package imap

import (
	"strings"
	"testing"
)

func TestHTMLToText_NestedDivs(t *testing.T) {
	html := `<div><div><div>text</div></div></div>`
	got := htmlToMarkdown(html)
	if got != "text" {
		t.Errorf("got %q, want %q", got, "text")
	}
}

func TestHTMLToText_ParagraphSeparation(t *testing.T) {
	html := `<p>A</p><p>B</p>`
	got := htmlToMarkdown(html)
	if got != "A\n\nB" {
		t.Errorf("got %q, want %q", got, "A\n\nB")
	}
}

func TestHTMLToText_NoTripleBlankLines(t *testing.T) {
	html := `<div><p>Hello</p></div><div><p>World</p></div>`
	got := htmlToMarkdown(html)
	if strings.Contains(got, "\n\n\n") {
		t.Errorf("got triple newline in %q", got)
	}
}

func TestHTMLToText_TableCells(t *testing.T) {
	html := `<table><tr><td>A</td><td>B</td></tr></table>`
	got := htmlToMarkdown(html)
	if !strings.Contains(got, "A") || !strings.Contains(got, "B") {
		t.Errorf("expected A and B in %q", got)
	}
	if strings.Contains(got, "\n\n") {
		t.Errorf("unexpected blank line in table output: %q", got)
	}
}

func TestHTMLToText_BRTag(t *testing.T) {
	html := `<p>line1<br>line2</p>`
	got := htmlToMarkdown(html)
	if !strings.Contains(got, "line1\nline2") {
		t.Errorf("expected single newline between lines, got %q", got)
	}
}

func TestHTMLToText_RemoteImagesBecomeMarkdownLinks(t *testing.T) {
	html := `<p>Logo below</p><img alt="Taskpad logo" src="https://taskpad.example/logo.png"><img alt="cid" src="cid:logo">`
	got := htmlToMarkdown(html)
	if !strings.Contains(got, "![Taskpad logo](https://taskpad.example/logo.png)") {
		t.Fatalf("expected remote image to become markdown image link, got %q", got)
	}
	if strings.Contains(got, "cid:logo") {
		t.Fatalf("cid images should not become remote links, got %q", got)
	}
}

func TestHTMLToText_SkipsStyleScript(t *testing.T) {
	html := `<style>body{color:red}</style><p>Hello</p><script>alert(1)</script>`
	got := htmlToMarkdown(html)
	if strings.Contains(got, "body") || strings.Contains(got, "alert") {
		t.Errorf("style/script content leaked into output: %q", got)
	}
	if !strings.Contains(got, "Hello") {
		t.Errorf("expected Hello in output: %q", got)
	}
}

func TestHTMLToText_EmptyInput(t *testing.T) {
	got := htmlToMarkdown("")
	if got != "" {
		t.Errorf("got %q, want empty", got)
	}
}

func TestHTMLToText_AdjacentInlineElements(t *testing.T) {
	tests := []struct {
		name string
		html string
		want string // substring that must appear
	}{
		{
			name: "adjacent anchors no href",
			html: `<a>Click</a><a>Yes</a><a>to</a>`,
			want: "Click Yes to",
		},
		{
			name: "text before and after anchor",
			html: `<p>this<a href="#">was NOT</a>your</p>`,
			want: "this was NOT your",
		},
		{
			name: "adjacent anchors with href",
			html: `<p><a href="http://a">Click</a><a href="http://b">Yes</a> to</p>`,
			want: "Yes",
		},
		{
			name: "adjacent spans",
			html: `<span>Click</span><span>Yes</span><span>to</span>`,
			want: "Click Yes to",
		},
		{
			name: "mixed inline elements",
			html: `<p>this <strong>was NOT</strong> your card</p>`,
			want: "this **was NOT** your card",
		},
		{
			name: "button-like links in table cells",
			html: `<table><tr><td><a href="http://yes">Yes</a></td><td><a href="http://no">No</a></td></tr></table>`,
			want: "Yes",
		},
		{
			name: "whitespace-only text node between elements",
			html: `<a href="http://a">Click</a> <a href="http://b">Yes</a>`,
			want: "Yes",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := htmlToMarkdown(tt.html)
			if !strings.Contains(got, tt.want) {
				t.Errorf("output %q does not contain %q", got, tt.want)
			}
		})
	}
}

func TestParseMIMEBody_ExposesEditableDraftHeaders(t *testing.T) {
	raw := []byte(strings.Join([]string{
		"From: Rowan <rowan@example.com>",
		"To: Rae <rae@cobalt-works.example>, Mina <mina@cobalt-works.example>",
		"Cc: Recruiter <recruiting@example.com>",
		"Bcc: Hidden <hidden@example.com>",
		"Subject: Re: Invitation to Technical Interview",
		"MIME-Version: 1.0",
		"Content-Type: text/plain; charset=utf-8",
		"",
		"Hi Rae,",
		"",
		"Thanks for the details.",
	}, "\r\n"))

	body, err := parseMIMEBody(raw)
	if err != nil {
		t.Fatalf("parseMIMEBody: %v", err)
	}

	if body.From != "Rowan <rowan@example.com>" {
		t.Fatalf("From = %q", body.From)
	}
	if body.To != "Rae <rae@cobalt-works.example>, Mina <mina@cobalt-works.example>" {
		t.Fatalf("To = %q", body.To)
	}
	if body.CC != "Recruiter <recruiting@example.com>" {
		t.Fatalf("CC = %q", body.CC)
	}
	if body.BCC != "Hidden <hidden@example.com>" {
		t.Fatalf("BCC = %q", body.BCC)
	}
	if body.Subject != "Re: Invitation to Technical Interview" {
		t.Fatalf("Subject = %q", body.Subject)
	}
	if !strings.Contains(body.TextPlain, "Thanks for the details.") {
		t.Fatalf("TextPlain missing draft body: %q", body.TextPlain)
	}
}

func TestParseMIMEBody_DecodesEditableDraftHeaders(t *testing.T) {
	raw := []byte(strings.Join([]string{
		"From: =?UTF-8?Q?Fl=C3=A1via_da_Silva?= <flavia.iespa@fractional.ai>",
		"To: =?UTF-8?Q?Anton_Golubtsov?= <logrusadm@gmail.com>",
		"Subject: =?UTF-8?B?UmU6IFN0YWZmIFNvZnR3YXJlIEVuZ2luZWVyIGF0IEZyYWN0aW9uYWwgQUk6IHdvcnRoIGEgbG9vaz8=?=",
		"MIME-Version: 1.0",
		"Content-Type: text/plain; charset=utf-8",
		"",
		"Hi Flavia,",
	}, "\r\n"))

	body, err := parseMIMEBody(raw)
	if err != nil {
		t.Fatalf("parseMIMEBody: %v", err)
	}
	if body.Subject != "Re: Staff Software Engineer at Fractional AI: worth a look?" {
		t.Fatalf("Subject = %q", body.Subject)
	}
	if strings.Contains(body.From, "=?") || !strings.Contains(body.From, "Flávia da Silva") {
		t.Fatalf("From header was not decoded: %q", body.From)
	}
	if strings.Contains(body.To, "=?") || !strings.Contains(body.To, "Anton Golubtsov") {
		t.Fatalf("To header was not decoded: %q", body.To)
	}
}
