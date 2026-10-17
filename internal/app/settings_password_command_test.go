package app

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/herald-email/herald-mail-app/internal/config"
)

func TestSettingsPreservesPasswordCommandUntilReplaced(t *testing.T) {
	for _, explicit := range []bool{false, true} {
		for _, replacement := range []string{"", "new-literal"} {
			t.Run(fmt.Sprintf("explicit=%v/replacement=%v", explicit, replacement != ""), func(t *testing.T) {
				marker := filepath.Join(t.TempDir(), "called")
				command := &config.PasswordCommand{Args: []string{"sh", "-c", `printf called > "$1"; printf password`, "helper", marker}}
				cfg := &config.Config{Credentials: config.CredentialsConfig{Username: "user@example.test", PasswordCommand: command}, Server: config.ServerConfig{Host: "localhost", Port: 1143}, SMTP: config.ServerConfig{Host: "localhost", Port: 1025}}
				if explicit {
					cfg.Sources = []config.SourceConfig{{ID: "work-mail", AccountID: "work", Kind: "mail", Provider: "imap", Credentials: cfg.Credentials, IMAP: cfg.Server, SMTP: cfg.SMTP}}
				}
				s := NewSettings(SettingsModePanel, cfg)
				s.panelSection = settingsPanelSectionAccounts
				groups := cfg.AccountGroups()
				if len(groups) != 1 {
					t.Fatal("missing account")
				}
				s.accountsMenuChoice = "account:" + groups[0].AccountID
				s.buildForm()
				if !s.openSelectedAccountFromList() {
					t.Fatal("open account")
				}
				if s.password != "" {
					t.Fatal("command copied to password input")
				}
				s.accountDisplayName = "Renamed account"
				s.password = replacement
				next := s.buildConfig()
				path := filepath.Join(t.TempDir(), "conf.yaml")
				if err := next.Save(path); err != nil {
					t.Fatal(err)
				}
				loaded, err := config.Load(path)
				if err != nil {
					t.Fatal(err)
				}
				credentials := loaded.NormalizedSources()[0].Credentials
				if replacement == "" {
					if !reflect.DeepEqual(credentials.PasswordCommand, command) || credentials.Password != "" {
						t.Fatal("settings lost command")
					}
				} else if credentials.PasswordCommand != nil || credentials.Password != replacement {
					t.Fatal("literal did not replace command")
				}
				if _, err := os.Stat(marker); !os.IsNotExist(err) {
					t.Fatal("editing/saving executed command")
				}
				if cfg.Credentials.PasswordCommand != command || cfg.Credentials.Password != "" {
					t.Fatal("editing mutated active config")
				}
			})
		}
	}
}

func TestSettingsNewAccountDoesNotInheritPasswordCommand(t *testing.T) {
	cfg := &config.Config{Credentials: config.CredentialsConfig{PasswordCommand: &config.PasswordCommand{Shell: "existing-account-command"}}}
	s := NewSettings(SettingsModePanel, cfg)
	s.accountEditMode = settingsAccountEditAddMail
	s.password = ""
	if s.mailCredentialsConfig().PasswordCommand != nil {
		t.Fatal("new account inherited existing command")
	}
}
