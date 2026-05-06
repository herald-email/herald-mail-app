package app

import "strings"

func (m *Model) configuredSignature() string {
	if m == nil || m.cfg == nil {
		return ""
	}
	return normalizeSignatureText(m.cfg.Compose.Signature.Text)
}

func normalizeSignatureText(raw string) string {
	raw = strings.ReplaceAll(raw, "\r\n", "\n")
	raw = strings.ReplaceAll(raw, "\r", "\n")
	lines := strings.Split(raw, "\n")
	for len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "" {
		lines = lines[:len(lines)-1]
	}
	return strings.Join(lines, "\n")
}

func appendSignature(body, signature string) string {
	signature = normalizeSignatureText(signature)
	if signature == "" {
		return body
	}
	if strings.TrimSpace(body) == "" {
		return signature
	}
	if strings.HasSuffix(strings.TrimRight(body, " \t\r\n"), signature) {
		return body
	}
	return strings.TrimRight(body, "\r\n") + "\n\n" + signature
}

func (m *Model) applyConfiguredSignatureToComposeBody() {
	signature := m.configuredSignature()
	if signature == "" {
		return
	}
	m.composeBody.SetValue(appendSignature(m.composeBody.Value(), signature))
}

func (m *Model) composeBodyHasUserContent() bool {
	body := m.composeBody.Value()
	if strings.TrimSpace(body) == "" {
		return false
	}
	signature := m.configuredSignature()
	if signature != "" && strings.TrimSpace(body) == strings.TrimSpace(signature) {
		return false
	}
	return true
}
