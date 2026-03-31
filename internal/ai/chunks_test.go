package ai

import (
	"strings"
	"testing"
	"unicode/utf8"
)

// --- StripQuotedText ---

func TestStripQuotedText_NoQuotesNoSignature(t *testing.T) {
	body := "Hello\nThis is a plain email\nNo quotes here"
	got := StripQuotedText(body)
	if got != body {
		t.Errorf("expected unchanged body, got %q", got)
	}
}

func TestStripQuotedText_RemovesQuotedLines(t *testing.T) {
	body := "Reply text\n> Original line 1\n> Original line 2\nMore reply"
	got := StripQuotedText(body)
	want := "Reply text\nMore reply"
	if got != want {
		t.Errorf("expected %q, got %q", want, got)
	}
}

func TestStripQuotedText_RemovesSignatureDashDash(t *testing.T) {
	body := "Hello world\n--\nJohn Doe\njohn@example.com"
	got := StripQuotedText(body)
	want := "Hello world"
	if got != want {
		t.Errorf("expected %q, got %q", want, got)
	}
}

func TestStripQuotedText_RemovesSignatureDashDashSpace(t *testing.T) {
	body := "Hello world\n-- \nJohn Doe\njohn@example.com"
	got := StripQuotedText(body)
	want := "Hello world"
	if got != want {
		t.Errorf("expected %q, got %q", want, got)
	}
}

func TestStripQuotedText_BothQuotesAndSignature(t *testing.T) {
	body := "My reply\n> quoted line\nAnother line\n--\nSignature"
	got := StripQuotedText(body)
	want := "My reply\nAnother line"
	if got != want {
		t.Errorf("expected %q, got %q", want, got)
	}
}

func TestStripQuotedText_EmptyBody(t *testing.T) {
	got := StripQuotedText("")
	if got != "" {
		t.Errorf("expected empty string, got %q", got)
	}
}

func TestStripQuotedText_OnlyQuotedLines(t *testing.T) {
	body := "> line one\n> line two"
	got := StripQuotedText(body)
	if got != "" {
		t.Errorf("expected empty string, got %q", got)
	}
}

func TestStripQuotedText_OnlySignature(t *testing.T) {
	body := "--\nSignature content"
	got := StripQuotedText(body)
	if got != "" {
		t.Errorf("expected empty string, got %q", got)
	}
}

func TestStripQuotedText_QuoteWithLeadingWhitespace(t *testing.T) {
	body := "Text\n  > indented quote\nMore text"
	got := StripQuotedText(body)
	want := "Text\nMore text"
	if got != want {
		t.Errorf("expected %q, got %q", want, got)
	}
}

func TestStripQuotedText_TrimsTrailingWhitespace(t *testing.T) {
	body := "Hello\n> quote\n\n\n"
	got := StripQuotedText(body)
	if strings.HasSuffix(got, "\n") || strings.HasSuffix(got, " ") {
		t.Errorf("expected no trailing whitespace/newline, got %q", got)
	}
}

// --- ChunkText ---

func TestChunkText_ShorterThanChunkSize(t *testing.T) {
	text := "short text"
	chunks := ChunkText(text, 100, 10, 10)
	if len(chunks) != 1 {
		t.Fatalf("expected 1 chunk, got %d", len(chunks))
	}
	if chunks[0] != text {
		t.Errorf("expected %q, got %q", text, chunks[0])
	}
}

func TestChunkText_ExactlyChunkSize(t *testing.T) {
	text := "abcde"
	chunks := ChunkText(text, 5, 0, 10)
	if len(chunks) != 1 {
		t.Fatalf("expected 1 chunk, got %d", len(chunks))
	}
	if chunks[0] != text {
		t.Errorf("expected %q, got %q", text, chunks[0])
	}
}

func TestChunkText_LargerWithNoOverlap(t *testing.T) {
	text := "abcdefghij" // 10 bytes
	chunks := ChunkText(text, 5, 0, 10)
	if len(chunks) != 2 {
		t.Fatalf("expected 2 chunks, got %d: %v", len(chunks), chunks)
	}
	if chunks[0] != "abcde" {
		t.Errorf("chunk 0: expected %q, got %q", "abcde", chunks[0])
	}
	if chunks[1] != "fghij" {
		t.Errorf("chunk 1: expected %q, got %q", "fghij", chunks[1])
	}
}

