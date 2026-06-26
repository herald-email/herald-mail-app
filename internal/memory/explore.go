package memory

import (
	"context"
	"sort"
	"strings"
	"time"
)

const (
	ExploreFilterAll       = "all"
	ExploreFilterOpenLoops = "open_loops"
	ExploreFilterWaiting   = "waiting"
	ExploreFilterReview    = "review"
	ExploreFilterStale     = "stale"
	ExploreFilterConflicts = "conflicts"

	ExploreFilterSourceEmail      = "source_email"
	ExploreFilterSourceSentEmail  = "source_sent_email"
	ExploreFilterSourceObsidian   = "source_obsidian"
	ExploreFilterSourceCalendar   = "source_calendar"
	ExploreFilterSourceAttachment = "source_attachment"
	ExploreFilterSourceResearch   = "source_research"

	ExploreDateAny = "any"
	ExploreDate7d  = "7d"
	ExploreDate30d = "30d"
	ExploreDate90d = "90d"

	ExploreRowMemory = "memory"
	ExploreRowTrack  = "track"
)

type ExploreQuery struct {
	Text      string    `json:"text,omitempty" yaml:"text,omitempty"`
	Filter    string    `json:"filter,omitempty" yaml:"filter,omitempty"`
	DateRange string    `json:"date_range,omitempty" yaml:"date_range,omitempty"`
	Limit     int       `json:"limit,omitempty" yaml:"limit,omitempty"`
	Settings  Settings  `json:"-" yaml:"-"`
	Now       time.Time `json:"-" yaml:"-"`
}

type FacetCount struct {
	ID    string `json:"id" yaml:"id"`
	Label string `json:"label" yaml:"label"`
	Count int    `json:"count" yaml:"count"`
}

type ExploreFacets struct {
	Filters []FacetCount `json:"filters" yaml:"filters"`
	Tracks  []FacetCount `json:"tracks" yaml:"tracks"`
	Sources []FacetCount `json:"sources" yaml:"sources"`
	Dates   []FacetCount `json:"dates" yaml:"dates"`
}

type SourceSummary struct {
	SourceType string    `json:"source_type" yaml:"source_type"`
	Label      string    `json:"label" yaml:"label"`
	Folder     string    `json:"folder,omitempty" yaml:"folder,omitempty"`
	Date       time.Time `json:"date,omitempty" yaml:"date,omitempty"`
	Snippet    string    `json:"snippet,omitempty" yaml:"snippet,omitempty"`
}

type ExploreRow struct {
	ID             string          `json:"id" yaml:"id"`
	RowType        string          `json:"row_type" yaml:"row_type"`
	Title          string          `json:"title" yaml:"title"`
	Summary        string          `json:"summary,omitempty" yaml:"summary,omitempty"`
	Kind           string          `json:"kind,omitempty" yaml:"kind,omitempty"`
	Status         string          `json:"status,omitempty" yaml:"status,omitempty"`
	Freshness      string          `json:"freshness,omitempty" yaml:"freshness,omitempty"`
	Confidence     float64         `json:"confidence,omitempty" yaml:"confidence,omitempty"`
	People         []string        `json:"people,omitempty" yaml:"people,omitempty"`
	Company        string          `json:"company,omitempty" yaml:"company,omitempty"`
	Domain         string          `json:"domain,omitempty" yaml:"domain,omitempty"`
	Topic          string          `json:"topic,omitempty" yaml:"topic,omitempty"`
	ObsidianTarget string          `json:"obsidian_target,omitempty" yaml:"obsidian_target,omitempty"`
	Tags           []string        `json:"tags,omitempty" yaml:"tags,omitempty"`
	PromptVersion  string          `json:"prompt_version,omitempty" yaml:"prompt_version,omitempty"`
	ReviewReason   string          `json:"review_reason,omitempty" yaml:"review_reason,omitempty"`
	LastActivityAt time.Time       `json:"last_activity_at,omitempty" yaml:"last_activity_at,omitempty"`
	SourceLabel    string          `json:"source_label,omitempty" yaml:"source_label,omitempty"`
	Sources        []SourceSummary `json:"sources,omitempty" yaml:"sources,omitempty"`
	Memory         *Memory         `json:"memory,omitempty" yaml:"memory,omitempty"`
	Track          *Track          `json:"track,omitempty" yaml:"track,omitempty"`
}

