package memory

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
)

var ErrMemoryExists = errors.New("memory already exists")

type FileStore struct {
	root string
	now  func() time.Time
}

type StoreStats struct {
	Root         string `json:"root" yaml:"root"`
	Total        int    `json:"total" yaml:"total"`
	Stale        int    `json:"stale" yaml:"stale"`
	ReviewNeeded int    `json:"review_needed" yaml:"review_needed"`
	Unavailable  bool   `json:"unavailable,omitempty" yaml:"unavailable,omitempty"`
	Error        string `json:"error,omitempty" yaml:"error,omitempty"`
}

func NewFileStore(root string) (*FileStore, error) {
	expanded, err := ExpandDirectory(root)
	if err != nil {
		return nil, err
	}
	return &FileStore{
		root: expanded,
		now:  time.Now,
	}, nil
}

func NewFileStoreWithClock(root string, now func() time.Time) (*FileStore, error) {
	store, err := NewFileStore(root)
	if err != nil {
		return nil, err
	}
	if now != nil {
		store.now = now
	}
	return store, nil
}

func (s *FileStore) Root() string {
	if s == nil {
		return ""
	}
	return s.root
}

func ExpandDirectory(path string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		path = DefaultDirectory
	}
	if path == "~" || strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("could not determine home directory: %w", err)
		}
		if path == "~" {
			return home, nil
		}
		return filepath.Join(home, path[2:]), nil
	}
	return path, nil
}

func (s *FileStore) Append(ctx context.Context, memory Memory) (Memory, string, error) {
	if err := ctx.Err(); err != nil {
		return Memory{}, "", err
	}
	if s == nil {
		return Memory{}, "", fmt.Errorf("memory store is nil")
	}
	now := s.now()
	if now.IsZero() {
		now = time.Now()
	}
	memory = PrepareMemoryForAppend(memory, now)
	if err := ValidateMemory(memory); err != nil {
		return Memory{}, "", err
	}
	path := s.pathFor(memory)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return Memory{}, "", fmt.Errorf("create memory directory: %w", err)
	}
	data, err := json.MarshalIndent(memory, "", "  ")
	if err != nil {
		return Memory{}, "", fmt.Errorf("marshal memory: %w", err)
	}
	data = append(data, '\n')
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		if errors.Is(err, os.ErrExist) {
			return memory, path, ErrMemoryExists
		}
		return Memory{}, "", fmt.Errorf("create immutable memory record: %w", err)
	}
	defer func() { _ = file.Close() }()
	if _, err := file.Write(data); err != nil {
		return Memory{}, "", fmt.Errorf("write memory record: %w", err)
	}
	if err := file.Close(); err != nil {
		return Memory{}, "", fmt.Errorf("close memory record: %w", err)
	}
	return memory, path, nil
}

func (s *FileStore) List(ctx context.Context) ([]Memory, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if s == nil {
		return nil, fmt.Errorf("memory store is nil")
	}
	recordsRoot := filepath.Join(s.root, "records")
	var memories []Memory
	err := filepath.WalkDir(recordsRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return nil
			}
			return err
		}
		if d == nil || d.IsDir() || filepath.Ext(path) != ".json" {
			return nil
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		var memory Memory
		if err := json.Unmarshal(data, &memory); err != nil {
			return fmt.Errorf("read memory %s: %w", path, err)
		}
		if strings.TrimSpace(memory.ID) != "" {
			memories = append(memories, memory)
		}
		return nil
	})
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	SortMemoriesNewestFirst(memories)
	return memories, nil
}

func StoreStatsForSettings(ctx context.Context, settings Settings) StoreStats {
	settings.ApplyDefaults()
	store, err := NewFileStore(settings.Directory)
	if err != nil {
		return StoreStats{Unavailable: true, Error: err.Error()}
	}
	stats, err := store.Stats(ctx, settings)
	if err != nil {
		stats.Unavailable = true
		stats.Error = err.Error()
	}
	return stats
}

func (s *FileStore) Stats(ctx context.Context, settings Settings) (StoreStats, error) {
	stats := StoreStats{}
	if s != nil {
		stats.Root = s.root
	}
	memories, err := s.EffectiveList(ctx, settings)
	if err != nil {
		return stats, err
	}
	settings.ApplyDefaults()
	stats.Total = len(memories)
	for _, memory := range memories {
		if memoryIsStale(memory) {
			stats.Stale++
		}
		if memoryNeedsReview(memory, settings) {
			stats.ReviewNeeded++
		}
	}
	return stats, nil
}

