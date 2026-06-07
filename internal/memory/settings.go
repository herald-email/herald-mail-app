package memory

import (
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	FrontmatterFull      = "full"
	FrontmatterMinimal   = "minimal"
	FrontmatterGenerated = "generated_section"
	FrontmatterNone      = "none"

	LinkModeWiki     = "wiki"
	LinkModeMarkdown = "markdown"
	LinkModePath     = "path"
	LinkModeNone     = "none"

	TagModeNone         = "none"
	TagModeConservative = "conservative"
	TagModeWorkflow     = "workflow"
	TagModeCustom       = "custom"

	LowConfidenceHidden = "hidden"
	LowConfidenceChat   = "chat"
	LowConfidenceReview = "review_queue"
)

type Settings struct {
	Enabled      bool                `yaml:"enabled" json:"enabled"`
	Directory    string              `yaml:"directory,omitempty" json:"directory,omitempty"`
	Immutable    bool                `yaml:"immutable,omitempty" json:"immutable,omitempty"`
	Profile      string              `yaml:"profile,omitempty" json:"profile,omitempty"`
	Sources      SourceSettings      `yaml:"sources,omitempty" json:"sources,omitempty"`
	Tasks        TaskSettings        `yaml:"tasks,omitempty" json:"tasks,omitempty"`
	Destinations DestinationSettings `yaml:"destinations,omitempty" json:"destinations,omitempty"`
	Thresholds   ThresholdSettings   `yaml:"thresholds,omitempty" json:"thresholds,omitempty"`
	UpdateRules  UpdateRuleSettings  `yaml:"update_rules,omitempty" json:"update_rules,omitempty"`
	Obsidian     ObsidianSettings    `yaml:"obsidian,omitempty" json:"obsidian,omitempty"`
	Prompts      []PromptTemplate    `yaml:"prompts,omitempty" json:"prompts,omitempty"`
	Research     ResearchSettings    `yaml:"research,omitempty" json:"research,omitempty"`
	enabledSet   bool                `yaml:"-" json:"-"`
	immutableSet bool                `yaml:"-" json:"-"`
}

type SourceSettings struct {
	Folders               []string `yaml:"folders,omitempty" json:"folders,omitempty"`
	Accounts              []string `yaml:"accounts,omitempty" json:"accounts,omitempty"`
	Contacts              bool     `yaml:"contacts,omitempty" json:"contacts,omitempty"`
	Calendar              bool     `yaml:"calendar,omitempty" json:"calendar,omitempty"`
	CalendarLookbackDays  int      `yaml:"calendar_lookback_days,omitempty" json:"calendar_lookback_days,omitempty"`
	CalendarLookaheadDays int      `yaml:"calendar_lookahead_days,omitempty" json:"calendar_lookahead_days,omitempty"`
	Obsidian              bool     `yaml:"obsidian,omitempty" json:"obsidian,omitempty"`
	MaxObsidianNotes      int      `yaml:"max_obsidian_notes,omitempty" json:"max_obsidian_notes,omitempty"`
	ResearchNotes         bool     `yaml:"research_notes,omitempty" json:"research_notes,omitempty"`
	MaxResearchNotes      int      `yaml:"max_research_notes,omitempty" json:"max_research_notes,omitempty"`

	contactsSet bool `yaml:"-" json:"-"`
}

type TaskSettings struct {
	MemoryExtraction      bool `yaml:"memory_extraction" json:"memory_extraction"`
	TrackStatusUpdate     bool `yaml:"track_status_update" json:"track_status_update"`
	ComposeRadarNudges    bool `yaml:"compose_radar_nudges" json:"compose_radar_nudges"`
	Dossiers              bool `yaml:"dossiers" json:"dossiers"`
	ObsidianSectionFormat bool `yaml:"obsidian_section_format" json:"obsidian_section_format"`
	ResearchNoteSummary   bool `yaml:"research_note_summary" json:"research_note_summary"`

	memoryExtractionSet      bool `yaml:"-" json:"-"`
	trackStatusUpdateSet     bool `yaml:"-" json:"-"`
	composeRadarNudgesSet    bool `yaml:"-" json:"-"`
	dossiersSet              bool `yaml:"-" json:"-"`
	obsidianSectionFormatSet bool `yaml:"-" json:"-"`
	researchNoteSummarySet   bool `yaml:"-" json:"-"`
}

