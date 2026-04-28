package app

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type attachmentPathCandidate struct {
	Display string
	Value   string
	IsDir   bool
}

type attachmentPathCompletion struct {
	Completed  string
	Candidates []attachmentPathCandidate
	Status     string
}

func completeAttachmentPath(input, cwd string) attachmentPathCompletion {
	if cwd == "" {
		cwd = "."
	}

	fsDir, valuePrefix, segment := attachmentCompletionParts(input, cwd)
	entries, err := os.ReadDir(fsDir)
	if err != nil {
		return attachmentPathCompletion{Status: fmt.Sprintf("Cannot read directory: %v", err)}
	}

	segmentLower := strings.ToLower(segment)
	showHidden := strings.HasPrefix(segment, ".")
	candidates := make([]attachmentPathCandidate, 0, len(entries))
	for _, entry := range entries {
		name := entry.Name()
		if strings.HasPrefix(name, ".") && !showHidden {
			continue
		}
		if !strings.HasPrefix(strings.ToLower(name), segmentLower) {
			continue
		}
		isDir := entry.IsDir()
		display := name
		value := valuePrefix + name
		if isDir {
			display += string(os.PathSeparator)
			value += string(os.PathSeparator)
		}
		candidates = append(candidates, attachmentPathCandidate{
			Display: display,
			Value:   value,
			IsDir:   isDir,
		})
	}

	if len(candidates) == 0 {
		return attachmentPathCompletion{Status: "No matches"}
	}

	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].IsDir != candidates[j].IsDir {
			return candidates[i].IsDir
		}
		left := strings.ToLower(candidates[i].Display)
		right := strings.ToLower(candidates[j].Display)
		if left == right {
			return candidates[i].Display < candidates[j].Display
		}
		return left < right
	})

	completed := candidates[0].Value
	if len(candidates) > 1 {
		values := make([]string, len(candidates))
		for i, candidate := range candidates {
			values[i] = candidate.Value
		}
		completed = longestCommonStringPrefix(values)
	}

	return attachmentPathCompletion{
		Completed:  completed,
		Candidates: candidates,
	}
}

func attachmentCompletionParts(input, cwd string) (fsDir, valuePrefix, segment string) {
	if input == "~" {
		if home, err := os.UserHomeDir(); err == nil {
			return home, "~" + string(os.PathSeparator), ""
		}
	}

	valuePrefix, segment = filepath.Split(input)
	fsPrefix := expandTilde(valuePrefix)
	if fsPrefix == "" {
		fsDir = cwd
	} else if filepath.IsAbs(fsPrefix) {
		fsDir = fsPrefix
	} else {
		fsDir = filepath.Join(cwd, fsPrefix)
	}
	return filepath.Clean(fsDir), valuePrefix, segment
}

func longestCommonStringPrefix(values []string) string {
	if len(values) == 0 {
		return ""
	}
	prefix := []rune(values[0])
	for _, value := range values[1:] {
		runes := []rune(value)
		limit := len(prefix)
		if len(runes) < limit {
			limit = len(runes)
		}
		i := 0
		for i < limit && prefix[i] == runes[i] {
			i++
		}
		prefix = prefix[:i]
		if len(prefix) == 0 {
			break
		}
	}
	return string(prefix)
}

func ensureTrailingPathSeparator(path string) string {
	if path == "" || strings.HasSuffix(path, string(os.PathSeparator)) {
		return path
	}
	return path + string(os.PathSeparator)
}

func isDirectoryPath(path string) bool {
	info, err := os.Stat(expandTilde(path))
	return err == nil && info.IsDir()
}
