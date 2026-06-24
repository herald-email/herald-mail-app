package backend

import (
	"context"
	"fmt"
	"time"

	"github.com/herald-email/herald-mail-app/internal/cache"
	"github.com/herald-email/herald-mail-app/internal/config"
	"github.com/herald-email/herald-mail-app/internal/imap"
	"github.com/herald-email/herald-mail-app/internal/models"
)

// IMAPMailSource adapts the existing IMAP client to the MailSource boundary.
// Queueing, cache policy, stale-result filtering, and UI priority stay owned by
// Herald services and callers above this adapter.
type IMAPMailSource struct {
	client    *imap.Client
	sourceID  models.SourceID
	accountID models.AccountID
}

func NewIMAPMailSource(cfg *config.Config, configPath string, cache *cache.Cache, progressCh chan models.ProgressInfo) *IMAPMailSource {
	return NewScopedIMAPMailSource(cfg, configPath, cache, progressCh, models.DefaultMailSourceID, models.DefaultAccountID)
}

func NewScopedIMAPMailSource(cfg *config.Config, configPath string, cache *cache.Cache, progressCh chan models.ProgressInfo, sourceID models.SourceID, accountID models.AccountID) *IMAPMailSource {
	sourceID = models.NormalizeSourceID(sourceID, models.DefaultMailSourceID)
	accountID = models.NormalizeAccountID(accountID)
	client := imap.New(cfg, configPath, cache, progressCh)
	client.SetSourceScope(sourceID, accountID)
	return &IMAPMailSource{
		client:    client,
		sourceID:  sourceID,
		accountID: accountID,
	}
}

func (s *IMAPMailSource) ensureClient() (*imap.Client, error) {
	if s == nil || s.client == nil {
		return nil, fmt.Errorf("IMAP client unavailable")
	}
	return s.client, nil
}

func (s *IMAPMailSource) Connect(ctx context.Context) error {
	client, err := s.ensureClient()
	if err != nil {
		return err
	}
	if err := mailSourceContextErr(ctx); err != nil {
		return err
	}
	if err := client.Connect(); err != nil {
		return err
	}
	return mailSourceContextErr(ctx)
}

func (s *IMAPMailSource) Close() error {
	client, err := s.ensureClient()
	if err != nil {
		return nil
	}
	return client.Close()
}

func (s *IMAPMailSource) ListFolders(ctx context.Context) ([]string, error) {
	client, err := s.ensureClient()
	if err != nil {
		return nil, err
	}
	if err := mailSourceContextErr(ctx); err != nil {
		return nil, err
	}
	folders, err := client.ListFolders()
	if err != nil {
		return nil, err
	}
	return folders, mailSourceContextErr(ctx)
}

func (s *IMAPMailSource) GetFolderStatus(ctx context.Context, folders []string) (map[string]models.FolderStatus, error) {
	client, err := s.ensureClient()
	if err != nil {
		return nil, err
	}
	if err := mailSourceContextErr(ctx); err != nil {
		return nil, err
	}
	status, err := client.GetFolderStatus(folders)
	if err != nil {
		return nil, err
	}
	return status, mailSourceContextErr(ctx)
}

func (s *IMAPMailSource) ProcessEmailsIncremental(ctx context.Context, folder string) error {
	client, err := s.ensureClient()
	if err != nil {
		return err
	}
	if err := mailSourceContextErr(ctx); err != nil {
		return err
	}
	if err := client.ProcessEmailsIncremental(folder); err != nil {
		return err
	}
	return mailSourceContextErr(ctx)
}

func (s *IMAPMailSource) GetSenderStatistics(ctx context.Context, folder string) (map[string]*models.SenderStats, error) {
	client, err := s.ensureClient()
	if err != nil {
		return nil, err
	}
	if err := mailSourceContextErr(ctx); err != nil {
		return nil, err
	}
	stats, err := client.GetSenderStatistics(folder)
	if err != nil {
		return nil, err
	}
	return stats, mailSourceContextErr(ctx)
}

func (s *IMAPMailSource) GetEmailsBySender(ctx context.Context, folder string) (map[string][]*models.EmailData, error) {
	client, err := s.ensureClient()
	if err != nil {
		return nil, err
	}
	if err := mailSourceContextErr(ctx); err != nil {
		return nil, err
	}
	grouped, err := client.GetEmailsBySender(folder)
	if err != nil {
		return nil, err
	}
	return grouped, mailSourceContextErr(ctx)
}

func (s *IMAPMailSource) StartBackgroundReconcile(ctx context.Context, folder string, validIDsCh chan<- map[string]bool) {
	client, err := s.ensureClient()
	if err != nil {
		close(validIDsCh)
		return
	}
	if err := mailSourceContextErr(ctx); err != nil {
		close(validIDsCh)
		return
	}
	client.StartBackgroundReconcile(folder, validIDsCh)
}

func (s *IMAPMailSource) GetFolderMessageIDs(ctx context.Context, folders []string) (map[string]map[string]bool, error) {
	client, err := s.ensureClient()
	if err != nil {
		return nil, err
	}
	if err := mailSourceContextErr(ctx); err != nil {
		return nil, err
	}
	membership, err := client.GetFolderMessageIDs(folders)
	if err != nil {
		return nil, err
	}
	return membership, mailSourceContextErr(ctx)
}

