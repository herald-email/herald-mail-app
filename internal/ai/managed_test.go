package ai

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type managedStubAI struct {
	chatFn func([]ChatMessage) (string, error)
}

func (s *managedStubAI) Chat(messages []ChatMessage) (string, error) {
	if s.chatFn != nil {
		return s.chatFn(messages)
	}
	return "", nil
}
func (s *managedStubAI) ChatWithTools(_ []ChatMessage, _ []Tool) (string, []ToolCall, error) {
	return "", nil, ErrToolsNotSupported
}
func (s *managedStubAI) Classify(_, _ string) (Category, error)                { return "", nil }
func (s *managedStubAI) Embed(_ string) ([]float32, error)                     { return []float32{1, 2, 3}, nil }
func (s *managedStubAI) SetEmbeddingModel(_ string)                            {}
func (s *managedStubAI) GenerateQuickReplies(_, _, _ string) ([]string, error) { return nil, nil }
func (s *managedStubAI) EnrichContact(_ string, _ []string) (string, []string, error) {
	return "", nil, nil
}
func (s *managedStubAI) HasVisionModel() bool { return false }
func (s *managedStubAI) DescribeImage(_ context.Context, _ []byte, _ string) (string, error) {
	return "", nil
}
func (s *managedStubAI) Ping() error { return nil }

func TestManagedClient_SerializesWhenMaxConcurrencyIsOne(t *testing.T) {
	var running int32
	var maxRunning int32
	release := make(chan struct{})
	base := &managedStubAI{
		chatFn: func(_ []ChatMessage) (string, error) {
			current := atomic.AddInt32(&running, 1)
			for {
				old := atomic.LoadInt32(&maxRunning)
				if current <= old || atomic.CompareAndSwapInt32(&maxRunning, old, current) {
					break
				}
			}
			<-release
			atomic.AddInt32(&running, -1)
			return "ok", nil
		},
	}
	client := NewManagedClient(base, ManagedConfig{
		MaxConcurrency:                  1,
		QueueLimit:                      8,
		PauseBackgroundWhileInteractive: true,
	})

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		_, _ = client.Chat([]ChatMessage{{Role: "user", Content: "one"}})
	}()
	go func() {
		defer wg.Done()
		_, _ = client.Chat([]ChatMessage{{Role: "user", Content: "two"}})
	}()

	time.Sleep(50 * time.Millisecond)
	close(release)
	wg.Wait()

	if got := atomic.LoadInt32(&maxRunning); got != 1 {
		t.Fatalf("expected max concurrency 1, got %d", got)
	}
}

func TestManagedClient_PrioritizesInteractiveWorkAheadOfQueuedBackground(t *testing.T) {
	started := make(chan string, 3)
	releaseFirst := make(chan struct{})
	scheduler := newManagedScheduler(ManagedConfig{
		MaxConcurrency:                  1,
		QueueLimit:                      8,
		PauseBackgroundWhileInteractive: true,
	})

	done1 := make(chan struct{})
	done2 := make(chan struct{})
	done3 := make(chan struct{})
	go func() {
		_ = scheduler.submit(PriorityBackground, func() error {
			started <- "bg-1"
			<-releaseFirst
			return nil
		})
		close(done1)
	}()

	first := <-started
	if first != "bg-1" {
		t.Fatalf("expected first started call to be bg-1, got %q", first)
	}

	go func() {
		_ = scheduler.submit(PriorityBackground, func() error {
			started <- "bg-2"
			return nil
		})
		close(done2)
	}()
	time.Sleep(20 * time.Millisecond)
	go func() {
		_ = scheduler.submit(PriorityInteractive, func() error {
			started <- "interactive"
			return nil
		})
		close(done3)
	}()
	time.Sleep(20 * time.Millisecond)

	close(releaseFirst)

	second := <-started
	if second != "interactive" {
		t.Fatalf("expected interactive call to run before queued background work, got %q", second)
	}

	<-done1
	<-done2
	<-done3
	third := <-started
	if third != "bg-2" {
		t.Fatalf("expected queued background work to run last, got %q", third)
	}
}

func TestManagedClient_BackgroundQueueLimitFailsOpen(t *testing.T) {
	release := make(chan struct{})
	base := &managedStubAI{
		chatFn: func(_ []ChatMessage) (string, error) {
			<-release
			return "ok", nil
		},
	}
	client := NewManagedClient(base, ManagedConfig{
		MaxConcurrency:                  1,
		QueueLimit:                      1,
		PauseBackgroundWhileInteractive: true,
	})
	background := WithPriority(client, PriorityBackground)

	go func() {
		_, _ = background.Chat([]ChatMessage{{Role: "user", Content: "first"}})
	}()
	time.Sleep(20 * time.Millisecond)
	go func() {
		_, _ = background.Chat([]ChatMessage{{Role: "user", Content: "second"}})
	}()
	time.Sleep(20 * time.Millisecond)

	_, err := background.Chat([]ChatMessage{{Role: "user", Content: "third"}})
	close(release)

	if err == nil {
		t.Fatal("expected queue saturation error for background work")
	}
	if err != ErrDeferred {
		t.Fatalf("expected ErrDeferred, got %v", err)
	}
}
