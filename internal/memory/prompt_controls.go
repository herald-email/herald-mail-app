package memory

import (
	"fmt"
	"strings"
)

const (
	PromptWarningUnknownVariable = "unknown_variable"
	PromptWarningPrivateExport   = "private_export"
	PromptWarningWeakEvidence    = "weak_evidence"
	PromptWarningMutation        = "mutation"
	PromptWarningRadarNoise      = "compose_radar_noise"
)

type PromptValidationWarning struct {
	Type    string `json:"type" yaml:"type"`
	Message string `json:"message" yaml:"message"`
}

type PromptTestSnapshot struct {
	SourceSnippets          []string          `json:"source_snippets,omitempty" yaml:"source_snippets,omitempty"`
	EvidenceMetadata        []Evidence        `json:"evidence_metadata,omitempty" yaml:"evidence_metadata,omitempty"`
	CurrentDraftExcerpt     string            `json:"current_draft_excerpt,omitempty" yaml:"current_draft_excerpt,omitempty"`
	ConfiguredVaultTargets  []string          `json:"configured_vault_targets,omitempty" yaml:"configured_vault_targets,omitempty"`
	UserStylePreferences    string            `json:"user_style_preferences,omitempty" yaml:"user_style_preferences,omitempty"`
	AdditionalBoundedValues map[string]string `json:"additional_bounded_values,omitempty" yaml:"additional_bounded_values,omitempty"`
}

type PromptTestResult struct {
	TemplateName string                    `json:"template_name" yaml:"template_name"`
	Version      string                    `json:"version" yaml:"version"`
	Snapshot     PromptTestSnapshot        `json:"snapshot" yaml:"snapshot"`
	Warnings     []PromptValidationWarning `json:"warnings,omitempty" yaml:"warnings,omitempty"`
	Preview      string                    `json:"preview" yaml:"preview"`
}

func AllowedPromptVariables() map[string]bool {
	return map[string]bool{
		"source_snippets":          true,
		"evidence_metadata":        true,
		"configured_destinations":  true,
		"existing_track":           true,
		"new_evidence":             true,
		"configured_update_rules":  true,
		"reply_context":            true,
		"draft_excerpt":            true,
		"memory_candidates":        true,
		"memories":                 true,
		"tracks":                   true,
		"research_notes":           true,
		"source_list":              true,
		"frontmatter_mode":         true,
		"link_mode":                true,
		"tag_mode":                 true,
		"public_sources":           true,
		"person_or_company":        true,
		"last_contact_context":     true,
		"current_draft_excerpt":    true,
		"configured_vault_targets": true,
		"user_style_preferences":   true,
		"bounded_demo_fixture":     true,
		"selected_source_snapshot": true,
	}
}

func InternalGuardrailPromptNames() []string {
	return []string{
		"privacy_policy",
		"external_research_boundary",
		"evidence_requirement",
		"no_mutation_policy",
	}
}

func ResetPromptTemplate(prompts []PromptTemplate, name string) []PromptTemplate {
	name = strings.TrimSpace(name)
	defaults := DefaultPromptTemplates()
	var replacement PromptTemplate
	for _, candidate := range defaults {
		if strings.EqualFold(candidate.Name, name) {
			replacement = candidate
			break
		}
	}
	if replacement.Name == "" {
		return prompts
	}
	out := append([]PromptTemplate(nil), prompts...)
	replaced := false
	for i, candidate := range out {
		if strings.EqualFold(candidate.Name, name) {
			out[i] = replacement
			replaced = true
			break
		}
	}
	if !replaced {
		out = append(out, replacement)
	}
	return out
}

