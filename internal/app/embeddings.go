package app

import (
	"crypto/sha256"
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"mail-processor/internal/ai"
	"mail-processor/internal/logger"
	"mail-processor/internal/models"
)

// --- Embedding helpers ---

// embedChunksForEmail strips, chunks, and embeds an email body for semantic search.
// Uses nomic-embed-text's search_document: prefix for asymmetric retrieval.
// Returns nil if classifier is nil, body is empty, or all embeddings fail.
func embedChunksForEmail(email *models.EmailData, bodyText string, classifier ai.AIClient) []models.EmbeddingChunk {
	if classifier == nil || email == nil || bodyText == "" {
		return nil
	}
	cleaned := ai.StripQuotedText(bodyText)
	if cleaned == "" {
		cleaned = email.Subject // fallback: at least embed the subject
	}
	rawChunks := ai.ChunkText(cleaned, 800, 200, 10)
	if len(rawChunks) == 0 {
		return nil
	}
	date := email.Date.Format("2006-01-02")
	var result []models.EmbeddingChunk
	for i, chunk := range rawChunks {
		doc := ai.BuildDocumentChunk(email.Sender, date, email.Subject, chunk)
		vec, err := classifier.Embed(doc)
		if err != nil {
			logger.Debug("embed chunk %d for %s: %v", i, email.MessageID, err)
			continue
		}
		hash := fmt.Sprintf("%x", sha256.Sum256([]byte(doc)))
		result = append(result, models.EmbeddingChunk{
			MessageID:   email.MessageID,
			ChunkIndex:  i,
			Embedding:   vec,
			ContentHash: hash,
		})
	}
	return result
}

// runEmbeddingBatch processes one batch of emails for semantic search embedding.
// Pass 1 embeds emails with cached body text.
// Pass 2 lazily fetches bodies for emails not yet cached (rate-limited to 5 per call).
func (m *Model) runEmbeddingBatch() tea.Cmd {
	folder := m.currentFolder
	return func() tea.Msg {
		// Pass 1: embed emails that already have body_text in cache
		if ids, err := m.backend.GetUnembeddedIDsWithBody(folder); err == nil && len(ids) > 0 {
			if len(ids) > 20 {
				ids = ids[:20]
			}
			for _, id := range ids {
				email, err := m.backend.GetEmailByID(id)
				if err != nil || email == nil {
					continue
				}
				bodyText, err := m.backend.GetBodyText(id)
				if err != nil || bodyText == "" {
					continue
				}
				chunks := embedChunksForEmail(email, bodyText, m.classifier)
				if len(chunks) > 0 {
					if err := m.backend.StoreEmbeddingChunks(id, chunks); err != nil {
						logger.Warn("StoreEmbeddingChunks %s: %v", id, err)
					}
				}
			}
		}

		// Pass 2: lazily fetch bodies for emails with neither body_text nor chunks
		if uncached, err := m.backend.GetUncachedBodyIDs(folder, 5); err == nil {
			for _, id := range uncached {
				email, err := m.backend.GetEmailByID(id)
				if err != nil || email == nil {
					continue
				}
				body, err := m.backend.FetchAndCacheBody(id)
				if err != nil || body == nil || body.TextPlain == "" {
					continue
				}
				chunks := embedChunksForEmail(email, body.TextPlain, m.classifier)
				if len(chunks) > 0 {
					if err := m.backend.StoreEmbeddingChunks(id, chunks); err != nil {
						logger.Warn("StoreEmbeddingChunks (lazy) %s: %v", id, err)
					}
				}
			}
		}

		done, total, _ := m.backend.GetEmbeddingProgress(folder)
		if total > 0 && done >= total {
			return EmbeddingDoneMsg{}
		}
		return EmbeddingProgressMsg{Done: done, Total: total}
	}
}

// runContactEnrichment fetches up to 5 unenriched contacts (email_count >= 3),
// calls Ollama to extract company + topics, stores the results, then embeds each
// enriched contact and stores the embedding. Returns ContactEnrichedMsg.
// This is a no-op (returns Count: 0) when no contacts need enrichment.
func (m *Model) runContactEnrichment() tea.Cmd {
	return func() tea.Msg {
		if m.classifier == nil {
			return ContactEnrichedMsg{Count: 0}
		}
		contacts, err := m.backend.GetContactsToEnrich(3, 5)
		if err != nil {
			logger.Warn("runContactEnrichment: GetContactsToEnrich: %v", err)
			return ContactEnrichedMsg{Count: 0}
		}
		if len(contacts) == 0 {
			return nil
		}

		enriched := 0
		for _, contact := range contacts {
			// Fetch recent email subjects for this contact
			subjects, err := m.backend.GetRecentSubjectsByContact(contact.Email, 10)
			if err != nil {
				logger.Warn("runContactEnrichment: GetRecentSubjectsByContact %s: %v", contact.Email, err)
				continue
			}

			// Ask Ollama to extract company and topics
			company, topics, err := m.classifier.EnrichContact(contact.Email, subjects)
			if err != nil {
				logger.Warn("runContactEnrichment: EnrichContact %s: %v", contact.Email, err)
				continue
			}

			// Store enrichment result (even if company and topics are empty — marks as processed)
			if err := m.backend.UpdateContactEnrichment(contact.Email, company, topics); err != nil {
				logger.Warn("runContactEnrichment: UpdateContactEnrichment %s: %v", contact.Email, err)
				continue
			}

			// Build embedding text and embed
			displayName := contact.DisplayName
			if displayName == "" {
				displayName = contact.Email
			}
			topicsStr := strings.Join(topics, ", ")
			embText := displayName + " " + contact.Email
			if company != "" {
				embText += " from " + company
			}
			if topicsStr != "" {
				embText += ", topics: " + topicsStr
			}

			vec, embErr := m.classifier.Embed(embText)
			if embErr != nil {
				logger.Warn("runContactEnrichment: Embed %s: %v", contact.Email, embErr)
				// Enrichment still counts even if embedding fails
			} else {
				if storeErr := m.backend.UpdateContactEmbedding(contact.Email, vec); storeErr != nil {
					logger.Warn("runContactEnrichment: UpdateContactEmbedding %s: %v", contact.Email, storeErr)
				}
			}

			enriched++
		}

		return ContactEnrichedMsg{Count: enriched}
	}
}
