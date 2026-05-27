package backend

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/herald-email/herald-mail-app/internal/ai"
	"github.com/herald-email/herald-mail-app/internal/config"
	"github.com/herald-email/herald-mail-app/internal/models"
)

const AllAccountsSourceID models.SourceID = "all-accounts"

// AccountInfo is the minimal account identity surfaced to TUI account chrome.
type AccountInfo struct {
	SourceID    models.SourceID
	AccountID   models.AccountID
	DisplayName string
	Provider    string
	Address     string
}

// AccountStatus is cached account state suitable for compact UI display.
type AccountStatus struct {
	SourceID models.SourceID
	State    string
	Error    string
	Unread   int
	Total    int
}

// AccountBackend pairs account identity with a legacy Backend implementation.
type AccountBackend struct {
	Info    AccountInfo
	Backend Backend
}

// AccountAwareBackend is an additive interface. Existing single-account
// backends do not need to implement it.
type AccountAwareBackend interface {
	Backend
	Accounts() []AccountInfo
	ActiveAccount() AccountInfo
	HasMultipleAccounts() bool
	SwitchAccount(models.SourceID) error
	AccountStatuses() map[models.SourceID]AccountStatus
}

type accountSlot struct {
	info    AccountInfo
	backend Backend
}

// MultiBackend keeps the public Backend API legacy-compatible while allowing
// the TUI to switch which account receives those legacy folder/message-ID calls.
type MultiBackend struct {
	Backend

	mu        sync.RWMutex
	order     []models.SourceID
	slots     map[models.SourceID]*accountSlot
	active    models.SourceID
	progress  chan models.ProgressInfo
	syncs     chan models.FolderSyncEvent
	newEmails chan models.NewEmailsNotification
	validIDs  chan models.ValidIDsNotification
	closed    bool
	groupBy   bool
}

var _ AccountAwareBackend = (*MultiBackend)(nil)

func allAccountsInfo() AccountInfo {
	return AccountInfo{
		SourceID:    AllAccountsSourceID,
		AccountID:   models.AccountID("all"),
		DisplayName: "All Accounts",
		Provider:    "virtual",
	}
}

func NewMultiBackend(accounts []AccountBackend) (*MultiBackend, error) {
	if len(accounts) == 0 {
		return nil, fmt.Errorf("multi backend requires at least one account")
	}
	m := &MultiBackend{
		slots:     make(map[models.SourceID]*accountSlot, len(accounts)),
		progress:  make(chan models.ProgressInfo, 100),
		syncs:     make(chan models.FolderSyncEvent, 256),
		newEmails: make(chan models.NewEmailsNotification, 20),
		validIDs:  make(chan models.ValidIDsNotification, 20),
	}
	for _, account := range accounts {
		if account.Backend == nil {
			return nil, fmt.Errorf("account %q has no backend", account.Info.DisplayName)
		}
		info := normalizeAccountInfo(account.Info)
		if _, exists := m.slots[info.SourceID]; exists {
			return nil, fmt.Errorf("duplicate source id %q", info.SourceID)
		}
		slot := &accountSlot{info: info, backend: account.Backend}
		m.order = append(m.order, info.SourceID)
		m.slots[info.SourceID] = slot
		m.startFanIn(slot)
	}
	m.active = m.order[0]
	m.Backend = m.slots[m.active].backend
	return m, nil
}

func normalizeAccountInfo(info AccountInfo) AccountInfo {
	info.SourceID = models.NormalizeSourceID(info.SourceID, models.DefaultMailSourceID)
	info.AccountID = models.NormalizeAccountID(info.AccountID)
	info.DisplayName = strings.TrimSpace(info.DisplayName)
	if info.DisplayName == "" {
		info.DisplayName = string(info.SourceID)
	}
	info.Provider = strings.TrimSpace(info.Provider)
	if info.Provider == "" {
		info.Provider = "imap"
	}
	info.Address = strings.TrimSpace(info.Address)
	return info
}

func (m *MultiBackend) startFanIn(slot *accountSlot) {
	if ch := slot.backend.Progress(); ch != nil {
		go func() {
			for p := range ch {
				p.SourceID = slot.info.SourceID
				p.AccountID = slot.info.AccountID
				if m.isActive(slot.info.SourceID) {
					m.sendProgress(p)
				}
			}
		}()
	}
	if ch := slot.backend.SyncEvents(); ch != nil {
		go func() {
			for event := range ch {
				event.SourceID = slot.info.SourceID
				event.AccountID = slot.info.AccountID
				if event.CollectionID == "" {
					event.CollectionID = event.Folder
				}
				if m.isActive(slot.info.SourceID) {
					m.sendSyncEvent(event)
				}
			}
		}()
	}
	if ch := slot.backend.NewEmailsCh(); ch != nil {
		go func() {
			for notification := range ch {
				notification.SourceID = slot.info.SourceID
				notification.AccountID = slot.info.AccountID
				if notification.CollectionID == "" {
					notification.CollectionID = notification.Folder
				}
				if m.isActive(slot.info.SourceID) {
					m.sendNewEmails(notification)
				}
			}
		}()
	}
	if provider, ok := slot.backend.(interface {
		ScopedValidIDsCh() <-chan models.ValidIDsNotification
	}); ok {
		if ch := provider.ScopedValidIDsCh(); ch != nil {
			go func() {
				for notification := range ch {
					notification.SourceID = slot.info.SourceID
					notification.AccountID = slot.info.AccountID
					if m.isActive(slot.info.SourceID) {
						m.sendValidIDs(notification)
					}
				}
			}()
		}
	}
}