type DestinationSettings struct {
	People        string `yaml:"people,omitempty" json:"people,omitempty"`
	Companies     string `yaml:"companies,omitempty" json:"companies,omitempty"`
	JobSearch     string `yaml:"job_search,omitempty" json:"job_search,omitempty"`
	Projects      string `yaml:"projects,omitempty" json:"projects,omitempty"`
	Threads       string `yaml:"threads,omitempty" json:"threads,omitempty"`
	Research      string `yaml:"research,omitempty" json:"research,omitempty"`
	DailyBriefing string `yaml:"daily_briefing,omitempty" json:"daily_briefing,omitempty"`
	Inbox         string `yaml:"inbox,omitempty" json:"inbox,omitempty"`
}

type ThresholdSettings struct {
	ChatRetrieval float64 `yaml:"chat_retrieval,omitempty" json:"chat_retrieval,omitempty"`
	Dossier       float64 `yaml:"dossier,omitempty" json:"dossier,omitempty"`
	ObsidianWrite float64 `yaml:"obsidian_write,omitempty" json:"obsidian_write,omitempty"`
	ComposeRadar  float64 `yaml:"compose_radar,omitempty" json:"compose_radar,omitempty"`
	Match         float64 `yaml:"match,omitempty" json:"match,omitempty"`
}

type UpdateRuleSettings struct {
	Cadence                  string  `yaml:"cadence,omitempty" json:"cadence,omitempty"`
	MatchThreshold           float64 `yaml:"match_threshold,omitempty" json:"match_threshold,omitempty"`
	ConflictCreatesState     bool    `yaml:"conflict_creates_state,omitempty" json:"conflict_creates_state,omitempty"`
	StaleAfterDays           int     `yaml:"stale_after_days,omitempty" json:"stale_after_days,omitempty"`
	RetentionDays            int     `yaml:"retention_days,omitempty" json:"retention_days,omitempty"`
	LowConfidenceDisposition string  `yaml:"low_confidence_disposition,omitempty" json:"low_confidence_disposition,omitempty"`
	DismissalScope           string  `yaml:"dismissal_scope,omitempty" json:"dismissal_scope,omitempty"`

	conflictCreatesStateSet bool `yaml:"-" json:"-"`
}

type ObsidianSettings struct {
	Enabled                 bool     `yaml:"enabled,omitempty" json:"enabled,omitempty"`
	VaultPath               string   `yaml:"vault_path,omitempty" json:"vault_path,omitempty"`
	FrontmatterMode         string   `yaml:"frontmatter_mode,omitempty" json:"frontmatter_mode,omitempty"`
	YAMLHeaders             bool     `yaml:"yaml_headers,omitempty" json:"yaml_headers,omitempty"`
	LinkMode                string   `yaml:"link_mode,omitempty" json:"link_mode,omitempty"`
	TagMode                 string   `yaml:"tag_mode,omitempty" json:"tag_mode,omitempty"`
	CustomTags              []string `yaml:"custom_tags,omitempty" json:"custom_tags,omitempty"`
	DataviewFields          bool     `yaml:"dataview_fields,omitempty" json:"dataview_fields,omitempty"`
	Backlinks               bool     `yaml:"backlinks,omitempty" json:"backlinks,omitempty"`
	GeneratedSectionMarkers bool     `yaml:"generated_section_markers,omitempty" json:"generated_section_markers,omitempty"`
	PreviewBeforeWrite      bool     `yaml:"preview_before_write,omitempty" json:"preview_before_write,omitempty"`

	yamlHeadersSet        bool `yaml:"-" json:"-"`
	dataviewFieldsSet     bool `yaml:"-" json:"-"`
	backlinksSet          bool `yaml:"-" json:"-"`
	markersSet            bool `yaml:"-" json:"-"`
	previewBeforeWriteSet bool `yaml:"-" json:"-"`
}

