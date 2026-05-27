package work

import "sync"

type LatestIntentQueueItem[T any] struct {
	IntentKey   IntentKey
	ResourceKey ResourceKey
	Generation  int64
	Value       T
}

type LatestIntentQueue[T any] struct {
	mu          sync.Mutex
	nextGen     map[IntentKey]int64
	pending     map[IntentKey]LatestIntentQueueItem[T]
	pendingKeys []IntentKey
	wakeCh      chan struct{}
}

func NewLatestIntentQueue[T any]() *LatestIntentQueue[T] {
	return &LatestIntentQueue[T]{
		nextGen: make(map[IntentKey]int64),
		pending: make(map[IntentKey]LatestIntentQueueItem[T]),
		wakeCh:  make(chan struct{}, 1),
	}
}

func (q *LatestIntentQueue[T]) Submit(intent IntentKey, resource ResourceKey, value T) LatestIntentQueueItem[T] {
	q.mu.Lock()
	defer q.mu.Unlock()

	q.nextGen[intent]++
	item := LatestIntentQueueItem[T]{
		IntentKey:   intent,
		ResourceKey: resource,
		Generation:  q.nextGen[intent],
		Value:       value,
	}
	if _, exists := q.pending[intent]; !exists {
		q.pendingKeys = append(q.pendingKeys, intent)
	}
	q.pending[intent] = item
	select {
	case q.wakeCh <- struct{}{}:
	default:
	}
	return item
}

func (q *LatestIntentQueue[T]) Pending(intent IntentKey) (LatestIntentQueueItem[T], bool) {
	q.mu.Lock()
	defer q.mu.Unlock()

	item, ok := q.pending[intent]
	return item, ok
}

func (q *LatestIntentQueue[T]) DrainPending() (LatestIntentQueueItem[T], bool) {
	q.mu.Lock()
	defer q.mu.Unlock()

	var zero LatestIntentQueueItem[T]
	if len(q.pendingKeys) == 0 {
		return zero, false
	}
	intent := q.pendingKeys[0]
	q.pendingKeys = q.pendingKeys[1:]
	item := q.pending[intent]
	delete(q.pending, intent)
	return item, true
}

func (q *LatestIntentQueue[T]) Wake() <-chan struct{} {
	return q.wakeCh
}
