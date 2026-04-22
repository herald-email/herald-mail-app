package ai

import (
	"container/heap"
	"context"
	"errors"
	"sync"
	"sync/atomic"
)

// ErrDeferred is returned when low-priority background AI work is skipped to
// preserve responsiveness under queue pressure.
var ErrDeferred = errors.New("ai work deferred")

type Priority int

const (
	PriorityBackground Priority = iota
	PriorityUserAction
	PriorityInteractive
)

const priorityUnset Priority = -1

type ManagedConfig struct {
	MaxConcurrency                  int
	QueueLimit                      int
	PauseBackgroundWhileInteractive bool
}

type prioritizedClient interface {
	withPriority(priority Priority) AIClient
}

type taskKindClient interface {
	withTaskKind(kind TaskKind) AIClient
}

type managedTask struct {
	priority Priority
	kind     TaskKind
	seq      int64
	run      func() error
	done     chan error
}

type managedQueue []*managedTask

func (q managedQueue) Len() int { return len(q) }

func (q managedQueue) Less(i, j int) bool {
	if q[i].priority == q[j].priority {
		return q[i].seq < q[j].seq
	}
	return q[i].priority > q[j].priority
}

func (q managedQueue) Swap(i, j int) { q[i], q[j] = q[j], q[i] }

func (q *managedQueue) Push(x any) {
	*q = append(*q, x.(*managedTask))
}

func (q *managedQueue) Pop() any {
	old := *q
	n := len(old)
	item := old[n-1]
	*q = old[:n-1]
	return item
}

type managedScheduler struct {
	cfg     ManagedConfig
	mu      sync.Mutex
	cond    *sync.Cond
	queue   managedQueue
	seq     int64
	closing bool

	activeKinds       map[TaskKind]int
	activePriority    map[TaskKind]Priority
	activeInteractive int
	activeBackground  int
	lastDeferred      bool
	lastUnavailable   bool
}

func newManagedScheduler(cfg ManagedConfig) *managedScheduler {
	if cfg.MaxConcurrency <= 0 {
		cfg.MaxConcurrency = 1
	}
	if cfg.QueueLimit <= 0 {
		cfg.QueueLimit = 64
	}
	s := &managedScheduler{cfg: cfg}
	s.cond = sync.NewCond(&s.mu)
	s.activeKinds = make(map[TaskKind]int)
	s.activePriority = make(map[TaskKind]Priority)
	for i := 0; i < cfg.MaxConcurrency; i++ {
		go s.worker()
	}
	return s
}

func (s *managedScheduler) worker() {
	for {
		s.mu.Lock()
		for len(s.queue) == 0 && !s.closing {
			s.cond.Wait()
		}
		if s.closing {
			s.mu.Unlock()
			return
		}
		task := s.popNextLocked()
		if task == nil {
			s.cond.Wait()
			s.mu.Unlock()
			continue
		}
		s.markActiveLocked(task)
		s.mu.Unlock()

		err := task.run()
		s.mu.Lock()
		s.markDoneLocked(task, err)
		s.cond.Broadcast()
		s.mu.Unlock()
		task.done <- err
	}
}

func (s *managedScheduler) popNextLocked() *managedTask {
	if len(s.queue) == 0 {
		return nil
	}
	interactivePending := s.activeInteractive > 0
	for _, queued := range s.queue {
		if queued.priority >= PriorityInteractive {
			interactivePending = true
			break
		}
	}
	bestIdx := -1
	for i, queued := range s.queue {
		if s.cfg.PauseBackgroundWhileInteractive &&
			queued.priority == PriorityBackground &&
			interactivePending {
			continue
		}
		if bestIdx == -1 ||
			queued.priority > s.queue[bestIdx].priority ||
			(queued.priority == s.queue[bestIdx].priority && queued.seq < s.queue[bestIdx].seq) {
			bestIdx = i
		}
	}
	if bestIdx == -1 {
		return nil
	}
	return heap.Remove(&s.queue, bestIdx).(*managedTask)
}

func (s *managedScheduler) markActiveLocked(task *managedTask) {
	s.lastDeferred = false
	s.activeKinds[task.kind]++
	s.activePriority[task.kind] = task.priority
	if task.priority >= PriorityInteractive {
		s.activeInteractive++
	} else if task.priority == PriorityBackground {
		s.activeBackground++
	}
}

func (s *managedScheduler) markDoneLocked(task *managedTask, err error) {
	if s.activeKinds[task.kind] > 1 {
		s.activeKinds[task.kind]--
	} else {
		delete(s.activeKinds, task.kind)
		delete(s.activePriority, task.kind)
	}
	if task.priority >= PriorityInteractive {
		s.activeInteractive--
	} else if task.priority == PriorityBackground {
		s.activeBackground--
	}
	if err == nil {
		s.lastUnavailable = false
		return
	}
	if IsUnavailableError(err) {
		s.lastUnavailable = true
	}
}

func (s *managedScheduler) status() SchedulerStatus {
	s.mu.Lock()
	defer s.mu.Unlock()

	status := SchedulerStatus{
		Deferred:    s.lastDeferred,
		Unavailable: s.lastUnavailable,
	}
	for kind, count := range s.activeKinds {
		if count == 0 || kind == TaskKindUnknown {
			continue
		}
		priority := s.activePriority[kind]
		if status.ActiveKind == TaskKindUnknown || priority > status.ActivePriority {
			status.ActiveKind = kind
			status.ActivePriority = priority
		}
	}
	for _, queued := range s.queue {
		switch {
		case queued.priority >= PriorityInteractive:
			status.QueuedInteractiveCount++
			if status.QueuedInteractiveKind == TaskKindUnknown {
				status.QueuedInteractiveKind = queued.kind
			}
		case queued.priority == PriorityBackground:
			status.QueuedBackgroundCount++
			if status.QueuedBackgroundKind == TaskKindUnknown {
				status.QueuedBackgroundKind = queued.kind
			}
		}
	}
	return status
}

