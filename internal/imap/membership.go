package imap

import (
	"fmt"

	"github.com/emersion/go-imap"
)

// GetFolderMessageIDs fetches live Message-ID membership for each folder.
// It fails closed: any select/fetch error aborts the entire inspection.
func (c *Client) GetFolderMessageIDs(folders []string) (map[string]map[string]bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.client == nil {
		return nil, fmt.Errorf("not connected")
	}

	result := make(map[string]map[string]bool, len(folders))
	for _, folder := range folders {
		mbox, err := c.client.Select(folder, true)
		if err != nil {
			return nil, fmt.Errorf("select %s: %w", folder, err)
		}
		ids := make(map[string]bool)
		if mbox.Messages > 0 {
			seqset := new(imap.SeqSet)
			seqset.AddRange(1, mbox.Messages)
			messages := make(chan *imap.Message, 20)
			done := make(chan error, 1)
			go func() {
				done <- c.client.Fetch(seqset, []imap.FetchItem{imap.FetchEnvelope}, messages)
			}()
			for msg := range messages {
				if msg == nil || msg.Envelope == nil {
					continue
				}
				messageID := msg.Envelope.MessageId
				if messageID == "" {
					continue
				}
				ids[messageID] = true
			}
			if err := <-done; err != nil {
				return nil, fmt.Errorf("fetch %s membership: %w", folder, err)
			}
		}
		result[folder] = ids
	}
	return result, nil
}
