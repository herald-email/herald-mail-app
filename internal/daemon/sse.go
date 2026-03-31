package daemon

import (
	"fmt"
	"net/http"
	"sync"
	"time"
)

// subscriber holds a channel that receives SSE lines for a single connected client.
type subscriber struct {
	ch chan string
}

// Broadcaster fans out SSE events to all connected HTTP clients.
// Clients connect via ServeHTTP; events are published via Send.
type Broadcaster struct {
	mu   sync.Mutex
	subs map[*subscriber]struct{}
}

// NewBroadcaster returns an initialised Broadcaster.
func NewBroadcaster() *Broadcaster {
	return &Broadcaster{subs: make(map[*subscriber]struct{})}
}

// Send delivers an SSE event line to every currently connected subscriber.
// Slow clients are skipped (non-blocking send).
func (b *Broadcaster) Send(event, data string) {
	line := fmt.Sprintf("event: %s\ndata: %s\n\n", event, data)
	b.mu.Lock()
	defer b.mu.Unlock()
	for sub := range b.subs {
		select {
		case sub.ch <- line:
		default:
		}
	}
}

// ServeHTTP implements http.Handler and streams SSE to a single client.
// It blocks until the client disconnects.
func (b *Broadcaster) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	sub := &subscriber{ch: make(chan string, 64)}
	b.mu.Lock()
	b.subs[sub] = struct{}{}
	b.mu.Unlock()

	defer func() {
		b.mu.Lock()
		delete(b.subs, sub)
		b.mu.Unlock()
	}()

	// Send a keepalive comment immediately so the client knows we're connected.
	_, _ = fmt.Fprint(w, ": connected\n\n")
	flusher.Flush()

	keepalive := time.NewTicker(25 * time.Second)
	defer keepalive.Stop()

	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			return
		case line := <-sub.ch:
			_, err := fmt.Fprint(w, line)
			if err != nil {
				return
			}
			flusher.Flush()
		case <-keepalive.C:
			fmt.Fprintf(w, ": keepalive\n\n")
			flusher.Flush()
		}
	}
}
