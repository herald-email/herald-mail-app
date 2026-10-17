package mcpserver

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/herald-email/herald-mail-app/internal/config"
	"github.com/herald-email/herald-mail-app/internal/testmail"
)

func TestCacheOnlyMCPNeverExecutesPasswordCommand(t *testing.T) {
	c, _ := populateVirtualLabScenarioCache(t, testmail.ScenarioCalendlyInvite)
	marker := filepath.Join(t.TempDir(), "called")
	cfg := &config.Config{Credentials: config.CredentialsConfig{Username: "user@example.test", PasswordCommand: &config.PasswordCommand{Args: []string{"sh", "-c", `printf called > "$1"; printf password`, "helper", marker}}}, Server: config.ServerConfig{Host: "localhost", Port: 1143}}
	path := filepath.Join(t.TempDir(), "conf.yaml")
	if err := cfg.Save(path); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	s := newMCPServerWithConfig(c, nil, cfg)
	result := callVirtualLabTool(t, s, 1, "list_recent_emails", map[string]any{"folder": "INBOX", "limit": 5})
	if !strings.Contains(result, "Invitation: Product review with Bob") {
		t.Fatal("cached mail unavailable")
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatal("cache-only MCP executed password command")
	}
}
