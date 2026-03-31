package cache

import (
	"math"
	"testing"
	"time"

	"mail-processor/internal/models"
)

// seedContact inserts a single contact row directly via UpsertContacts.
func seedContact(t *testing.T, c *Cache, email, name string, emailCount int) {
	t.Helper()
	// Insert base row
	addrs := []models.ContactAddr{{Email: email, Name: name}}
	for i := 0; i < emailCount; i++ {
		if err := c.UpsertContacts(addrs, "from"); err != nil {
			t.Fatalf("UpsertContacts: %v", err)
		}
	}
}

func TestGetContactsToEnrich_ReturnsOnlyUnderThreshold(t *testing.T) {
	c := newTestCache(t)

	// Contact with emailCount == 2: should NOT be returned (below threshold of 3)
	seedContact(t, c, "low@example.com", "Low", 2)
	// Contact with emailCount == 3: should be returned
	seedContact(t, c, "enough@example.com", "Enough", 3)
	// Contact with emailCount == 5: should be returned
	seedContact(t, c, "many@example.com", "Many", 5)

	got, err := c.GetContactsToEnrich(3, 10)
	if err != nil {
		t.Fatalf("GetContactsToEnrich: %v", err)
	}
	emails := make(map[string]bool)
	for _, cd := range got {
		emails[cd.Email] = true
	}
	if emails["low@example.com"] {
		t.Error("low@example.com (count=2) should not be returned")
	}
	if !emails["enough@example.com"] {
		t.Error("enough@example.com (count=3) should be returned")
	}
	if !emails["many@example.com"] {
		t.Error("many@example.com (count=5) should be returned")
	}
}

func TestGetContactsToEnrich_ExcludesAlreadyEnriched(t *testing.T) {
	c := newTestCache(t)

	seedContact(t, c, "enriched@example.com", "Enriched", 5)
	// Mark as enriched
	if err := c.UpdateContactEnrichment("enriched@example.com", "Acme", []string{"sales"}); err != nil {
		t.Fatalf("UpdateContactEnrichment: %v", err)
	}

	seedContact(t, c, "pending@example.com", "Pending", 4)

	got, err := c.GetContactsToEnrich(3, 10)
	if err != nil {
		t.Fatalf("GetContactsToEnrich: %v", err)
	}
	for _, cd := range got {
		if cd.Email == "enriched@example.com" {
			t.Error("enriched contact should not appear in GetContactsToEnrich results")
		}
	}
	found := false
	for _, cd := range got {
		if cd.Email == "pending@example.com" {
			found = true
		}
	}
	if !found {
		t.Error("pending@example.com should appear in GetContactsToEnrich results")
	}
}

func TestGetContactsToEnrich_RespectsLimit(t *testing.T) {
	c := newTestCache(t)

	for i := 0; i < 10; i++ {
		email := "user" + string(rune('0'+i)) + "@example.com"
		seedContact(t, c, email, "User", 5)
	}

	got, err := c.GetContactsToEnrich(3, 3)
	if err != nil {
		t.Fatalf("GetContactsToEnrich: %v", err)
	}
	if len(got) != 3 {
		t.Errorf("expected 3 results with limit=3, got %d", len(got))
	}
}

func TestGetRecentSubjectsByContact(t *testing.T) {
	c := newTestCache(t)

	// Seed emails with a matching sender
	now := time.Now()
	emails := []*models.EmailData{
		{MessageID: "m1", Sender: "Alice <alice@work.com>", Subject: "Project update", Date: now, Folder: "INBOX"},
		{MessageID: "m2", Sender: "Alice <alice@work.com>", Subject: "Meeting tomorrow", Date: now.Add(-time.Hour), Folder: "INBOX"},
		{MessageID: "m3", Sender: "Bob <bob@other.com>", Subject: "Unrelated", Date: now, Folder: "INBOX"},
	}
	for _, e := range emails {
		if err := c.CacheEmail(e); err != nil {
			t.Fatalf("CacheEmail: %v", err)
		}
	}

	subjects, err := c.GetRecentSubjectsByContact("alice@work.com", 10)
	if err != nil {
		t.Fatalf("GetRecentSubjectsByContact: %v", err)
	}
	if len(subjects) != 2 {
		t.Errorf("expected 2 subjects for alice@work.com, got %d: %v", len(subjects), subjects)
	}
	for _, s := range subjects {
		if s == "Unrelated" {
			t.Error("Bob's email should not appear in alice's results")
		}
	}
}

