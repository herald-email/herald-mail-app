package config

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"gopkg.in/yaml.v3"
	"os"
	"sort"
	"strings"
)

// Grants are versioned fingerprints, not a default-on feature switch. Changing
// accounts or recipients invalidates them and requires another affirmative choice.
type SharingConfig struct {
	AI         string `yaml:"ai,omitempty"`
	Background string `yaml:"background,omitempty"`
	MCP        string `yaml:"mcp,omitempty"`
}

func valueOr(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return strings.TrimSpace(value)
}

func (c *Config) SharingRecipients() string {
	provider := valueOr(c.AI.Provider, "ollama")
	chat := valueOr(c.Ollama.Host, "http://localhost:11434")
	switch provider {
	case "openai":
		chat = valueOr(c.OpenAI.BaseURL, "https://api.openai.com/v1")
	case "claude", "anthropic":
		chat = "https://api.anthropic.com"
	case "disabled":
		chat = "disabled"
	}
	embed := valueOr(c.Ollama.Host, "http://localhost:11434")
	if c.EffectiveEmbeddingProvider() == EmbeddingProviderOpenAI {
		embed = valueOr(c.OpenAI.BaseURL, "https://api.openai.com/v1")
	}
	agentProvider := valueOr(c.AI.Agent.Provider, provider)
	agentURL := strings.TrimSpace(c.AI.Agent.BaseURL)
	if agentURL == "" {
		switch agentProvider {
		case "openai":
			agentURL = valueOr(c.OpenAI.BaseURL, "https://api.openai.com/v1")
		case "anthropic", "claude":
			agentURL = "https://api.anthropic.com"
		case "kimi":
			agentURL = "https://api.moonshot.ai/v1"
		case "fireworks":
			agentURL = "https://api.fireworks.ai/inference/v1"
		default:
			agentURL = valueOr(c.Ollama.Host, "http://localhost:11434")
		}
	}
	return fmt.Sprintf("Chat (%s): %s\nEmbeddings (%s): %s\nAgent (%s): %s", provider, chat, c.EffectiveEmbeddingProvider(), embed, agentProvider, agentURL)
}

func (c *Config) SharingFingerprint(kind string) string {
	accounts := []string{}
	for _, s := range c.NormalizedSources() {
		accounts = append(accounts, strings.Join([]string{s.ID, s.Kind, s.Provider, s.AccountID, s.Google.Email, s.Credentials.Username, s.CalDAV.Username, s.CalDAV.URL, s.IMAP.Host}, "|"))
	}
	sort.Strings(accounts)
	recipients := "attached MCP clients (all enabled tools)"
	if kind != "mcp" {
		recipients = c.SharingRecipients()
	}
	raw, _ := json.Marshal([]any{"herald-sharing-v1", kind, accounts, recipients})
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func (c *Config) GrantSharing(ai, background, mcp bool) {
	c.Sharing = SharingConfig{}
	if ai {
		c.Sharing.AI = c.SharingFingerprint("ai")
	}
	if ai && background {
		c.Sharing.Background = c.SharingFingerprint("background")
	}
	if mcp {
		c.Sharing.MCP = c.SharingFingerprint("mcp")
	}
}

func (c *Config) sharingAllowed(kind string) bool {
	if c == nil {
		return false
	}
	grant := ""
	switch kind {
	case "ai":
		grant = c.Sharing.AI
	case "background":
		grant = c.Sharing.Background
	case "mcp":
		grant = c.Sharing.MCP
	}
	return grant != "" && grant == c.SharingFingerprint(kind)
}

// CheckSharing also observes revocation from another Herald process. The loaded
// client must still match the newly saved recipients before using private data.
func (c *Config) CheckSharing(kind string) error {
	denied := func() error {
		return fmt.Errorf("%s data sharing needs consent: open Settings > AI, review recipients, and explicitly allow sharing", kind)
	}
	if !c.sharingAllowed(kind) {
		return denied()
	}
	if c.sharingPath != "" {
		data, err := os.ReadFile(c.sharingPath)
		if err != nil {
			return denied()
		}
		var latest Config
		if yaml.Unmarshal(data, &latest) != nil {
			return denied()
		}
		latest.ApplyVendorPreset()
		latest.applyDefaults()
		if !latest.sharingAllowed(kind) || latest.SharingFingerprint(kind) != c.SharingFingerprint(kind) {
			return denied()
		}
	}
	return nil
}