func (s *IMAPMailSource) DeleteSenderEmails(ctx context.Context, sender, folder string) error {
	client, err := s.ensureClient()
	if err != nil {
		return err
	}
	if err := mailSourceContextErr(ctx); err != nil {
		return err
	}
	if err := client.DeleteSenderEmails(sender, folder); err != nil {
		return err
	}
	return mailSourceContextErr(ctx)
}

func (s *IMAPMailSource) DeleteDomainEmails(ctx context.Context, domain, folder string) error {
	client, err := s.ensureClient()
	if err != nil {
		return err
	}
	if err := mailSourceContextErr(ctx); err != nil {
		return err
	}
	if err := client.DeleteDomainEmails(domain, folder); err != nil {
		return err
	}
	return mailSourceContextErr(ctx)
}

func (s *IMAPMailSource) DeleteEmail(ctx context.Context, messageID, folder string) error {
	client, err := s.ensureClient()
	if err != nil {
		return err
	}
	if err := mailSourceContextErr(ctx); err != nil {
		return err
	}
	if err := client.DeleteEmail(messageID, folder); err != nil {
		return err
	}
	return mailSourceContextErr(ctx)
}

func (s *IMAPMailSource) DeleteEmailsByRef(ctx context.Context, refs []models.MessageRef) error {
	client, err := s.ensureClient()
	if err != nil {
		return err
	}
	if err := mailSourceContextErr(ctx); err != nil {
		return err
	}
	if err := client.DeleteEmailsByRef(refs); err != nil {
		return err
	}
	return mailSourceContextErr(ctx)
}

func (s *IMAPMailSource) FetchMessageNoCache(ctx context.Context, ref models.MessageRef) (*models.EmailBody, error) {
	client, err := s.ensureClient()
	if err != nil {
		return nil, err
	}
	if err := mailSourceContextErr(ctx); err != nil {
		return nil, err
	}
	ref = ref.WithDefaults()
	body, err := client.FetchEmailBody(ref.UID, ref.Folder)
	if err != nil {
		return nil, err
	}
	if body != nil && body.MessageID == "" {
		body.MessageID = ref.MessageID
	}
	return body, mailSourceContextErr(ctx)
}

func (s *IMAPMailSource) FetchMessagePreviewNoCache(ctx context.Context, ref models.MessageRef) (*models.EmailBody, error) {
	client, err := s.ensureClient()
	if err != nil {
		return nil, err
	}
	if err := mailSourceContextErr(ctx); err != nil {
		return nil, err
	}
	ref = ref.WithDefaults()
	body, err := client.FetchEmailPreviewBody(ref.UID, ref.Folder)
	if err != nil {
		return nil, err
	}
	if body != nil && body.MessageID == "" {
		body.MessageID = ref.MessageID
	}
	return body, mailSourceContextErr(ctx)
}

func (s *IMAPMailSource) SetGroupByDomain(groupByDomain bool) {
	if client, err := s.ensureClient(); err == nil {
		client.SetGroupByDomain(groupByDomain)
	}
}

func (s *IMAPMailSource) ArchiveEmail(ctx context.Context, messageID, folder string) error {
	client, err := s.ensureClient()
	if err != nil {
		return err
	}
	if err := mailSourceContextErr(ctx); err != nil {
		return err
	}
	if err := client.ArchiveEmail(messageID, folder); err != nil {
		return err
	}
	return mailSourceContextErr(ctx)
}

func (s *IMAPMailSource) ArchiveEmailsByRef(ctx context.Context, refs []models.MessageRef) error {
	client, err := s.ensureClient()
	if err != nil {
		return err
	}
	if err := mailSourceContextErr(ctx); err != nil {
		return err
	}
	if err := client.ArchiveEmailsByRef(refs); err != nil {
		return err
	}
	return mailSourceContextErr(ctx)
}

func (s *IMAPMailSource) ArchiveSenderEmails(ctx context.Context, sender, folder string) error {
	client, err := s.ensureClient()
	if err != nil {
		return err
	}
	if err := mailSourceContextErr(ctx); err != nil {
		return err
	}
	if err := client.ArchiveSenderEmails(sender, folder); err != nil {
		return err
	}
	return mailSourceContextErr(ctx)
}

func (s *IMAPMailSource) SearchIMAP(ctx context.Context, folder, query string) ([]*models.EmailData, error) {
	client, err := s.ensureClient()
	if err != nil {
		return nil, err
	}
	if err := mailSourceContextErr(ctx); err != nil {
		return nil, err
	}
	results, err := client.SearchIMAP(folder, query)
	if err != nil {
		return nil, err
	}
	return results, mailSourceContextErr(ctx)
}

func (s *IMAPMailSource) MarkRead(ctx context.Context, uid uint32, folder string) error {
	client, err := s.ensureClient()
	if err != nil {
		return err
	}
	if err := mailSourceContextErr(ctx); err != nil {
		return err
	}
	if err := client.MarkRead(uid, folder); err != nil {
		return err
	}
	return mailSourceContextErr(ctx)
}

