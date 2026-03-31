package contacts

import "testing"

func TestParseAppleScriptOutput(t *testing.T) {
	input := "Alice Smith|alice@example.com\nBob Jones|bob@work.org\n"
	got := parseAppleScriptOutput(input)
	if len(got) != 2 {
		t.Fatalf("expected 2, got %d", len(got))
	}
	if got[0].Name != "Alice Smith" || got[0].Email != "alice@example.com" {
		t.Errorf("unexpected first contact: %+v", got[0])
	}
}

func TestParseAppleScriptOutput_SkipsBlankLines(t *testing.T) {
	input := "\nAlice|alice@x.com\n\n"
	got := parseAppleScriptOutput(input)
	if len(got) != 1 {
		t.Fatalf("expected 1, got %d", len(got))
	}
}

func TestParseAppleScriptOutput_SkipsMissingEmail(t *testing.T) {
	input := "Alice|\n"
	got := parseAppleScriptOutput(input)
	if len(got) != 0 {
		t.Fatalf("expected 0, got %d", len(got))
	}
}

func TestParseAppleScriptOutput_SkipsMalformedLine(t *testing.T) {
	input := "JustANameWithNoPipe\nalice@example.com\nBob|bob@example.com\n"
	got := parseAppleScriptOutput(input)
	// Only Bob has a valid pipe-separated line
	if len(got) != 1 {
		t.Fatalf("expected 1, got %d: %v", len(got), got)
	}
	if got[0].Name != "Bob" || got[0].Email != "bob@example.com" {
		t.Errorf("unexpected contact: %+v", got[0])
	}
}