type ExploreResult struct {
	Query     ExploreQuery  `json:"query" yaml:"query"`
	Rows      []ExploreRow  `json:"rows" yaml:"rows"`
	Facets    ExploreFacets `json:"facets" yaml:"facets"`
	Total     int           `json:"total" yaml:"total"`
	Capped    bool          `json:"capped" yaml:"capped"`
	Generated time.Time     `json:"generated_at" yaml:"generated_at"`
}

func (s *Service) Explore(ctx context.Context, q ExploreQuery) (ExploreResult, error) {
	if s == nil || s.store == nil {
		return ExploreResult{}, nil
	}
	memories, err := s.store.EffectiveList(ctx, s.settings)
	if err != nil {
		return ExploreResult{}, err
	}
	q.Settings = s.settings
	if q.Now.IsZero() {
		q.Now = s.now()
	}
	return BuildExploreResult(memories, q), nil
}

func BuildExploreResult(memories []Memory, q ExploreQuery) ExploreResult {
	q.Filter = normalizeExploreFilter(q.Filter)
	q.DateRange = normalizeExploreDateRange(q.DateRange)
	if q.Limit <= 0 {
		q.Limit = 100
	}
	if q.Now.IsZero() {
		q.Now = time.Now()
	}
	q.Settings.ApplyDefaults()
	memories = append([]Memory(nil), memories...)
	SortMemoriesNewestFirst(memories)
	facets := buildExploreFacets(memories, q)
	rows := exploreRowsForMemories(memories, q)
	total := len(rows)
	capped := total > q.Limit
	if capped {
		rows = rows[:q.Limit]
	}
	return ExploreResult{
		Query:     q,
		Rows:      rows,
		Facets:    facets,
		Total:     total,
		Capped:    capped,
		Generated: q.Now,
	}
}

func exploreRowsForMemories(memories []Memory, q ExploreQuery) []ExploreRow {
	rows := make([]ExploreRow, 0, len(memories))
	for _, track := range BuildTracksFromMemories(memories, q.Settings, q.Now) {
		row := rowFromTrack(track)
		if exploreRowMatches(row, q) {
			rows = append(rows, row)
		}
	}
	for _, mem := range memories {
		row := rowFromMemory(mem, q.Settings)
		if exploreRowMatches(row, q) {
			rows = append(rows, row)
		}
	}
	sortExploreRows(rows)
	return rows
}

func rowFromMemory(mem Memory, settings Settings) ExploreRow {
	mem = PrepareMemoryForAppend(mem, mem.CreatedAt)
	sources := sourceSummaries(mem.Evidence)
	sourceLabel := ""
	if len(sources) > 0 {
		sourceLabel = sources[0].SourceType
		if sources[0].Folder != "" {
			sourceLabel = sources[0].Folder
		}
	}
	memoryCopy := mem
	return ExploreRow{
		ID:             mem.ID,
		RowType:        ExploreRowMemory,
		Title:          memoryExploreTitle(mem),
		Summary:        memorySummary(mem),
		Kind:           mem.Kind,
		Status:         mem.Status,
		Freshness:      mem.Freshness,
		Confidence:     mem.Confidence,
		People:         CompactStrings(mem.People),
		Company:        mem.Company,
		Domain:         mem.Domain,
		Topic:          mem.Topic,
		ObsidianTarget: mem.ObsidianTarget,
		Tags:           CompactStrings(mem.Tags),
		PromptVersion:  mem.PromptVersion,
		ReviewReason:   reviewReason(mem, settings),
		LastActivityAt: activityTime(mem),
		SourceLabel:    sourceLabel,
		Sources:        sources,
		Memory:         &memoryCopy,
	}
}

