package ai

import (
	"fmt"
	"strings"
)

// StripQuotedText removes quoted reply blocks and email signatures from body text.
// Quoted blocks: lines starting with ">" (common in replies)
// Signatures: everything after a line that is exactly "--" or "-- "
func StripQuotedText(body string) string {
	body = strings.ReplaceAll(body, "\r\n", "\n")
	lines := strings.Split(body, "\n")
	var result []string
	for _, line := range lines {
		trimmed := strings.TrimRight(line, " \t")
		// Signature delimiter: stop here
		if trimmed == "--" {
			break
		}
		// Skip quoted lines (optional leading whitespace then >)
		stripped := strings.TrimLeft(line, " \t")
		if strings.HasPrefix(stripped, ">") {
			continue
		}
		result = append(result, line)
	}
	return strings.TrimRight(strings.Join(result, "\n"), " \t\n")
}

// ChunkText splits text into chunks of up to chunkSize runes with overlap runes of overlap
// between consecutive chunks. Returns at most maxChunks chunks.
// If text is shorter than chunkSize runes, returns a single chunk.
// If maxChunks is 0, returns nil.
// If overlap >= chunkSize, step is clamped to 1 rune.
func ChunkText(text string, chunkSize, overlap, maxChunks int) []string {
	runes := []rune(text)
	total := len(runes)
	if total == 0 || maxChunks == 0 {
		return nil
	}
	if total <= chunkSize {
		return []string{text}
	}
	step := chunkSize - overlap
	if step <= 0 {
		step = 1
	}
	var chunks []string
	for i := 0; i < total && len(chunks) < maxChunks; i += step {
		end := i + chunkSize
		if end > total {
			end = total
		}
		chunks = append(chunks, string(runes[i:end]))
	}
	return chunks
}

// BuildDocumentChunk constructs the full embedding input for one chunk of an email body.
// This uses nomic-embed-text's asymmetric "search_document:" prefix for document-side embeddings.
// Format:
//
//	search_document: Email from {sender} on {date}
//	Subject: {subject}
//
//	{chunk}
func BuildDocumentChunk(sender, date, subject, chunk string) string {
	return fmt.Sprintf("search_document: Email from %s on %s\nSubject: %s\n\n%s", sender, date, subject, chunk)
}

// BuildQueryText wraps a user search query for nomic-embed-text asymmetric retrieval.
// The "search_query:" prefix tells the model this is a query, not a document.
// Callers MUST use this for all query embeddings, and BuildDocumentChunk for all document embeddings.
func BuildQueryText(query string) string {
	return "search_query: " + query
}
