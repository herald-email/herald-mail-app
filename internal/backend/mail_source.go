package backend

import (
	"context"
	"time"

	"github.com/herald-email/herald-mail-app/internal/models"
)

// MailSource is the provider boundary used by LocalBackend. It intentionally
// mirrors the current IMAP-backed capabilities so this phase can extract the
// transport without changing Backend's public API.
type MailSource interface {
	MessageSource

	Connect(context.Context) error
	Close() error

	ListFolders(context.Context) ([]string, error)
	GetFolderStatus(context.Context, []string) (map[string]models.FolderStatus, error)
	ProcessEmailsIncremental(context.Context, string) error
	GetSenderStatistics(context.Context, string) (map[string]*models.SenderStats, error)
	GetEmailsBySender(context.Context, string) (map[string][]*models.EmailData, error)
	StartBackgroundReconcile(context.Context, string, chan<- map[string]bool)
	GetFolderMessageIDs(context.Context, []string) (map[string]map[string]bool, error)

	DeleteSenderEmails(context.Context, string, string) error
	DeleteDomainEmails(context.Context, string, string) error
	DeleteEmail(context.Context, string, string) error
	ArchiveEmail(context.Context, string, string) error
	ArchiveSenderEmails(context.Context, string, string) error
	MoveEmail(context.Context, string, string, string) error

	SearchIMAP(context.Context, string, string) ([]*models.EmailData, error)
	SetGroupByDomain(bool)
	MarkRead(context.Context, uint32, string) error
	MarkUnread(context.Context, uint32, string) error
	MarkStarred(context.Context, uint32, string) error
	UnmarkStarred(context.Context, uint32, string) error

	StartIDLE(context.Context, string, chan<- models.NewEmailsNotification) error
	StopIDLE()
	PollForNewEmails(context.Context, string, time.Time) ([]*models.EmailData, error)

	AppendDraft(context.Context, []byte) (uint32, string, error)
	ListDrafts(context.Context) ([]*models.Draft, error)
	DeleteDraft(context.Context, uint32, string) error

	CreateMailbox(context.Context, string) error
	RenameMailbox(context.Context, string, string) error
	DeleteMailbox(context.Context, string) error
}

func mailSourceContextErr(ctx context.Context) error {
	if ctx == nil {
		return nil
	}
	return ctx.Err()
}