func TestChunkText_WithOverlap(t *testing.T) {
	// text = "abcdefgh" (8 bytes), chunkSize=6, overlap=2 => step=4
	// chunk 0: [0:6] = "abcdef"
	// chunk 1: [4:8] = "efgh"  (last chunk, shorter)
	// i=0 -> chunk, i=4 -> chunk, i=8 -> stop (>= len)
	text := "abcdefgh"
	chunks := ChunkText(text, 6, 2, 10)
	if len(chunks) != 2 {
		t.Fatalf("expected 2 chunks, got %d: %v", len(chunks), chunks)
	}
	if chunks[0] != "abcdef" {
		t.Errorf("chunk 0: expected %q, got %q", "abcdef", chunks[0])
	}
	if chunks[1] != "efgh" {
		t.Errorf("chunk 1: expected %q, got %q", "efgh", chunks[1])
	}
	// Verify overlap: last 2 chars of chunk 0 == first 2 chars of chunk 1
	if chunks[0][4:6] != chunks[1][0:2] {
		t.Errorf("overlap mismatch: end of chunk 0 = %q, start of chunk 1 = %q",
			chunks[0][4:6], chunks[1][0:2])
	}
}

func TestChunkText_MaxChunksLimit(t *testing.T) {
	// 20 bytes, chunkSize=4, overlap=0, maxChunks=3 => only 3 chunks returned
	text := "abcdefghijklmnopqrst"
	chunks := ChunkText(text, 4, 0, 3)
	if len(chunks) != 3 {
		t.Fatalf("expected 3 chunks (maxChunks), got %d", len(chunks))
	}
}

func TestChunkText_LastChunkShorterThanChunkSize(t *testing.T) {
	// 7 bytes, chunkSize=5, overlap=0 => chunks: [0:5], [5:7]
	text := "abcdefg"
	chunks := ChunkText(text, 5, 0, 10)
	if len(chunks) != 2 {
		t.Fatalf("expected 2 chunks, got %d", len(chunks))
	}
	if chunks[1] != "fg" {
		t.Errorf("last chunk: expected %q, got %q", "fg", chunks[1])
	}
}

func TestChunkText_EmptyText(t *testing.T) {
	chunks := ChunkText("", 100, 10, 10)
	if len(chunks) != 0 {
		t.Fatalf("expected 0 chunks for empty text, got %d: %v", len(chunks), chunks)
	}
}

func TestChunkText_NonASCIIRune(t *testing.T) {
	// "éàü" are 2-byte UTF-8 runes; chunk boundaries must not split them
	text := "aéb" // 3 runes, but 4 bytes
	chunks := ChunkText(text, 2, 0, 10)
	for _, c := range chunks {
		if !utf8.ValidString(c) {
			t.Errorf("chunk %q is not valid UTF-8", c)
		}
	}
	// First chunk should be "aé" (2 runes), second "b" (1 rune)
	if len(chunks) != 2 {
		t.Fatalf("expected 2 chunks, got %d: %v", len(chunks), chunks)
	}
	if chunks[0] != "aé" {
		t.Errorf("chunk[0] = %q, want %q", chunks[0], "aé")
	}
	if chunks[1] != "b" {
		t.Errorf("chunk[1] = %q, want %q", chunks[1], "b")
	}
}

func TestStripQuotedText_CRLFLineEndings(t *testing.T) {
	input := "Hello\r\n-- \r\nSignature line"
	got := StripQuotedText(input)
	if got != "Hello" {
		t.Errorf("got %q, want %q", got, "Hello")
	}
}

// --- BuildDocumentChunk ---

func TestBuildDocumentChunk_Format(t *testing.T) {
	got := BuildDocumentChunk("alice@example.com", "2026-01-15", "Hello there", "Body chunk text")
	if !strings.HasPrefix(got, "search_document:") {
		t.Errorf("expected search_document: prefix, got %q", got)
	}
	if !strings.Contains(got, "Email from alice@example.com on 2026-01-15") {
		t.Errorf("missing sender/date in output: %q", got)
	}
	if !strings.Contains(got, "Subject: Hello there") {
		t.Errorf("missing Subject line in output: %q", got)
	}
	if !strings.Contains(got, "Body chunk text") {
		t.Errorf("missing chunk body in output: %q", got)
	}
}

func TestBuildDocumentChunk_ExactFormat(t *testing.T) {
	want := "search_document: Email from sender@x.com on 2026-03-30\nSubject: Test\n\nchunk content"
	got := BuildDocumentChunk("sender@x.com", "2026-03-30", "Test", "chunk content")
	if got != want {
		t.Errorf("format mismatch\nwant: %q\ngot:  %q", want, got)
	}
}

// --- BuildQueryText ---

func TestBuildQueryText_Prefix(t *testing.T) {
	got := BuildQueryText("find invoices from last month")
	if !strings.HasPrefix(got, "search_query: ") {
		t.Errorf("expected search_query: prefix, got %q", got)
	}
}

func TestBuildQueryText_PreservesQuery(t *testing.T) {
	query := "urgent emails from boss"
	got := BuildQueryText(query)
	want := "search_query: " + query
	if got != want {
		t.Errorf("expected %q, got %q", want, got)
	}
}

func TestBuildQueryText_EmptyQuery(t *testing.T) {
	got := BuildQueryText("")
	want := "search_query: "
	if got != want {
		t.Errorf("expected %q, got %q", want, got)
	}
}
