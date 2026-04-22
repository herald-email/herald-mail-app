package app

import (
	"crypto/sha256"
	"fmt"
	"strings"
	"sync"

	tea "github.com/charmbracelet/bubbletea"
	"mail-processor/internal/ai"
	"mail-processor/internal/logger"
	"mail-processor/internal/models"
)

// --- Embedding helpers ---

var backgroundAIWarnings sync.Map

func (m *Model) startEmbeddingBatchIfNeeded() tea.Cmd {
	if m.classifier == nil || m.embeddingBatchActive {
		return nil
	}
	m.embeddingBatchActive = true
	return m.runEmbeddingBatch()
}

func (m *Model) startContactEnrichmentIfNeeded() tea.Cmd {
	if m.classifier == nil || m.contactEnrichmentActive {
		return nil
	}
	m.contactEnrichmentActive = true
	return m.runContactEnrichment()
}

// embedChunksForEmail strips, chunks, and embeds an email body for semantic search.
// Uses nomic-embed-text's search_document: prefix for asymmetric retrieval.
// Returns nil if classifier is nil, body is empty, or all embeddings fail.
func embedChunksForEmail(email *models.EmailData, bodyText string, classifier ai.AIClient) ([]models.EmbeddingChunk, error) {
	if classifier == nil || email == nil || bodyText == "" {
		return nil, nil
	}
	cleaned := ai.StripQuotedText(bodyText)
	if cleaned == "" {
		cleaned = email.Subject // fallback: at least embed the subject
	}
	rawChunks := ai.ChunkText(cleaned, 800, 200, 10)
	if len(rawChunks) == 0 {
		return nil, nil
	}
	date := email.Date.Format("2006-01-02")
	var result []models.EmbeddingChunk
	var firstErr error
	for i, chunk := range rawChunks {
		vec, doc, err := embedDocumentChunkWithFallback(classifier, email.Sender, date, email.Subject, chunk)
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
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
	if len(result) == 0 {
		return nil, firstErr
	}
	return result, nil
}

func embedDocumentChunkWithFallback(classifier ai.AIClient, sender, date, subject, chunk string) ([]float32, string, error) {
	buildDoc := func(body string) string {
		return ai.BuildDocumentChunk(sender, date, subject, body)
	}

	body := chunk
	doc := buildDoc(body)
	vec, err := classifier.Embed(doc)
	if err == nil {
		return vec, doc, nil
	}
	if !ai.IsContextLengthError(err) {
		return nil, "", err
	}

	for {
		runes := []rune(body)
		if len(runes) <= 120 {
			break
		}
		nextLen := len(runes) / 2
		if nextLen < 120 {
			nextLen = 120
		}
		body = string(runes[:nextLen])
		doc = buildDoc(body)
		vec, err = classifier.Embed(doc)
		if err == nil {
			return vec, doc, nil
		}
		if !ai.IsContextLengthError(err) {
			return nil, "", err
		}
	}

	doc = buildDoc(subject)
	vec, err = classifier.Embed(doc)
	return vec, doc, err
}

func warnBackgroundAIOnce(format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	if _, loaded := backgroundAIWarnings.LoadOrStore(msg, struct{}{}); loaded {
		return
	}
	logger.Warn("%s", msg)
}

// runEmbeddingBatch processes one batch of emails for semantic search embedding.
// Pass 1 embeds emails with cached body text.
// Pass 2 lazily fetches bodies for emails not yet cached (rate-limited to 5 per call).
func (m *Model) runEmbeddingBatch() tea.Cmd {
	folder := m.currentFolder
	backgroundAI := ai.WithTaskKind(ai.WithPriority(m.classifier, ai.PriorityBackground), ai.TaskKindEmbedding)
	return func() tea.Msg {
		if backgroundAI == nil {
			return EmbeddingDoneMsg{}
		}
		notice := ""
		// Pass 1: embed emails that already have body_text in cache
		if ids, err := m.backend.GetUnembeddedIDsWithBody(folder); err == nil && len(ids) > 0 {
			if len(ids) > 5 {
				ids = ids[:5]
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
				chunks, embErr := embedChunksForEmail(email, bodyText, backgroundAI)
				if len(chunks) > 0 {
					if err := m.backend.StoreEmbeddingChunks(id, chunks); err != nil {
						logger.Warn("StoreEmbeddingChunks %s: %v", id, err)
					}
				} else if embErr != nil {
					warnBackgroundAIOnce("runEmbeddingBatch: embeddings deferred or unavailable: %v", embErr)
					if notice == "" && (ai.IsUnavailableError(embErr) || embErr == ai.ErrDeferred) {
						notice = aiGuidanceNotice(embErr)
					}
					if ai.IsUnavailableError(embErr) || embErr == ai.ErrDeferred {
						break
					}
				}
			}
		}

		// Pass 2: lazily fetch bodies for emails with neither body_text nor chunks
		if uncached, err := m.backend.GetUncachedBodyIDs(folder, 2); err == nil {
			for _, id := range uncached {
				email, err := m.backend.GetEmailByID(id)
				if err != nil || email == nil {
					continue
				}
				body, err := m.backend.FetchAndCacheBody(id)
				if err != nil || body == nil || body.TextPlain == "" {
					continue
				}
				chunks, embErr := embedChunksForEmail(email, body.TextPlain, backgroundAI)
				if len(chunks) > 0 {
					if err := m.backend.StoreEmbeddingChunks(id, chunks); err != nil {
						logger.Warn("StoreEmbeddingChunks (lazy) %s: %v", id, err)
					}
				} else if embErr != nil {
					warnBackgroundAIOnce("runEmbeddingBatch: lazy embeddings deferred or unavailable: %v", embErr)
					if notice == "" && (ai.IsUnavailableError(embErr) || embErr == ai.ErrDeferred) {
						notice = aiGuidanceNotice(embErr)
					}
					if ai.IsUnavailableError(embErr) || embErr == ai.ErrDeferred {
						break
					}
				}
			}
		}

		done, total, _ := m.backend.GetEmbeddingProgress(folder)
		if total == 0 || done >= total {
			return EmbeddingDoneMsg{}
		}
		return EmbeddingProgressMsg{Done: done, Total: total, Notice: notice}
	}
}

// runContactEnrichment fetches up to 5 unenriched contacts (email_count >= 3),
// calls Ollama to extract company + topics, stores the results, then embeds each
// enriched contact and stores the embedding. Returns ContactEnrichedMsg.
// This is a no-op (returns Count: 0) when no contacts need enrichment.
func (m *Model) runContactEnrichment() tea.Cmd {
	return func() tea.Msg {
		backgroundAI := ai.WithTaskKind(ai.WithPriority(m.classifier, ai.PriorityBackground), ai.TaskKindContactEnrich)
		if backgroundAI == nil {
			return ContactEnrichedMsg{Count: 0, Background: true}
		}
		contacts, err := m.backend.GetContactsToEnrich(3, 1)
		if err != nil {
			logger.Warn("runContactEnrichment: GetContactsToEnrich: %v", err)
			return ContactEnrichedMsg{Count: 0, Background: true}
		}
		if len(contacts) == 0 {
			return ContactEnrichedMsg{Count: 0, Background: true}
		}

		enriched := 0
		notice := ""
		seenWarnings := map[string]bool{}
		logWarnOnce := func(format string, args ...any) {
			msg := fmt.Sprintf(format, args...)
			if seenWarnings[msg] {
				return
			}
			seenWarnings[msg] = true
			logger.Warn("%s", msg)
		}
		for _, contact := range contacts {
			// Fetch recent email subjects for this contact
			subjects, err := m.backend.GetRecentSubjectsByContact(contact.Email, 10)
			if err != nil {
				logWarnOnce("runContactEnrichment: GetRecentSubjectsByContact %s: %v", contact.Email, err)
				continue
			}

			// Ask Ollama to extract company and topics
			company, topics, err := backgroundAI.EnrichContact(contact.Email, subjects)
			if err != nil {
				logWarnOnce("runContactEnrichment: EnrichContact %s: %v", contact.Email, err)
				if notice == "" {
					notice = aiGuidanceNotice(err)
				}
				break
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

			vec, embErr := backgroundAI.Embed(embText)
			if embErr != nil {
				logWarnOnce("runContactEnrichment: Embed %s: %v", contact.Email, embErr)
				if notice == "" {
					notice = aiGuidanceNotice(embErr)
				}
				// Enrichment still counts even if embedding fails
			} else {
				if storeErr := m.backend.UpdateContactEmbedding(contact.Email, vec); storeErr != nil {
					logWarnOnce("runContactEnrichment: UpdateContactEmbedding %s: %v", contact.Email, storeErr)
				}
			}

			enriched++
		}

		return ContactEnrichedMsg{Count: enriched, Notice: notice, Background: true}
	}
}
