package searchquery

import (
	"strings"
	"unicode"
)

// Terms normalizes a user mailbox query into lowercase searchable terms.
func Terms(query string) []string {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil
	}
	var normalized strings.Builder
	normalized.Grow(len(query))
	for _, r := range query {
		switch {
		case unicode.IsLetter(r), unicode.IsDigit(r):
			normalized.WriteRune(unicode.ToLower(r))
		case r == '@', r == '.', r == '_', r == '-', r == '+':
			normalized.WriteRune(unicode.ToLower(r))
		default:
			normalized.WriteByte(' ')
		}
	}

	raw := strings.Fields(normalized.String())
	terms := make([]string, 0, len(raw))
	seen := make(map[string]bool, len(raw))
	for _, term := range raw {
		term = strings.Trim(term, "._-+")
		if term == "" || seen[term] {
			continue
		}
		seen[term] = true
		terms = append(terms, term)
	}
	return terms
}

// TermVariants returns lightweight spelling variants for a normalized term.
func TermVariants(term string) []string {
	term = strings.Trim(strings.ToLower(strings.TrimSpace(term)), "._-+")
	if term == "" {
		return nil
	}
	variants := []string{term}
	add := func(candidate string) {
		candidate = strings.Trim(candidate, "._-+")
		if candidate == "" {
			return
		}
		for _, existing := range variants {
			if existing == candidate {
				return
			}
		}
		variants = append(variants, candidate)
	}
	if strings.ContainsAny(term, "@.") || len(term) < 4 {
		return variants
	}
	if strings.HasSuffix(term, "ies") && len(term) > 4 {
		add(strings.TrimSuffix(term, "ies") + "y")
	}
	if strings.HasSuffix(term, "es") && len(term) > 4 {
		stem := strings.TrimSuffix(term, "es")
		if strings.HasSuffix(stem, "s") ||
			strings.HasSuffix(stem, "x") ||
			strings.HasSuffix(stem, "z") ||
			strings.HasSuffix(stem, "ch") ||
			strings.HasSuffix(stem, "sh") {
			add(stem)
		}
	}
	if strings.HasSuffix(term, "s") && !strings.HasSuffix(term, "ss") && len(term) > 3 {
		add(strings.TrimSuffix(term, "s"))
	}
	return variants
}

// TermVariantGroups returns one variant group per normalized query term.
func TermVariantGroups(query string) [][]string {
	terms := Terms(query)
	if len(terms) == 0 {
		return nil
	}
	groups := make([][]string, 0, len(terms))
	for _, term := range terms {
		groups = append(groups, TermVariants(term))
	}
	return groups
}

// MatchTerms reports whether every normalized query term appears in the joined fields.
func MatchTerms(query string, fields ...string) bool {
	groups := TermVariantGroups(query)
	if len(groups) == 0 {
		return false
	}
	haystack := strings.ToLower(strings.Join(fields, "\n"))
	for _, group := range groups {
		matched := false
		for _, term := range group {
			if strings.Contains(haystack, term) {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}
	return true
}
