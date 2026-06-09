package agent

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/fugue-labs/gollem/core"
	"github.com/herald-email/herald-mail-app/internal/models"
	"github.com/herald-email/herald-mail-app/internal/retrieval"
)

const (
	defaultEmailToolFolder      = "INBOX"
	defaultEmailToolMaxResults  = 20
	defaultEmailToolContextSize = 1200
	defaultEmailToolMinScore    = 0.30
)

type EmailToolSource interface {
	SearchEmails(folder, query string, bodySearch bool) ([]*models.EmailData, error)
	SearchEmailsCrossFolder(query string) ([]*models.EmailData, error)
	SearchEmailsSemantic(folder, query string, limit int, minScore float64) ([]*models.EmailData, error)
	GetEmailByID(messageID string) (*models.EmailData, error)
	GetBodyText(messageID string) (string, error)
}

type EmailToolOptions struct {
	CurrentFolder   string
	MaxResults      int
	MaxContextChars int
	MinScore        float64
}

type EmailToolService struct {
	source EmailToolSource
	opts   EmailToolOptions
}

type FindEmailsParams struct {
	Query    string `json:"query,omitempty" jsonschema:"description=Keyword or semantic query to search for"`
	Topic    string `json:"topic,omitempty" jsonschema:"description=Topic to search for when query is empty"`
	Sender   string `json:"sender,omitempty" jsonschema:"description=Sender email/name filter"`
	Folder   string `json:"folder,omitempty" jsonschema:"description=Folder to search; defaults to the current folder"`
	Mode     string `json:"mode,omitempty" jsonschema:"enum=keyword,enum=semantic,enum=hybrid,description=Search mode; defaults to hybrid"`
	DateHint string `json:"date_hint,omitempty" jsonschema:"description=Natural-language date hint for the model to preserve in the query"`
	Unread   bool   `json:"unread,omitempty" jsonschema:"description=When true only return unread messages"`
	Limit    int    `json:"limit,omitempty" jsonschema:"description=Maximum result rows to return"`
}

type FindEmailsResult struct {
	Tool   string          `json:"tool"`
	Query  string          `json:"query"`
	Mode   string          `json:"mode"`
	Source string          `json:"source"`
	Total  int             `json:"total"`
	Capped bool            `json:"capped"`
	Emails []EmailMetadata `json:"emails"`
}

type GetEmailContextParams struct {
	MessageIDs []string `json:"message_ids" jsonschema:"description=Message IDs to inspect"`
	Limit      int      `json:"limit,omitempty" jsonschema:"description=Maximum messages to inspect"`
}

type EmailContextResult struct {
	Tool   string         `json:"tool"`
	Total  int            `json:"total"`
	Capped bool           `json:"capped"`
	Emails []EmailContext `json:"emails"`
}

type EmailContext struct {
	EmailMetadata
	BodySnippet string `json:"body_snippet,omitempty"`
	ThreadHint  string `json:"thread_hint,omitempty"`
}

type SummarizeEmailSetParams struct {
	MessageIDs []string `json:"message_ids" jsonschema:"description=Message IDs to summarize"`
	Topic      string   `json:"topic,omitempty" jsonschema:"description=Topic label for the summary"`
	Limit      int      `json:"limit,omitempty" jsonschema:"description=Maximum messages to summarize"`
}

type ExplainPeopleParams struct {
	MessageIDs []string `json:"message_ids" jsonschema:"description=Message IDs to inspect"`
	Limit      int      `json:"limit,omitempty" jsonschema:"description=Maximum messages to inspect"`
}

type PeopleExplanation struct {
	Tool     string   `json:"tool"`
	People   []Person `json:"people"`
	CitedIDs []string `json:"cited_ids,omitempty"`
}

type EmailMetadata struct {
	MessageID      string `json:"message_id"`
	Sender         string `json:"sender"`
	Subject        string `json:"subject"`
	Date           string `json:"date"`
	Folder         string `json:"folder"`
	Read           bool   `json:"read"`
	Starred        bool   `json:"starred"`
	Draft          bool   `json:"draft"`
	HasAttachments bool   `json:"has_attachments"`
}