type PromptTemplate struct {
	Name      string   `yaml:"name" json:"name"`
	Version   string   `yaml:"version" json:"version"`
	Purpose   string   `yaml:"purpose,omitempty" json:"purpose,omitempty"`
	Variables []string `yaml:"variables,omitempty" json:"variables,omitempty"`
	Template  string   `yaml:"template" json:"template"`
}

type ResearchSettings struct {
	Enabled              bool `yaml:"enabled,omitempty" json:"enabled,omitempty"`
	ExternalOptIn        bool `yaml:"external_opt_in,omitempty" json:"external_opt_in,omitempty"`
	PrivateBodiesAllowed bool `yaml:"private_bodies_allowed,omitempty" json:"private_bodies_allowed,omitempty"`
	StaleAfterDays       int  `yaml:"stale_after_days,omitempty" json:"stale_after_days,omitempty"`
}

func (s *Settings) UnmarshalYAML(value *yaml.Node) error {
	type rawSettings struct {
		Enabled      *bool               `yaml:"enabled"`
		Directory    string              `yaml:"directory"`
		Immutable    *bool               `yaml:"immutable"`
		Profile      string              `yaml:"profile"`
		Sources      SourceSettings      `yaml:"sources"`
		Tasks        TaskSettings        `yaml:"tasks"`
		Destinations DestinationSettings `yaml:"destinations"`
		Thresholds   ThresholdSettings   `yaml:"thresholds"`
		UpdateRules  UpdateRuleSettings  `yaml:"update_rules"`
		Obsidian     ObsidianSettings    `yaml:"obsidian"`
		Prompts      []PromptTemplate    `yaml:"prompts"`
		Research     ResearchSettings    `yaml:"research"`
	}
	var decoded rawSettings
	if err := value.Decode(&decoded); err != nil {
		return err
	}
	if decoded.Enabled != nil {
		s.Enabled = *decoded.Enabled
		s.enabledSet = true
	}
	if decoded.Immutable != nil {
		s.Immutable = *decoded.Immutable
		s.immutableSet = true
	}
	s.Directory = decoded.Directory
	s.Profile = decoded.Profile
	s.Sources = decoded.Sources
	s.Tasks = decoded.Tasks
	s.Destinations = decoded.Destinations
	s.Thresholds = decoded.Thresholds
	s.UpdateRules = decoded.UpdateRules
	s.Obsidian = decoded.Obsidian
	s.Prompts = decoded.Prompts
	s.Research = decoded.Research
	return nil
}

func (t *TaskSettings) UnmarshalYAML(value *yaml.Node) error {
	type rawTasks struct {
		MemoryExtraction      *bool `yaml:"memory_extraction"`
		TrackStatusUpdate     *bool `yaml:"track_status_update"`
		ComposeRadarNudges    *bool `yaml:"compose_radar_nudges"`
		Dossiers              *bool `yaml:"dossiers"`
		ObsidianSectionFormat *bool `yaml:"obsidian_section_format"`
		ResearchNoteSummary   *bool `yaml:"research_note_summary"`
	}
	var decoded rawTasks
	if err := value.Decode(&decoded); err != nil {
		return err
	}
	if decoded.MemoryExtraction != nil {
		t.MemoryExtraction = *decoded.MemoryExtraction
		t.memoryExtractionSet = true
	}
	if decoded.TrackStatusUpdate != nil {
		t.TrackStatusUpdate = *decoded.TrackStatusUpdate
		t.trackStatusUpdateSet = true
	}
	if decoded.ComposeRadarNudges != nil {
		t.ComposeRadarNudges = *decoded.ComposeRadarNudges
		t.composeRadarNudgesSet = true
	}
	if decoded.Dossiers != nil {
		t.Dossiers = *decoded.Dossiers
		t.dossiersSet = true
	}
	if decoded.ObsidianSectionFormat != nil {
		t.ObsidianSectionFormat = *decoded.ObsidianSectionFormat
		t.obsidianSectionFormatSet = true
	}
	if decoded.ResearchNoteSummary != nil {
		t.ResearchNoteSummary = *decoded.ResearchNoteSummary
		t.researchNoteSummarySet = true
	}
	return nil
}

