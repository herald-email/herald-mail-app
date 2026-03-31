package models

import "time"

// CleanupRule defines an automated inbox cleanup rule that matches emails by
// sender or domain and applies a delete or archive action to older messages.
type CleanupRule struct {
	ID            int64
	Name          string
	MatchType     string     // "sender" | "domain"
	MatchValue    string
	Action        string     // "delete" | "archive"
	OlderThanDays int
	Enabled       bool
	LastRun       *time.Time
	CreatedAt     time.Time
}
