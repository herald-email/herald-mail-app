package notifications

import (
	"context"
	"sync"
)

type Recorder struct {
	mu        sync.Mutex
	delivered []Request
	responses chan string
}

func NewRecorder() *Recorder {
	return &Recorder{responses: make(chan string, 16)}
}

func (r *Recorder) Notify(_ context.Context, req Request) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.delivered = append(r.delivered, req)
	return nil
}

func (r *Recorder) Delivered() []Request {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]Request(nil), r.delivered...)
}

func (r *Recorder) Activate(deepLink string) {
	select {
	case r.responses <- deepLink:
	default:
	}
}

func (r *Recorder) Responses() <-chan string {
	return r.responses
}

func (r *Recorder) Capabilities() Capabilities {
	return Capabilities{Delivery: true, Activation: true, Platform: "recorder"}
}

func (r *Recorder) Close() error { return nil }