func (o *ObsidianSettings) UnmarshalYAML(value *yaml.Node) error {
	type rawObsidian struct {
		Enabled                 bool     `yaml:"enabled"`
		VaultPath               string   `yaml:"vault_path"`
		FrontmatterMode         string   `yaml:"frontmatter_mode"`
		YAMLHeaders             *bool    `yaml:"yaml_headers"`
		LinkMode                string   `yaml:"link_mode"`
		TagMode                 string   `yaml:"tag_mode"`
		CustomTags              []string `yaml:"custom_tags"`
		DataviewFields          *bool    `yaml:"dataview_fields"`
		Backlinks               *bool    `yaml:"backlinks"`
		GeneratedSectionMarkers *bool    `yaml:"generated_section_markers"`
		PreviewBeforeWrite      *bool    `yaml:"preview_before_write"`
	}
	var decoded rawObsidian
	if err := value.Decode(&decoded); err != nil {
		return err
	}
	o.Enabled = decoded.Enabled
	o.VaultPath = decoded.VaultPath
	o.FrontmatterMode = decoded.FrontmatterMode
	o.LinkMode = decoded.LinkMode
	o.TagMode = decoded.TagMode
	o.CustomTags = decoded.CustomTags
	if decoded.YAMLHeaders != nil {
		o.YAMLHeaders = *decoded.YAMLHeaders
		o.yamlHeadersSet = true
	}
	if decoded.DataviewFields != nil {
		o.DataviewFields = *decoded.DataviewFields
		o.dataviewFieldsSet = true
	}
	if decoded.Backlinks != nil {
		o.Backlinks = *decoded.Backlinks
		o.backlinksSet = true
	}
	if decoded.GeneratedSectionMarkers != nil {
		o.GeneratedSectionMarkers = *decoded.GeneratedSectionMarkers
		o.markersSet = true
	}
	if decoded.PreviewBeforeWrite != nil {
		o.PreviewBeforeWrite = *decoded.PreviewBeforeWrite
		o.previewBeforeWriteSet = true
	}
	return nil
}

func (u *UpdateRuleSettings) UnmarshalYAML(value *yaml.Node) error {
	type rawUpdateRules struct {
		Cadence                  string  `yaml:"cadence"`
		MatchThreshold           float64 `yaml:"match_threshold"`
		ConflictCreatesState     *bool   `yaml:"conflict_creates_state"`
		StaleAfterDays           int     `yaml:"stale_after_days"`
		RetentionDays            int     `yaml:"retention_days"`
		LowConfidenceDisposition string  `yaml:"low_confidence_disposition"`
		DismissalScope           string  `yaml:"dismissal_scope"`
	}
	var decoded rawUpdateRules
	if err := value.Decode(&decoded); err != nil {
		return err
	}
	u.Cadence = decoded.Cadence
	u.MatchThreshold = decoded.MatchThreshold
	if decoded.ConflictCreatesState != nil {
		u.ConflictCreatesState = *decoded.ConflictCreatesState
		u.conflictCreatesStateSet = true
	}
	u.StaleAfterDays = decoded.StaleAfterDays
	u.RetentionDays = decoded.RetentionDays
	u.LowConfidenceDisposition = decoded.LowConfidenceDisposition
	u.DismissalScope = decoded.DismissalScope
	return nil
}

