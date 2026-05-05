package cleanup

import (
	"context"
	"sync"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/herald-email/herald-mail-app/internal/models"
)

// CleanupDoneMsg is sent when a cleanup run completes.
type CleanupDoneMsg struct {
	Results map[int64]int
}

// Scheduler periodically runs all enabled cleanup rules.
type Scheduler struct {
	engine   *Engine
	interval time.Duration
	stopCh   chan struct{}
	once     sync.Once
}

// NewScheduler creates a new Scheduler. intervalHours <= 0 defaults to 24 hours.
func NewScheduler(e *Engine, intervalHours int) *Scheduler {
	d := time.Duration(intervalHours) * time.Hour
	if d <= 0 {
		d = 24 * time.Hour
	}
	return &Scheduler{engine: e, interval: d, stopCh: make(chan struct{})}
}

// Start begins background periodic cleanup runs. Non-blocking.
func (s *Scheduler) Start(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(s.interval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				_, _ = s.engine.RunAll(ctx)
			case <-s.stopCh:
				return
			case <-ctx.Done():
				return
			}
		}
	}()
}

// Stop signals the background goroutine to exit.
func (s *Scheduler) Stop() {
	s.once.Do(func() { close(s.stopCh) })
}

// RunNow executes all cleanup rules immediately and returns a tea.Cmd
// that emits CleanupDoneMsg when done.
func (s *Scheduler) RunNow(ctx context.Context) tea.Cmd {
	return func() tea.Msg {
		results, _ := s.engine.RunAll(ctx)
		return CleanupDoneMsg{Results: results}
	}
}

func (s *Scheduler) RunNowRequest(req models.RuleDryRunRequest) tea.Cmd {
	return func() tea.Msg {
		results, _ := s.engine.Run(context.Background(), req)
		return CleanupDoneMsg{Results: results}
	}
}
