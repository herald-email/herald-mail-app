package retrieval

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/herald-email/herald-mail-app/internal/ai"
	"github.com/herald-email/herald-mail-app/internal/models"
)

const (
	ModeKeyword  = "keyword"
	ModeSemantic = "semantic"
	ModeHybrid   = "hybrid"
	ModeBody     = "body"
	ModeCross    = "cross"
)

type Source interface {
	SearchEmails(folder, query string, bodySearch bool) ([]*models.EmailData, error)
	SearchEmailsCrossFolder(query string) ([]*models.EmailData, error)
}

type SemanticEmailSource interface {
	SearchEmailsSemantic(folder, query string, limit int, minScore float64) ([]*models.EmailData, error)
}

type SemanticChunkSource interface {
	SearchSemanticChunked(folder string, queryVec []float32, limit int, minScore float64) ([]*models.SemanticSearchResult, error)
}

type Embedder interface {
	Embed(text string) ([]float32, error)
}

type Request struct {
	Folder   string
	Query    string
	Mode     string
	Limit    int
	MinScore float64
}

type Result struct {
	Query  string
	Mode   string
	Source string
	Total  int
	Capped bool
	Emails []*models.EmailData
	Scores map[string]float64
}

func Search(ctx context.Context, source Source, embedder Embedder, req Request) (Result, error) {
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	if source == nil {
		return Result{}, fmt.Errorf("retrieval source is not configured")
	}
	query := strings.TrimSpace(req.Query)
	if query == "" {
		return Result{Query: query, Mode: normalizeMode(req.Mode)}, nil
	}
	mode := normalizeMode(req.Mode)
	limit := req.Limit
	if limit <= 0 {
		limit = 100
	}
	minScore := req.MinScore
	if minScore <= 0 {
		minScore = 0.30
	}

	var (
		emails []*models.EmailData
		scores map[string]float64
		err    error
		src    = mode
	)
	switch mode {
	case ModeKeyword:
		src = "local"
		emails, err = source.SearchEmails(req.Folder, query, false)
	case ModeBody:
		src = "fts"
		emails, err = source.SearchEmails(req.Folder, query, true)
	case ModeCross:
		src = "cross"
		emails, err = source.SearchEmailsCrossFolder(query)
	case ModeSemantic:
		src = "semantic"
		emails, scores, err = searchSemantic(ctx, source, embedder, req.Folder, query, limit, minScore)
	case ModeHybrid:
		src = "hybrid"
		var keywordEmails []*models.EmailData
		keywordEmails, err = source.SearchEmails(req.Folder, query, false)
		if err != nil {
			break
		}
		emails = keywordEmails
		var semanticEmails []*models.EmailData
		semanticEmails, scores, err = searchSemantic(ctx, source, embedder, req.Folder, query, limit, minScore)
		if err != nil {
			err = nil
			break
		}
		emails = mergeEmailRows(keywordEmails, semanticEmails)
	default:
		return Result{}, fmt.Errorf("unsupported retrieval mode: %s", req.Mode)
	}
	if err != nil {
		return Result{}, err
	}
	total := len(emails)
	if total > limit {
		emails = emails[:limit]
	}
	return Result{
		Query:  query,
		Mode:   mode,
		Source: src,
		Total:  total,
		Capped: total > len(emails),
		Emails: emails,
		Scores: scores,
	}, nil
}

func normalizeMode(mode string) string {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case ModeKeyword:
		return ModeKeyword
	case ModeSemantic:
		return ModeSemantic
	case ModeBody, "fts":
		return ModeBody
	case ModeCross, "cross_folder":
		return ModeCross
	default:
		return ModeHybrid
	}
}

func searchSemantic(ctx context.Context, source Source, embedder Embedder, folder, query string, limit int, minScore float64) ([]*models.EmailData, map[string]float64, error) {
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}
	if embedder != nil {
		if chunkSource, ok := source.(SemanticChunkSource); ok {
			vec, err := embedder.Embed(ai.BuildQueryText(query))
			if err != nil {
				return nil, nil, fmt.Errorf("semantic search unavailable: %w", err)
			}
			results, err := chunkSource.SearchSemanticChunked(folder, vec, limit, minScore)
			if err != nil {
				return nil, nil, err
			}
			return emailsAndScoresFromSemanticResults(results)
		}
	}
	if semanticSource, ok := source.(SemanticEmailSource); ok {
		emails, err := semanticSource.SearchEmailsSemantic(folder, query, limit, minScore)
		return emails, nil, err
	}
	return nil, nil, fmt.Errorf("semantic search unavailable: source does not support semantic search")
}

func emailsAndScoresFromSemanticResults(results []*models.SemanticSearchResult) ([]*models.EmailData, map[string]float64, error) {
	sort.SliceStable(results, func(i, j int) bool {
		if results[i] == nil {
			return false
		}
		if results[j] == nil {
			return true
		}
		return results[i].Score > results[j].Score
	})
	emails := make([]*models.EmailData, 0, len(results))
	scores := make(map[string]float64, len(results))
	for _, result := range results {
		if result == nil || result.Email == nil {
			continue
		}
		if prev, ok := scores[result.Email.MessageID]; !ok || result.Score > prev {
			scores[result.Email.MessageID] = result.Score
		}
		emails = append(emails, result.Email)
	}
	if len(scores) == 0 {
		scores = nil
	}
	return emails, scores, nil
}

func mergeEmailRows(primary, secondary []*models.EmailData) []*models.EmailData {
	merged := make([]*models.EmailData, 0, len(primary)+len(secondary))
	seen := make(map[string]bool, len(primary)+len(secondary))
	for _, email := range primary {
		if email == nil || seen[email.MessageID] {
			continue
		}
		seen[email.MessageID] = true
		merged = append(merged, email)
	}
	for _, email := range secondary {
		if email == nil || seen[email.MessageID] {
			continue
		}
		seen[email.MessageID] = true
		merged = append(merged, email)
	}
	return merged
}
