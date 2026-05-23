package work

import (
	"context"
	"sync"
)

type Coordinator struct {
	mu        sync.Mutex
	closed    bool
	intents   map[IntentKey]intentState
	inflight  map[ResourceKey]*call
	completed map[ResourceKey]resultState
	calls     map[*call]struct{}
	serial    map[SourceID]*serialLane
}

type intentState struct {
	generation int64
	resource   ResourceKey
}

type call struct {
	ready  chan struct{}
	state  resultState
	status Status
	done   bool
}

type serialLane struct {
	queue   []serialTask
	running bool
}

type serialTask struct {
	ctx    context.Context
	spec   Spec
	active *call
}

func NewCoordinator() *Coordinator {
	return &Coordinator{
		intents:   make(map[IntentKey]intentState),
		inflight:  make(map[ResourceKey]*call),
		completed: make(map[ResourceKey]resultState),
		calls:     make(map[*call]struct{}),
		serial:    make(map[SourceID]*serialLane),
	}
}

func (c *Coordinator) Close() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.closed = true
	for key := range c.inflight {
		delete(c.inflight, key)
	}
	for active := range c.calls {
		c.finishCallLocked(active, resultState{err: ErrClosed})
	}
}

func (c *Coordinator) Submit(ctx context.Context, spec Spec) Result {
	if spec.Run == nil {
		return completedResult(resultState{err: ErrClosed})
	}

	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return completedResult(resultState{err: ErrClosed})
	}

	generation := int64(0)
	if spec.Policy&PolicyTakeLatestByIntent != 0 {
		state := c.intents[spec.IntentKey]
		state.generation++
		state.resource = spec.ResourceKey
		c.intents[spec.IntentKey] = state
		generation = state.generation
	}
	filter := c.intentFilter(spec, generation)

	if spec.Policy&PolicyReplayCompletedResource != 0 {
		if completed, ok := c.completed[spec.ResourceKey]; ok {
			c.mu.Unlock()
			result := completedResult(completed)
			result.filter = filter
			return result
		}
	}

	if spec.Policy&PolicyCoalesceByResource != 0 {
		if active, ok := c.inflight[spec.ResourceKey]; ok {
			c.mu.Unlock()
			return Result{ready: active.ready, state: &active.state, filter: filter}
		}
	}

	active := &call{
		ready:  make(chan struct{}),
		status: StatusQueued,
	}
	c.calls[active] = struct{}{}
	if spec.Policy&PolicyCoalesceByResource != 0 {
		c.inflight[spec.ResourceKey] = active
	}

	if spec.Policy&PolicySerialBySource != 0 {
		sourceID := c.sourceID(spec)
		lane := c.serial[sourceID]
		if lane == nil {
			lane = &serialLane{}
			c.serial[sourceID] = lane
		}
		lane.queue = append(lane.queue, serialTask{ctx: ctx, spec: spec, active: active})
		if !lane.running {
			lane.running = true
			go c.runSerial(sourceID)
		}
		c.mu.Unlock()
		return Result{ready: active.ready, state: &active.state, filter: filter}
	}

	c.mu.Unlock()
	go c.runOne(ctx, spec, active)
	return Result{ready: active.ready, state: &active.state, filter: filter}
}

func (c *Coordinator) runSerial(sourceID SourceID) {
	for {
		c.mu.Lock()
		lane := c.serial[sourceID]
		if lane == nil || len(lane.queue) == 0 {
			if lane != nil {
				lane.running = false
			}
			c.mu.Unlock()
			return
		}
		task := lane.queue[0]
		lane.queue = lane.queue[1:]
		done := task.active.done
		c.mu.Unlock()

		if done {
			continue
		}
		c.runOne(task.ctx, task.spec, task.active)
	}
}

func (c *Coordinator) runOne(ctx context.Context, spec Spec, active *call) {
	c.mu.Lock()
	if active.done {
		c.mu.Unlock()
		return
	}
	active.status = StatusRunning
	c.mu.Unlock()

	value, err := spec.Run(ctx)
	state := resultState{value: value, err: err}

	c.mu.Lock()
	if spec.Policy&PolicyCoalesceByResource != 0 {
		delete(c.inflight, spec.ResourceKey)
	}
	if err == nil && spec.Policy&PolicyReplayCompletedResource != 0 {
		c.completed[spec.ResourceKey] = state
	}
	c.finishCallLocked(active, state)
	c.mu.Unlock()
}

func (c *Coordinator) finishCallLocked(active *call, state resultState) {
	if active.done {
		return
	}
	active.state = state
	active.status = StatusCompleted
	active.done = true
	delete(c.calls, active)
	close(active.ready)
}

func (c *Coordinator) sourceID(spec Spec) SourceID {
	if spec.SourceID != "" {
		return spec.SourceID
	}
	if spec.ResourceKey.SourceID != "" {
		return SourceID(spec.ResourceKey.SourceID)
	}
	return "__default__"
}

func (c *Coordinator) intentFilter(spec Spec, generation int64) func(resultState) resultState {
	if spec.Policy&PolicyTakeLatestByIntent == 0 {
		return nil
	}
	return func(state resultState) resultState {
		c.mu.Lock()
		latest := c.intents[spec.IntentKey]
		current := latest.generation == generation || latest.resource == spec.ResourceKey
		c.mu.Unlock()
		if !current {
			return resultState{err: ErrStaleIntent}
		}
		return state
	}
}
