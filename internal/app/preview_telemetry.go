package app

import (
	"context"
	"fmt"
	"strings"
	"time"

	backendpkg "github.com/herald-email/herald-mail-app/internal/backend"
	"github.com/herald-email/herald-mail-app/internal/logger"
	"github.com/herald-email/herald-mail-app/internal/models"
)

const (
	previewLoadSourceCache       = "cache"
	previewLoadSourceIMAP        = "imap"
	previewLoadSourceUnavailable = "unavailable"
)

type previewCacheBackend interface {
	GetCachedPreviewBody(messageID string) (*models.EmailBody, error)
	CachePreviewBody(messageID string, body *models.EmailBody) error
}

type previewFetchBackend interface {
	FetchPreviewBody(messageID, folder string, uid uint32) (*models.EmailBody, error)
}

type messagePreviewServiceBackend interface {
	GetMessagePreview(context.Context, models.MessageRef, backendpkg.MessageReadIntent) (backendpkg.MessageReadResult, error)
}

type messageBodyServiceBackend interface {
	GetMessage(context.Context, models.MessageRef) (backendpkg.MessageReadResult, error)
}

type previewLoadTelemetry struct {
	MessageRef models.MessageRef
	MessageID  string
	Folder     string
	UID        uint32
	Source     string
	StartedAt  time.Time
	FinishedAt time.Time
	Duration   time.Duration
	Err        string
}

func previewTelemetryFromEmailBodyMsg(msg EmailBodyMsg) previewLoadTelemetry {
	return previewLoadTelemetry{
		MessageRef: msg.MessageRef,
		MessageID:  msg.MessageID,
		Folder:     msg.Folder,
		UID:        msg.UID,
		Source:     msg.LoadSource,
		StartedAt:  msg.LoadStartedAt,
		FinishedAt: msg.LoadFinishedAt,
		Duration:   msg.LoadDuration,
		Err:        previewLoadErrString(msg.Err),
	}
}

func previewLoadErrString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func previewLoadTag(t previewLoadTelemetry, messageID string) string {
	if t.MessageID == "" || t.MessageID != messageID || t.Duration <= 0 {
		return ""
	}
	source := strings.TrimSpace(t.Source)
	if source == "" {
		source = previewLoadSourceIMAP
	}
	if t.Err != "" {
		return fmt.Sprintf("failed after %s %s", formatPreviewLoadDuration(t.Duration), source)
	}
	return fmt.Sprintf("%s %s", formatPreviewLoadDuration(t.Duration), source)
}

func formatPreviewLoadDuration(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	if d < time.Second {
		ms := d.Round(time.Millisecond) / time.Millisecond
		if ms < 1 && d > 0 {
			ms = 1
		}
		return fmt.Sprintf("%dms", ms)
	}
	return fmt.Sprintf("%.2fs", d.Seconds())
}

func logPreviewLoad(surface string, t previewLoadTelemetry, stale bool) {
	status := "ok"
	if t.Err != "" {
		status = "error"
	}
	logger.Info(
		"Preview load: surface=%s message_id=%s source_id=%s account_id=%s local_id=%s folder=%s uid=%d uid_validity=%d source=%s duration=%s status=%s stale=%t error=%q",
		surface,
		t.MessageID,
		t.MessageRef.SourceID,
		t.MessageRef.AccountID,
		t.MessageRef.LocalID,
		t.Folder,
		t.UID,
		t.MessageRef.UIDValidity,
		t.Source,
		t.Duration.Round(time.Millisecond),
		status,
		stale,
		t.Err,
	)
}
