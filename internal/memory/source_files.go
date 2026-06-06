package memory

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

var researchURLPattern = regexp.MustCompile(`https?://[^\s<>)"\]]+`)

func LoadConfiguredObsidianNotes(ctx context.Context, settings Settings) ([]ObsidianNoteSnapshot, error) {
	settings.ApplyDefaults()
	limit := settings.Sources.MaxObsidianNotes
	if limit <= 0 {
		limit = 100
	}
	root, err := expandedObsidianVault(settings)
	if err != nil || root == "" {
		return nil, err
	}
	paths, err := markdownFilesUnderConfiguredDirs(ctx, root, obsidianMemoryDirs(settings), limit)
	if err != nil {
		return nil, err
	}
	notes := make([]ObsidianNoteSnapshot, 0, len(paths))
	for _, path := range paths {
		if err := ctx.Err(); err != nil {
			return notes, err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return notes, err
		}
		body := stripObsidianGeneratedSections(stripYAMLFrontmatter(string(data)))
		body = strings.TrimSpace(body)
		if body == "" {
			continue
		}
		info, _ := os.Stat(path)
		modified := time.Time{}
		if info != nil {
			modified = info.ModTime()
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			rel = filepath.Base(path)
		}
		notes = append(notes, ObsidianNoteSnapshot{
			Path:       filepath.ToSlash(rel),
			Title:      markdownTitle(body, path),
			BodyText:   body,
			ModifiedAt: modified,
		})
	}
	return notes, nil
}

func LoadConfiguredResearchNotes(ctx context.Context, settings Settings) ([]ResearchNoteInput, error) {
	settings.ApplyDefaults()
	limit := settings.Sources.MaxResearchNotes
	if limit <= 0 {
		limit = 50
	}
	root, err := expandedObsidianVault(settings)
	if err != nil || root == "" {
		return nil, err
	}
	paths, err := markdownFilesUnderConfiguredDirs(ctx, root, []string{settings.Destinations.Research}, limit)
	if err != nil {
		return nil, err
	}
	notes := make([]ResearchNoteInput, 0, len(paths))
	for _, path := range paths {
		if err := ctx.Err(); err != nil {
			return notes, err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return notes, err
		}
		body := strings.TrimSpace(stripObsidianGeneratedSections(stripYAMLFrontmatter(string(data))))
		if body == "" {
			continue
		}
		url := researchURLPattern.FindString(body)
		if url == "" {
			continue
		}
		info, _ := os.Stat(path)
		retrievedAt := time.Now()
		if info != nil && !info.ModTime().IsZero() {
			retrievedAt = info.ModTime()
		}
		title := markdownTitle(body, path)
		summary := bounded(firstUsefulSentence(removeURLs(body), title), 300)
		notes = append(notes, ResearchNoteInput{
			Action:      ResearchActionRefreshDossier,
			Title:       title,
			Summary:     summary,
			URL:         strings.TrimRight(url, ".,;"),
			Query:       "saved research note",
			RetrievedAt: retrievedAt,
			Confidence:  settings.Thresholds.Dossier,
		})
	}
	return notes, nil
}

func expandedObsidianVault(settings Settings) (string, error) {
	vault := strings.TrimSpace(settings.Obsidian.VaultPath)
	if vault == "" {
		return "", nil
	}
	return ExpandDirectory(vault)
}

func obsidianMemoryDirs(settings Settings) []string {
	return CompactStrings([]string{
		settings.Destinations.People,
		settings.Destinations.Companies,
		settings.Destinations.JobSearch,
		settings.Destinations.Projects,
		settings.Destinations.Threads,
		settings.Destinations.Research,
		settings.Destinations.Inbox,
	})
}

func markdownFilesUnderConfiguredDirs(ctx context.Context, root string, dirs []string, limit int) ([]string, error) {
	root = filepath.Clean(root)
	var paths []string
	for _, dir := range dirs {
		if err := ctx.Err(); err != nil {
			return paths, err
		}
		child, ok := safeVaultChild(root, dir)
		if !ok {
			continue
		}
		err := filepath.WalkDir(child, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				if errors.Is(err, os.ErrNotExist) {
					return nil
				}
				return err
			}
			if d == nil {
				return nil
			}
			if err := ctx.Err(); err != nil {
				return err
			}
			if d.IsDir() {
				name := d.Name()
				if strings.HasPrefix(name, ".") || strings.EqualFold(name, "node_modules") {
					return filepath.SkipDir
				}
				return nil
			}
			if strings.EqualFold(filepath.Ext(path), ".md") {
				paths = append(paths, path)
			}
			return nil
		})
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return paths, err
		}
	}
	sort.Strings(paths)
	paths = compactFilePaths(paths)
	if limit > 0 && len(paths) > limit {
		paths = paths[:limit]
	}
	return paths, nil
}

func safeVaultChild(root, rel string) (string, bool) {
	rel = strings.TrimSpace(filepath.FromSlash(rel))
	if rel == "" {
		return "", false
	}
	if filepath.IsAbs(rel) {
		return "", false
	}
	clean := filepath.Clean(rel)
	if clean == "." || strings.HasPrefix(clean, "..") {
		return "", false
	}
	child := filepath.Join(root, clean)
	relative, err := filepath.Rel(root, child)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", false
	}
	return child, true
}

func stripYAMLFrontmatter(text string) string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	if !strings.HasPrefix(text, "---\n") {
		return text
	}
	end := strings.Index(text[4:], "\n---")
	if end < 0 {
		return text
	}
	after := 4 + end + len("\n---")
	if after < len(text) && text[after] == '\n' {
		after++
	}
	return text[after:]
}

func stripObsidianGeneratedSections(text string) string {
	for {
		start := strings.Index(text, generatedBegin)
		end := strings.Index(text, generatedEnd)
		if start < 0 || end < 0 || end < start {
			return text
		}
		end += len(generatedEnd)
		text = text[:start] + text[end:]
	}
}

func markdownTitle(body, path string) string {
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "# ") {
			return strings.TrimSpace(strings.TrimPrefix(line, "# "))
		}
	}
	name := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	if name == "" {
		return "Obsidian note"
	}
	return name
}

func removeURLs(value string) string {
	return strings.TrimSpace(researchURLPattern.ReplaceAllString(value, ""))
}

func compactFilePaths(paths []string) []string {
	seen := make(map[string]bool, len(paths))
	out := make([]string, 0, len(paths))
	for _, path := range paths {
		key := filepath.Clean(path)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, path)
	}
	return out
}