func TestGetRecentSubjectsByContact_RespectsLimit(t *testing.T) {
	c := newTestCache(t)

	now := time.Now()
	for i := 0; i < 15; i++ {
		e := &models.EmailData{
			MessageID: "m" + string(rune('a'+i)),
			Sender:    "sender@test.com",
			Subject:   "Subject " + string(rune('A'+i)),
			Date:      now.Add(-time.Duration(i) * time.Minute),
			Folder:    "INBOX",
		}
		if err := c.CacheEmail(e); err != nil {
			t.Fatalf("CacheEmail: %v", err)
		}
	}

	subjects, err := c.GetRecentSubjectsByContact("sender@test.com", 5)
	if err != nil {
		t.Fatalf("GetRecentSubjectsByContact: %v", err)
	}
	if len(subjects) != 5 {
		t.Errorf("expected 5 subjects with limit=5, got %d", len(subjects))
	}
}

func TestUpdateContactEnrichment(t *testing.T) {
	c := newTestCache(t)

	seedContact(t, c, "contact@example.com", "Contact", 3)

	if err := c.UpdateContactEnrichment("contact@example.com", "Acme Corp", []string{"sales", "marketing"}); err != nil {
		t.Fatalf("UpdateContactEnrichment: %v", err)
	}

	// Verify enriched_at is now set (contact no longer returned by GetContactsToEnrich)
	got, err := c.GetContactsToEnrich(3, 10)
	if err != nil {
		t.Fatalf("GetContactsToEnrich: %v", err)
	}
	for _, cd := range got {
		if cd.Email == "contact@example.com" {
			t.Error("enriched contact should not appear again")
		}
	}
}

func TestUpdateContactEnrichment_NilTopics(t *testing.T) {
	c := newTestCache(t)
	seedContact(t, c, "nil@example.com", "Nil Topics", 3)
	// Should not panic or error with nil topics slice
	if err := c.UpdateContactEnrichment("nil@example.com", "", nil); err != nil {
		t.Fatalf("UpdateContactEnrichment with nil topics: %v", err)
	}
}

func TestUpdateContactEmbedding(t *testing.T) {
	c := newTestCache(t)
	seedContact(t, c, "emb@example.com", "Embed Me", 3)

	vec := []float32{0.1, 0.2, 0.3, 0.4}
	if err := c.UpdateContactEmbedding("emb@example.com", vec); err != nil {
		t.Fatalf("UpdateContactEmbedding: %v", err)
	}

	// Verify the embedding is stored and decodable via SearchContactsSemantic
	results, err := c.SearchContactsSemantic(vec, 10, 0.0)
	if err != nil {
		t.Fatalf("SearchContactsSemantic: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("expected at least one result after storing embedding")
	}
	found := false
	for _, r := range results {
		if r.Contact.Email == "emb@example.com" {
			found = true
		}
	}
	if !found {
		t.Error("emb@example.com not found in semantic search results")
	}
}

func TestSearchContactsSemantic_CosineSimilarity(t *testing.T) {
	c := newTestCache(t)

	// Two contacts: one aligned with query, one orthogonal
	seedContact(t, c, "aligned@example.com", "Aligned", 3)
	seedContact(t, c, "orthogonal@example.com", "Orthogonal", 3)

	// aligned vector: [1, 0]  — same direction as query [1, 0]
	alignedVec := []float32{1.0, 0.0}
	// orthogonal vector: [0, 1] — perpendicular to query
	orthVec := []float32{0.0, 1.0}
	queryVec := []float32{1.0, 0.0}

	if err := c.UpdateContactEmbedding("aligned@example.com", alignedVec); err != nil {
		t.Fatalf("UpdateContactEmbedding aligned: %v", err)
	}
	if err := c.UpdateContactEmbedding("orthogonal@example.com", orthVec); err != nil {
		t.Fatalf("UpdateContactEmbedding orthogonal: %v", err)
	}

	// minScore = 0.5: should only return aligned (score ~1.0), not orthogonal (score 0.0)
	results, err := c.SearchContactsSemantic(queryVec, 10, 0.5)
	if err != nil {
		t.Fatalf("SearchContactsSemantic: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result with minScore=0.5, got %d", len(results))
	}
	if results[0].Contact.Email != "aligned@example.com" {
		t.Errorf("expected aligned@example.com, got %s", results[0].Contact.Email)
	}
}

func TestSearchContactsSemantic_Ordering(t *testing.T) {
	c := newTestCache(t)

	// Three contacts with decreasing similarity to [1, 1, 0]
	seedContact(t, c, "a@example.com", "A", 3)
	seedContact(t, c, "b@example.com", "B", 3)
	seedContact(t, c, "cc@example.com", "C", 3)

	vA := []float32{1.0, 1.0, 0.0} // angle = 0 to query
	vB := []float32{1.0, 0.0, 0.0} // angle ~45°
	vC := []float32{0.0, 1.0, 0.0} // angle ~45° (same as B)
	query := []float32{1.0, 1.0, 0.0}

	for em, v := range map[string][]float32{"a@example.com": vA, "b@example.com": vB, "cc@example.com": vC} {
		if err := c.UpdateContactEmbedding(em, v); err != nil {
			t.Fatalf("UpdateContactEmbedding %s: %v", em, err)
		}
	}

	results, err := c.SearchContactsSemantic(query, 10, 0.0)
	if err != nil {
		t.Fatalf("SearchContactsSemantic: %v", err)
	}
	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}
	// First result must be "a@example.com" (score ~1.0)
	if results[0].Contact.Email != "a@example.com" {
		t.Errorf("expected a@example.com first, got %s", results[0].Contact.Email)
	}
}