func rowFromTrack(track Track) ExploreRow {
	trackCopy := track
	return ExploreRow{
		ID:             track.ID,
		RowType:        ExploreRowTrack,
		Title:          trackExploreTitle(track),
		Summary:        firstNonEmpty(firstExploreString(track.OpenLoops), firstExploreString(track.Commitments), firstExploreString(track.Claims)),
		Kind:           KindTrackStatus,
		Status:         track.Status,
		People:         CompactStrings(track.People),
		Company:        track.Company,
		Domain:         track.Domain,
		Topic:          track.Topic,
		LastActivityAt: track.LastActivityAt,
		SourceLabel:    sourceLabel(track.Evidence),
		Sources:        sourceSummaries(track.Evidence),
		Track:          &trackCopy,
	}
}

func exploreRowMatches(row ExploreRow, q ExploreQuery) bool {
	if !rowMatchesDateRange(row, q) {
		return false
	}
	if !rowMatchesFilter(row, q.Filter, q.Settings) {
		return false
	}
	text := strings.TrimSpace(q.Text)
	if text == "" {
		return true
	}
	return rowContainsText(row, text)
}

func rowMatchesFilter(row ExploreRow, filter string, settings Settings) bool {
	switch normalizeExploreFilter(filter) {
	case ExploreFilterAll:
		return true
	case ExploreFilterOpenLoops:
		return row.Kind == KindOpenQuestion || row.Kind == KindDeadline || row.Kind == KindCommitment || row.Status == StatusWaiting || (row.Track != nil && len(row.Track.OpenLoops) > 0)
	case ExploreFilterWaiting:
		return strings.EqualFold(row.Status, StatusWaiting)
	case ExploreFilterReview:
		if row.Memory != nil {
			return memoryNeedsReview(*row.Memory, settings)
		}
		return strings.EqualFold(row.Status, StatusConflict) || strings.EqualFold(row.Status, StatusSourceMissing)
	case ExploreFilterStale:
		return strings.EqualFold(row.Status, StatusStale) || strings.EqualFold(row.Freshness, FreshnessStale)
	case ExploreFilterConflicts:
		return strings.EqualFold(row.Status, StatusConflict)
	case ExploreFilterSourceEmail:
		return rowHasSource(row, SourceEmail)
	case ExploreFilterSourceSentEmail:
		return rowHasSource(row, SourceSentEmail)
	case ExploreFilterSourceObsidian:
		return rowHasSource(row, SourceObsidian)
	case ExploreFilterSourceCalendar:
		return rowHasSource(row, SourceCalendar)
	case ExploreFilterSourceAttachment:
		return rowHasSource(row, SourceAttachment)
	case ExploreFilterSourceResearch:
		return rowHasSource(row, SourceResearch)
	default:
		return true
	}
}

func rowMatchesDateRange(row ExploreRow, q ExploreQuery) bool {
	days := exploreDateRangeDays(q.DateRange)
	if days <= 0 {
		return true
	}
	activity := row.LastActivityAt
	if activity.IsZero() {
		return false
	}
	return !activity.Before(q.Now.AddDate(0, 0, -days))
}

func rowHasSource(row ExploreRow, sourceType string) bool {
	for _, source := range row.Sources {
		if source.SourceType == sourceType {
			return true
		}
	}
	return false
}

func rowContainsText(row ExploreRow, text string) bool {
	haystack := strings.Join([]string{
		row.ID,
		row.RowType,
		row.Title,
		row.Summary,
		row.Kind,
		row.Status,
		row.Freshness,
		row.Company,
		row.Domain,
		row.Topic,
		row.ObsidianTarget,
		strings.Join(row.People, " "),
		strings.Join(row.Tags, " "),
	}, " ")
	if containsFold(haystack, text) {
		return true
	}
	for _, source := range row.Sources {
		if containsFold(source.Label, text) || containsFold(source.Folder, text) || containsFold(source.Snippet, text) || containsFold(source.SourceType, text) {
			return true
		}
	}
	return false
}

