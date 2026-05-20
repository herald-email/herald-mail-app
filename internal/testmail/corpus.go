package testmail

import (
	"bytes"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// CorpusScenario is a named group of sanitized raw mail fixtures.
type CorpusScenario struct {
	Name     string
	Messages []CorpusMessage
}

// CorpusMessage is one sanitized fixture file.
type CorpusMessage struct {
	Name string
	Path string
	Data []byte
}

var (
	emailPattern       = regexp.MustCompile(`[A-Za-z0-9._%+\-]+@[A-Za-z0-9.\-]+\.[A-Za-z]{2,}`)
	urlPattern         = regexp.MustCompile(`https?://[^\s"'<>)]*`)
	uuidPattern        = regexp.MustCompile(`(?i)\b[0-9a-f]{8}-[0-9a-f]{4}-[1-5][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}\b`)
	trackingParam      = regexp.MustCompile(`(?i)(utm_[a-z0-9_]*|fbclid|gclid|mc_cid|mc_eid)=`)
	secretTokenPattern = regexp.MustCompile(`(?i)(sk-[a-z0-9_\-]{12,}|gh[pousr]_[a-z0-9_]{12,}|xox[baprs]-[a-z0-9\-]{12,}|ya29\.[a-z0-9_\-]+|AKIA[0-9A-Z]{16})`)
	dateHeaderPattern  = regexp.MustCompile(`(?m)^Date: .+$`)
)

// LoadCorpus reads sanitized fixture scenarios from root. Each immediate child
// directory is a scenario; each .eml or .ics file inside it is a message.
func LoadCorpus(root string) (map[string]CorpusScenario, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}
	scenarios := make(map[string]CorpusScenario)
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		dir := filepath.Join(root, name)
		files, err := os.ReadDir(dir)
		if err != nil {
			return nil, err
		}
		scenario := CorpusScenario{Name: name}
		for _, file := range files {
			if file.IsDir() || !isCorpusFile(file.Name()) {
				continue
			}
			path := filepath.Join(dir, file.Name())
			data, err := os.ReadFile(path)
			if err != nil {
				return nil, err
			}
			scenario.Messages = append(scenario.Messages, CorpusMessage{
				Name: file.Name(),
				Path: path,
				Data: data,
			})
		}
		sort.Slice(scenario.Messages, func(i, j int) bool {
			return scenario.Messages[i].Name < scenario.Messages[j].Name
		})
		scenarios[name] = scenario
	}
	return scenarios, nil
}

// ValidateCorpus checks every committed corpus fixture for PII-shaped values.
func ValidateCorpus(root string) error {
	scenarios, err := LoadCorpus(root)
	if err != nil {
		return err
	}
	var failures []string
	for _, scenario := range scenarios {
		for _, msg := range scenario.Messages {
			if err := ValidateSanitizedBytes(msg.Path, msg.Data); err != nil {
				failures = append(failures, err.Error())
			}
		}
	}
	if len(failures) > 0 {
		sort.Strings(failures)
		return errors.New(strings.Join(failures, "\n"))
	}
	return nil
}

// ValidateSanitizedBytes checks a single sanitized mail fixture.
func ValidateSanitizedBytes(label string, data []byte) error {
	var failures []string
	for _, match := range emailPattern.FindAll(data, -1) {
		addr := strings.ToLower(string(match))
		_, domain, ok := strings.Cut(addr, "@")
		if !ok || !allowedTestDomain(domain) {
			failures = append(failures, fmt.Sprintf("%s: non-test email address %q", label, addr))
		}
	}
	for _, match := range urlPattern.FindAll(data, -1) {
		rawURL := strings.TrimRight(string(match), ".,;")
		parsed, err := url.Parse(rawURL)
		if err != nil || parsed.Hostname() == "" {
			failures = append(failures, fmt.Sprintf("%s: unparsable URL %q", label, rawURL))
			continue
		}
		if !allowedTestHost(parsed.Hostname()) {
			failures = append(failures, fmt.Sprintf("%s: external URL %q", label, rawURL))
		}
		if trackingParam.MatchString(parsed.RawQuery) {
			failures = append(failures, fmt.Sprintf("%s: tracking query parameter in %q", label, rawURL))
		}
	}
	if uuidPattern.Match(data) {
		failures = append(failures, fmt.Sprintf("%s: UUID-like value present", label))
	}
	if secretTokenPattern.Match(data) {
		failures = append(failures, fmt.Sprintf("%s: secret-token-like value present", label))
	}
	if bytes.Contains(bytes.ToUpper(data), []byte("BEGIN PRIVATE KEY")) {
		failures = append(failures, fmt.Sprintf("%s: private-key-like block present", label))
	}
	if len(failures) > 0 {
		return errors.New(strings.Join(failures, "\n"))
	}
	return nil
}

// SanitizeBytes performs deterministic rewrites for raw quarantine mail. It is
// intentionally conservative; ValidateSanitizedBytes remains the commit gate.
func SanitizeBytes(data []byte) []byte {
	out := append([]byte(nil), data...)
	emailMap := make(map[string]string)
	emailCount := 0
	out = emailPattern.ReplaceAllFunc(out, func(match []byte) []byte {
		key := strings.ToLower(string(match))
		if replacement, ok := emailMap[key]; ok {
			return []byte(replacement)
		}
		emailCount++
		replacement := fmt.Sprintf("person%d@herald.test", emailCount)
		emailMap[key] = replacement
		return []byte(replacement)
	})
	linkCount := 0
	out = urlPattern.ReplaceAllFunc(out, func(match []byte) []byte {
		linkCount++
		return []byte(fmt.Sprintf("https://example.test/redacted-link-%d", linkCount))
	})
	out = dateHeaderPattern.ReplaceAll(out, []byte("Date: Wed, 20 May 2026 10:00:00 -0700"))
	out = uuidPattern.ReplaceAll(out, []byte("fixture-id-redacted"))
	out = secretTokenPattern.ReplaceAll(out, []byte("REDACTED-TOKEN"))
	return out
}

func isCorpusFile(name string) bool {
	ext := strings.ToLower(filepath.Ext(name))
	return ext == ".eml" || ext == ".ics"
}

func allowedTestDomain(domain string) bool {
	domain = strings.Trim(strings.ToLower(domain), " \t\r\n.,;:")
	return strings.HasSuffix(domain, ".test") || domain == "herald.demo"
}

func allowedTestHost(host string) bool {
	host = strings.Trim(strings.ToLower(host), "[]")
	return host == "localhost" || host == "127.0.0.1" || strings.HasSuffix(host, ".test")
}
