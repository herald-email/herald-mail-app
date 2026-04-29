package backend

import (
	"fmt"
	"sync"

	"github.com/herald-email/herald-mail-app/internal/logger"
)

type folderLoadRequest struct {
	Folder     string
	Generation int64
}

// latestWinsLoadCoordinator keeps only the newest pending load request.
// One worker drains requests serially; newer submissions replace older queued
// ones instead of preserving stale work.
type latestWinsLoadCoordinator struct {
	mu      sync.Mutex
	nextGen int64
	pending *folderLoadRequest
	wakeCh  chan struct{}
}

func newLatestWinsLoadCoordinator() *latestWinsLoadCoordinator {
	return &latestWinsLoadCoordinator{
		wakeCh: make(chan struct{}, 1),
	}
}

func (c *latestWinsLoadCoordinator) Submit(folder string) folderLoadRequest {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.nextGen++
	req := folderLoadRequest{
		Folder:     folder,
		Generation: c.nextGen,
	}
	replaced := "none"
	if c.pending != nil {
		replaced = fmt.Sprintf("%s#%d", c.pending.Folder, c.pending.Generation)
	}
	c.pending = &req
	logger.Debug("LoadCoordinator.Submit: folder=%s generation=%d replaces=%s", folder, req.Generation, replaced)
	select {
	case c.wakeCh <- struct{}{}:
	default:
	}
	return req
}

func (c *latestWinsLoadCoordinator) DrainPending() (folderLoadRequest, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.pending == nil {
		return folderLoadRequest{}, false
	}
	req := *c.pending
	c.pending = nil
	logger.Debug("LoadCoordinator.DrainPending: folder=%s generation=%d", req.Folder, req.Generation)
	return req, true
}

func (c *latestWinsLoadCoordinator) Wake() <-chan struct{} {
	return c.wakeCh
}