func (m *MultiBackend) isActive(sourceID models.SourceID) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return !m.closed && (m.active == sourceID || m.active == AllAccountsSourceID)
}

func (m *MultiBackend) sendProgress(p models.ProgressInfo) {
	defer func() { _ = recover() }()
	select {
	case m.progress <- p:
	default:
	}
}

func (m *MultiBackend) sendSyncEvent(event models.FolderSyncEvent) {
	defer func() { _ = recover() }()
	select {
	case m.syncs <- event:
	default:
	}
}

func (m *MultiBackend) sendNewEmails(notification models.NewEmailsNotification) {
	defer func() { _ = recover() }()
	select {
	case m.newEmails <- notification:
	default:
	}
}

func (m *MultiBackend) sendValidIDs(notification models.ValidIDsNotification) {
	defer func() { _ = recover() }()
	select {
	case m.validIDs <- notification:
	default:
	}
}

func (m *MultiBackend) Accounts() []AccountInfo {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]AccountInfo, 0, len(m.order))
	for _, id := range m.order {
		out = append(out, m.slots[id].info)
	}
	return out
}

func (m *MultiBackend) ActiveAccount() AccountInfo {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.active == AllAccountsSourceID {
		return allAccountsInfo()
	}
	if slot := m.slots[m.active]; slot != nil {
		return slot.info
	}
	return AccountInfo{}
}

func (m *MultiBackend) HasMultipleAccounts() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.order) > 1
}

func (m *MultiBackend) SwitchAccount(sourceID models.SourceID) error {
	sourceID = models.NormalizeSourceID(sourceID, "")
	m.mu.Lock()
	defer m.mu.Unlock()
	if sourceID == AllAccountsSourceID {
		m.active = sourceID
		return nil
	}
	slot := m.slots[sourceID]
	if slot == nil {
		return fmt.Errorf("unknown source id %q", sourceID)
	}
	m.active = sourceID
	m.Backend = slot.backend
	if m.groupBy {
		slot.backend.SetGroupByDomain(true)
	}
	return nil
}

func (m *MultiBackend) AccountStatuses() map[models.SourceID]AccountStatus {
	m.mu.RLock()
	slots := make([]*accountSlot, 0, len(m.order))
	for _, id := range m.order {
		slots = append(slots, m.slots[id])
	}
	m.mu.RUnlock()

	statuses := make(map[models.SourceID]AccountStatus, len(slots))
	for _, slot := range slots {
		status := AccountStatus{SourceID: slot.info.SourceID, State: "live"}
		folders, err := slot.backend.ListFolders()
		if err != nil {
			status.State = "error"
			status.Error = err.Error()
			statuses[slot.info.SourceID] = status
			continue
		}
		folderStatus, err := slot.backend.GetFolderStatus(folders)
		if err != nil {
			status.State = "error"
			status.Error = err.Error()
			statuses[slot.info.SourceID] = status
			continue
		}
		for _, st := range folderStatus {
			status.Unread += st.Unseen
			status.Total += st.Total
		}
		statuses[slot.info.SourceID] = status
	}
	return statuses
}

func (m *MultiBackend) Progress() <-chan models.ProgressInfo { return m.progress }

func (m *MultiBackend) SyncEvents() <-chan models.FolderSyncEvent { return m.syncs }

func (m *MultiBackend) NewEmailsCh() <-chan models.NewEmailsNotification { return m.newEmails }

func (m *MultiBackend) ValidIDsCh() <-chan map[string]bool {
	m.mu.RLock()
	active := m.Backend
	m.mu.RUnlock()
	if active == nil {
		return nil
	}
	return active.ValidIDsCh()
}

func (m *MultiBackend) ScopedValidIDsCh() <-chan models.ValidIDsNotification {
	return m.validIDs
}

func (m *MultiBackend) allAccountsActive() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.active == AllAccountsSourceID
}

func (m *MultiBackend) activeBackend() Backend {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.active == AllAccountsSourceID {
		return m.Backend
	}
	if slot := m.slots[m.active]; slot != nil {
		return slot.backend
	}
	return m.Backend
}

func (m *MultiBackend) activeRealSlot() *accountSlot {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if slot := m.slots[m.active]; slot != nil {
		return slot
	}
	for _, id := range m.order {
		if slot := m.slots[id]; slot != nil {
			return slot
		}
	}
	return nil
}

