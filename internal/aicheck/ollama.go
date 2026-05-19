package aicheck

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/herald-email/herald-mail-app/internal/config"
)

const (
	defaultOllamaHost           = "http://localhost:11434"
	defaultOllamaModel          = "gemma3:4b"
	defaultOllamaEmbeddingModel = "nomic-embed-text-v2-moe"
	defaultOllamaTimeout        = 6 * time.Second
)

// MissingModel describes one configured local Ollama model that is not installed.
type MissingModel struct {
	Role string
	Name string
}

// Result is the bounded readiness result for local Ollama model validation.
type Result struct {
	Host    string
	Missing []MissingModel
	Failure error
}

func (r Result) OK() bool {
	return r.Failure == nil && len(r.Missing) == 0
}

func (r Result) Err() error {
	if r.OK() {
		return nil
	}
	if r.Failure != nil {
		return r.Failure
	}
	names := make([]string, 0, len(r.Missing))
	for _, missing := range r.Missing {
		names = append(names, missing.Name)
	}
	return fmt.Errorf("missing Ollama model(s): %s", strings.Join(dedupeStrings(names), ", "))
}

func (r Result) InstallCommands() []string {
	names := make([]string, 0, len(r.Missing))
	for _, missing := range r.Missing {
		if trimmed := strings.TrimSpace(missing.Name); trimmed != "" {
			names = append(names, trimmed)
		}
	}
	names = dedupeStrings(names)
	commands := make([]string, 0, len(names))
	for _, name := range names {
		commands = append(commands, "ollama pull "+name)
	}
	return commands
}

func (r Result) UserMessage(logPath, configPath string) string {
	if r.OK() {
		return "Ollama models are installed."
	}
	var parts []string
	if r.Failure != nil {
		host := strings.TrimSpace(r.Host)
		if host == "" {
			host = defaultOllamaHost
		}
		parts = append(parts, fmt.Sprintf("Ollama is not reachable at %s: %s.", host, sanitizeError(r.Failure)))
	} else {
		descriptions := make([]string, 0, len(r.Missing))
		for _, missing := range r.Missing {
			descriptions = append(descriptions, strings.TrimSpace(missing.Role)+" "+strings.TrimSpace(missing.Name))
		}
		parts = append(parts, "Ollama model(s) are not installed: "+strings.Join(dedupeStrings(descriptions), ", ")+".")
	}
	if commands := r.InstallCommands(); len(commands) > 0 {
		parts = append(parts, "Install: "+strings.Join(commands, " ; ")+".")
	}
	if configPath != "" {
		parts = append(parts, "Settings were not saved to "+configPath+".")
	} else {
		parts = append(parts, "Settings were not saved.")
	}
	if logPath != "" {
		parts = append(parts, "Debug log: "+logPath)
	}
	return strings.Join(parts, " ")
}

func (r Result) CompactMessage() string {
	if r.OK() {
		return "Ollama models are installed."
	}
	if commands := r.InstallCommands(); len(commands) > 0 {
		return "AI unavailable. Install: " + strings.Join(commands, " ; ")
	}
	return "AI unavailable. " + sanitizeError(r.Err())
}

type tagsResponse struct {
	Models []struct {
		Name  string `json:"name"`
		Model string `json:"model"`
	} `json:"models"`
}

// ValidateOllamaModels confirms that configured local Ollama model names are
// present in /api/tags. It never pulls or runs a model.
func ValidateOllamaModels(ctx context.Context, cfg *config.Config) Result {
	host := ollamaHost(cfg)
	result := Result{Host: host}
	if !OllamaConfigured(cfg) {
		return result
	}
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithTimeout(ctx, defaultOllamaTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(host, "/")+"/api/tags", nil)
	if err != nil {
		result.Failure = err
		return result
	}
	client := &http.Client{Timeout: defaultOllamaTimeout}
	resp, err := client.Do(req)
	if err != nil {
		result.Failure = err
		return result
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		payload, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		body := strings.TrimSpace(string(payload))
		if body != "" {
			result.Failure = fmt.Errorf("ollama /api/tags returned %d: %s", resp.StatusCode, body)
		} else {
			result.Failure = fmt.Errorf("ollama /api/tags returned %d", resp.StatusCode)
		}
		return result
	}

	var tags tagsResponse
	if err := json.NewDecoder(resp.Body).Decode(&tags); err != nil {
		result.Failure = fmt.Errorf("decode Ollama model list: %w", err)
		return result
	}
	installed := make(map[string]bool)
	for _, model := range tags.Models {
		if name := strings.TrimSpace(model.Name); name != "" {
			installed[name] = true
		}
		if name := strings.TrimSpace(model.Model); name != "" {
			installed[name] = true
		}
	}
	for _, required := range requiredModels(cfg) {
		if !modelInstalled(installed, required.Name) {
			result.Missing = append(result.Missing, required)
		}
	}
	return result
}

func RequiresOllamaModelValidation(previous, candidate *config.Config) bool {
	if !OllamaConfigured(candidate) {
		return false
	}
	if previous == nil || !OllamaConfigured(previous) {
		return true
	}
	return strings.TrimRight(ollamaHost(previous), "/") != strings.TrimRight(ollamaHost(candidate), "/") ||
		modelValue(previous.Ollama.Model, defaultOllamaModel) != modelValue(candidate.Ollama.Model, defaultOllamaModel) ||
		effectiveEmbeddingModel(previous) != effectiveEmbeddingModel(candidate)
}

func OllamaConfigured(cfg *config.Config) bool {
	if cfg == nil {
		return false
	}
	provider := strings.TrimSpace(cfg.AI.Provider)
	if provider != "" && provider != "ollama" {
		return false
	}
	return strings.TrimSpace(cfg.Ollama.Host) != ""
}

func requiredModels(cfg *config.Config) []MissingModel {
	if cfg == nil {
		return nil
	}
	chat := modelValue(cfg.Ollama.Model, defaultOllamaModel)
	embed := effectiveEmbeddingModel(cfg)
	if chat == embed {
		return []MissingModel{{Role: "chat/classification and embedding", Name: chat}}
	}
	return []MissingModel{
		{Role: "chat/classification", Name: chat},
		{Role: "embedding", Name: embed},
	}
}

func ollamaHost(cfg *config.Config) string {
	if cfg == nil || strings.TrimSpace(cfg.Ollama.Host) == "" {
		return defaultOllamaHost
	}
	return strings.TrimSpace(cfg.Ollama.Host)
}

func effectiveEmbeddingModel(cfg *config.Config) string {
	if cfg == nil {
		return defaultOllamaEmbeddingModel
	}
	return modelValue(cfg.EffectiveEmbeddingModel(), defaultOllamaEmbeddingModel)
}

func modelValue(value, fallback string) string {
	if trimmed := strings.TrimSpace(value); trimmed != "" {
		return trimmed
	}
	return fallback
}

func modelInstalled(installed map[string]bool, required string) bool {
	required = strings.TrimSpace(required)
	if required == "" {
		return true
	}
	if installed[required] {
		return true
	}
	if !strings.Contains(required, ":") && installed[required+":latest"] {
		return true
	}
	return false
}

func dedupeStrings(values []string) []string {
	seen := make(map[string]bool, len(values))
	var out []string
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

func sanitizeError(err error) string {
	if err == nil {
		return "unknown error"
	}
	msg := strings.TrimSpace(err.Error())
	msg = strings.ReplaceAll(msg, "\n", " ")
	msg = strings.ReplaceAll(msg, "\r", " ")
	if len(msg) > 220 {
		msg = msg[:217] + "..."
	}
	return msg
}