func NewEmailToolService(source EmailToolSource, opts EmailToolOptions) *EmailToolService {
	if opts.MaxResults <= 0 {
		opts.MaxResults = defaultEmailToolMaxResults
	}
	if opts.MaxContextChars <= 0 {
		opts.MaxContextChars = defaultEmailToolContextSize
	}
	if opts.MinScore <= 0 {
		opts.MinScore = defaultEmailToolMinScore
	}
	return &EmailToolService{source: source, opts: opts}
}

func (s *EmailToolService) GollemTools() []core.Tool {
	if s == nil {
		return nil
	}
	return []core.Tool{
		core.FuncTool[FindEmailsParams](
			"find_emails",
			"Search email metadata by keyword, semantic meaning, sender, unread state, folder, and topic. Returns capped source message rows.",
			func(ctx context.Context, rc *core.RunContext, params FindEmailsParams) (FindEmailsResult, error) {
				params.Folder = s.folderFromRun(rc, params.Folder)
				return s.FindEmails(ctx, params)
			},
		),
		core.FuncTool[GetEmailContextParams](
			"get_email_context",
			"Fetch bounded context snippets for selected message IDs without raw MIME or unbounded bodies.",
			func(ctx context.Context, _ *core.RunContext, params GetEmailContextParams) (EmailContextResult, error) {
				return s.GetEmailContext(ctx, params)
			},
		),
		core.FuncTool[SummarizeEmailSetParams](
			"summarize_email_set",
			"Summarize an explicit bounded message set with people, dates, action items, open questions, and cited message IDs.",
			func(ctx context.Context, _ *core.RunContext, params SummarizeEmailSetParams) (EmailSummary, error) {
				return s.SummarizeEmailSet(ctx, params)
			},
		),
		core.FuncTool[ExplainPeopleParams](
			"explain_people",
			"Identify people involved in a bounded message set and explain likely roles with evidence message IDs.",
			func(ctx context.Context, _ *core.RunContext, params ExplainPeopleParams) (PeopleExplanation, error) {
				return s.ExplainPeople(ctx, params)
			},
		),
	}
}

func (s *EmailToolService) FindEmails(ctx context.Context, params FindEmailsParams) (FindEmailsResult, error) {
	if err := ctx.Err(); err != nil {
		return FindEmailsResult{}, err
	}
	if s == nil || s.source == nil {
		return FindEmailsResult{}, fmt.Errorf("email tools are not configured")
	}
	query := firstNonEmpty(params.Query, params.Topic, params.Sender, params.DateHint)
	query = strings.TrimSpace(query)
	if query == "" {
		return FindEmailsResult{}, fmt.Errorf("find_emails requires query, topic, sender, or date_hint")
	}
	mode := normalizeTimelineMode(params.Mode)
	if mode == "" || mode == TimelineModeExplicitIDs {
		mode = TimelineModeHybrid
	}
	folder := s.folder(params.Folder)
	limit := s.limit(params.Limit)
	retrievalLimit := limit
	if retrievalLimit < defaultEmailToolMaxResults {
		retrievalLimit = defaultEmailToolMaxResults
	}

	result, err := retrieval.Search(ctx, s.source, nil, retrieval.Request{
		Folder:   folder,
		Query:    query,
		Mode:     mode,
		Limit:    retrievalLimit,
		MinScore: s.opts.MinScore,
	})
	if err != nil {
		return FindEmailsResult{}, err
	}
	emails := filterEmailRows(result.Emails, params)
	total := len(emails)
	if len(emails) > limit {
		emails = emails[:limit]
	}
	return FindEmailsResult{
		Tool:   "find_emails",
		Query:  query,
		Mode:   mode,
		Source: result.Source,
		Total:  total,
		Capped: total > len(emails),
		Emails: emailMetadataRows(emails),
	}, nil
}

func (s *EmailToolService) GetEmailContext(ctx context.Context, params GetEmailContextParams) (EmailContextResult, error) {
	if err := ctx.Err(); err != nil {
		return EmailContextResult{}, err
	}
	contexts, total, capped, err := s.emailContexts(params.MessageIDs, params.Limit)
	if err != nil {
		return EmailContextResult{}, err
	}
	return EmailContextResult{Tool: "get_email_context", Total: total, Capped: capped, Emails: contexts}, nil
}