func (m *MultiBackend) slotForCompose(sourceID models.SourceID, from string) (*accountSlot, error) {
	if strings.TrimSpace(string(sourceID)) != "" {
		sourceID = models.NormalizeSourceID(sourceID, "")
	}
	from = strings.ToLower(strings.TrimSpace(from))
	m.mu.RLock()
	defer m.mu.RUnlock()
	if sourceID != "" && sourceID != AllAccountsSourceID {
		if slot := m.slots[sourceID]; slot != nil {
			return slot, nil
		}
		return nil, fmt.Errorf("unknown source id %q", sourceID)
	}
	if from != "" {
		for _, id := range m.order {
			slot := m.slots[id]
			if slot != nil && strings.EqualFold(strings.TrimSpace(slot.info.Address), from) {
				return slot, nil
			}
		}
	}
	if slot := m.slots[m.active]; slot != nil {
		return slot, nil
	}
	for _, id := range m.order {
		if slot := m.slots[id]; slot != nil {
			return slot, nil
		}
	}
	return nil, fmt.Errorf("no mail source available")
}

func (m *MultiBackend) slotForRef(ref models.MessageRef) (*accountSlot, models.MessageRef, error) {
	rawSource := ref.SourceID
	hadLocalID := strings.TrimSpace(ref.LocalID) != ""
	hadCollectionID := strings.TrimSpace(ref.Folder) != ""
	ref = ref.WithDefaults()
	if !hadLocalID && (!hadCollectionID || strings.TrimSpace(ref.MessageID) == "") {
		ref.LocalID = ""
	}
	m.mu.RLock()
	slot := m.slots[ref.SourceID]
	if slot == nil && rawSource == "" {
		if m.active != AllAccountsSourceID {
			slot = m.slots[m.active]
		}
		if slot == nil && len(m.order) > 0 {
			slot = m.slots[m.order[0]]
		}
		if slot != nil {
			ref.SourceID = slot.info.SourceID
			ref.AccountID = slot.info.AccountID
			ref.LocalID = ""
			ref = ref.WithDefaults()
		}
	}
	m.mu.RUnlock()
	if slot == nil {
		return nil, ref, fmt.Errorf("unknown source id %q", ref.SourceID)
	}
	return slot, ref, nil
}

func emailForAccountSlot(slot *accountSlot, email *models.EmailData) *models.EmailData {
	if email == nil {
		return nil
	}
	clone := *email
	clone.SourceID = slot.info.SourceID
	clone.AccountID = slot.info.AccountID
	ref := clone.MessageRef()
	clone.LocalID = ref.LocalID
	return &clone
}

func sortEmailsNewestFirst(emails []*models.EmailData) {
	sort.SliceStable(emails, func(i, j int) bool {
		if emails[i] == nil || emails[j] == nil {
			return emails[j] == nil
		}
		return emails[i].Date.After(emails[j].Date)
	})
}

func (m *MultiBackend) aggregateEmails(fn func(*accountSlot) ([]*models.EmailData, error)) ([]*models.EmailData, error) {
	slots := m.snapshotSlots()
	var out []*models.EmailData
	for _, slot := range slots {
		emails, err := fn(slot)
		if err != nil {
			return out, err
		}
		for _, email := range emails {
			out = append(out, emailForAccountSlot(slot, email))
		}
	}
	sortEmailsNewestFirst(out)
	return out, nil
}

func (m *MultiBackend) Load(folder string) {
	if !m.allAccountsActive() {
		if active := m.activeBackend(); active != nil {
			active.Load(folder)
		}
		return
	}
	for _, slot := range m.snapshotSlots() {
		slot.backend.Load(folder)
	}
}

func (m *MultiBackend) ListFolders() ([]string, error) {
	if !m.allAccountsActive() {
		if active := m.activeBackend(); active != nil {
			return active.ListFolders()
		}
		return nil, nil
	}
	seen := make(map[string]bool)
	var folders []string
	for _, slot := range m.snapshotSlots() {
		accountFolders, err := slot.backend.ListFolders()
		if err != nil {
			return nil, err
		}
		for _, folder := range accountFolders {
			if !seen[folder] {
				seen[folder] = true
				folders = append(folders, folder)
			}
		}
	}
	sort.Strings(folders)
	return folders, nil
}

func (m *MultiBackend) GetFolderStatus(folders []string) (map[string]models.FolderStatus, error) {
	if !m.allAccountsActive() {
		if active := m.activeBackend(); active != nil {
			return active.GetFolderStatus(folders)
		}
		return nil, nil
	}
	total := make(map[string]models.FolderStatus, len(folders))
	for _, slot := range m.snapshotSlots() {
		statuses, err := slot.backend.GetFolderStatus(folders)
		if err != nil {
			return total, err
		}
		for folder, status := range statuses {
			merged := total[folder]
			merged.Total += status.Total
			merged.Unseen += status.Unseen
			total[folder] = merged
		}
	}
	return total, nil
}

