package contacts

import "testing"

func TestParseContactsJSON(t *testing.T) {
	input := `[{"name":"Alice Smith","email":"alice@example.com"},{"name":"Bob Jones","email":"bob@work.org"}]`
	got := parseContactsJSON(input)
	if len(got) != 2 {
		t.Fatalf("expected 2, got %d", len(got))
	}
	if got[0].Name != "Alice Smith" || got[0].Email != "alice@example.com" {
		t.Errorf("unexpected first contact: %+v", got[0])
	}
}

func TestParseContactsJSON_EmptyInput(t *testing.T) {
	got := parseContactsJSON("\n\n")
	if len(got) != 0 {
		t.Fatalf("expected 0, got %d", len(got))
	}
}

func TestParseContactsJSON_SkipsMissingEmail(t *testing.T) {
	input := `[{"name":"Alice","email":""}]`
	got := parseContactsJSON(input)
	if len(got) != 0 {
		t.Fatalf("expected 0, got %d", len(got))
	}
}

func TestParseContactsJSON_MalformedInputIsEmpty(t *testing.T) {
	got := parseContactsJSON(`not json`)
	if len(got) != 0 {
		t.Fatalf("expected 0, got %d: %v", len(got), got)
	}
}
