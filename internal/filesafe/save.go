package filesafe

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// ExistingFileError reports a refused write and the next available path.
type ExistingFileError struct {
	Path          string
	SuggestedPath string
}

func (e *ExistingFileError) Error() string {
	if e.SuggestedPath == "" {
		return fmt.Sprintf("file already exists: %s", e.Path)
	}
	return fmt.Sprintf("file already exists: %s (suggested: %s)", e.Path, e.SuggestedPath)
}

// AsExistingFileError unwraps err when it represents an existing destination.
func AsExistingFileError(err error) (*ExistingFileError, bool) {
	var existingErr *ExistingFileError
	if errors.As(err, &existingErr) {
		return existingErr, true
	}
	return nil, false
}

// PathExists reports whether path currently exists.
func PathExists(path string) (bool, error) {
	_, err := os.Stat(path)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	return false, err
}

// NextAvailablePath returns the first numbered sibling path that does not exist.
func NextAvailablePath(path string) (string, error) {
	dir := filepath.Dir(path)
	name := filepath.Base(path)
	stem, ext := splitNameExt(name)

	for i := 1; i < 10000; i++ {
		candidate := filepath.Join(dir, fmt.Sprintf("%s (%d)%s", stem, i, ext))
		exists, err := PathExists(candidate)
		if err != nil {
			return "", err
		}
		if !exists {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("no available filename found for %s", path)
}

// SuggestIfExists returns a non-conflicting suggestion only when path exists.
func SuggestIfExists(path string) (suggested string, exists bool, err error) {
	exists, err = PathExists(path)
	if err != nil || !exists {
		return path, exists, err
	}
	suggested, err = NextAvailablePath(path)
	return suggested, true, err
}

// WriteFileExclusive writes data only if path does not exist.
func WriteFileExclusive(path string, data []byte, perm fs.FileMode) error {
	if suggested, exists, err := SuggestIfExists(path); err != nil {
		return err
	} else if exists {
		return &ExistingFileError{Path: path, SuggestedPath: suggested}
	}

	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, perm)
	if err != nil {
		if errors.Is(err, fs.ErrExist) {
			suggested, suggestErr := NextAvailablePath(path)
			if suggestErr != nil {
				return suggestErr
			}
			return &ExistingFileError{Path: path, SuggestedPath: suggested}
		}
		return err
	}
	defer f.Close()

	if _, err := f.Write(data); err != nil {
		return err
	}
	return f.Close()
}

func splitNameExt(name string) (string, string) {
	ext := filepath.Ext(name)
	stem := strings.TrimSuffix(name, ext)
	if stem == "" || strings.HasPrefix(name, ".") && strings.Count(name, ".") == 1 {
		return name, ""
	}
	return stem, ext
}