func (s *SourceSettings) UnmarshalYAML(value *yaml.Node) error {
	type rawSources struct {
		Folders               []string `yaml:"folders"`
		Accounts              []string `yaml:"accounts"`
		Contacts              *bool    `yaml:"contacts"`
		Calendar              bool     `yaml:"calendar"`
		CalendarLookbackDays  int      `yaml:"calendar_lookback_days"`
		CalendarLookaheadDays int      `yaml:"calendar_lookahead_days"`
		Obsidian              bool     `yaml:"obsidian"`
		MaxObsidianNotes      int      `yaml:"max_obsidian_notes"`
		ResearchNotes         bool     `yaml:"research_notes"`
		MaxResearchNotes      int      `yaml:"max_research_notes"`
	}
	var decoded rawSources
	if err := value.Decode(&decoded); err != nil {
		return err
	}
	s.Folders = decoded.Folders
	s.Accounts = decoded.Accounts
	if decoded.Contacts != nil {
		s.Contacts = *decoded.Contacts
		s.contactsSet = true
	}
	s.Calendar = decoded.Calendar
	s.CalendarLookbackDays = decoded.CalendarLookbackDays
	s.CalendarLookaheadDays = decoded.CalendarLookaheadDays
	s.Obsidian = decoded.Obsidian
	s.MaxObsidianNotes = decoded.MaxObsidianNotes
	s.ResearchNotes = decoded.ResearchNotes
	s.MaxResearchNotes = decoded.MaxResearchNotes
	return nil
}

func DefaultSettings() Settings {
	settings := Settings{}
	settings.ApplyDefaults()
	return settings
}

func NewTaskSettings(memoryExtraction, trackStatusUpdate, composeRadarNudges, dossiers, obsidianSectionFormat, researchNoteSummary bool) TaskSettings {
	return TaskSettings{
		MemoryExtraction:         memoryExtraction,
		TrackStatusUpdate:        trackStatusUpdate,
		ComposeRadarNudges:       composeRadarNudges,
		Dossiers:                 dossiers,
		ObsidianSectionFormat:    obsidianSectionFormat,
		ResearchNoteSummary:      researchNoteSummary,
		memoryExtractionSet:      true,
		trackStatusUpdateSet:     true,
		composeRadarNudgesSet:    true,
		dossiersSet:              true,
		obsidianSectionFormatSet: true,
		researchNoteSummarySet:   true,
	}
}