func (m *MultiBackend) GetTimelineEmails(folder string) ([]*models.EmailData, error) {
	if !m.allAccountsActive() {
		if active := m.activeBackend(); active != nil {
			return active.GetTimelineEmails(folder)
		}
		return nil, nil
	}
	return m.aggregateEmails(func(slot *accountSlot) ([]*models.EmailData, error) {
		return slot.backend.GetTimelineEmails(folder)
	})
}

func (m *MultiBackend) SearchEmails(folder, query string, bodySearch bool) ([]*models.EmailData, error) {
	if !m.allAccountsActive() {
		if active := m.activeBackend(); active != nil {
			return active.SearchEmails(folder, query, bodySearch)
		}
		return nil, nil
	}
	return m.aggregateEmails(func(slot *accountSlot) ([]*models.EmailData, error) {
		return slot.backend.SearchEmails(folder, query, bodySearch)
	})
}

func (m *MultiBackend) SearchEmailsCrossFolder(query string) ([]*models.EmailData, error) {
	if !m.allAccountsActive() {
		if active := m.activeBackend(); active != nil {
			return active.SearchEmailsCrossFolder(query)
		}
		return nil, nil
	}
	return m.aggregateEmails(func(slot *accountSlot) ([]*models.EmailData, error) {
		return slot.backend.SearchEmailsCrossFolder(query)
	})
}

func (m *MultiBackend) SearchEmailsIMAP(folder, query string) ([]*models.EmailData, error) {
	if !m.allAccountsActive() {
		if active := m.activeBackend(); active != nil {
			return active.SearchEmailsIMAP(folder, query)
		}
		return nil, nil
	}
	return m.aggregateEmails(func(slot *accountSlot) ([]*models.EmailData, error) {
		return slot.backend.SearchEmailsIMAP(folder, query)
	})
}

func (m *MultiBackend) SearchEmailsSemantic(folder, query string, limit int, minScore float64) ([]*models.EmailData, error) {
	if !m.allAccountsActive() {
		if active := m.activeBackend(); active != nil {
			return active.SearchEmailsSemantic(folder, query, limit, minScore)
		}
		return nil, nil
	}
	emails, err := m.aggregateEmails(func(slot *accountSlot) ([]*models.EmailData, error) {
		return slot.backend.SearchEmailsSemantic(folder, query, limit, minScore)
	})
	if err != nil || limit <= 0 || len(emails) <= limit {
		return emails, err
	}
	return emails[:limit], nil
}

func (m *MultiBackend) SearchSemanticChunked(folder string, queryVec []float32, limit int, minScore float64) ([]*models.SemanticSearchResult, error) {
	if !m.allAccountsActive() {
		if active := m.activeBackend(); active != nil {
			return active.SearchSemanticChunked(folder, queryVec, limit, minScore)
		}
		return nil, nil
	}
	var out []*models.SemanticSearchResult
	for _, slot := range m.snapshotSlots() {
		results, err := slot.backend.SearchSemanticChunked(folder, queryVec, limit, minScore)
		if err != nil {
			return out, err
		}
		for _, result := range results {
			if result == nil {
				continue
			}
			clone := *result
			clone.Email = emailForAccountSlot(slot, result.Email)
			out = append(out, &clone)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i] == nil || out[j] == nil {
			return out[j] == nil
		}
		return out[i].Score > out[j].Score
	})
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (m *MultiBackend) GetEmailByRef(ref models.MessageRef) (*models.EmailData, error) {
	slot, ref, err := m.slotForRef(ref)
	if err != nil {
		return nil, err
	}
	if getter, ok := slot.backend.(interface {
		GetEmailByRef(models.MessageRef) (*models.EmailData, error)
	}); ok {
		email, err := getter.GetEmailByRef(ref)
		if err != nil || email == nil {
			return email, err
		}
		return emailForAccountSlot(slot, email), nil
	}
	email, err := slot.backend.GetEmailByID(ref.MessageID)
	if err != nil || email == nil {
		return email, err
	}
	return emailForAccountSlot(slot, email), nil
}

func (m *MultiBackend) GetMessage(ctx context.Context, ref models.MessageRef) (MessageReadResult, error) {
	slot, ref, err := m.slotForRef(ref)
	if err != nil {
		return MessageReadResult{}, err
	}
	getter, ok := slot.backend.(interface {
		GetMessage(context.Context, models.MessageRef) (MessageReadResult, error)
	})
	if !ok {
		return MessageReadResult{}, fmt.Errorf("source %q does not support cache-first message reads", slot.info.SourceID)
	}
	return getter.GetMessage(ctx, ref)
}

