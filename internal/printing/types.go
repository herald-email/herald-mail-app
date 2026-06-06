package printing

import (
	"context"
	"errors"
	"fmt"
	htmlstd "html"
	"os"
	"strings"
	"time"

	"github.com/herald-email/herald-mail-app/internal/models"
)

type Mode string

const (
	ModeOriginalVisual   Mode = "original_visual"
	ModeRenderedMarkdown Mode = "rendered_markdown"
)

type Status string

const (
	StatusOpened      Status = "opened"
	StatusCanceled    Status = "canceled"
	StatusUnsupported Status = "unsupported"
)

type Request struct {
	Email             *models.EmailData
	Body              *models.EmailBody
	Mode              Mode
	Theme             Theme
	Title             string
	AllowRemoteImages bool
}

type Result struct {
	Status  Status
	Message string
	Path    string
}

type Printer interface {
	Print(context.Context, Request) (Result, error)
}

type UnsupportedPrinter struct {
	Reason string
}

func (p UnsupportedPrinter) Print(_ context.Context, _ Request) (Result, error) {
	reason := strings.TrimSpace(p.Reason)
	if reason == "" {
		reason = "this build does not support the macOS print dialog"
	}
	return Result{
		Status:  StatusUnsupported,
		Message: boundedStatus("Printing unsupported: " + reason),
	}, nil
}

func WriteTempHTML(document string) (string, error) {
	f, err := os.CreateTemp("", "herald-print-*.html")
	if err != nil {
		return "", fmt.Errorf("create print document: %w", err)
	}
	path := f.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(path)
		}
	}()
	if err := f.Chmod(0o600); err != nil {
		_ = f.Close()
		return "", fmt.Errorf("secure print document: %w", err)
	}
	if _, err := f.WriteString(document); err != nil {
		_ = f.Close()
		return "", fmt.Errorf("write print document: %w", err)
	}
	if err := f.Close(); err != nil {
		return "", fmt.Errorf("close print document: %w", err)
	}
	cleanup = false
	return path, nil
}

func normalizeMode(mode Mode) Mode {
	switch mode {
	case ModeOriginalVisual, ModeRenderedMarkdown:
		return mode
	default:
		return ModeOriginalVisual
	}
}

func requestTitle(req Request) string {
	if title := strings.TrimSpace(req.Title); title != "" {
		return title
	}
	if req.Email != nil && strings.TrimSpace(req.Email.Subject) != "" {
		return "Herald - " + strings.TrimSpace(req.Email.Subject)
	}
	if req.Body != nil && strings.TrimSpace(req.Body.Subject) != "" {
		return "Herald - " + strings.TrimSpace(req.Body.Subject)
	}
	return "Herald Email"
}

func messageSubject(req Request) string {
	if req.Body != nil && strings.TrimSpace(req.Body.Subject) != "" {
		return strings.TrimSpace(req.Body.Subject)
	}
	if req.Email != nil {
		return strings.TrimSpace(req.Email.Subject)
	}
	return ""
}

func messageFrom(req Request) string {
	if req.Body != nil && strings.TrimSpace(req.Body.From) != "" {
		return strings.TrimSpace(req.Body.From)
	}
	if req.Email != nil {
		return strings.TrimSpace(req.Email.Sender)
	}
	return ""
}

func messageDate(req Request) string {
	if req.Email == nil || req.Email.Date.IsZero() {
		return ""
	}
	return req.Email.Date.In(time.Local).Format("Mon, 02 Jan 2006 15:04 MST")
}

func escaped(s string) string {
	return htmlstd.EscapeString(strings.TrimSpace(s))
}

func boundedStatus(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	const limit = 220
	r := []rune(s)
	if len(r) > limit {
		return string(r[:limit-1]) + "..."
	}
	return s
}

var errMissingPrintBody = errors.New("print body is not loaded")
