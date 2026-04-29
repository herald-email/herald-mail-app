package cache

import (
	"database/sql"
	"testing"

	"github.com/herald-email/herald-mail-app/internal/models"
)

func TestSaveGetCustomCategory(t *testing.T) {
	c := newTestCache(t)

	// Seed a custom prompt so the prompt_id exists (not required by FK, but realistic)
	p := &models.CustomPrompt{
		Name:         "urgency",
		SystemText:   "Rate urgency 1-5",
		UserTemplate: "Subject: {{.Subject}}",
		OutputVar:    "urgency",
	}
	if err := c.SaveCustomPrompt(p); err != nil {
		t.Fatalf("SaveCustomPrompt: %v", err)
	}

	const msgID = "<test@example.com>"

	// Save a result
	if err := c.SaveCustomCategory(msgID, p.ID, "3"); err != nil {
		t.Fatalf("SaveCustomCategory: %v", err)
	}

	// Retrieve it
	got, err := c.GetCustomCategory(msgID, p.ID)
	if err != nil {
		t.Fatalf("GetCustomCategory: %v", err)
	}
	if got != "3" {
		t.Errorf("got %q, want %q", got, "3")
	}

	// Update the result (upsert)
	if err := c.SaveCustomCategory(msgID, p.ID, "5"); err != nil {
		t.Fatalf("SaveCustomCategory upsert: %v", err)
	}
	got, err = c.GetCustomCategory(msgID, p.ID)
	if err != nil {
		t.Fatalf("GetCustomCategory after upsert: %v", err)
	}
	if got != "5" {
		t.Errorf("after upsert got %q, want %q", got, "5")
	}

	// Missing entry returns sql.ErrNoRows
	_, err = c.GetCustomCategory("<missing@example.com>", p.ID)
	if err != sql.ErrNoRows {
		t.Errorf("expected sql.ErrNoRows for missing entry, got %v", err)
	}
}

func TestGetCustomCategoriesForEmail(t *testing.T) {
	c := newTestCache(t)

	p1 := &models.CustomPrompt{Name: "urgency", UserTemplate: "Subject: {{.Subject}}", OutputVar: "urgency"}
	p2 := &models.CustomPrompt{Name: "tone", UserTemplate: "Subject: {{.Subject}}", OutputVar: "tone"}
	if err := c.SaveCustomPrompt(p1); err != nil {
		t.Fatalf("SaveCustomPrompt p1: %v", err)
	}
	if err := c.SaveCustomPrompt(p2); err != nil {
		t.Fatalf("SaveCustomPrompt p2: %v", err)
	}

	const msgID = "<multi@example.com>"

	if err := c.SaveCustomCategory(msgID, p1.ID, "high"); err != nil {
		t.Fatalf("SaveCustomCategory p1: %v", err)
	}
	if err := c.SaveCustomCategory(msgID, p2.ID, "formal"); err != nil {
		t.Fatalf("SaveCustomCategory p2: %v", err)
	}

	cats, err := c.GetCustomCategoriesForEmail(msgID)
	if err != nil {
		t.Fatalf("GetCustomCategoriesForEmail: %v", err)
	}
	if len(cats) != 2 {
		t.Fatalf("expected 2 categories, got %d", len(cats))
	}
	if cats[p1.ID] != "high" {
		t.Errorf("p1 result: got %q, want %q", cats[p1.ID], "high")
	}
	if cats[p2.ID] != "formal" {
		t.Errorf("p2 result: got %q, want %q", cats[p2.ID], "formal")
	}

	// Email with no results returns empty map
	empty, err := c.GetCustomCategoriesForEmail("<none@example.com>")
	if err != nil {
		t.Fatalf("GetCustomCategoriesForEmail empty: %v", err)
	}
	if len(empty) != 0 {
		t.Errorf("expected empty map, got %v", empty)
	}
}