func (m *MultiBackend) GetMessagePreview(ctx context.Context, ref models.MessageRef, intent MessageReadIntent) (MessageReadResult, error) {
	slot, ref, err := m.slotForRef(ref)
	if err != nil {
		return MessageReadResult{}, err
	}
	getter, ok := slot.backend.(interface {
		GetMessagePreview(context.Context, models.MessageRef, MessageReadIntent) (MessageReadResult, error)
	})
	if !ok {
		return MessageReadResult{}, fmt.Errorf("source %q does not support cache-first preview reads", slot.info.SourceID)
	}
	return getter.GetMessagePreview(ctx, ref, intent)
}

func resolveMessageRefForSlot(slot *accountSlot, ref models.MessageRef) models.MessageRef {
	if slot == nil || strings.TrimSpace(ref.Folder) != "" {
		return ref
	}
	if getter, ok := slot.backend.(interface {
		GetEmailByRef(models.MessageRef) (*models.EmailData, error)
	}); ok {
		if email, err := getter.GetEmailByRef(ref); err == nil && email != nil {
			return emailForAccountSlot(slot, email).MessageRef()
		}
	}
	if strings.TrimSpace(ref.MessageID) != "" {
		if email, err := slot.backend.GetEmailByID(ref.MessageID); err == nil && email != nil {
			return emailForAccountSlot(slot, email).MessageRef()
		}
	}
	return ref
}

func (m *MultiBackend) ArchiveEmailByRef(ref models.MessageRef) error {
	slot, ref, err := m.slotForRef(ref)
	if err != nil {
		return err
	}
	ref = resolveMessageRefForSlot(slot, ref)
	return slot.backend.ArchiveEmail(ref.MessageID, ref.Folder)
}

func (m *MultiBackend) MoveEmailByRef(ref models.MessageRef, to string) error {
	slot, ref, err := m.slotForRef(ref)
	if err != nil {
		return err
	}
	ref = resolveMessageRefForSlot(slot, ref)
	return slot.backend.MoveEmail(ref.MessageID, ref.Folder, to)
}

func (m *MultiBackend) DeleteEmailByRef(ref models.MessageRef) error {
	slot, ref, err := m.slotForRef(ref)
	if err != nil {
		return err
	}
	ref = resolveMessageRefForSlot(slot, ref)
	return slot.backend.DeleteEmail(ref.MessageID, ref.Folder)
}

func (m *MultiBackend) MarkReadByRef(ref models.MessageRef) error {
	slot, ref, err := m.slotForRef(ref)
	if err != nil {
		return err
	}
	ref = resolveMessageRefForSlot(slot, ref)
	return slot.backend.MarkRead(ref.MessageID, ref.Folder)
}

func (m *MultiBackend) MarkUnreadByRef(ref models.MessageRef) error {
	slot, ref, err := m.slotForRef(ref)
	if err != nil {
		return err
	}
	ref = resolveMessageRefForSlot(slot, ref)
	return slot.backend.MarkUnread(ref.MessageID, ref.Folder)
}

func (m *MultiBackend) MarkStarredByRef(ref models.MessageRef) error {
	slot, ref, err := m.slotForRef(ref)
	if err != nil {
		return err
	}
	ref = resolveMessageRefForSlot(slot, ref)
	return slot.backend.MarkStarred(ref.MessageID, ref.Folder)
}

func (m *MultiBackend) UnmarkStarredByRef(ref models.MessageRef) error {
	slot, ref, err := m.slotForRef(ref)
	if err != nil {
		return err
	}
	ref = resolveMessageRefForSlot(slot, ref)
	return slot.backend.UnmarkStarred(ref.MessageID, ref.Folder)
}

func (m *MultiBackend) ReplyToEmailByRef(ref models.MessageRef, opts models.ReplyEmailOptions) error {
	slot, ref, err := m.slotForRef(ref)
	if err != nil {
		return err
	}
	return slot.backend.ReplyToEmailWithOptions(ref.MessageID, opts)
}

func (m *MultiBackend) ForwardEmailByRef(ref models.MessageRef, opts models.ForwardEmailOptions) error {
	slot, ref, err := m.slotForRef(ref)
	if err != nil {
		return err
	}
	return slot.backend.ForwardEmailWithOptions(ref.MessageID, opts)
}

func (m *MultiBackend) UnsubscribeSenderByRef(ref models.MessageRef) error {
	slot, ref, err := m.slotForRef(ref)
	if err != nil {
		return err
	}
	return slot.backend.UnsubscribeSender(ref.MessageID)
}

func (m *MultiBackend) DeleteThreadForSource(sourceID models.SourceID, folder, subject string) error {
	slot, err := m.slotForCompose(sourceID, "")
	if err != nil {
		return err
	}
	return slot.backend.DeleteThread(folder, subject)
}

func (m *MultiBackend) ArchiveThreadForSource(sourceID models.SourceID, folder, subject string) error {
	slot, err := m.slotForCompose(sourceID, "")
	if err != nil {
		return err
	}
	return slot.backend.ArchiveThread(folder, subject)
}