func buildExploreFacets(memories []Memory, q ExploreQuery) ExploreFacets {
	rows := exploreRowsForMemories(memories, ExploreQuery{
		Filter:    ExploreFilterAll,
		DateRange: q.DateRange,
		Text:      q.Text,
		Settings:  q.Settings,
		Now:       q.Now,
		Limit:     100000,
	})
	count := func(filter string) int {
		total := 0
		for _, row := range rows {
			if rowMatchesFilter(row, filter, q.Settings) {
				total++
			}
		}
		return total
	}
	sourceCount := func(sourceType string) int {
		total := 0
		for _, row := range rows {
			if rowHasSource(row, sourceType) {
				total++
			}
		}
		return total
	}
	dateCount := func(dateRange string) int {
		total := 0
		for _, row := range rows {
			if rowMatchesDateRange(row, ExploreQuery{DateRange: dateRange, Now: q.Now}) {
				total++
			}
		}
		return total
	}
	return ExploreFacets{
		Filters: []FacetCount{
			{ID: ExploreFilterAll, Label: "All memories", Count: count(ExploreFilterAll)},
			{ID: ExploreFilterOpenLoops, Label: "Open loops", Count: count(ExploreFilterOpenLoops)},
			{ID: ExploreFilterWaiting, Label: "Waiting", Count: count(ExploreFilterWaiting)},
			{ID: ExploreFilterReview, Label: "Review", Count: count(ExploreFilterReview)},
			{ID: ExploreFilterStale, Label: "Stale", Count: count(ExploreFilterStale)},
			{ID: ExploreFilterConflicts, Label: "Conflicts", Count: count(ExploreFilterConflicts)},
		},
		Tracks: []FacetCount{
			{ID: "people", Label: "People", Count: uniqueCount(memories, func(m Memory) []string { return m.People })},
			{ID: "companies", Label: "Companies", Count: uniqueCount(memories, func(m Memory) []string { return []string{m.Company} })},
			{ID: "topics", Label: "Topics", Count: uniqueCount(memories, func(m Memory) []string { return []string{m.Topic} })},
			{ID: "tracks", Label: "Tracks", Count: len(BuildTracksFromMemories(memories, q.Settings, q.Now))},
		},
		Sources: []FacetCount{
			{ID: ExploreFilterSourceEmail, Label: "INBOX", Count: sourceCount(SourceEmail)},
			{ID: ExploreFilterSourceSentEmail, Label: "Sent", Count: sourceCount(SourceSentEmail)},
			{ID: ExploreFilterSourceObsidian, Label: "Obsidian", Count: sourceCount(SourceObsidian)},
			{ID: ExploreFilterSourceCalendar, Label: "Calendar", Count: sourceCount(SourceCalendar)},
			{ID: ExploreFilterSourceAttachment, Label: "Attachments", Count: sourceCount(SourceAttachment)},
			{ID: ExploreFilterSourceResearch, Label: "Research", Count: sourceCount(SourceResearch)},
		},
		Dates: []FacetCount{
			{ID: ExploreDateAny, Label: "Any time", Count: len(rows)},
			{ID: ExploreDate7d, Label: "Last 7 days", Count: dateCount(ExploreDate7d)},
			{ID: ExploreDate30d, Label: "Last 30 days", Count: dateCount(ExploreDate30d)},
			{ID: ExploreDate90d, Label: "Last 90 days", Count: dateCount(ExploreDate90d)},
		},
	}
}

func sourceSummaries(evidence []Evidence) []SourceSummary {
	out := make([]SourceSummary, 0, len(evidence))
	for _, item := range NormalizeEvidenceList(evidence) {
		label := displayEvidenceLabel(item)
		if label == "" {
			continue
		}
		out = append(out, SourceSummary{
			SourceType: item.SourceType,
			Label:      label,
			Folder:     item.Folder,
			Date:       item.Date,
			Snippet:    item.Snippet,
		})
	}
	return out
}

func sourceLabel(evidence []Evidence) string {
	for _, item := range NormalizeEvidenceList(evidence) {
		if item.Folder != "" {
			return item.Folder
		}
		if item.SourceType != "" {
			return item.SourceType
		}
	}
	return ""
}

