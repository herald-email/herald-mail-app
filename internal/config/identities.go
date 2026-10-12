package config

import (
	"fmt"
	"net/mail"
	"strings"
)

// IdentityConfig is a sender authorized by the mail provider, not a login.
type IdentityConfig struct {
	Name    string `yaml:"name,omitempty"`
	Address string `yaml:"address"`
}

// ValidateIdentities checks sender addresses and the configured default.
func (c ComposeConfig) ValidateIdentities(primary string) error {
	seen := map[string]bool{strings.ToLower(strings.TrimSpace(primary)): true}
	for _, identity := range c.Identities {
		address := strings.TrimSpace(identity.Address)
		parsed, err := mail.ParseAddress(address)
		if err != nil || parsed.Address != address || !strings.Contains(address, "@") || strings.ContainsAny(identity.Name+identity.Address, "\r\n") {
			return fmt.Errorf("invalid compose identity %q", identity.Address)
		}
		key := strings.ToLower(address)
		if seen[key] {
			return fmt.Errorf("duplicate compose identity %q", address)
		}
		seen[key] = true
	}
	if d := strings.ToLower(strings.TrimSpace(c.DefaultIdentity)); d != "" && !seen[d] {
		return fmt.Errorf("compose.default_identity %q is not a configured sender", c.DefaultIdentity)
	}
	return nil
}