func (m *MultiBackend) DeleteSenderEmailsForSource(sourceID models.SourceID, sender, folder string) error {
	slot, err := m.slotForCompose(sourceID, "")
	if err != nil {
		return err
	}
	return slot.backend.DeleteSenderEmails(sender, folder)
}

func (m *MultiBackend) ArchiveSenderEmailsForSource(sourceID models.SourceID, sender, folder string) error {
	slot, err := m.slotForCompose(sourceID, "")
	if err != nil {
		return err
	}
	return slot.backend.ArchiveSenderEmails(sender, folder)
}

func (m *MultiBackend) SoftUnsubscribeSenderForSource(sourceID models.SourceID, sender, toFolder string) error {
	slot, err := m.slotForCompose(sourceID, "")
	if err != nil {
		return err
	}
	return slot.backend.SoftUnsubscribeSender(sender, toFolder)
}

func (m *MultiBackend) SendEmail(to, subject, body, from string) error {
	slot, err := m.slotForCompose("", from)
	if err != nil {
		return err
	}
	if strings.TrimSpace(from) == "" {
		from = slot.info.Address
	}
	return slot.backend.SendEmail(to, subject, body, from)
}

func (m *MultiBackend) SendCompose(req ComposeSendRequest) error {
	slot, err := m.slotForCompose(req.SourceID, req.From)
	if err != nil {
		return err
	}
	req.SourceID = slot.info.SourceID
	if strings.TrimSpace(req.From) == "" {
		req.From = slot.info.Address
	}
	if sender, ok := slot.backend.(interface {
		SendCompose(ComposeSendRequest) error
	}); ok {
		return sender.SendCompose(req)
	}
	return slot.backend.SendEmail(req.To, req.Subject, req.MarkdownBody, req.From)
}

func (m *MultiBackend) SaveDraft(to, cc, bcc, subject, body string) (uint32, string, error) {
	slot, err := m.slotForCompose("", "")
	if err != nil {
		return 0, "", err
	}
	return slot.backend.SaveDraft(to, cc, bcc, subject, body)
}

func (m *MultiBackend) SaveRawDraft(raw []byte) (uint32, string, error) {
	slot, err := m.slotForCompose("", "")
	if err != nil {
		return 0, "", err
	}
	return slot.backend.SaveRawDraft(raw)
}

func (m *MultiBackend) DeleteDraft(uid uint32, folder string) error {
	slot, err := m.slotForCompose("", "")
	if err != nil {
		return err
	}
	return slot.backend.DeleteDraft(uid, folder)
}

func (m *MultiBackend) SendDraft(uid uint32, folder string) error {
	slot, err := m.slotForCompose("", "")
	if err != nil {
		return err
	}
	return slot.backend.SendDraft(uid, folder)
}

func (m *MultiBackend) SaveDraftForAccount(sourceID models.SourceID, to, cc, bcc, subject, body string) (uint32, string, error) {
	slot, err := m.slotForCompose(sourceID, "")
	if err != nil {
		return 0, "", err
	}
	if saver, ok := slot.backend.(interface {
		SaveDraftForAccount(models.SourceID, string, string, string, string, string) (uint32, string, error)
	}); ok {
		return saver.SaveDraftForAccount(slot.info.SourceID, to, cc, bcc, subject, body)
	}
	return slot.backend.SaveDraft(to, cc, bcc, subject, body)
}

func (m *MultiBackend) SaveRawDraftForAccount(sourceID models.SourceID, raw []byte) (uint32, string, error) {
	slot, err := m.slotForCompose(sourceID, "")
	if err != nil {
		return 0, "", err
	}
	if saver, ok := slot.backend.(interface {
		SaveRawDraftForAccount(models.SourceID, []byte) (uint32, string, error)
	}); ok {
		return saver.SaveRawDraftForAccount(slot.info.SourceID, raw)
	}
	return slot.backend.SaveRawDraft(raw)
}

func (m *MultiBackend) DeleteDraftForAccount(sourceID models.SourceID, uid uint32, folder string) error {
	slot, err := m.slotForCompose(sourceID, "")
	if err != nil {
		return err
	}
	if deleter, ok := slot.backend.(interface {
		DeleteDraftForAccount(models.SourceID, uint32, string) error
	}); ok {
		return deleter.DeleteDraftForAccount(slot.info.SourceID, uid, folder)
	}
	return slot.backend.DeleteDraft(uid, folder)
}

func (m *MultiBackend) SendDraftForAccount(sourceID models.SourceID, uid uint32, folder string) error {
	slot, err := m.slotForCompose(sourceID, "")
	if err != nil {
		return err
	}
	if sender, ok := slot.backend.(interface {
		SendDraftForAccount(models.SourceID, uint32, string) error
	}); ok {
		return sender.SendDraftForAccount(slot.info.SourceID, uid, folder)
	}
	return slot.backend.SendDraft(uid, folder)
}