func TestSearchContactsSemantic_Limit(t *testing.T) {
	c := newTestCache(t)

	for i := 0; i < 5; i++ {
		email := "u" + string(rune('0'+i)) + "@example.com"
		seedContact(t, c, email, "User", 3)
		v := []float32{float32(i + 1), 0.0}
		if err := c.UpdateContactEmbedding(email, v); err != nil {
			t.Fatalf("UpdateContactEmbedding: %v", err)
		}
	}

	query := []float32{1.0, 0.0}
	results, err := c.SearchContactsSemantic(query, 3, 0.0)
	if err != nil {
		t.Fatalf("SearchContactsSemantic: %v", err)
	}
	if len(results) > 3 {
		t.Errorf("expected at most 3 results with limit=3, got %d", len(results))
	}
}

func TestSearchContactsSemantic_TopicsDecoded(t *testing.T) {
	c := newTestCache(t)
	seedContact(t, c, "topics@example.com", "Topics", 3)

	if err := c.UpdateContactEnrichment("topics@example.com", "TopicCo", []string{"alpha", "beta"}); err != nil {
		t.Fatalf("UpdateContactEnrichment: %v", err)
	}
	if err := c.UpdateContactEmbedding("topics@example.com", []float32{1.0, 0.0}); err != nil {
		t.Fatalf("UpdateContactEmbedding: %v", err)
	}

	results, err := c.SearchContactsSemantic([]float32{1.0, 0.0}, 10, 0.0)
	if err != nil {
		t.Fatalf("SearchContactsSemantic: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("no results returned")
	}
	r := results[0].Contact
	if r.Company != "TopicCo" {
		t.Errorf("expected Company=TopicCo, got %q", r.Company)
	}
	if len(r.Topics) != 2 || r.Topics[0] != "alpha" || r.Topics[1] != "beta" {
		t.Errorf("unexpected topics: %v", r.Topics)
	}
}

// TestCosineSimilarity_SanityCheck ensures the helper returns expected values.
func TestCosineSimilarity_SanityCheck(t *testing.T) {
	// Identical vectors → 1.0
	a := []float32{1, 2, 3}
	if got := cosineSimilarity(a, a); math.Abs(got-1.0) > 1e-6 {
		t.Errorf("identical vectors: expected ~1.0, got %f", got)
	}
	// Orthogonal vectors → 0.0
	x := []float32{1, 0}
	y := []float32{0, 1}
	if got := cosineSimilarity(x, y); math.Abs(got) > 1e-6 {
		t.Errorf("orthogonal vectors: expected ~0.0, got %f", got)
	}
	// Opposite vectors → -1.0
	neg := []float32{-1, -2, -3}
	if got := cosineSimilarity(a, neg); math.Abs(got+1.0) > 1e-6 {
		t.Errorf("opposite vectors: expected ~-1.0, got %f", got)
	}
}