func (s *EmailToolService) SummarizeEmailSet(ctx context.Context, params SummarizeEmailSetParams) (EmailSummary, error) {
	if err := ctx.Err(); err != nil {
		return EmailSummary{}, err
	}
	contexts, _, _, err := s.emailContexts(params.MessageIDs, params.Limit)
	if err != nil {
		return EmailSummary{}, err
	}
	summary := EmailSummary{
		Topic:    strings.TrimSpace(params.Topic),
		People:   peopleFromContexts(contexts),
		CitedIDs: citedIDsFromContexts(contexts),
	}
	if summary.Topic == "" {
		summary.Topic = "selected emails"
	}
	if len(contexts) > 0 {
		summary.Summary = fmt.Sprintf("%d email(s) about %s. Latest subject: %s.", len(contexts), summary.Topic, contexts[0].Subject)
	}
	summary.Dates = datesFromContexts(contexts)
	summary.ActionItems = actionItemsFromContexts(contexts)
	summary.OpenQuestions = questionsFromContexts(contexts)
	return summary, nil
}

func (s *EmailToolService) ExplainPeople(ctx context.Context, params ExplainPeopleParams) (PeopleExplanation, error) {
	if err := ctx.Err(); err != nil {
		return PeopleExplanation{}, err
	}
	contexts, _, _, err := s.emailContexts(params.MessageIDs, params.Limit)
	if err != nil {
		return PeopleExplanation{}, err
	}
	return PeopleExplanation{
		Tool:     "explain_people",
		People:   peopleFromContexts(contexts),
		CitedIDs: citedIDsFromContexts(contexts),
	}, nil
}

func (s *EmailToolService) emailContexts(messageIDs []string, requestedLimit int) ([]EmailContext, int, bool, error) {
	if s == nil || s.source == nil {
		return nil, 0, false, fmt.Errorf("email tools are not configured")
	}
	ids := compactUniqueStrings(messageIDs)
	total := len(ids)
	limit := s.limit(requestedLimit)
	if len(ids) > limit {
		ids = ids[:limit]
	}
	contexts := make([]EmailContext, 0, len(ids))
	for _, id := range ids {
		email, err := s.source.GetEmailByID(id)
		if err != nil {
			return nil, total, total > len(ids), err
		}
		if email == nil {
			continue
		}
		body, err := s.source.GetBodyText(id)
		if err != nil {
			return nil, total, total > len(ids), err
		}
		contexts = append(contexts, EmailContext{
			EmailMetadata: metadataForEmail(email),
			BodySnippet:   boundedText(body, s.opts.MaxContextChars),
			ThreadHint:    threadHint(email.Subject),
		})
	}
	return contexts, total, total > len(ids), nil
}

func (s *EmailToolService) folder(explicit string) string {
	folder := strings.TrimSpace(explicit)
	if folder != "" {
		return folder
	}
	folder = strings.TrimSpace(s.opts.CurrentFolder)
	if folder != "" {
		return folder
	}
	return defaultEmailToolFolder
}

func (s *EmailToolService) folderFromRun(rc *core.RunContext, explicit string) string {
	if strings.TrimSpace(explicit) != "" {
		return strings.TrimSpace(explicit)
	}
	if input, ok := core.TryGetDeps[ChatInput](rc); ok && strings.TrimSpace(input.CurrentFolder) != "" {
		return strings.TrimSpace(input.CurrentFolder)
	}
	return s.folder("")
}

func (s *EmailToolService) limit(requested int) int {
	limit := s.opts.MaxResults
	if requested > 0 && requested < limit {
		limit = requested
	}
	if limit <= 0 {
		return defaultEmailToolMaxResults
	}
	return limit
}

func normalizeTimelineMode(mode string) string {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case TimelineModeExplicitIDs:
		return TimelineModeExplicitIDs
	case TimelineModeKeyword:
		return TimelineModeKeyword
	case TimelineModeSemantic:
		return TimelineModeSemantic
	case TimelineModeHybrid:
		return TimelineModeHybrid
	default:
		return ""
	}
}

