package memory

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

var (
	ErrObsidianSyncDisabled        = errors.New("obsidian sync is disabled")
	ErrObsidianVaultPathRequired   = errors.New("obsidian vault path is required")
	ErrObsidianPreviewApprovalNeed = errors.New("obsidian sync preview approval required")
)

const (
	generatedBegin = "<!-- HERALD:MEMORIES:BEGIN -->"
	generatedEnd   = "<!-- HERALD:MEMORIES:END -->"
)

const obsidianSyncStateRelPath = "state/obsidian_sync.json"

type NotePreview struct {
	Path              string   `json:"path" yaml:"path"`
	Existing          string   `json:"existing,omitempty" yaml:"existing,omitempty"`
	Generated         string   `json:"generated" yaml:"generated"`
	Merged            string   `json:"merged" yaml:"merged"`
	FrontmatterMode   string   `json:"frontmatter_mode" yaml:"frontmatter_mode"`
	LinkMode          string   `json:"link_mode" yaml:"link_mode"`
	TagMode           string   `json:"tag_mode" yaml:"tag_mode"`
	SourceEvidenceIDs []string `json:"source_evidence_ids,omitempty" yaml:"source_evidence_ids,omitempty"`
	WouldCreate       bool     `json:"would_create" yaml:"would_create"`
	WouldUpdate       bool     `json:"would_update" yaml:"would_update"`
}

type ObsidianSyncState struct {
	Enabled         bool      `json:"enabled" yaml:"enabled"`
	VaultPath       string    `json:"vault_path,omitempty" yaml:"vault_path,omitempty"`
	LastRun         time.Time `json:"last_run,omitempty" yaml:"last_run,omitempty"`
	PendingWrites   int       `json:"pending_writes" yaml:"pending_writes"`
	CreatedNotes    int       `json:"created_notes" yaml:"created_notes"`
	UpdatedNotes    int       `json:"updated_notes" yaml:"updated_notes"`
	UnchangedNotes  int       `json:"unchanged_notes" yaml:"unchanged_notes"`
	AppliedWrites   int       `json:"applied_writes" yaml:"applied_writes"`
	FailedWrites    int       `json:"failed_writes" yaml:"failed_writes"`
	Approved        bool      `json:"approved" yaml:"approved"`
	PreviewRequired bool      `json:"preview_required" yaml:"preview_required"`
	Unavailable     bool      `json:"unavailable,omitempty" yaml:"unavailable,omitempty"`
	Error           string    `json:"error,omitempty" yaml:"error,omitempty"`
}

type ObsidianSyncPlan struct {
	VaultPath       string            `json:"vault_path" yaml:"vault_path"`
	GeneratedAt     time.Time         `json:"generated_at" yaml:"generated_at"`
	Approved        bool              `json:"approved" yaml:"approved"`
	PreviewRequired bool              `json:"preview_required" yaml:"preview_required"`
	Previews        []NotePreview     `json:"previews" yaml:"previews"`
	State           ObsidianSyncState `json:"state" yaml:"state"`
}

type ObsidianSyncFailure struct {
	Path  string `json:"path" yaml:"path"`
	Error string `json:"error" yaml:"error"`
}

type ObsidianSyncResult struct {
	State    ObsidianSyncState     `json:"state" yaml:"state"`
	Written  []string              `json:"written,omitempty" yaml:"written,omitempty"`
	Failures []ObsidianSyncFailure `json:"failures,omitempty" yaml:"failures,omitempty"`
}

