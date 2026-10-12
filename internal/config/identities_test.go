package config

import (
	"fmt"
	"path/filepath"
	"testing"
)

func TestComposeIdentitiesValidation(t *testing.T) {
	for _, tc := range []struct {
		name    string
		compose ComposeConfig
		bad     bool
	}{
		{"default primary", ComposeConfig{DefaultIdentity: "me@example.test"}, false},
		{"alias", ComposeConfig{Identities: []IdentityConfig{{Name: "Shop", Address: "shop@example.test"}}, DefaultIdentity: "shop@example.test"}, false},
		{"unknown default", ComposeConfig{DefaultIdentity: "other@example.test"}, true},
		{"invalid mailbox", ComposeConfig{Identities: []IdentityConfig{{Address: "bad"}}}, true},
		{"header injection", ComposeConfig{Identities: []IdentityConfig{{Name: "Shop\r\nBcc: other", Address: "shop@example.test"}}}, true},
		{"duplicate", ComposeConfig{Identities: []IdentityConfig{{Address: "shop@example.test"}, {Address: "SHOP@example.test"}}}, true},
		{"primary duplicate", ComposeConfig{Identities: []IdentityConfig{{Address: "ME@example.test"}}}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.compose.ValidateIdentities("me@example.test")
			if (err != nil) != tc.bad {
				t.Fatalf("error=%v, want bad=%v", err, tc.bad)
			}
		})
	}
}

func TestComposeIdentitiesLoadAndRoundTrip(t *testing.T) {
	for _, explicit := range []bool{false, true} {
		t.Run(fmt.Sprint(explicit), func(t *testing.T) {
			cfg := Config{Credentials: CredentialsConfig{Username: "me@example.test", Password: "test"}, Server: ServerConfig{Host: "127.0.0.1", Port: 1143}, SMTP: ServerConfig{Host: "127.0.0.1", Port: 1025}, Compose: ComposeConfig{Identities: []IdentityConfig{{Name: "Shop", Address: "shop@example.test"}}, DefaultIdentity: "shop@example.test"}}
			if explicit {
				cfg.Sources = cfg.NormalizedSources()
				cfg.Compose = ComposeConfig{}
			}
			path := filepath.Join(t.TempDir(), "config.yaml")
			if err := cfg.Save(path); err != nil {
				t.Fatal(err)
			}
			loaded, err := Load(path)
			if err != nil {
				t.Fatal(err)
			}
			compose := loaded.NormalizedSources()[0].Compose
			if compose.DefaultIdentity != "shop@example.test" || len(compose.Identities) != 1 {
				t.Fatalf("lost identities: %+v", compose)
			}
			if explicit {
				loaded.Sources[0].Compose.DefaultIdentity = "unknown@example.test"
			} else {
				loaded.Compose.DefaultIdentity = "unknown@example.test"
			}
			if err := loaded.validate(); err == nil {
				t.Fatal("validation accepted unknown identity")
			}
		})
	}
}