func filterEmailRows(emails []*models.EmailData, params FindEmailsParams) []*models.EmailData {
	sender := strings.ToLower(strings.TrimSpace(params.Sender))
	out := make([]*models.EmailData, 0, len(emails))
	for _, email := range emails {
		if email == nil {
			continue
		}
		if sender != "" && !strings.Contains(strings.ToLower(email.Sender), sender) {
			continue
		}
		if params.Unread && email.IsRead {
			continue
		}
		out = append(out, email)
	}
	return out
}

func emailMetadataRows(emails []*models.EmailData) []EmailMetadata {
	rows := make([]EmailMetadata, 0, len(emails))
	for _, email := range emails {
		if email == nil {
			continue
		}
		rows = append(rows, metadataForEmail(email))
	}
	return rows
}

func metadataForEmail(email *models.EmailData) EmailMetadata {
	return EmailMetadata{
		MessageID:      email.MessageID,
		Sender:         email.Sender,
		Subject:        email.Subject,
		Date:           email.Date.Format("2006-01-02"),
		Folder:         email.Folder,
		Read:           email.IsRead,
		Starred:        email.IsStarred,
		Draft:          email.IsDraft,
		HasAttachments: email.HasAttachments,
	}
}

func boundedText(value string, limit int) string {
	value = strings.TrimSpace(value)
	if value == "" || limit <= 0 {
		return ""
	}
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	return string(runes[:limit]) + "...[truncated]"
}

func threadHint(subject string) string {
	subject = strings.TrimSpace(subject)
	lower := strings.ToLower(subject)
	for _, prefix := range []string{"re:", "fwd:", "fw:"} {
		if strings.HasPrefix(lower, prefix) {
			return strings.TrimSpace(subject[len(prefix):])
		}
	}
	return subject
}

func peopleFromContexts(contexts []EmailContext) []Person {
	seen := make(map[string]bool)
	people := make([]Person, 0, len(contexts))
	for _, ctx := range contexts {
		sender := strings.TrimSpace(ctx.Sender)
		if sender == "" {
			continue
		}
		key := strings.ToLower(sender)
		if seen[key] {
			continue
		}
		seen[key] = true
		people = append(people, Person{NameOrEmail: sender, Role: "sender", EvidenceID: ctx.MessageID})
	}
	sort.SliceStable(people, func(i, j int) bool {
		return strings.ToLower(people[i].NameOrEmail) < strings.ToLower(people[j].NameOrEmail)
	})
	return people
}

func citedIDsFromContexts(contexts []EmailContext) []string {
	ids := make([]string, 0, len(contexts))
	for _, ctx := range contexts {
		if ctx.MessageID != "" {
			ids = append(ids, ctx.MessageID)
		}
	}
	return ids
}

func datesFromContexts(contexts []EmailContext) []string {
	seen := make(map[string]bool)
	var dates []string
	for _, ctx := range contexts {
		if ctx.Date == "" || seen[ctx.Date] {
			continue
		}
		seen[ctx.Date] = true
		dates = append(dates, ctx.Date)
	}
	return dates
}

func actionItemsFromContexts(contexts []EmailContext) []string {
	var items []string
	for _, ctx := range contexts {
		for _, sentence := range splitSentences(ctx.BodySnippet) {
			lower := strings.ToLower(sentence)
			if strings.Contains(lower, "action item") || strings.Contains(lower, "please ") || strings.Contains(lower, " by ") {
				items = append(items, boundedText(sentence, 180))
				break
			}
		}
	}
	return compactUniqueStrings(items)
}

func questionsFromContexts(contexts []EmailContext) []string {
	var questions []string
	for _, ctx := range contexts {
		for _, sentence := range splitSentences(ctx.BodySnippet) {
			if strings.Contains(sentence, "?") {
				questions = append(questions, boundedText(sentence, 180))
			}
		}
	}
	return compactUniqueStrings(questions)
}

func splitSentences(text string) []string {
	fields := strings.FieldsFunc(text, func(r rune) bool {
		return r == '.' || r == '\n'
	})
	out := make([]string, 0, len(fields))
	for _, field := range fields {
		field = strings.TrimSpace(field)
		if field != "" {
			out = append(out, field)
		}
	}
	return out
}

func compactUniqueStrings(values []string) []string {
	seen := make(map[string]bool, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