func PreviewObsidianSync(memories []Memory, settings Settings, existingByPath map[string]string) []NotePreview {
	settings.ApplyDefaults()
	byPath := groupMemoriesByTarget(memories, settings)
	paths := make([]string, 0, len(byPath))
	for path := range byPath {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	previews := make([]NotePreview, 0, len(paths))
	for _, path := range paths {
		group := byPath[path]
		generated := RenderGeneratedSection(group, settings)
		existing := existingByPath[path]
		merged := MergeGeneratedSection(existing, generated)
		previews = append(previews, NotePreview{
			Path:              path,
			Existing:          existing,
			Generated:         generated,
			Merged:            merged,
			FrontmatterMode:   NormalizeFrontmatterMode(settings.Obsidian.FrontmatterMode),
			LinkMode:          NormalizeLinkMode(settings.Obsidian.LinkMode),
			TagMode:           NormalizeTagMode(settings.Obsidian.TagMode),
			SourceEvidenceIDs: evidenceLabels(group),
			WouldCreate:       strings.TrimSpace(existing) == "",
			WouldUpdate:       strings.TrimSpace(existing) != strings.TrimSpace(merged),
		})
	}
	return previews
}

func PlanObsidianSync(ctx context.Context, memories []Memory, settings Settings, approved bool) (ObsidianSyncPlan, error) {
	if err := ctx.Err(); err != nil {
		return ObsidianSyncPlan{}, err
	}
	settings.ApplyDefaults()
	vaultRoot, err := obsidianVaultRoot(settings)
	if err != nil {
		return ObsidianSyncPlan{}, err
	}
	memories = ObsidianSyncEligibleMemories(memories, settings)
	byPath := groupMemoriesByTarget(memories, settings)
	existingByPath := make(map[string]string, len(byPath))
	for path := range byPath {
		if err := ctx.Err(); err != nil {
			return ObsidianSyncPlan{}, err
		}
		notePath, err := obsidianSafeNotePath(vaultRoot, path)
		if err != nil {
			return ObsidianSyncPlan{}, err
		}
		data, err := os.ReadFile(notePath)
		if err == nil {
			existingByPath[path] = string(data)
			continue
		}
		if !errors.Is(err, os.ErrNotExist) {
			return ObsidianSyncPlan{}, fmt.Errorf("read Obsidian note %s: %w", path, err)
		}
	}
	previews := PreviewObsidianSync(memories, settings, existingByPath)
	state := obsidianSyncStateFromPreviews(settings, vaultRoot, previews, approved)
	return ObsidianSyncPlan{
		VaultPath:       vaultRoot,
		GeneratedAt:     time.Now(),
		Approved:        approved,
		PreviewRequired: state.PreviewRequired,
		Previews:        previews,
		State:           state,
	}, nil
}

func (s *FileStore) PlanObsidianSync(ctx context.Context, settings Settings, approved bool) (ObsidianSyncPlan, error) {
	if err := ctx.Err(); err != nil {
		return ObsidianSyncPlan{}, err
	}
	if s == nil {
		return ObsidianSyncPlan{}, fmt.Errorf("memory store is nil")
	}
	memories, err := s.EffectiveList(ctx, settings)
	if err != nil {
		return ObsidianSyncPlan{}, err
	}
	return PlanObsidianSync(ctx, memories, settings, approved)
}

func (s *FileStore) ApplyObsidianSync(ctx context.Context, plan ObsidianSyncPlan) (ObsidianSyncResult, error) {
	if err := ctx.Err(); err != nil {
		return ObsidianSyncResult{}, err
	}
	if s == nil {
		return ObsidianSyncResult{}, fmt.Errorf("memory store is nil")
	}
	result := ObsidianSyncResult{State: plan.State}
	if plan.PreviewRequired && !plan.Approved {
		result.State.Error = ErrObsidianPreviewApprovalNeed.Error()
		return result, ErrObsidianPreviewApprovalNeed
	}
	state := plan.State
	state.LastRun = time.Now()
	state.AppliedWrites = 0
	state.FailedWrites = 0
	state.Approved = plan.Approved || !plan.PreviewRequired
	state.PreviewRequired = false
	state.Error = ""
	for _, preview := range plan.Previews {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		if !preview.WouldCreate && !preview.WouldUpdate {
			continue
		}
		notePath, err := obsidianSafeNotePath(plan.VaultPath, preview.Path)
		if err == nil {
			err = os.MkdirAll(filepath.Dir(notePath), 0o700)
		}
		if err == nil {
			err = os.WriteFile(notePath, []byte(preview.Merged), 0o600)
		}
		if err != nil {
			result.Failures = append(result.Failures, ObsidianSyncFailure{Path: preview.Path, Error: err.Error()})
			continue
		}
		result.Written = append(result.Written, preview.Path)
	}
	state.AppliedWrites = len(result.Written)
	state.FailedWrites = len(result.Failures)
	state.PendingWrites = state.FailedWrites
	if state.FailedWrites > 0 {
		state.Error = fmt.Sprintf("%d Obsidian write(s) failed", state.FailedWrites)
	}
	result.State = state
	if err := s.writeObsidianSyncState(ctx, state); err != nil {
		return result, err
	}
	if len(result.Failures) > 0 {
		return result, errors.New(state.Error)
	}
	return result, nil
}

func ObsidianSyncEligibleMemories(memories []Memory, settings Settings) []Memory {
	settings.ApplyDefaults()
	threshold := settings.Thresholds.ObsidianWrite
	if threshold <= 0 {
		threshold = 0.70
	}
	out := make([]Memory, 0, len(memories))
	for _, memory := range memories {
		if memory.Confidence < threshold {
			continue
		}
		if len(memory.Evidence) == 0 {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(memory.Status), StatusSourceMissing) {
			continue
		}
		out = append(out, memory)
	}
	return out
}

func ObsidianSyncStateForSettings(ctx context.Context, settings Settings) ObsidianSyncState {
	settings.ApplyDefaults()
	state := ObsidianSyncState{
		Enabled:   settings.Obsidian.Enabled,
		VaultPath: strings.TrimSpace(settings.Obsidian.VaultPath),
	}
	if !settings.Obsidian.Enabled {
		return state
	}
	store, err := NewFileStore(settings.Directory)
	if err != nil {
		state.Unavailable = true
		state.Error = err.Error()
		return state
	}
	if prior, err := store.ReadObsidianSyncState(ctx); err == nil {
		state = prior
		state.Enabled = settings.Obsidian.Enabled
		state.VaultPath = strings.TrimSpace(settings.Obsidian.VaultPath)
	}
	plan, err := store.PlanObsidianSync(ctx, settings, false)
	if err != nil {
		state.Unavailable = true
		state.Error = err.Error()
		return state
	}
	state.VaultPath = plan.VaultPath
	state.PendingWrites = plan.State.PendingWrites
	state.CreatedNotes = plan.State.CreatedNotes
	state.UpdatedNotes = plan.State.UpdatedNotes
	state.UnchangedNotes = plan.State.UnchangedNotes
	state.PreviewRequired = plan.State.PreviewRequired
	if state.PendingWrites > 0 {
		state.Approved = !state.PreviewRequired
	}
	state.Unavailable = false
	state.Error = ""
	return state
}

func (s *FileStore) ReadObsidianSyncState(ctx context.Context) (ObsidianSyncState, error) {
	if err := ctx.Err(); err != nil {
		return ObsidianSyncState{}, err
	}
	if s == nil {
		return ObsidianSyncState{}, fmt.Errorf("memory store is nil")
	}
	data, err := os.ReadFile(filepath.Join(s.root, obsidianSyncStateRelPath))
	if err != nil {
		return ObsidianSyncState{}, err
	}
	var state ObsidianSyncState
	if err := json.Unmarshal(data, &state); err != nil {
		return ObsidianSyncState{}, err
	}
	return state, nil
}

func (s *FileStore) writeObsidianSyncState(ctx context.Context, state ObsidianSyncState) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if s == nil {
		return fmt.Errorf("memory store is nil")
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	path := filepath.Join(s.root, obsidianSyncStateRelPath)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

func RenderGeneratedSection(memories []Memory, settings Settings) string {
	settings.ApplyDefaults()
	SortMemoriesNewestFirst(memories)
	var sb strings.Builder
	if settings.Obsidian.GeneratedSectionMarkers {
		sb.WriteString(generatedBegin)
		sb.WriteString("\n")
	}
	if frontmatter := renderFrontmatter(memories, settings); frontmatter != "" {
		sb.WriteString(frontmatter)
		sb.WriteString("\n")
	}
	sb.WriteString("## Herald Memories\n\n")
	if len(memories) == 0 {
		sb.WriteString("_No source-backed memories yet._\n")
	} else {
		for _, memory := range memories {
			sb.WriteString(renderMemoryMarkdown(memory, settings))
			sb.WriteString("\n")
		}
	}
	if settings.Obsidian.GeneratedSectionMarkers {
		sb.WriteString(generatedEnd)
		sb.WriteString("\n")
	}
	return strings.TrimRight(sb.String(), "\n") + "\n"
}

func MergeGeneratedSection(existing, generated string) string {
	existing = strings.TrimRight(existing, "\n")
	generated = strings.TrimRight(generated, "\n")
	if strings.TrimSpace(existing) == "" {
		return generated + "\n"
	}
	start := strings.Index(existing, generatedBegin)
	end := strings.Index(existing, generatedEnd)
	if start >= 0 && end >= 0 && end > start {
		end += len(generatedEnd)
		merged := existing[:start] + generated + existing[end:]
		return strings.TrimRight(merged, "\n") + "\n"
	}
	return existing + "\n\n" + generated + "\n"
}

func groupMemoriesByTarget(memories []Memory, settings Settings) map[string][]Memory {
	out := make(map[string][]Memory)
	for _, memory := range memories {
		path := strings.TrimSpace(memory.ObsidianTarget)
		if path == "" {
			path = fallbackTarget(memory, settings)
		}
		out[path] = append(out[path], memory)
	}
	return out
}

func fallbackTarget(memory Memory, settings Settings) string {
	if len(memory.People) > 0 {
		return filepath.ToSlash(filepath.Join(settings.Destinations.People, safeNoteName(memory.People[0])+".md"))
	}
	if memory.Company != "" {
		return filepath.ToSlash(filepath.Join(settings.Destinations.Companies, safeNoteName(memory.Company)+".md"))
	}
	return filepath.ToSlash(filepath.Join(settings.Destinations.Inbox, "Memory.md"))
}

func renderFrontmatter(memories []Memory, settings Settings) string {
	mode := NormalizeFrontmatterMode(settings.Obsidian.FrontmatterMode)
	if mode == FrontmatterNone || !settings.Obsidian.YAMLHeaders {
		return renderGeneratedMetadataComment(memories, settings)
	}
	fields := frontmatterFields(memories, settings)
	if mode == FrontmatterMinimal {
		allowed := map[string]bool{
			"herald_memory_id": true,
			"memory_updated":   true,
			"last_contact":     true,
			"status":           true,
			"tags":             true,
		}
		for key := range fields {
			if !allowed[key] {
				delete(fields, key)
			}
		}
	}
	var keys []string
	for key := range fields {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	var sb strings.Builder
	sb.WriteString("---\n")
	for _, key := range keys {
		sb.WriteString(key)
		sb.WriteString(": ")
		sb.WriteString(fields[key])
		sb.WriteString("\n")
	}
	sb.WriteString("---\n")
	return strings.TrimRight(sb.String(), "\n")
}

func renderGeneratedMetadataComment(memories []Memory, settings Settings) string {
	if NormalizeFrontmatterMode(settings.Obsidian.FrontmatterMode) != FrontmatterGenerated &&
		settings.Obsidian.YAMLHeaders {
		return ""
	}
	ids := memoryIDs(memories)
	if len(ids) == 0 {
		return "<!-- herald_memory_ids: [] -->"
	}
	return fmt.Sprintf("<!-- herald_memory_ids: %s -->", strings.Join(ids, ", "))
}

func frontmatterFields(memories []Memory, settings Settings) map[string]string {
	fields := map[string]string{
		"memory_updated": quoteYAML(time.Now().Format("2006-01-02")),
	}
	ids := memoryIDs(memories)
	if len(ids) == 1 {
		fields["herald_memory_id"] = quoteYAML(ids[0])
	} else if len(ids) > 1 {
		fields["herald_memory_id"] = yamlList(ids)
	}
	if latest := latestMemoryDate(memories); !latest.IsZero() {
		fields["last_contact"] = quoteYAML(latest.Format("2006-01-02"))
	}
	if status := aggregateStatus(memories); status != "" {
		fields["status"] = quoteYAML(status)
	}
	if company := aggregateCompany(memories); company != "" {
		fields["company"] = quoteYAML(company)
	}
	if source := strings.Join(evidenceLabels(memories), ", "); source != "" {
		fields["source"] = quoteYAML(source)
	}
	tags := aggregateTags(memories, settings)
	if len(tags) > 0 {
		fields["tags"] = yamlList(tags)
	}
	return fields
}

func renderMemoryMarkdown(memory Memory, settings Settings) string {
	var sb strings.Builder
	title := firstNonEmpty(memory.Summary, memory.Claim)
	sb.WriteString("- ")
	if memory.Kind != "" {
		sb.WriteString("**")
		sb.WriteString(strings.ReplaceAll(memory.Kind, "_", " "))
		sb.WriteString("**: ")
	}
	sb.WriteString(escapeMarkdownLine(title))
	if memory.Confidence > 0 {
		sb.WriteString(fmt.Sprintf(" _(%.0f%%)_", memory.Confidence*100))
	}
	if sources := renderEvidenceLinks(memory.Evidence, settings); sources != "" {
		sb.WriteString(" - ")
		sb.WriteString(sources)
	}
	if len(memory.Tags) > 0 && NormalizeTagMode(settings.Obsidian.TagMode) != TagModeNone {
		sb.WriteString(" ")
		sb.WriteString(strings.Join(memory.Tags, " "))
	}
	sb.WriteString("\n")
	return sb.String()
}

func renderEvidenceLinks(evidence []Evidence, settings Settings) string {
	mode := NormalizeLinkMode(settings.Obsidian.LinkMode)
	if mode == LinkModeNone || len(evidence) == 0 {
		return ""
	}
	links := make([]string, 0, len(evidence))
	for _, item := range evidence {
		label := displayEvidenceLabel(item)
		if label == "" {
			continue
		}
		switch item.SourceType {
		case SourceObsidian:
			links = append(links, renderPathLink(label, item.Path, mode))
		case SourceResearch:
			links = append(links, renderURLLink("research", item.URL, mode))
		default:
			links = append(links, "source "+label)
		}
	}
	return strings.Join(CompactStrings(links), ", ")
}

func renderPathLink(label, path, mode string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		path = label
	}
	switch mode {
	case LinkModeMarkdown:
		return fmt.Sprintf("[%s](%s)", label, path)
	case LinkModePath:
		return path
	default:
		stem := strings.TrimSuffix(path, filepath.Ext(path))
		return "[[" + filepath.ToSlash(stem) + "]]"
	}
}

func renderURLLink(label, url, mode string) string {
	url = strings.TrimSpace(url)
	if url == "" {
		return ""
	}
	switch mode {
	case LinkModeMarkdown, LinkModeWiki:
		return fmt.Sprintf("[%s](%s)", label, url)
	case LinkModePath:
		return url
	default:
		return ""
	}
}

func evidenceLabels(memories []Memory) []string {
	var labels []string
	for _, memory := range memories {
		for _, evidence := range memory.Evidence {
			labels = append(labels, displayEvidenceLabel(evidence))
		}
	}
	return CompactStrings(labels)
}

func memoryIDs(memories []Memory) []string {
	ids := make([]string, 0, len(memories))
	for _, memory := range memories {
		ids = append(ids, memory.ID)
	}
	return CompactStrings(ids)
}

func latestMemoryDate(memories []Memory) time.Time {
	var latest time.Time
	for _, memory := range memories {
		date := memory.LastActivityAt
		if date.IsZero() {
			date = memory.CreatedAt
		}
		if !date.IsZero() && (latest.IsZero() || date.After(latest)) {
			latest = date
		}
	}
	return latest
}

func aggregateStatus(memories []Memory) string {
	for _, status := range []string{StatusConflict, StatusWaiting, StatusActive, StatusStale, StatusResolved, StatusSourceMissing} {
		for _, memory := range memories {
			if memory.Status == status {
				return status
			}
		}
	}
	return ""
}

func aggregateCompany(memories []Memory) string {
	for _, memory := range memories {
		if strings.TrimSpace(memory.Company) != "" {
			return strings.TrimSpace(memory.Company)
		}
	}
	return ""
}

func aggregateTags(memories []Memory, settings Settings) []string {
	var tags []string
	for _, memory := range memories {
		tags = append(tags, memory.Tags...)
	}
	if NormalizeTagMode(settings.Obsidian.TagMode) == TagModeWorkflow {
		tags = append(tags, "#herald/memory")
	}
	return CompactStrings(tags)
}

func yamlList(values []string) string {
	values = CompactStrings(values)
	if len(values) == 0 {
		return "[]"
	}
	quoted := make([]string, 0, len(values))
	for _, value := range values {
		quoted = append(quoted, quoteYAML(value))
	}
	return "[" + strings.Join(quoted, ", ") + "]"
}

func quoteYAML(value string) string {
	value = strings.ReplaceAll(value, `"`, `\"`)
	return `"` + value + `"`
}

func escapeMarkdownLine(value string) string {
	return strings.ReplaceAll(strings.TrimSpace(value), "\n", " ")
}

func obsidianSyncStateFromPreviews(settings Settings, vaultRoot string, previews []NotePreview, approved bool) ObsidianSyncState {
	state := ObsidianSyncState{
		Enabled:   settings.Obsidian.Enabled,
		VaultPath: vaultRoot,
		Approved:  approved,
	}
	for _, preview := range previews {
		switch {
		case preview.WouldCreate:
			state.CreatedNotes++
		case preview.WouldUpdate:
			state.UpdatedNotes++
		default:
			state.UnchangedNotes++
		}
	}
	state.PendingWrites = state.CreatedNotes + state.UpdatedNotes
	state.PreviewRequired = state.PendingWrites > 0 && settings.Obsidian.PreviewBeforeWrite && !approved
	return state
}

func obsidianVaultRoot(settings Settings) (string, error) {
	if !settings.Obsidian.Enabled {
		return "", ErrObsidianSyncDisabled
	}
	vaultPath := strings.TrimSpace(settings.Obsidian.VaultPath)
	if vaultPath == "" {
		return "", ErrObsidianVaultPathRequired
	}
	root, err := ExpandDirectory(vaultPath)
	if err != nil {
		return "", err
	}
	return filepath.Clean(root), nil
}

func obsidianSafeNotePath(vaultRoot, notePath string) (string, error) {
	vaultRoot = filepath.Clean(vaultRoot)
	notePath = strings.TrimSpace(notePath)
	if notePath == "" {
		return "", fmt.Errorf("empty Obsidian note path")
	}
	rel := filepath.Clean(filepath.FromSlash(notePath))
	if filepath.IsAbs(rel) || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return "", fmt.Errorf("unsafe Obsidian note path %q", notePath)
	}
	rootAbs, err := filepath.Abs(vaultRoot)
	if err != nil {
		return "", err
	}
	noteAbs, err := filepath.Abs(filepath.Join(rootAbs, rel))
	if err != nil {
		return "", err
	}
	if noteAbs != rootAbs && !strings.HasPrefix(noteAbs, rootAbs+string(os.PathSeparator)) {
		return "", fmt.Errorf("Obsidian note path escapes vault: %q", notePath)
	}
	return noteAbs, nil
}
