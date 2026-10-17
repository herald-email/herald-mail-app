package sshserver

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/herald-email/herald-mail-app/internal/config"
	"github.com/herald-email/herald-mail-app/internal/testmail"
)

func TestVirtualLabSSHWithPasswordCommand(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX helper")
	}
	marker := filepath.Join(t.TempDir(), "called")
	h := startVirtualLabSSH(t, testmail.ScenarioCalendlyInvite, 80, 24, func(cfg *config.Config) {
		cfg.Credentials.Password = ""
		cfg.Credentials.PasswordCommand = &config.PasswordCommand{Args: []string{"sh", "-c", `printf called > "$1"; printf password`, "helper", marker}}
	})
	h.waitForVisible(t, "Invitation")
	if _, err := os.Stat(marker); err != nil {
		t.Fatal("SSH mailbox authentication did not execute helper")
	}
	h.sendKey(t, "enter")
	h.waitForVisible(t, "Join meeting")
}