func ValidatePromptTemplate(prompt PromptTemplate) []PromptValidationWarning {
	allowed := AllowedPromptVariables()
	var warnings []PromptValidationWarning
	for _, variable := range prompt.Variables {
		variable = strings.TrimSpace(variable)
		if variable == "" {
			continue
		}
		if !allowed[variable] {
			warnings = append(warnings, PromptValidationWarning{
				Type:    PromptWarningUnknownVariable,
				Message: fmt.Sprintf("Variable %q is not part of Herald's bounded prompt snapshot contract.", variable),
			})
		}
	}
	text := strings.ToLower(strings.Join([]string{prompt.Name, prompt.Purpose, prompt.Template}, " "))
	for _, phrase := range []string{"private email body", "full email body", "full thread", "attachment contents", "export private"} {
		if strings.Contains(text, phrase) {
			warnings = append(warnings, PromptValidationWarning{
				Type:    PromptWarningPrivateExport,
				Message: "Prompt appears to request private-data export beyond bounded source snapshots.",
			})
			break
		}
	}
	for _, phrase := range []string{"ignore evidence", "without evidence", "invent", "guess facts"} {
		if strings.Contains(text, phrase) {
			warnings = append(warnings, PromptValidationWarning{
				Type:    PromptWarningWeakEvidence,
				Message: "Prompt appears to weaken source-backed evidence discipline.",
			})
			break
		}
	}
	for _, phrase := range []string{"send automatically", "delete automatically", "archive automatically", "mutate draft", "rewrite user notes"} {
		if strings.Contains(text, phrase) {
			warnings = append(warnings, PromptValidationWarning{
				Type:    PromptWarningMutation,
				Message: "Prompt appears to request mutation outside explicit user action.",
			})
			break
		}
	}
	if strings.EqualFold(prompt.Name, "compose_radar_nudge") {
		for _, phrase := range []string{"show every", "all memories", "low confidence", "warn often"} {
			if strings.Contains(text, phrase) {
				warnings = append(warnings, PromptValidationWarning{
					Type:    PromptWarningRadarNoise,
					Message: "Prompt may increase Compose Radar noise beyond the bounded nudge contract.",
				})
				break
			}
		}
	}
	return warnings
}

func TestPromptTemplate(prompt PromptTemplate, snapshot PromptTestSnapshot) PromptTestResult {
	warnings := ValidatePromptTemplate(prompt)
	snapshot = boundPromptSnapshot(snapshot)
	return PromptTestResult{
		TemplateName: prompt.Name,
		Version:      prompt.Version,
		Snapshot:     snapshot,
		Warnings:     warnings,
		Preview:      promptPreview(prompt, snapshot),
	}
}

func boundPromptSnapshot(snapshot PromptTestSnapshot) PromptTestSnapshot {
	for i, value := range snapshot.SourceSnippets {
		snapshot.SourceSnippets[i] = bounded(value, 300)
	}
	snapshot.EvidenceMetadata = compactBriefingEvidence(snapshot.EvidenceMetadata)
	snapshot.CurrentDraftExcerpt = bounded(snapshot.CurrentDraftExcerpt, 600)
	snapshot.ConfiguredVaultTargets = CompactStrings(snapshot.ConfiguredVaultTargets)
	snapshot.UserStylePreferences = bounded(snapshot.UserStylePreferences, 600)
	for key, value := range snapshot.AdditionalBoundedValues {
		snapshot.AdditionalBoundedValues[key] = bounded(value, 600)
	}
	return snapshot
}

func promptPreview(prompt PromptTemplate, snapshot PromptTestSnapshot) string {
	var parts []string
	parts = append(parts, fmt.Sprintf("%s@%s", prompt.Name, prompt.Version))
	if len(snapshot.SourceSnippets) > 0 {
		parts = append(parts, "snippets="+strings.Join(snapshot.SourceSnippets, " | "))
	}
	if len(snapshot.EvidenceMetadata) > 0 {
		parts = append(parts, "evidence="+renderBriefingEvidence(snapshot.EvidenceMetadata))
	}
	if snapshot.CurrentDraftExcerpt != "" {
		parts = append(parts, "draft="+snapshot.CurrentDraftExcerpt)
	}
	if len(snapshot.ConfiguredVaultTargets) > 0 {
		parts = append(parts, "targets="+strings.Join(snapshot.ConfiguredVaultTargets, ", "))
	}
	return bounded(strings.Join(parts, "\n"), 2000)
}
