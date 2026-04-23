package backend

import "sync"

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
	c.pending = &req
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
	return req, true
}

func (c *latestWinsLoadCoordinator) Wake() <-chan struct{} {
	return c.wakeCh
}
