package agent

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/fugue-labs/gollem/core"
	"github.com/herald-email/herald-mail-app/internal/models"
)

type fakeEmailToolSource struct {
	keywordResults  []*models.EmailData
	semanticResults []*models.EmailData
	emailsByID      map[string]*models.EmailData
	bodyByID        map[string]string

	searchCalls      int
	semanticCalls    int
	getByIDCalls     int
	getBodyTextCalls int
	lastFolder       string
	lastQuery        string
}

func (f *fakeEmailToolSource) SearchEmails(folder, query string, _ bool) ([]*models.EmailData, error) {
	f.searchCalls++
	f.lastFolder = folder
	f.lastQuery = query
	return f.keywordResults, nil
}

func (f *fakeEmailToolSource) SearchEmailsSemantic(folder, query string, _ int, _ float64) ([]*models.EmailData, error) {
	f.semanticCalls++
	f.lastFolder = folder
	f.lastQuery = query
	return f.semanticResults, nil
}

func (f *fakeEmailToolSource) GetEmailByID(messageID string) (*models.EmailData, error) {
	f.getByIDCalls++
	if f.emailsByID == nil {
		return nil, nil
	}
	return f.emailsByID[messageID], nil
}

func (f *fakeEmailToolSource) GetBodyText(messageID string) (string, error) {
	f.getBodyTextCalls++
	if f.bodyByID == nil {
		return "", nil
	}
	return f.bodyByID[messageID], nil
}

func toolEmail(id, sender, subject string) *models.EmailData {
	return &models.EmailData{
		MessageID: id,
		Sender:    sender,
		Subject:   subject,
		Folder:    "INBOX",
		Date:      time.Date(2026, 6, 6, 12, 0, 0, 0, time.UTC),
		IsRead:    false,
	}
}

func TestEmailToolServiceFindEmailsKeywordCapsMetadata(t *testing.T) {
	source := &fakeEmailToolSource{
		keywordResults: []*models.EmailData{
			toolEmail("msg-1", "alice@example.com", "Invoice one"),
			toolEmail("msg-2", "bob@example.com", "Invoice two"),
			toolEmail("msg-3", "cora@example.com", "Invoice three"),
		},
	}
	service := NewEmailToolService(source, EmailToolOptions{CurrentFolder: "INBOX", MaxResults: 2})

	result, err := service.FindEmails(context.Background(), FindEmailsParams{
		Query: "invoice",
		Mode:  "keyword",
		Limit: 10,
	})
	if err != nil {
		t.Fatalf("FindEmails returned error: %v", err)
	}
	if source.searchCalls != 1 || source.semanticCalls != 0 {
		t.Fatalf("search calls keyword=%d semantic=%d", source.searchCalls, source.semanticCalls)
	}
	if source.lastFolder != "INBOX" || source.lastQuery != "invoice" {
		t.Fatalf("search folder/query = %q/%q", source.lastFolder, source.lastQuery)
	}
	if result.Total != 3 || !result.Capped || len(result.Emails) != 2 {
		t.Fatalf("result total=%d capped=%v len=%d", result.Total, result.Capped, len(result.Emails))
	}
	if result.Emails[0].MessageID != "msg-1" || result.Emails[0].Sender != "alice@example.com" {
		t.Fatalf("first metadata row = %#v", result.Emails[0])
	}
}

func TestEmailToolServiceContextSummaryAndPeopleAreBounded(t *testing.T) {
	source := &fakeEmailToolSource{
		emailsByID: map[string]*models.EmailData{
			"msg-1": toolEmail("msg-1", "Alice Example <alice@example.com>", "Budget review"),
			"msg-2": toolEmail("msg-2", "bob@example.com", "Budget follow-up"),
		},
		bodyByID: map[string]string{
			"msg-1": "Alice asked Bob to review the budget by Friday. " + strings.Repeat("More budget context. ", 40),
			"msg-2": "Bob said the action item is to send numbers by Monday.",
		},
	}
	service := NewEmailToolService(source, EmailToolOptions{CurrentFolder: "INBOX", MaxContextChars: 48, MaxResults: 5})

	ctxResult, err := service.GetEmailContext(context.Background(), GetEmailContextParams{MessageIDs: []string{"msg-1", "msg-2"}})
	if err != nil {
		t.Fatalf("GetEmailContext returned error: %v", err)
	}
	if len(ctxResult.Emails) != 2 {
		t.Fatalf("context emails = %d", len(ctxResult.Emails))
	}
	if len([]rune(ctxResult.Emails[0].BodySnippet)) > 64 {
		t.Fatalf("body snippet was not bounded: %q", ctxResult.Emails[0].BodySnippet)
	}

	summary, err := service.SummarizeEmailSet(context.Background(), SummarizeEmailSetParams{MessageIDs: []string{"msg-1", "msg-2"}, Topic: "budget"})
	if err != nil {
		t.Fatalf("SummarizeEmailSet returned error: %v", err)
	}
	if summary.Topic != "budget" || len(summary.CitedIDs) != 2 {
		t.Fatalf("summary = %#v", summary)
	}
	if len(summary.People) == 0 || summary.People[0].EvidenceID == "" {
		t.Fatalf("summary people missing evidence: %#v", summary.People)
	}
	if len(summary.ActionItems) == 0 {
		t.Fatalf("expected deterministic action item in summary: %#v", summary)
	}

	people, err := service.ExplainPeople(context.Background(), ExplainPeopleParams{MessageIDs: []string{"msg-1", "msg-2"}})
	if err != nil {
		t.Fatalf("ExplainPeople returned error: %v", err)
	}
	if len(people.People) != 2 {
		t.Fatalf("people = %#v", people.People)
	}
}

func TestEmailToolServiceExposesGollemToolsAndRunnerCanCallThem(t *testing.T) {
	source := &fakeEmailToolSource{
		keywordResults: []*models.EmailData{toolEmail("msg-1", "alice@example.com", "Invoice one")},
	}
	service := NewEmailToolService(source, EmailToolOptions{MaxResults: 5})
	tools := service.GollemTools()
	if len(tools) != 4 {
		t.Fatalf("GollemTools len = %d, want 4", len(tools))
	}
	names := map[string]bool{}
	for _, tool := range tools {
		names[tool.Definition.Name] = true
	}
	for _, want := range []string{"find_emails", "get_email_context", "summarize_email_set", "explain_people"} {
		if !names[want] {
			t.Fatalf("missing tool %s in %#v", want, names)
		}
	}
	for _, tool := range tools {
		if tool.Definition.Strict != nil && *tool.Definition.Strict {
			t.Fatalf("%s should not force strict tool schemas; OpenAI requires strict schemas to list every property as required", tool.Definition.Name)
		}
	}

	model := core.NewTestModel(
		core.ToolCallResponse("find_emails", `{"query":"invoice","mode":"keyword"}`),
		core.ToolCallResponse("final_result", `{"reply":"Found one invoice.","timeline":{"mode":"explicit_ids","message_ids":["msg-1"],"label":"invoice"}}`),
	)
	runner := NewGollemRunner(model, core.WithTools[ChatResult](tools...))

	result, err := runner.Run(context.Background(), ChatInput{UserMessage: "find invoices", CurrentFolder: "INBOX"})
	if err != nil {
		t.Fatalf("runner returned error: %v", err)
	}
	if source.searchCalls != 1 || source.lastFolder != "INBOX" {
		t.Fatalf("tool search calls=%d folder=%q", source.searchCalls, source.lastFolder)
	}
	if result.Timeline == nil || result.Timeline.MessageIDs[0] != "msg-1" {
		t.Fatalf("runner result = %#v", result)
	}
}
