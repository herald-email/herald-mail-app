package app

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/herald-email/herald-mail-app/internal/ai"
	"github.com/herald-email/herald-mail-app/internal/logger"
	"github.com/herald-email/herald-mail-app/internal/models"
	"github.com/herald-email/herald-mail-app/internal/retrieval"
)

// --- Search helpers ---

const hybridSemanticLimit = 100

func (m *Model) semanticSearchConfig() (limit int, minScore float64) {
	limit = hybridSemanticLimit
	minScore = 0.30
	if m.cfg != nil {
		if m.cfg.Semantic.MinScore > 0 {
			minScore = m.cfg.Semantic.MinScore
		}
	}
	return limit, minScore
}

// performSearch runs a local or semantic search and returns the result as a tea.Cmd.
func (m *Model) performSearch(query string) tea.Cmd {
	return m.performSearchWithToken(query, m.timeline.searchToken)
}

// performSearchWithToken runs a local or semantic search and tags the result so
// stale responses can be ignored when the user keeps typing.
func (m *Model) performSearchWithToken(query string, token int) tea.Cmd {
	if query == "" {
		return func() tea.Msg { return SearchResultMsg{Query: "", Token: token} }
	}
	folder := m.currentFolder
	keywordMode := strings.HasPrefix(query, "/k ")
	bodyMode := strings.HasPrefix(query, "/b ")
	crossFolder := strings.HasPrefix(query, "/*")
	semanticMode := strings.HasPrefix(query, "?")

	actualQuery := query
	switch {
	case keywordMode:
		actualQuery = strings.TrimPrefix(query, "/k ")
	case bodyMode:
		actualQuery = strings.TrimPrefix(query, "/b ")
	case crossFolder:
		actualQuery = strings.TrimPrefix(strings.TrimPrefix(query, "/* "), "/*")
	case semanticMode:
		actualQuery = strings.TrimPrefix(query, "?")
	}
	actualQuery = strings.TrimSpace(actualQuery)
	if actualQuery == "" {
		return func() tea.Msg { return SearchResultMsg{Query: "", Token: token} }
	}
	if isVirtualAllMailOnlyFolder(folder) {
		baseEmails := m.timeline.emailsCache
		if baseEmails == nil {
			baseEmails = m.timeline.emails
		}
		baseSnapshot := append([]*models.EmailData(nil), baseEmails...)
		return func() tea.Msg {
			switch {
			case bodyMode:
				return SearchResultMsg{
					Emails: []*models.EmailData{},
					Query:  query,
					Source: "local",
					Token:  token,
					Err:    fmt.Errorf("body search is unavailable in All Mail only; use local search here"),
				}
			case crossFolder:
				return SearchResultMsg{
					Emails: []*models.EmailData{},
					Query:  query,
					Source: "local",
					Token:  token,
					Err:    fmt.Errorf("cross-folder search is unavailable in All Mail only; this view is already a derived diagnostic set"),
				}
			case semanticMode:
				return SearchResultMsg{
					Emails: []*models.EmailData{},
					Query:  query,
					Source: "local",
					Token:  token,
					Err:    fmt.Errorf("semantic search is unavailable in All Mail only; use local search here"),
				}
			default:
				return SearchResultMsg{
					Emails: filterVirtualFolderEmails(baseSnapshot, actualQuery),
					Query:  query,
					Source: "local",
					Token:  token,
				}
			}
		}
	}

	classifier := m.classifier
	backend := m.backend
	semanticLimit, semanticMinScore := m.semanticSearchConfig()
	mode := retrieval.ModeHybrid
	switch {
	case keywordMode:
		mode = retrieval.ModeKeyword
	case semanticMode:
		mode = retrieval.ModeSemantic
	case bodyMode:
		mode = retrieval.ModeBody
	case crossFolder:
		mode = retrieval.ModeCross
	}
	return func() tea.Msg {
		source := mode
		if source == retrieval.ModeKeyword {
			source = "local"
		}
		if semanticMode && classifier == nil {
			logger.Warn("Semantic search requires AI classifier — not configured")
			return SearchResultMsg{Emails: nil, Query: query, Source: source, Token: token, Err: fmt.Errorf("semantic search unavailable: AI is not configured")}
		}
		var embedder retrieval.Embedder
		if classifier != nil {
			embedder = ai.WithTaskKind(classifier, ai.TaskKindSemanticSearch)
		}
		result, err := retrieval.Search(context.Background(), backend, embedder, retrieval.Request{
			Folder:   folder,
			Query:    actualQuery,
			Mode:     mode,
			Limit:    semanticLimit,
			MinScore: semanticMinScore,
		})
		if err != nil {
			logger.Warn("Search error: %v", err)
			if semanticMode && strings.Contains(err.Error(), "not supported") {
				return SearchResultMsg{Emails: nil, Query: query, Source: source, Token: token, Err: fmt.Errorf("semantic search requires local backend")}
			}
			if semanticMode {
				guidance := aiGuidanceNotice(err)
				if guidance != "" {
					return SearchResultMsg{Emails: nil, Query: query, Source: source, Token: token, Err: fmt.Errorf("semantic search unavailable: %s", guidance)}
				}
				return SearchResultMsg{Emails: nil, Query: query, Source: source, Token: token, Err: err}
			}
			return SearchResultMsg{Emails: []*models.EmailData{}, Query: query, Source: source, Token: token}
		}
		if result.Emails == nil {
			result.Emails = []*models.EmailData{}
		}
		return SearchResultMsg{Emails: result.Emails, Scores: result.Scores, Query: query, Source: result.Source, Token: token}
	}
}

// performIMAPSearch performs a server-side IMAP search as a tea.Cmd.
func (m *Model) performIMAPSearch(query string) tea.Cmd {
	return m.performIMAPSearchWithToken(query, m.timeline.searchToken)
}

func (m *Model) performIMAPSearchWithToken(query string, token int) tea.Cmd {
	if query == "" {
		return nil
	}
	folder := m.currentFolder
	if isVirtualAllMailOnlyFolder(folder) {
		return func() tea.Msg {
			return SearchResultMsg{
				Emails: []*models.EmailData{},
				Query:  query,
				Source: "imap",
				Token:  token,
				Err:    fmt.Errorf("server search is unavailable in All Mail only; this inspector is local and read-only"),
			}
		}
	}
	return func() tea.Msg {
		emails, err := m.backend.SearchEmailsIMAP(folder, query)
		if err != nil {
			logger.Warn("IMAP search error: %v", err)
			return SearchResultMsg{Emails: []*models.EmailData{}, Query: query, Source: "imap", Token: token}
		}
		return SearchResultMsg{Emails: emails, Query: query, Source: "imap", Token: token}
	}
}

func scheduleTimelineSearchDebounce(token int, query string) tea.Cmd {
	return tea.Tick(300*time.Millisecond, func(time.Time) tea.Msg {
		return TimelineSearchDebounceMsg{Query: query, Token: token}
	})
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
