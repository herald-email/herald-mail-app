package models

import (
	"time"
)

// EmailData represents a cached email message
type EmailData struct {
	MessageID      string    `db:"message_id"`
	UID            uint32    `db:"uid"`
	Sender         string    `db:"sender"`
	Subject        string    `db:"subject"`
	Date           time.Time `db:"date"`
	Size           int       `db:"size"`
	HasAttachments bool      `db:"has_attachments"`
	Folder         string    `db:"folder"`
	LastUpdated    time.Time `db:"last_updated"`
	IsRead         bool      `db:"is_read"`
	IsStarred      bool      `db:"is_starred"`
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

type DeletionResult struct {
	MessageID      string `json:"message_id"`
	Sender         string `json:"sender"`
	Folder         string `json:"folder"`
	DeletedCount   int    `json:"deleted_count"`
	Error          error
	ConnectionLost bool // true when the IMAP connection dropped during deletion
}

// FolderStatus holds message counts for a mailbox
type FolderStatus struct {
	Total  int
	Unseen int
}

// EmailBody holds the fetched body content of a single email message.
type EmailBody struct {
	TextPlain           string
	TextHTML            string
	InlineImages        []InlineImage
	Attachments         []Attachment
	IsFromHTML          bool   // TextPlain was converted from HTML; render via markdown
	ListUnsubscribe     string // raw List-Unsubscribe header value
	ListUnsubscribePost string // raw List-Unsubscribe-Post header value (RFC 8058)
}

// InlineImage is an image MIME part embedded inline in an email body.
type InlineImage struct {
	ContentID string
	MIMEType  string
	Data      []byte
}

// Attachment represents a downloadable email attachment (not inline).
type Attachment struct {
	Filename string
	MIMEType string
	Size     int    // bytes
	PartPath string // MIME part path for targeted fetch (e.g. "2", "1.2")
	Data     []byte // populated during FetchEmailBody (full message already in memory)
}

// ComposeAttachment is a local file staged for sending.
type ComposeAttachment struct {
	Path     string
	Filename string
	Size     int64
	Data     []byte // loaded at add time; nil = load error
}

// Deletion Request
type DeletionRequest struct {
	MessageID string `json:"message_id"`
	Sender    string `json:"sender"`
	IsDomain  bool   `json:"is_domain"` // True if Sender is a domain, not a full email
	Folder    string `json:"folder"`
	IsArchive bool   `json:"is_archive"` // True = archive instead of delete
	Response  chan DeletionResult
}

// NewEmailsNotification carries new emails found by background polling
type NewEmailsNotification struct {
	Emails []*EmailData
	Folder string
}

// SavedSearch represents a user-saved search query
type SavedSearch struct {
	ID        int
	Name      string
	Query     string
	Folder    string
	CreatedAt time.Time
}

// ContactAddr is a parsed email address from headers (From/To/CC/BCC)
type ContactAddr struct {
	Email string
	Name  string
}

// EmbeddingChunk is one chunk of a message's embedding
type EmbeddingChunk struct {
	MessageID   string // message this chunk belongs to
	ChunkIndex  int
	Embedding   []float32
	ContentHash string // SHA256 hex of the chunk text
}

// SemanticSearchResult pairs an email with its similarity score.
type SemanticSearchResult struct {
	Email *EmailData
	Score float64 // cosine similarity 0.0–1.0
}

// Draft represents a saved compose draft.
type Draft struct {
	UID     uint32
	Folder  string
	To      string
	CC      string
	BCC     string
	Subject string
	Body    string // Markdown body as stored
	Date    time.Time
}

// UnsubscribedSender records a sender the user has unsubscribed from.
type UnsubscribedSender struct {
	ID             int64
	Sender         string
	UnsubbedAt     time.Time
	Method         string
	UnsubscribeURL string
}

// ContactSearchResult pairs a contact with its similarity score from semantic search.
type ContactSearchResult struct {
	Contact ContactData
	Score   float64 // cosine similarity 0.0–1.0
}

// ContactData represents an enriched contact from the contacts table
type ContactData struct {
	ID          int64
	Email       string
	DisplayName string
	Company     string
	Topics      []string // stored as JSON in DB, deserialized here
	Notes       string
	FirstSeen   time.Time
	LastSeen    time.Time
	EmailCount  int
	SentCount   int
	CardDAVUID  string
	EnrichedAt  *time.Time // nil if never enriched
	Embedding   []float32  // nil if not yet embedded; stored as little-endian float32 BLOB (same encoding as email_embedding_chunks)
}

// VirtualFolderResult carries the result of a derived, non-server folder view.
type VirtualFolderResult struct {
	Name         string
	Supported    bool
	Reason       string
	SourceFolder string
	Emails       []*EmailData
}
