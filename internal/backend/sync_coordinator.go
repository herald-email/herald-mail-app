package backend

import (
	"fmt"

	"github.com/herald-email/herald-mail-app/internal/logger"
	"github.com/herald-email/herald-mail-app/internal/work"
)

type folderLoadRequest struct {
	Folder     string
	Generation int64
}

// latestWinsLoadCoordinator keeps only the newest pending load request.
// One worker drains requests serially; newer submissions replace older queued
// ones instead of preserving stale work.
type latestWinsLoadCoordinator struct {
	intentKey work.IntentKey
	queue     *work.LatestIntentQueue[string]
}

func newLatestWinsLoadCoordinator() *latestWinsLoadCoordinator {
	return &latestWinsLoadCoordinator{
		intentKey: work.IntentKey{ViewID: "active-collection-sync"},
		queue:     work.NewLatestIntentQueue[string](),
	}
}

func (c *latestWinsLoadCoordinator) Submit(folder string) folderLoadRequest {
	replaced := "none"
	if pending, ok := c.queue.Pending(c.intentKey); ok {
		replaced = fmt.Sprintf("%s#%d", pending.Value, pending.Generation)
	}
	item := c.queue.Submit(c.intentKey, work.ResourceKey{
		CollectionID: folder,
		Operation:    "active-collection-sync",
	}, folder)
	req := folderLoadRequest{
		Folder:     folder,
		Generation: item.Generation,
	}
	logger.Debug("LoadCoordinator.Submit: folder=%s generation=%d replaces=%s", folder, req.Generation, replaced)
	return req
}

func (c *latestWinsLoadCoordinator) DrainPending() (folderLoadRequest, bool) {
	item, ok := c.queue.DrainPending()
	if !ok {
		return folderLoadRequest{}, false
	}
	req := folderLoadRequest{
		Folder:     item.Value,
		Generation: item.Generation,
	}
	logger.Debug("LoadCoordinator.DrainPending: folder=%s generation=%d", req.Folder, req.Generation)
	return req, true
}

func (c *latestWinsLoadCoordinator) Wake() <-chan struct{} {
	return c.queue.Wake()
}
