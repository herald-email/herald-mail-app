//go:build live_keychain

package config

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/zalando/go-keyring"
	"gopkg.in/yaml.v3"
)

// Explicitly opt-in: exercises the real macOS Keychain with synthetic tokens.
// It never accesses or deletes an existing Herald credential.
func TestLiveMacOSKeychainRoundTrip(t *testing.T) {
	if runtime.GOOS != "darwin" || os.Getenv("HERALD_LIVE_KEYCHAIN_TEST") != "1" {
		t.Skip("requires explicit live macOS Keychain opt-in")
	}
	t.Setenv(TokenStoreKeyEnv, "")
	path := filepath.Join(t.TempDir(), "keychain-validation.yaml")
	cfg := minimalOAuthConfig()
	cfg.Gmail.AccessToken = "synthetic-keychain-access"
	cfg.Gmail.RefreshToken = "synthetic-keychain-refresh"
	if err := cfg.Save(path); err != nil {
		t.Fatal("secure save failed:", err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var disk Config
	if err := yaml.Unmarshal(raw, &disk); err != nil {
		t.Fatal(err)
	}
	ref := strings.TrimPrefix(disk.Gmail.TokenRef, "keyring:")
	if ref == disk.Gmail.TokenRef || ref == "" {
		t.Fatal("did not select OS credential storage")
	}
	t.Cleanup(func() {
		if err := keyring.Delete(tokenService, ref); err != nil {
			t.Error("synthetic credential cleanup failed:", err)
		}
	})
	if strings.Contains(string(raw), "synthetic-keychain-") {
		t.Fatal("plaintext token persisted in YAML")
	}
	loaded, err := Load(path)
	if err != nil {
		t.Fatal("Keychain reload failed:", err)
	}
	if loaded.Gmail.AccessToken != cfg.Gmail.AccessToken || loaded.Gmail.RefreshToken != cfg.Gmail.RefreshToken {
		t.Fatal("Keychain roundtrip mismatch")
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0600 {
		t.Fatal("configuration permissions are not 0600")
	}
	t.Log("Real macOS Keychain write/read passed; YAML contains only a token reference; synthetic entry removed at cleanup.")
}
