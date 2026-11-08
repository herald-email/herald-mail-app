package config

import (
	"bytes"
	"encoding/base64"
	"errors"
	"gopkg.in/yaml.v3"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSecureTokensMigrationAndTampering(t *testing.T) {
	t.Setenv(TokenStoreKeyEnv, base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{7}, 32)))
	cfg := minimalOAuthConfig()
	cfg.Gmail.AccessToken = "private-access"
	cfg.GrantSharing(true, true, true)
	path := filepath.Join(t.TempDir(), "config.yaml")
	original, _ := yaml.Marshal(cfg)
	if err := os.WriteFile(path, original, 0600); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	if bytes.Contains(data, []byte("rt-token")) || bytes.Contains(data, []byte("private-access")) {
		t.Fatal("tokens left in config")
	}
	if loaded.Gmail.AccessToken != "private-access" || loaded.Gmail.RefreshToken != "rt-token" {
		t.Fatal("migration lost tokens")
	}
	again, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if again.Gmail.RefreshToken != "rt-token" {
		t.Fatal("roundtrip lost token")
	}
	// Check consent also survives serialization without requiring plaintext tokens.
	if err := again.CheckSharing("ai"); err != nil {
		t.Fatal(err)
	}
	tokenPath := filepath.Join(path+".tokens", strings.TrimPrefix(again.Gmail.TokenRef, "encrypted:"))
	sealed, _ := os.ReadFile(tokenPath)
	sealed[len(sealed)-1] ^= 1
	os.WriteFile(tokenPath, sealed, 0600)
	if _, err := Load(path); err == nil {
		t.Fatal("tampered ciphertext accepted")
	}
}

func TestSecureStoreFailurePreservesLegacyConfig(t *testing.T) {
	t.Setenv(TokenStoreKeyEnv, "")
	old := keyringSet
	keyringSet = func(string, string, string) error { return errors.New("locked") }
	defer func() { keyringSet = old }()
	path := filepath.Join(t.TempDir(), "config.yaml")
	data, _ := yaml.Marshal(minimalOAuthConfig())
	os.WriteFile(path, data, 0600)
	if _, err := Load(path); err == nil {
		t.Fatal("migration silently fell back")
	}
	after, _ := os.ReadFile(path)
	if !bytes.Equal(data, after) {
		t.Fatal("failed migration changed config")
	}
}

func TestSharingInvalidationAndRevocation(t *testing.T) {
	cfg := &Config{}
	cfg.Credentials.Username = "a@example.com"
	cfg.Credentials.Password = "fake"
	cfg.Server.Host = "localhost"
	cfg.Server.Port = 993
	cfg.AI.Provider = "openai"
	cfg.GrantSharing(true, false, true)
	if cfg.CheckSharing("ai") != nil || cfg.CheckSharing("mcp") != nil {
		t.Fatal("grant rejected")
	}
	if cfg.CheckSharing("background") == nil {
		t.Fatal("background implicitly allowed")
	}
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := cfg.Save(path); err != nil {
		t.Fatal(err)
	}
	if err := cfg.CheckSharing("ai"); err != nil {
		t.Fatal(err)
	}
	next := *cfg
	next.GrantSharing(false, false, false)
	if err := next.Save(path); err != nil {
		t.Fatal(err)
	}
	if cfg.CheckSharing("ai") == nil || cfg.CheckSharing("mcp") == nil {
		t.Fatal("running client missed revocation")
	}
	cfg.sharingPath = ""
	cfg.Credentials.Username = "b@example.com"
	if cfg.CheckSharing("ai") == nil {
		t.Fatal("new account inherited consent")
	}
}

func TestKeyringMultiAccountRoundTrip(t *testing.T) {
	t.Setenv(TokenStoreKeyEnv, "")
	secrets := map[string]string{}
	oldSet, oldGet := keyringSet, keyringGet
	keyringSet = func(service, id, value string) error { secrets[id] = value; return nil }
	keyringGet = func(service, id string) (string, error) {
		v, ok := secrets[id]
		if !ok {
			return "", errors.New("missing")
		}
		return v, nil
	}
	defer func() { keyringSet, keyringGet = oldSet, oldGet }()
	cfg := &Config{Sources: []SourceConfig{
		{ID: "a", Kind: "mail", Provider: "gmail", Google: GoogleConfig{Email: "a@example.com", AccessToken: "access-a", RefreshToken: "refresh-a"}},
		{ID: "b", Kind: "calendar", Provider: "google_calendar", Google: GoogleConfig{Email: "b@example.com", AccessToken: "access-b", RefreshToken: "refresh-b"}},
	}}
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := cfg.Save(path); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Sources[0].Google.RefreshToken != "refresh-a" || loaded.Sources[1].Google.RefreshToken != "refresh-b" {
		t.Fatal("account tokens crossed")
	}
	data, _ := os.ReadFile(path)
	if bytes.Contains(data, []byte("refresh-a")) || bytes.Contains(data, []byte("access-b")) {
		t.Fatal("plaintext leak")
	}
	delete(secrets, strings.TrimPrefix(loaded.Sources[0].Google.TokenRef, "keyring:"))
	if _, err := Load(path); err == nil {
		t.Fatal("missing secure store silently accepted")
	}
}

func TestSecureSaveDoesNotMutateSharedSourceSlice(t *testing.T) {
	t.Setenv(TokenStoreKeyEnv, base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{8}, 32)))
	cfg := &Config{Sources: []SourceConfig{{ID: "mail", Kind: "mail", Provider: "gmail", Google: GoogleConfig{Email: "user@example.com", RefreshToken: "secret"}}}}
	source := cfg.Sources[0]
	if err := cfg.Save(filepath.Join(t.TempDir(), "config.yaml")); err != nil {
		t.Fatal(err)
	}
	if cfg.Sources[0].Google != source.Google {
		t.Fatal("save mutated the shared runtime source")
	}
}