func reviewReason(mem Memory, settings Settings) string {
	if reason := strings.TrimSpace(mem.Details.ReviewReason); reason != "" {
		return reason
	}
	if memoryNeedsReview(mem, settings) {
		switch strings.ToLower(strings.TrimSpace(mem.Status)) {
		case StatusConflict:
			return "conflicting source-backed memory"
		case StatusSourceMissing:
			return "source evidence is missing"
		}
		if settings.UpdateRules.LowConfidenceDisposition == LowConfidenceReview {
			return "below configured review threshold"
		}
		return "needs review"
	}
	return ""
}

func sortExploreRows(rows []ExploreRow) {
	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].LastActivityAt.Equal(rows[j].LastActivityAt) {
			if rows[i].RowType != rows[j].RowType {
				return rows[i].RowType == ExploreRowMemory
			}
			return rows[i].ID < rows[j].ID
		}
		return rows[i].LastActivityAt.After(rows[j].LastActivityAt)
	})
}

func memoryExploreTitle(mem Memory) string {
	topic := strings.TrimSpace(mem.Topic)
	company := strings.TrimSpace(mem.Company)
	if company != "" && topic != "" && !containsFold(topic, company) {
		return company + " - " + topic
	}
	return firstNonEmpty(topic, mem.Claim, mem.Kind, "Memory")
}

func trackExploreTitle(track Track) string {
	topic := strings.TrimSpace(track.Topic)
	company := strings.TrimSpace(track.Company)
	if company != "" && topic != "" && !containsFold(topic, company) {
		return company + " - " + topic
	}
	return firstNonEmpty(topic, company, track.Domain, "Track")
}

func normalizeExploreFilter(filter string) string {
	switch strings.ToLower(strings.TrimSpace(filter)) {
	case "", ExploreFilterAll:
		return ExploreFilterAll
	case ExploreFilterOpenLoops, "open", "loops":
		return ExploreFilterOpenLoops
	case ExploreFilterWaiting, "waiting_on_me":
		return ExploreFilterWaiting
	case ExploreFilterReview, "review_needed":
		return ExploreFilterReview
	case ExploreFilterStale:
		return ExploreFilterStale
	case ExploreFilterConflicts, "conflict":
		return ExploreFilterConflicts
	case ExploreFilterSourceEmail, SourceEmail:
		return ExploreFilterSourceEmail
	case ExploreFilterSourceSentEmail, SourceSentEmail:
		return ExploreFilterSourceSentEmail
	case ExploreFilterSourceObsidian, SourceObsidian:
		return ExploreFilterSourceObsidian
	case ExploreFilterSourceCalendar, SourceCalendar:
		return ExploreFilterSourceCalendar
	case ExploreFilterSourceAttachment, SourceAttachment:
		return ExploreFilterSourceAttachment
	case ExploreFilterSourceResearch, SourceResearch:
		return ExploreFilterSourceResearch
	default:
		return ExploreFilterAll
	}
}

func normalizeExploreDateRange(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", ExploreDateAny:
		return ExploreDateAny
	case ExploreDate7d, ExploreDate30d, ExploreDate90d:
		return strings.ToLower(strings.TrimSpace(value))
	default:
		return ExploreDateAny
	}
}

func exploreDateRangeDays(value string) int {
	switch normalizeExploreDateRange(value) {
	case ExploreDate7d:
		return 7
	case ExploreDate30d:
		return 30
	case ExploreDate90d:
		return 90
	default:
		return 0
	}
}

func uniqueCount(memories []Memory, values func(Memory) []string) int {
	seen := map[string]bool{}
	for _, mem := range memories {
		for _, value := range values(mem) {
			value = strings.ToLower(strings.TrimSpace(value))
			if value != "" {
				seen[value] = true
			}
		}
	}
	return len(seen)
}

func firstExploreString(values []string) string {
	if len(values) == 0 {
		return ""
	}
	return values[0]
}