func (s *managedScheduler) submit(priority Priority, kind TaskKind, run func() error) error {
	task := &managedTask{
		priority: priority,
		kind:     kind,
		seq:      atomic.AddInt64(&s.seq, 1),
		run:      run,
		done:     make(chan error, 1),
	}

	s.mu.Lock()
	backgroundQueued := 0
	for _, queued := range s.queue {
		if queued.priority == PriorityBackground {
			backgroundQueued++
		}
	}
	if priority == PriorityBackground && backgroundQueued >= s.cfg.QueueLimit {
		s.lastDeferred = true
		s.mu.Unlock()
		return ErrDeferred
	}
	heap.Push(&s.queue, task)
	s.cond.Signal()
	s.mu.Unlock()

	return <-task.done
}

type ManagedClient struct {
	base      AIClient
	scheduler *managedScheduler
	priority  Priority
	taskKind  TaskKind
}

func NewManagedClient(base AIClient, cfg ManagedConfig) *ManagedClient {
	return &ManagedClient{
		base:      base,
		scheduler: newManagedScheduler(cfg),
		priority:  priorityUnset,
		taskKind:  TaskKindUnknown,
	}
}

func (c *ManagedClient) withPriority(priority Priority) AIClient {
	return &ManagedClient{
		base:      c.base,
		scheduler: c.scheduler,
		priority:  priority,
		taskKind:  c.taskKind,
	}
}

func (c *ManagedClient) withTaskKind(kind TaskKind) AIClient {
	return &ManagedClient{
		base:      c.base,
		scheduler: c.scheduler,
		priority:  c.priority,
		taskKind:  kind,
	}
}

func WithPriority(client AIClient, priority Priority) AIClient {
	if client == nil {
		return nil
	}
	if p, ok := client.(prioritizedClient); ok {
		return p.withPriority(priority)
	}
	return client
}

func WithTaskKind(client AIClient, kind TaskKind) AIClient {
	if client == nil {
		return nil
	}
	if p, ok := client.(taskKindClient); ok {
		return p.withTaskKind(kind)
	}
	return client
}

func (c *ManagedClient) effectivePriority(defaultPriority Priority) Priority {
	if c.priority != priorityUnset {
		return c.priority
	}
	return defaultPriority
}

func (c *ManagedClient) effectiveKind(defaultKind TaskKind) TaskKind {
	if c.taskKind != TaskKindUnknown {
		return c.taskKind
	}
	return defaultKind
}

func (c *ManagedClient) do(priority Priority, kind TaskKind, fn func() error) error {
	if c == nil || c.base == nil || c.scheduler == nil {
		return nil
	}
	return c.scheduler.submit(c.effectivePriority(priority), c.effectiveKind(kind), fn)
}

func (c *ManagedClient) AIStatus() SchedulerStatus {
	if c == nil || c.scheduler == nil {
		return SchedulerStatus{}
	}
	return c.scheduler.status()
}

func (c *ManagedClient) Classify(sender, subject string) (Category, error) {
	var out Category
	err := c.do(PriorityUserAction, TaskKindClassification, func() error {
		var err error
		out, err = c.base.Classify(sender, subject)
		return err
	})
	return out, err
}

func (c *ManagedClient) Chat(messages []ChatMessage) (string, error) {
	var out string
	err := c.do(PriorityInteractive, TaskKindChat, func() error {
		var err error
		out, err = c.base.Chat(messages)
		return err
	})
	return out, err
}

func (c *ManagedClient) ChatWithTools(messages []ChatMessage, tools []Tool) (string, []ToolCall, error) {
	var out string
	var calls []ToolCall
	err := c.do(PriorityInteractive, TaskKindChat, func() error {
		var err error
		out, calls, err = c.base.ChatWithTools(messages, tools)
		return err
	})
	return out, calls, err
}

func (c *ManagedClient) Embed(text string) ([]float32, error) {
	var out []float32
	err := c.do(PriorityInteractive, TaskKindSemanticSearch, func() error {
		var err error
		out, err = c.base.Embed(text)
		return err
	})
	return out, err
}

func (c *ManagedClient) SetEmbeddingModel(model string) {
	c.base.SetEmbeddingModel(model)
}

func (c *ManagedClient) GenerateQuickReplies(sender, subject, bodyPreview string) ([]string, error) {
	var out []string
	err := c.do(PriorityInteractive, TaskKindQuickReply, func() error {
		var err error
		out, err = c.base.GenerateQuickReplies(sender, subject, bodyPreview)
		return err
	})
	return out, err
}

func (c *ManagedClient) EnrichContact(email string, subjects []string) (string, []string, error) {
	var company string
	var topics []string
	err := c.do(PriorityInteractive, TaskKindContactEnrich, func() error {
		var err error
		company, topics, err = c.base.EnrichContact(email, subjects)
		return err
	})
	return company, topics, err
}

func (c *ManagedClient) HasVisionModel() bool {
	return c.base.HasVisionModel()
}

func (c *ManagedClient) DescribeImage(ctx context.Context, imageBytes []byte, mimeType string) (string, error) {
	var out string
	err := c.do(PriorityInteractive, TaskKindImageDescription, func() error {
		var err error
		out, err = c.base.DescribeImage(ctx, imageBytes, mimeType)
		return err
	})
	return out, err
}

func (c *ManagedClient) Ping() error {
	return c.do(PriorityInteractive, TaskKindUnknown, func() error {
		return c.base.Ping()
	})
}

var _ AIClient = (*ManagedClient)(nil)
var _ StatusReporter = (*ManagedClient)(nil)