func (m *MultiBackend) SetGroupByDomain(groupByDomain bool) {
	m.mu.Lock()
	m.groupBy = groupByDomain
	slots := make([]*accountSlot, 0, len(m.order))
	for _, id := range m.order {
		slots = append(slots, m.slots[id])
	}
	m.mu.Unlock()
	for _, slot := range slots {
		slot.backend.SetGroupByDomain(groupByDomain)
	}
}

func (m *MultiBackend) Close() error {
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return nil
	}
	m.closed = true
	slots := make([]*accountSlot, 0, len(m.order))
	for _, id := range m.order {
		slots = append(slots, m.slots[id])
	}
	m.mu.Unlock()

	var firstErr error
	for _, slot := range slots {
		if err := slot.backend.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	close(m.progress)
	close(m.syncs)
	close(m.newEmails)
	close(m.validIDs)
	return firstErr
}

func (m *MultiBackend) SetAIClient(classifier ai.AIClient) {
	for _, slot := range m.snapshotSlots() {
		if setter, ok := slot.backend.(interface{ SetAIClient(ai.AIClient) }); ok {
			setter.SetAIClient(classifier)
		}
	}
}

func (m *MultiBackend) EnsureEmbeddingModel(model string) (bool, error) {
	invalidated := false
	for _, slot := range m.snapshotSlots() {
		ensurer, ok := slot.backend.(interface {
			EnsureEmbeddingModel(string) (bool, error)
		})
		if !ok {
			continue
		}
		changed, err := ensurer.EnsureEmbeddingModel(model)
		if err != nil {
			return invalidated, err
		}
		invalidated = invalidated || changed
	}
	return invalidated, nil
}

func (m *MultiBackend) ApplyCacheStoragePolicy(policy string) (models.PreviewCachePruneResult, error) {
	var total models.PreviewCachePruneResult
	for _, slot := range m.snapshotSlots() {
		applier, ok := slot.backend.(interface {
			ApplyCacheStoragePolicy(string) (models.PreviewCachePruneResult, error)
		})
		if !ok {
			continue
		}
		result, err := applier.ApplyCacheStoragePolicy(policy)
		if err != nil {
			return total, err
		}
		total.RowsScanned += result.RowsScanned
		total.RowsChanged += result.RowsChanged
		total.AttachmentBytesRemoved += result.AttachmentBytesRemoved
		total.InlineImageBytesRemoved += result.InlineImageBytesRemoved
	}
	return total, nil
}

func (m *MultiBackend) EstimateOfflineCacheStorageReclaim(policy string) (models.PreviewCacheStorageEstimate, error) {
	total := models.PreviewCacheStorageEstimate{Policy: policy}
	for _, slot := range m.snapshotSlots() {
		estimator, ok := slot.backend.(interface {
			EstimateOfflineCacheStorageReclaim(string) (models.PreviewCacheStorageEstimate, error)
		})
		if !ok {
			continue
		}
		estimate, err := estimator.EstimateOfflineCacheStorageReclaim(policy)
		if err != nil {
			return total, err
		}
		total.RowsScanned += estimate.RowsScanned
		total.RowsReclaimable += estimate.RowsReclaimable
		total.CurrentBytes += estimate.CurrentBytes
		total.ReclaimableBytes += estimate.ReclaimableBytes
		total.EstimatedAfterBytes += estimate.EstimatedAfterBytes
		total.AttachmentBytes += estimate.AttachmentBytes
		total.InlineImageBytes += estimate.InlineImageBytes
		total.ReclaimableAttachmentBytes += estimate.ReclaimableAttachmentBytes
		total.ReclaimableInlineImageBytes += estimate.ReclaimableInlineImageBytes
	}
	return total, nil
}

func (m *MultiBackend) ReclaimOfflineCacheStorage(policy string) (models.PreviewCacheReclaimResult, error) {
	var total models.PreviewCacheReclaimResult
	total.Estimate.Policy = policy
	total.Compacted = true
	for _, slot := range m.snapshotSlots() {
		reclaimer, ok := slot.backend.(interface {
			ReclaimOfflineCacheStorage(string) (models.PreviewCacheReclaimResult, error)
		})
		if !ok {
			continue
		}
		result, err := reclaimer.ReclaimOfflineCacheStorage(policy)
		if err != nil {
			return total, err
		}
		total.Estimate.RowsScanned += result.Estimate.RowsScanned
		total.Estimate.RowsReclaimable += result.Estimate.RowsReclaimable
		total.Estimate.CurrentBytes += result.Estimate.CurrentBytes
		total.Estimate.ReclaimableBytes += result.Estimate.ReclaimableBytes
		total.Estimate.EstimatedAfterBytes += result.Estimate.EstimatedAfterBytes
		total.Estimate.AttachmentBytes += result.Estimate.AttachmentBytes
		total.Estimate.InlineImageBytes += result.Estimate.InlineImageBytes
		total.Estimate.ReclaimableAttachmentBytes += result.Estimate.ReclaimableAttachmentBytes
		total.Estimate.ReclaimableInlineImageBytes += result.Estimate.ReclaimableInlineImageBytes
		total.PruneResult.RowsScanned += result.PruneResult.RowsScanned
		total.PruneResult.RowsChanged += result.PruneResult.RowsChanged
		total.PruneResult.AttachmentBytesRemoved += result.PruneResult.AttachmentBytesRemoved
		total.PruneResult.InlineImageBytesRemoved += result.PruneResult.InlineImageBytesRemoved
		total.Compacted = total.Compacted && result.Compacted
		if result.CompactionError != "" && total.CompactionError == "" {
			total.CompactionError = result.CompactionError
		}
	}
	return total, nil
}

