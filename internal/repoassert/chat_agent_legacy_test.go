package repoassert

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestChatPanelDoesNotUseLegacyDirectOllamaRuntime(t *testing.T) {
	disallowedFiles := []string{
		repoPath(t, "internal", "app", "chat_filter.go"),
		repoPath(t, "internal", "app", "chat_tools.go"),
	}
	for _, path := range disallowedFiles {
		if _, err := os.Stat(path); err == nil {
			t.Fatalf("legacy chat runtime file still exists: %s", path)
		} else if !os.IsNotExist(err) {
			t.Fatalf("stat %s: %v", path, err)
		}
	}

	needles := map[string][]string{
		repoPath(t, "internal", "app", "chat_panel.go"): {
			"ChatWithTools(",
			"chatToolRegistry",
			"<filter>",
			"maxToolRounds",
		},
		repoPath(t, "internal", "app", "app.go"): {
			"case ChatResponseMsg:",
			"parseChatFilter(",
			"stripChatFilter(",
		},
	}
	for path, disallowed := range needles {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		text := string(data)
		for _, needle := range disallowed {
			if strings.Contains(text, needle) {
				t.Fatalf("%s still contains legacy chat runtime marker %q", filepath.Base(path), needle)
			}
		}
	}
}