func (s *Settings) ApplyDefaults() {
	if !s.enabledSet {
		s.Enabled = true
	}
	// Immutability is the storage contract for Herald Memories. User-facing
	// updates are modeled as new records or generated-section previews.
	s.Immutable = true
	if strings.TrimSpace(s.Directory) == "" {
		s.Directory = DefaultDirectory
	}
	if strings.TrimSpace(s.Profile) == "" {
		s.Profile = "obsidian-friendly"
	}
	if len(s.Sources.Folders) == 0 {
		s.Sources.Folders = []string{"INBOX", "Sent"}
	}
	s.Sources.Folders = CompactStrings(s.Sources.Folders)
	s.Sources.Accounts = CompactStrings(s.Sources.Accounts)
	if !s.Sources.contactsSet {
		s.Sources.Contacts = true
	}
	if s.Sources.CalendarLookbackDays <= 0 {
		s.Sources.CalendarLookbackDays = 30
	}
	if s.Sources.CalendarLookaheadDays <= 0 {
		s.Sources.CalendarLookaheadDays = 90
	}
	if s.Sources.MaxObsidianNotes <= 0 {
		s.Sources.MaxObsidianNotes = 100
	}
	if s.Sources.MaxResearchNotes <= 0 {
		s.Sources.MaxResearchNotes = 50
	}
	s.Tasks.ApplyDefaults()
	if strings.TrimSpace(s.Destinations.People) == "" {
		s.Destinations.People = "People"
	}
	if strings.TrimSpace(s.Destinations.Companies) == "" {
		s.Destinations.Companies = "Job search/active"
	}
	if strings.TrimSpace(s.Destinations.JobSearch) == "" {
		s.Destinations.JobSearch = "Job search"
	}
	if strings.TrimSpace(s.Destinations.Projects) == "" {
		s.Destinations.Projects = "Projects"
	}
	if strings.TrimSpace(s.Destinations.Threads) == "" {
		s.Destinations.Threads = "Threads"
	}
	if strings.TrimSpace(s.Destinations.Research) == "" {
		s.Destinations.Research = "Research"
	}
	if strings.TrimSpace(s.Destinations.DailyBriefing) == "" {
		s.Destinations.DailyBriefing = "Scheduled Task Artifacts"
	}
	if strings.TrimSpace(s.Destinations.Inbox) == "" {
		s.Destinations.Inbox = "Memory Inbox"
	}
	if s.Thresholds.ChatRetrieval == 0 {
		s.Thresholds.ChatRetrieval = 0.35
	}
	if s.Thresholds.Dossier == 0 {
		s.Thresholds.Dossier = 0.55
	}
	if s.Thresholds.ObsidianWrite == 0 {
		s.Thresholds.ObsidianWrite = 0.70
	}
	if s.Thresholds.ComposeRadar == 0 {
		s.Thresholds.ComposeRadar = 0.75
	}
	if s.Thresholds.Match == 0 {
		s.Thresholds.Match = 0.80
	}
	if strings.TrimSpace(s.UpdateRules.Cadence) == "" {
		s.UpdateRules.Cadence = "manual"
	}
	if s.UpdateRules.MatchThreshold == 0 {
		s.UpdateRules.MatchThreshold = s.Thresholds.Match
	}
	if !s.UpdateRules.conflictCreatesStateSet {
		s.UpdateRules.ConflictCreatesState = true
	}
	if s.UpdateRules.StaleAfterDays == 0 {
		s.UpdateRules.StaleAfterDays = 45
	}
	if strings.TrimSpace(s.UpdateRules.LowConfidenceDisposition) == "" {
		s.UpdateRules.LowConfidenceDisposition = LowConfidenceChat
	}
	if strings.TrimSpace(s.UpdateRules.DismissalScope) == "" {
		s.UpdateRules.DismissalScope = "thread"
	}
	s.Obsidian.ApplyDefaults()
	if len(s.Prompts) == 0 {
		s.Prompts = DefaultPromptTemplates()
	}
	if s.Research.StaleAfterDays == 0 {
		s.Research.StaleAfterDays = 30
	}
}

func (t *TaskSettings) ApplyDefaults() {
	if !t.memoryExtractionSet {
		t.MemoryExtraction = true
	}
	if !t.trackStatusUpdateSet {
		t.TrackStatusUpdate = true
	}
	if !t.composeRadarNudgesSet {
		t.ComposeRadarNudges = true
	}
	if !t.dossiersSet {
		t.Dossiers = true
	}
	if !t.obsidianSectionFormatSet {
		t.ObsidianSectionFormat = true
	}
	if !t.researchNoteSummarySet {
		t.ResearchNoteSummary = true
	}
}

