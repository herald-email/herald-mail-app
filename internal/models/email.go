package models

import (
	"time"
)

// EmailData represents a cached email message
type EmailData struct {
	MessageID      string    `db:"message_id"`
	Sender         string    `db:"sender"`
	Subject        string    `db:"subject"`
	Date           time.Time `db:"date"`
	Size           int       `db:"size"`
	HasAttachments bool      `db:"has_attachments"`
	Folder         string    `db:"folder"`
	LastUpdated    time.Time `db:"last_updated"`
}

// SenderStats represents statistics for a sender
type SenderStats struct {
	TotalEmails     int       `json:"total_emails"`
	AvgSize         float64   `json:"avg_size"`
	WithAttachments int       `json:"with_attachments"`
	FirstEmail      time.Time `json:"first_email"`
	LastEmail       time.Time `json:"last_email"`
}

// ProgressInfo represents the current processing state
type ProgressInfo struct {
	Phase           string
	Current         int
	Total           int
	ProcessedEmails int
	NewEmails       int
	Message         string
}