func (m *MultiBackend) snapshotSlots() []*accountSlot {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]*accountSlot, 0, len(m.order))
	for _, id := range m.order {
		out = append(out, m.slots[id])
	}
	return out
}

func NewMultiLocal(cfg *config.Config, configPath string, classifier ai.AIClient) (*MultiBackend, error) {
	if cfg == nil {
		return nil, fmt.Errorf("config is nil")
	}
	var accounts []AccountBackend
	for _, source := range cfg.NormalizedSources() {
		if strings.TrimSpace(source.Kind) != "" && source.Kind != string(models.SourceKindMail) {
			continue
		}
		childCfg := configForMailSource(cfg, configPath, source)
		if path := strings.TrimSpace(childCfg.Cache.DatabasePath); path != "" {
			if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
				for _, account := range accounts {
					_ = account.Backend.Close()
				}
				return nil, fmt.Errorf("prepare cache directory for %s: %w", source.ID, err)
			}
		}
		childBackend, err := NewLocal(childCfg, "", classifier)
		if err != nil {
			for _, account := range accounts {
				_ = account.Backend.Close()
			}
			return nil, fmt.Errorf("open %s: %w", source.ID, err)
		}
		accounts = append(accounts, AccountBackend{
			Info:    accountInfoFromSource(source),
			Backend: childBackend,
		})
	}
	if len(accounts) == 0 {
		return nil, fmt.Errorf("no mail sources configured")
	}
	return NewMultiBackend(accounts)
}

func configForMailSource(profile *config.Config, configPath string, source config.SourceConfig) *config.Config {
	child := *profile
	child.Sources = nil
	child.Credentials = source.Credentials
	child.Server = source.IMAP
	child.SMTP = source.SMTP
	child.Gmail.AccessToken = source.Google.AccessToken
	child.Gmail.RefreshToken = source.Google.RefreshToken
	child.Gmail.TokenExpiry = source.Google.TokenExpiry
	child.Gmail.Email = source.Google.Email
	if source.Provider != "" {
		child.Vendor = source.Provider
	}
	child.Cache.DatabasePath = derivedSourceCachePath(profile.Cache.DatabasePath, configPath, source.ID)
	return &child
}

func derivedSourceCachePath(basePath, configPath, sourceID string) string {
	sourceID = sanitizeAccountCachePart(sourceID)
	if strings.TrimSpace(basePath) != "" {
		expanded, err := config.ExpandPath(basePath)
		if err != nil {
			expanded = basePath
		}
		ext := filepath.Ext(expanded)
		stem := strings.TrimSuffix(expanded, ext)
		if ext == "" {
			ext = ".db"
		}
		return stem + "-" + sourceID + ext
	}
	stem := strings.TrimSuffix(filepath.Base(configPath), filepath.Ext(configPath))
	if strings.TrimSpace(stem) == "" || stem == "." {
		stem = "config"
	}
	stem = sanitizeAccountCachePart(stem)
	return filepath.Join(userCacheDirFallback(), stem+"-"+sourceID+".db")
}

func userCacheDirFallback() string {
	home, err := filepath.Abs(".")
	if err != nil {
		return "."
	}
	if userHome, homeErr := config.ExpandPath("~/.herald/cached"); homeErr == nil {
		return userHome
	}
	return home
}

func sanitizeAccountCachePart(value string) string {
	var b strings.Builder
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
		case r >= 'A' && r <= 'Z':
			b.WriteRune(r)
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '-' || r == '_' || r == '.':
			b.WriteRune(r)
		default:
			b.WriteByte('-')
		}
	}
	clean := strings.Trim(b.String(), "-_.")
	if clean == "" {
		return "source"
	}
	return clean
}

func accountInfoFromSource(source config.SourceConfig) AccountInfo {
	address := strings.TrimSpace(source.Credentials.Username)
	if address == "" {
		address = strings.TrimSpace(source.Google.Email)
	}
	return normalizeAccountInfo(AccountInfo{
		SourceID:    models.SourceID(source.ID),
		AccountID:   models.AccountID(source.AccountID),
		DisplayName: source.DisplayName,
		Provider:    source.Provider,
		Address:     address,
	})
}