func (o *ObsidianSettings) ApplyDefaults() {
	if strings.TrimSpace(o.FrontmatterMode) == "" {
		o.FrontmatterMode = FrontmatterMinimal
	} else {
		o.FrontmatterMode = NormalizeFrontmatterMode(o.FrontmatterMode)
	}
	if !o.yamlHeadersSet {
		o.YAMLHeaders = o.FrontmatterMode != FrontmatterNone
	}
	if !o.YAMLHeaders {
		o.FrontmatterMode = FrontmatterNone
	}
	if strings.TrimSpace(o.LinkMode) == "" {
		o.LinkMode = LinkModeWiki
	} else {
		o.LinkMode = NormalizeLinkMode(o.LinkMode)
	}
	if strings.TrimSpace(o.TagMode) == "" {
		o.TagMode = TagModeConservative
	} else {
		o.TagMode = NormalizeTagMode(o.TagMode)
	}
	if !o.dataviewFieldsSet {
		o.DataviewFields = true
	}
	if !o.backlinksSet {
		o.Backlinks = true
	}
	if !o.markersSet {
		o.GeneratedSectionMarkers = true
	}
	if !o.previewBeforeWriteSet {
		o.PreviewBeforeWrite = true
	}
	o.CustomTags = CompactStrings(o.CustomTags)
}

func NormalizeFrontmatterMode(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case FrontmatterFull:
		return FrontmatterFull
	case FrontmatterGenerated, "generated", "section":
		return FrontmatterGenerated
	case FrontmatterNone, "off", "hidden", "hide":
		return FrontmatterNone
	default:
		return FrontmatterMinimal
	}
}

func NormalizeLinkMode(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case LinkModeMarkdown:
		return LinkModeMarkdown
	case LinkModePath:
		return LinkModePath
	case LinkModeNone, "off":
		return LinkModeNone
	default:
		return LinkModeWiki
	}
}

func NormalizeTagMode(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case TagModeNone, "off":
		return TagModeNone
	case TagModeWorkflow:
		return TagModeWorkflow
	case TagModeCustom:
		return TagModeCustom
	default:
		return TagModeConservative
	}
}

func DefaultPromptTemplates() []PromptTemplate {
	return []PromptTemplate{
		{
			Name:      "memory_extraction",
			Version:   "memory-extraction-v1",
			Purpose:   "Extract source-backed memory candidates from bounded email snippets.",
			Variables: []string{"source_snippets", "evidence_metadata", "configured_destinations"},
			Template:  "Extract concise memories only when the provided snippets contain direct evidence. Return claims, evidence IDs, confidence, and stale/conflict signals.",
		},
		{
			Name:      "track_status_update",
			Version:   "track-status-v1",
			Purpose:   "Classify a relationship or job-search track as active, waiting, resolved, stale, backlog, or done.",
			Variables: []string{"existing_track", "new_evidence", "configured_update_rules"},
			Template:  "Update the generated track status conservatively. Conflicts create a conflict state; resolved loops keep source evidence.",
		},
		{
			Name:      "compose_radar_nudge",
			Version:   "compose-radar-v1",
			Purpose:   "Generate at most three compose-time nudges from high-confidence memories.",
			Variables: []string{"reply_context", "draft_excerpt", "memory_candidates"},
			Template:  "Generate quiet, source-backed nudges that may affect a reply. Do not invent context and do not mutate the draft.",
		},
		{
			Name:      "dossier_summary",
			Version:   "dossier-summary-v1",
			Purpose:   "Summarize a person, company, or thread dossier from memory records and explicit research notes.",
			Variables: []string{"memories", "tracks", "research_notes", "source_list"},
			Template:  "Summarize relationship state, active tracks, open loops, freshness, and cited sources. Separate email, Obsidian, research, and inference.",
		},
		{
			Name:      "obsidian_section_format",
			Version:   "obsidian-section-v1",
			Purpose:   "Render Herald-managed sections without overwriting user-authored note content.",
			Variables: []string{"memories", "frontmatter_mode", "link_mode", "tag_mode"},
			Template:  "Format a concise Markdown generated section between Herald markers. Preserve user-authored content outside markers.",
		},
		{
			Name:      "research_note_summary",
			Version:   "research-note-v1",
			Purpose:   "Summarize explicit public research with URLs and freshness dates.",
			Variables: []string{"public_sources", "person_or_company", "last_contact_context"},
			Template:  "Summarize only public-source findings, cite URLs, and call out what changed since the last contact without using private email bodies.",
		},
	}
}