func (s *FileStore) Search(ctx context.Context, q Query) ([]Memory, error) {
	memories, err := s.EffectiveList(ctx, Settings{})
	if err != nil {
		return nil, err
	}
	out := make([]Memory, 0, len(memories))
	for _, memory := range memories {
		if MemoryMatches(memory, q) {
			out = append(out, memory)
		}
	}
	limit := q.Limit
	if limit <= 0 {
		limit = 20
	}
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func memoryIsStale(memory Memory) bool {
	return strings.EqualFold(strings.TrimSpace(memory.Status), StatusStale) ||
		strings.EqualFold(strings.TrimSpace(memory.Freshness), FreshnessStale)
}

func memoryNeedsReview(memory Memory, settings Settings) bool {
	switch strings.ToLower(strings.TrimSpace(memory.Status)) {
	case StatusConflict, StatusSourceMissing:
		return true
	}
	if strings.TrimSpace(memory.Details.ReviewReason) != "" {
		return true
	}
	if settings.UpdateRules.LowConfidenceDisposition == LowConfidenceReview {
		threshold := settings.UpdateRules.MatchThreshold
		if threshold <= 0 {
			threshold = settings.Thresholds.Match
		}
		if threshold <= 0 {
			threshold = 0.80
		}
		return memory.Confidence > 0 && memory.Confidence < threshold
	}
	return false
}

func ValidateMemory(memory Memory) error {
	if strings.TrimSpace(memory.ID) == "" {
		return fmt.Errorf("memory id is required")
	}
	if strings.TrimSpace(memory.Kind) == "" {
		return fmt.Errorf("memory kind is required")
	}
	if strings.TrimSpace(memory.Claim) == "" {
		return fmt.Errorf("memory claim is required")
	}
	if len(memory.Evidence) == 0 {
		return fmt.Errorf("memory evidence is required")
	}
	for i, evidence := range memory.Evidence {
		if err := ValidateEvidence(evidence); err != nil {
			return fmt.Errorf("memory evidence %d invalid: %w", i, err)
		}
	}
	return nil
}

func ValidateEvidence(evidence Evidence) error {
	sourceType := strings.TrimSpace(evidence.SourceType)
	if sourceType == "" {
		return fmt.Errorf("missing source type")
	}
	switch sourceType {
	case SourceEmail, SourceSentEmail:
		if firstNonEmpty(evidence.MessageID, evidence.LocalID, evidence.ID) == "" {
			return fmt.Errorf("%s evidence requires message_id, local_id, or id", sourceType)
		}
	case SourceObsidian:
		if strings.TrimSpace(evidence.Path) == "" {
			return fmt.Errorf("%s evidence requires path", sourceType)
		}
	case SourceResearch:
		if strings.TrimSpace(evidence.URL) == "" {
			return fmt.Errorf("%s evidence requires url", sourceType)
		}
	case SourceCalendar:
		if firstNonEmpty(evidence.ID, evidence.SourceID) == "" {
			return fmt.Errorf("%s evidence requires id or source_id", sourceType)
		}
	case SourceAttachment:
		if firstNonEmpty(evidence.ID, evidence.LocalID, evidence.Path) == "" {
			return fmt.Errorf("%s evidence requires id, local_id, or path", sourceType)
		}
	default:
		if displayEvidenceLabel(evidence) == "" {
			return fmt.Errorf("%s evidence requires a stable source pointer", sourceType)
		}
	}
	return nil
}

func (s *FileStore) pathFor(memory Memory) string {
	date := memory.CreatedAt
	if date.IsZero() {
		date = s.now()
	}
	kind := safePathPart(memory.Kind)
	if kind == "" {
		kind = "memory"
	}
	return filepath.Join(
		s.root,
		"records",
		date.Format("2006"),
		date.Format("01"),
		date.Format("02"),
		kind,
		safePathPart(memory.ID)+".json",
	)
}

func safePathPart(value string) string {
	value = strings.TrimSpace(value)
	var b strings.Builder
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
		case r >= 'A' && r <= 'Z':
			b.WriteRune(r)
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '-' || r == '_' || r == '.':
			b.WriteRune(r)
		default:
			b.WriteByte('-')
		}
	}
	return strings.Trim(b.String(), "-_.")
}
