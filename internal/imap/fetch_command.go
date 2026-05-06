package imap

import (
	"errors"
	"fmt"
	"time"

	"github.com/emersion/go-imap"
	"github.com/herald-email/herald-mail-app/internal/logger"
)

// ErrIMAPCommandTimeout marks a stalled IMAP fetch stream. The underlying
// go-imap API has no context parameter, so timeout recovery closes the current
// connection and retries the whole command from a clean reconnect.
var ErrIMAPCommandTimeout = errors.New("imap command timed out")

const (
	defaultIMAPCommandIdleTimeout = 90 * time.Second
	defaultIMAPCommandAbortWait   = 5 * time.Second
	defaultIMAPCommandAttempts    = 2
	defaultIMAPCommandBuffer      = 10
)

type imapStreamCommandOptions struct {
	Name          string
	Folder        string
	Phase         string
	RangeLabel    string
	IdleTimeout   time.Duration
	AbortWait     time.Duration
	MaxAttempts   int
	MessageBuffer int
	Reconnect     func() error
	Abort         func()
}

func (o imapStreamCommandOptions) idleTimeout() time.Duration {
	if o.IdleTimeout > 0 {
		return o.IdleTimeout
	}
	return defaultIMAPCommandIdleTimeout
}

func (o imapStreamCommandOptions) abortWait() time.Duration {
	if o.AbortWait > 0 {
		return o.AbortWait
	}
	return defaultIMAPCommandAbortWait
}

func (o imapStreamCommandOptions) attempts() int {
	if o.MaxAttempts > 0 {
		return o.MaxAttempts
	}
	return defaultIMAPCommandAttempts
}

func (o imapStreamCommandOptions) buffer() int {
	if o.MessageBuffer > 0 {
		return o.MessageBuffer
	}
	return defaultIMAPCommandBuffer
}

func (o imapStreamCommandOptions) label() string {
	label := o.Name
	if label == "" {
		label = "imap fetch"
	}
	if o.Folder != "" {
		label += " folder=" + o.Folder
	}
	if o.Phase != "" {
		label += " phase=" + o.Phase
	}
	if o.RangeLabel != "" {
		label += " range=" + o.RangeLabel
	}
	return label
}

func runIMAPStreamCommand(
	opts imapStreamCommandOptions,
	fetch func(messages chan *imap.Message) error,
	handle func(*imap.Message) error,
) error {
	if fetch == nil {
		return fmt.Errorf("imap stream command %s: nil fetch", opts.label())
	}

	var lastErr error
	maxAttempts := opts.attempts()
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		started := time.Now()
		messages, err := collectIMAPStreamAttempt(opts, attempt, fetch)
		duration := time.Since(started)
		if err == nil {
			logger.Debug(
				"IMAP command succeeded: %s attempt=%d/%d messages=%d duration=%s",
				opts.label(), attempt, maxAttempts, len(messages), duration.Round(time.Millisecond),
			)
			for _, msg := range messages {
				if msg == nil || handle == nil {
					continue
				}
				if err := handle(msg); err != nil {
					return fmt.Errorf("%s handle message: %w", opts.label(), err)
				}
			}
			return nil
		}

		lastErr = err
		retryable := errors.Is(err, ErrIMAPCommandTimeout) || isConnectionError(err)
		logger.Warn(
			"IMAP command failed: %s attempt=%d/%d duration=%s retryable=%t error=%v",
			opts.label(), attempt, maxAttempts, duration.Round(time.Millisecond), retryable, err,
		)
		if !retryable || attempt == maxAttempts {
			break
		}
		if opts.Reconnect != nil {
			if reconnectErr := opts.Reconnect(); reconnectErr != nil {
				return fmt.Errorf("reconnect after %s: %w (original error: %v)", opts.label(), reconnectErr, err)
			}
		}
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("unknown IMAP stream failure")
	}
	return lastErr
}

func collectIMAPStreamAttempt(
	opts imapStreamCommandOptions,
	attempt int,
	fetch func(messages chan *imap.Message) error,
) ([]*imap.Message, error) {
	messages := make(chan *imap.Message, opts.buffer())
	done := make(chan error, 1)
	go func() {
		done <- fetch(messages)
	}()

	idleTimeout := opts.idleTimeout()
	timer := time.NewTimer(idleTimeout)
	defer timer.Stop()

	collected := make([]*imap.Message, 0)
	for {
		select {
		case msg, ok := <-messages:
			if !ok {
				err := <-done
				if err != nil {
					return nil, fmt.Errorf("%s attempt=%d fetch: %w", opts.label(), attempt, err)
				}
				return collected, nil
			}
			if msg != nil {
				collected = append(collected, msg)
			}
			resetTimer(timer, idleTimeout)

		case <-timer.C:
			timeoutErr := fmt.Errorf(
				"%w: %s attempt=%d idle=%s messages=%d",
				ErrIMAPCommandTimeout,
				opts.label(),
				attempt,
				idleTimeout,
				len(collected),
			)
			if opts.Abort != nil {
				opts.Abort()
			}
			select {
			case err := <-done:
				if err != nil {
					logger.Debug("IMAP command abort completed after timeout: %s attempt=%d error=%v", opts.label(), attempt, err)
				}
			case <-time.After(opts.abortWait()):
				logger.Warn("IMAP command abort did not complete before retry: %s attempt=%d wait=%s", opts.label(), attempt, opts.abortWait())
			}
			return nil, timeoutErr
		}
	}
}

func resetTimer(timer *time.Timer, d time.Duration) {
	if !timer.Stop() {
		select {
		case <-timer.C:
		default:
		}
	}
	timer.Reset(d)
}

func (c *Client) runFetchStreamLocked(
	opts imapStreamCommandOptions,
	fetch func(messages chan *imap.Message) error,
	handle func(*imap.Message) error,
) error {
	opts.Reconnect = c.Reconnect
	opts.Abort = c.abortIMAPCommandLocked
	return runIMAPStreamCommand(opts, fetch, handle)
}

// abortIMAPCommandLocked interrupts a potentially wedged go-imap command.
// Caller must hold c.mu.
func (c *Client) abortIMAPCommandLocked() {
	if c.client == nil {
		return
	}
	logger.Warn("Closing IMAP connection to abort stalled command")
	_ = c.client.Logout()
	c.client = nil
}
