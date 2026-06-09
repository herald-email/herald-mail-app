package retrieval

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/herald-email/herald-mail-app/internal/models"
)

type fakeSource struct {
	keywordResults  []*models.EmailData
	semanticResults []*models.SemanticSearchResult
	semanticErr     error
	keywordCalls    int
	semanticCalls   int
}

func (f *fakeSource) SearchEmails(_, _ string, _ bool) ([]*models.EmailData, error) {
	f.keywordCalls++
	return f.keywordResults, nil
}

func (f *fakeSource) SearchEmailsCrossFolder(string) ([]*models.EmailData, error) {
	f.keywordCalls++
	return f.keywordResults, nil
}

func (f *fakeSource) SearchSemanticChunked(string, []float32, int, float64) ([]*models.SemanticSearchResult, error) {
	f.semanticCalls++
	if f.semanticErr != nil {
		return nil, f.semanticErr
	}
	return f.semanticResults, nil
}

type fakeEmbedder struct{}

func (fakeEmbedder) Embed(string) ([]float32, error) {
	return []float32{1, 0}, nil
}

func retrievalEmail(id string, date time.Time) *models.EmailData {
	return &models.EmailData{MessageID: id, Sender: "Herald Mail App", Subject: id, Date: date, Folder: "INBOX"}
}

func TestSearchHybridMergesKeywordAndSemanticResults(t *testing.T) {
	now := time.Date(2026, 6, 8, 10, 0, 0, 0, time.UTC)
	keyword := retrievalEmail("keyword", now)
	duplicate := retrievalEmail("duplicate", now.Add(-time.Minute))
	semantic := retrievalEmail("semantic", now.Add(-2*time.Minute))
	source := &fakeSource{
		keywordResults: []*models.EmailData{keyword, duplicate},
		semanticResults: []*models.SemanticSearchResult{
			{Email: semantic, Score: 0.82},
			{Email: duplicate, Score: 0.75},
		},
	}

	result, err := Search(context.Background(), source, fakeEmbedder{}, Request{
		Folder:   "INBOX",
		Query:    "Herald newsletter",
		Mode:     ModeHybrid,
		Limit:    10,
		MinScore: 0.3,
	})
	if err != nil {
		t.Fatalf("Search returned error: %v", err)
	}
	if source.keywordCalls != 1 || source.semanticCalls != 1 {
		t.Fatalf("calls keyword=%d semantic=%d", source.keywordCalls, source.semanticCalls)
	}
	gotIDs := []string{result.Emails[0].MessageID, result.Emails[1].MessageID, result.Emails[2].MessageID}
	wantIDs := []string{"keyword", "duplicate", "semantic"}
	for i := range wantIDs {
		if gotIDs[i] != wantIDs[i] {
			t.Fatalf("emails[%d] = %q, want %q; all %#v", i, gotIDs[i], wantIDs[i], gotIDs)
		}
	}
	if result.Scores["semantic"] != 0.82 || result.Scores["duplicate"] != 0.75 {
		t.Fatalf("scores = %#v", result.Scores)
	}
}

func TestSearchHybridFallsBackToKeywordWhenSemanticLegFails(t *testing.T) {
	keyword := retrievalEmail("keyword", time.Date(2026, 6, 8, 10, 0, 0, 0, time.UTC))
	source := &fakeSource{
		keywordResults: []*models.EmailData{keyword},
		semanticErr:    errors.New("semantic unavailable"),
	}

	result, err := Search(context.Background(), source, fakeEmbedder{}, Request{
		Folder: "INBOX",
		Query:  "Herald newsletter",
		Mode:   ModeHybrid,
		Limit:  10,
	})
	if err != nil {
		t.Fatalf("Search returned error: %v", err)
	}
	if len(result.Emails) != 1 || result.Emails[0].MessageID != "keyword" {
		t.Fatalf("emails = %#v", result.Emails)
	}
	if result.Scores != nil {
		t.Fatalf("scores = %#v, want nil after semantic fallback", result.Scores)
	}
}
