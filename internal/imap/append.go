package imap

import (
	"bytes"
	"fmt"
	"time"

	"github.com/emersion/go-imap"
	"github.com/herald-email/herald-mail-app/internal/logger"
	"github.com/herald-email/herald-mail-app/internal/models"
)

// draftFolderCandidates is the ordered list of well-known Drafts folder names to try.
var draftFolderCandidates = []string{"Drafts", "[Gmail]/Drafts", "INBOX.Drafts", "INBOX/Drafts"}

// findDraftFolder tries known draft folder names and returns the first that exists.
// Caller must hold c.mu.
func (c *Client) findDraftFolder() (string, error) {
	for _, name := range draftFolderCandidates {
		ch := make(chan *imap.MailboxInfo, 1)
		done := make(chan error, 1)
		go func() {
			done <- c.client.List("", name, ch)
		}()
		var found bool
		for range ch {
			found = true
		}
		if err := <-done; err != nil {
			logger.Debug("findDraftFolder: List %q error: %v", name, err)
			continue
		}
		if found {
			logger.Debug("findDraftFolder: found %q", name)
			return name, nil
		}
	}
	return "", fmt.Errorf("no Drafts folder found (tried %v)", draftFolderCandidates)
}

// AppendDraft stores a raw RFC 2822 message in the IMAP Drafts folder.
// Tries "Drafts", "[Gmail]/Drafts", "INBOX.Drafts", "INBOX/Drafts" in order.
// Returns the UID of the appended message (0 if APPENDUID extension unavailable).
func (c *Client) AppendDraft(rawMsg []byte) (uint32, string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.client == nil {
		return 0, "", fmt.Errorf("not connected")
	}

	folder, err := c.findDraftFolder()
	if err != nil {
		return 0, "", err
	}

	flags := []string{imap.SeenFlag, "\\Draft"}
	if err := c.client.Append(folder, flags, time.Now(), bytes.NewReader(rawMsg)); err != nil {
		return 0, "", fmt.Errorf("APPEND to %s: %w", folder, err)
	}
	logger.Info("AppendDraft: appended draft to %s (%d bytes)", folder, len(rawMsg))

	// Attempt to find the UID of the message we just appended by searching for
	// the most recent \Draft message in the folder.
	uid := uint32(0)
	if _, err := c.client.Select(folder, false); err == nil {
		criteria := imap.NewSearchCriteria()
		criteria.WithFlags = []string{"\\Draft"}
		uids, searchErr := c.client.UidSearch(criteria)
		if searchErr == nil && len(uids) > 0 {
			uid = uids[len(uids)-1]
		}
	}

	return uid, folder, nil
}

// ListDrafts returns all draft messages from the Drafts folder as models.Draft slices.
func (c *Client) ListDrafts() ([]*models.Draft, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.client == nil {
		return nil, fmt.Errorf("not connected")
	}

	folder, err := c.findDraftFolder()
	if err != nil {
		// No drafts folder — return empty list, not an error
		logger.Debug("ListDrafts: %v", err)
		return nil, nil
	}

	mbox, err := c.client.Select(folder, true)
	if err != nil {
		return nil, fmt.Errorf("select %s: %w", folder, err)
	}
	if mbox.Messages == 0 {
		return nil, nil
	}

	seqset := new(imap.SeqSet)
	seqset.AddRange(1, mbox.Messages)

	messages := make(chan *imap.Message, 10)
	done := make(chan error, 1)
	go func() {
		done <- c.client.Fetch(seqset, []imap.FetchItem{imap.FetchEnvelope, imap.FetchUid}, messages)
	}()

	var drafts []*models.Draft
	for msg := range messages {
		if msg.Envelope == nil {
			continue
		}
		d := &models.Draft{
			UID:    msg.Uid,
			Folder: folder,
			Date:   msg.Envelope.Date,
		}
		if msg.Envelope.Subject != "" {
			d.Subject = msg.Envelope.Subject
		}
		if len(msg.Envelope.To) > 0 && msg.Envelope.To[0] != nil {
			addr := msg.Envelope.To[0]
			if addr.MailboxName != "" && addr.HostName != "" {
				d.To = addr.MailboxName + "@" + addr.HostName
			}
		}
		drafts = append(drafts, d)
	}

	if err := <-done; err != nil {
		return nil, fmt.Errorf("fetch drafts: %w", err)
	}

	return drafts, nil
}

// DeleteDraft UID-stores \Deleted on the given uid in the given folder, then expunges.
func (c *Client) DeleteDraft(uid uint32, folder string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.client == nil {
		return fmt.Errorf("not connected")
	}

	if _, err := c.client.Select(folder, false); err != nil {
		return fmt.Errorf("select %s: %w", folder, err)
	}

	seqset := new(imap.SeqSet)
	seqset.AddNum(uid)

	if err := c.client.UidStore(seqset,
		imap.FormatFlagsOp(imap.AddFlags, true),
		[]interface{}{imap.DeletedFlag},
		nil,
	); err != nil {
		return fmt.Errorf("uid store \\Deleted on uid %d: %w", uid, err)
	}

	if err := c.client.Expunge(nil); err != nil {
		return fmt.Errorf("expunge draft uid %d: %w", uid, err)
	}

	logger.Info("DeleteDraft: removed draft uid %d from %s", uid, folder)
	return nil
}
