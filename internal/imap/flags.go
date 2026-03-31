package imap

import (
	"github.com/emersion/go-imap"
)

// MarkRead sets the \Seen flag on a message by UID in the given folder.
func (c *Client) MarkRead(uid uint32, folder string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.client == nil {
		return nil
	}
	if _, err := c.client.Select(folder, false); err != nil {
		return err
	}
	seqset := new(imap.SeqSet)
	seqset.AddNum(uid)
	return c.client.UidStore(seqset,
		imap.FormatFlagsOp(imap.AddFlags, true),
		[]interface{}{imap.SeenFlag},
		nil,
	)
}

// MarkUnread removes the \Seen flag from a message by UID in the given folder.
func (c *Client) MarkUnread(uid uint32, folder string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.client == nil {
		return nil
	}
	if _, err := c.client.Select(folder, false); err != nil {
		return err
	}
	seqset := new(imap.SeqSet)
	seqset.AddNum(uid)
	return c.client.UidStore(seqset,
		imap.FormatFlagsOp(imap.RemoveFlags, true),
		[]interface{}{imap.SeenFlag},
		nil,
	)
}
