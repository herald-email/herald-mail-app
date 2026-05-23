package work

import (
	"context"
	"errors"
	"sync"
)

var (
	ErrStaleIntent = errors.New("work result superseded by newer intent")
	ErrClosed      = errors.New("work coordinator closed")
)

type Policy uint32

const (
	PolicyTakeLatestByIntent Policy = 1 << iota
	PolicyCoalesceByResource
	PolicyReplayCompletedResource
	PolicySerialBySource
	PolicyFairBySource
	PolicyGlobalBudget
)

type Priority int

const (
	PriorityBackground Priority = iota
	PriorityUserAction
	PriorityInteractive
)

type SourceID string

type Status string

const (
	StatusQueued    Status = "queued"
	StatusRunning   Status = "running"
	StatusCompleted Status = "completed"
)

type IntentKey struct {
	ViewID string
}

type ResourceKey struct {
	SourceID     string
	AccountID    string
	CollectionID string
	ItemID       string
	Operation    string
	Freshness    string
}

type Spec struct {
	SourceID    SourceID
	IntentKey   IntentKey
	ResourceKey ResourceKey
	Policy      Policy
	Priority    Priority
	Run         func(context.Context) (any, error)
}

type Result struct {
	ready  <-chan struct{}
	state  *resultState
	filter func(resultState) resultState
}

type resultState struct {
	value any
	err   error
}

func (r Result) Await(ctx context.Context) (any, error) {
	select {
	case <-r.ready:
		state := *r.state
		if r.filter != nil {
			state = r.filter(state)
		}
		return state.value, state.err
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func completedResult(state resultState) Result {
	ready := make(chan struct{})
	close(ready)
	return Result{ready: ready, state: &state}
}

type FairQueue[T any] struct {
	mu     sync.Mutex
	order  []SourceID
	queues map[SourceID][]T
	next   int
}

func NewFairQueue[T any]() *FairQueue[T] {
	return &FairQueue[T]{
		queues: make(map[SourceID][]T),
	}
}

func (q *FairQueue[T]) Push(sourceID SourceID, item T) {
	q.mu.Lock()
	defer q.mu.Unlock()

	if sourceID == "" {
		sourceID = "__default__"
	}
	if len(q.queues[sourceID]) == 0 {
		q.order = append(q.order, sourceID)
	}
	q.queues[sourceID] = append(q.queues[sourceID], item)
}

func (q *FairQueue[T]) Pop() (T, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()

	var zero T
	if len(q.order) == 0 {
		return zero, false
	}
	if q.next >= len(q.order) {
		q.next = 0
	}

	sourceID := q.order[q.next]
	items := q.queues[sourceID]
	item := items[0]
	items = items[1:]
	if len(items) == 0 {
		delete(q.queues, sourceID)
		q.order = append(q.order[:q.next], q.order[q.next+1:]...)
		if q.next >= len(q.order) {
			q.next = 0
		}
	} else {
		q.queues[sourceID] = items
		q.next = (q.next + 1) % len(q.order)
	}

	return item, true
}

func (q *FairQueue[T]) Len() int {
	q.mu.Lock()
	defer q.mu.Unlock()

	total := 0
	for _, items := range q.queues {
		total += len(items)
	}
	return total
}
