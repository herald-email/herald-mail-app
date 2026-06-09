package searchquery

import "testing"

func TestTermsNormalizesMultiWordMailboxQueries(t *testing.T) {
	got := Terms(`  "Herald" newsletter, latest!  `)
	want := []string{"herald", "newsletter", "latest"}
	if len(got) != len(want) {
		t.Fatalf("Terms len = %d, want %d: %#v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("Terms[%d] = %q, want %q; all terms %#v", i, got[i], want[i], got)
		}
	}
}

func TestMatchTermsFindsWordsAcrossMailboxFields(t *testing.T) {
	if !MatchTerms("Herald newsletter", "Herald Mail App", "You're in! Welcome to Herald Mail App Newsletter") {
		t.Fatal("expected query terms to match across sender and subject")
	}
	if MatchTerms("Herald newsletter", "Herald Mail App", "You're in! Welcome to Herald Mail App") {
		t.Fatal("expected all query terms to be required")
	}
}

func TestMatchTermsTreatsSimplePluralAsSingularVariant(t *testing.T) {
	if !MatchTerms("Herald newsletters", "Herald Mail App", "Welcome to Herald Mail App Newsletter") {
		t.Fatal("expected plural query term to match singular mailbox text")
	}
}
