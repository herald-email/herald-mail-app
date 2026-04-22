package app

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"mail-processor/internal/ai"
	"mail-processor/internal/logger"
	"mail-processor/internal/models"
)

// --- Search helpers ---

// performSearch runs a local or semantic search and returns the result as a tea.Cmd.
func (m *Model) performSearch(query string) tea.Cmd {
	if query == "" {
		return func() tea.Msg { return SearchResultMsg{Query: ""} }
	}
	folder := m.currentFolder
	bodyMode := strings.HasPrefix(query, "/b ")
	crossFolder := strings.HasPrefix(query, "/*")
	semanticMode := strings.HasPrefix(query, "?")

	actualQuery := query
	switch {
	case bodyMode:
		actualQuery = strings.TrimPrefix(query, "/b ")
	case crossFolder:
		actualQuery = strings.TrimPrefix(strings.TrimPrefix(query, "/* "), "/*")
	case semanticMode:
		actualQuery = strings.TrimPrefix(query, "?")
	}
	actualQuery = strings.TrimSpace(actualQuery)
	if actualQuery == "" {
		return func() tea.Msg { return SearchResultMsg{Query: ""} }
	}

	classifier := m.classifier
	backend := m.backend
	return func() tea.Msg {
		var emails []*models.EmailData
		var scores map[string]float64
		var err error
		source := "local"
		switch {
		case semanticMode:
			source = "semantic"
			if classifier == nil {
				logger.Warn("Semantic search requires Ollama classifier — not configured")
				return SearchResultMsg{Emails: []*models.EmailData{}, Query: query, Source: source}
			}
			queryText := ai.BuildQueryText(actualQuery)
			vec, embedErr := classifier.Embed(queryText)
			if embedErr != nil {
				logger.Warn("Semantic search embed error: %v", embedErr)
				return SearchResultMsg{Emails: []*models.EmailData{}, Query: query, Source: source}
			}
			results, searchErr := backend.SearchSemanticChunked(folder, vec, 20, 0.3)
			if searchErr != nil {
				logger.Warn("semantic search: %v", searchErr)
				if strings.Contains(searchErr.Error(), "not supported") {
					return SearchResultMsg{Emails: nil, Err: fmt.Errorf("semantic search requires local backend")}
				}
				return SearchResultMsg{Emails: nil, Err: searchErr}
			}
			scores = make(map[string]float64, len(results))
			for _, r := range results {
				emails = append(emails, r.Email)
				scores[r.Email.MessageID] = r.Score
			}
		case bodyMode:
			emails, err = backend.SearchEmails(folder, actualQuery, true)
			source = "fts"
		case crossFolder:
			emails, err = backend.SearchEmailsCrossFolder(actualQuery)
			source = "cross"
		default:
			emails, err = backend.SearchEmails(folder, actualQuery, false)
		}
		if err != nil {
			logger.Warn("Search error: %v", err)
			return SearchResultMsg{Emails: []*models.EmailData{}, Query: query, Source: source}
		}
		if emails == nil {
			emails = []*models.EmailData{}
		}
		return SearchResultMsg{Emails: emails, Scores: scores, Query: query, Source: source}
	}
}

// performIMAPSearch performs a server-side IMAP search as a tea.Cmd.
func (m *Model) performIMAPSearch(query string) tea.Cmd {
	if query == "" {
		return nil
	}
	folder := m.currentFolder
	return func() tea.Msg {
		emails, err := m.backend.SearchEmailsIMAP(folder, query)
		if err != nil {
			logger.Warn("IMAP search error: %v", err)
			return SearchResultMsg{Emails: []*models.EmailData{}, Query: query, Source: "imap"}
		}
		return SearchResultMsg{Emails: emails, Query: query, Source: "imap"}
	}
}

// saveCurrentSearch persists the current search query with an auto-generated name.
func (m *Model) saveCurrentSearch(query string) tea.Cmd {
	folder := m.currentFolder
	name := query
	if len(name) > 30 {
		name = name[:27] + "..."
	}
	return func() tea.Msg {
		if err := m.backend.SaveSearch(name, query, folder); err != nil {
			logger.Warn("Failed to save search: %v", err)
		}
		return nil
	}
}

// updateTimelineTableFromSearch replaces the displayed emails with search results.
// Called from the SearchResultMsg handler when searchMode is active.
func (m *Model) updateTimelineTableFromSearch(emails []*models.EmailData) {
	if emails == nil {
		// Restore from cache
		if m.timeline.emailsCache != nil {
			m.timeline.emails = m.timeline.emailsCache
			m.timeline.emailsCache = nil
		}
	} else {
		m.timeline.emails = emails
	}
	m.updateTimelineTable()
}