func (s *IMAPMailSource) MarkUnread(ctx context.Context, uid uint32, folder string) error {
	client, err := s.ensureClient()
	if err != nil {
		return err
	}
	if err := mailSourceContextErr(ctx); err != nil {
		return err
	}
	if err := client.MarkUnread(uid, folder); err != nil {
		return err
	}
	return mailSourceContextErr(ctx)
}

func (s *IMAPMailSource) MarkStarred(ctx context.Context, uid uint32, folder string) error {
	client, err := s.ensureClient()
	if err != nil {
		return err
	}
	if err := mailSourceContextErr(ctx); err != nil {
		return err
	}
	if err := client.MarkStarred(uid, folder); err != nil {
		return err
	}
	return mailSourceContextErr(ctx)
}

func (s *IMAPMailSource) UnmarkStarred(ctx context.Context, uid uint32, folder string) error {
	client, err := s.ensureClient()
	if err != nil {
		return err
	}
	if err := mailSourceContextErr(ctx); err != nil {
		return err
	}
	if err := client.UnmarkStarred(uid, folder); err != nil {
		return err
	}
	return mailSourceContextErr(ctx)
}

func (s *IMAPMailSource) StartIDLE(ctx context.Context, folder string, newEmailsCh chan<- models.NewEmailsNotification) error {
	client, err := s.ensureClient()
	if err != nil {
		return err
	}
	if err := mailSourceContextErr(ctx); err != nil {
		return err
	}
	if err := client.StartIDLE(folder, newEmailsCh); err != nil {
		return err
	}
	return mailSourceContextErr(ctx)
}

func (s *IMAPMailSource) StopIDLE() {
	if client, err := s.ensureClient(); err == nil {
		client.StopIDLE()
	}
}

func (s *IMAPMailSource) PollForNewEmails(ctx context.Context, folder string, sinceDate time.Time) ([]*models.EmailData, error) {
	client, err := s.ensureClient()
	if err != nil {
		return nil, err
	}
	if err := mailSourceContextErr(ctx); err != nil {
		return nil, err
	}
	emails, err := client.PollForNewEmails(folder, sinceDate)
	if err != nil {
		return nil, err
	}
	return emails, mailSourceContextErr(ctx)
}

func (s *IMAPMailSource) MoveEmail(ctx context.Context, messageID, fromFolder, toFolder string) error {
	client, err := s.ensureClient()
	if err != nil {
		return err
	}
	if err := mailSourceContextErr(ctx); err != nil {
		return err
	}
	if err := client.MoveEmail(messageID, fromFolder, toFolder); err != nil {
		return err
	}
	return mailSourceContextErr(ctx)
}

func (s *IMAPMailSource) AppendDraft(ctx context.Context, raw []byte) (uint32, string, error) {
	client, err := s.ensureClient()
	if err != nil {
		return 0, "", err
	}
	if err := mailSourceContextErr(ctx); err != nil {
		return 0, "", err
	}
	uid, folder, err := client.AppendDraft(raw)
	if err != nil {
		return 0, "", err
	}
	return uid, folder, mailSourceContextErr(ctx)
}

func (s *IMAPMailSource) ListDrafts(ctx context.Context) ([]*models.Draft, error) {
	client, err := s.ensureClient()
	if err != nil {
		return nil, err
	}
	if err := mailSourceContextErr(ctx); err != nil {
		return nil, err
	}
	drafts, err := client.ListDrafts()
	if err != nil {
		return nil, err
	}
	return drafts, mailSourceContextErr(ctx)
}

func (s *IMAPMailSource) DeleteDraft(ctx context.Context, uid uint32, folder string) error {
	client, err := s.ensureClient()
	if err != nil {
		return err
	}
	if err := mailSourceContextErr(ctx); err != nil {
		return err
	}
	if err := client.DeleteDraft(uid, folder); err != nil {
		return err
	}
	return mailSourceContextErr(ctx)
}

func (s *IMAPMailSource) CreateMailbox(ctx context.Context, name string) error {
	client, err := s.ensureClient()
	if err != nil {
		return err
	}
	if err := mailSourceContextErr(ctx); err != nil {
		return err
	}
	if err := client.CreateMailbox(name); err != nil {
		return err
	}
	return mailSourceContextErr(ctx)
}

func (s *IMAPMailSource) RenameMailbox(ctx context.Context, existingName, newName string) error {
	client, err := s.ensureClient()
	if err != nil {
		return err
	}
	if err := mailSourceContextErr(ctx); err != nil {
		return err
	}
	if err := client.RenameMailbox(existingName, newName); err != nil {
		return err
	}
	return mailSourceContextErr(ctx)
}

func (s *IMAPMailSource) DeleteMailbox(ctx context.Context, name string) error {
	client, err := s.ensureClient()
	if err != nil {
		return err
	}
	if err := mailSourceContextErr(ctx); err != nil {
		return err
	}
	if err := client.DeleteMailbox(name); err != nil {
		return err
	}
	return mailSourceContextErr(ctx)
}
