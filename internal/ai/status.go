package ai

import "strings"

type TaskKind string

const (
	TaskKindUnknown          TaskKind = ""
	TaskKindEmbedding        TaskKind = "embedding"
	TaskKindQuickReply       TaskKind = "quick reply"
	TaskKindSemanticSearch   TaskKind = "semantic search"
	TaskKindChat             TaskKind = "chat"
	TaskKindImageDescription TaskKind = "image description"
	TaskKindContactEnrich    TaskKind = "contact enrichment"
	TaskKindClassification   TaskKind = "classification"
)

type SchedulerStatus struct {
	ActiveKind             TaskKind
	ActivePriority         Priority
	QueuedInteractiveKind  TaskKind
	QueuedInteractiveCount int
	QueuedBackgroundKind   TaskKind
	QueuedBackgroundCount  int
	Deferred               bool
	Unavailable            bool
}

func (s SchedulerStatus) DisplayKind() TaskKind {
	switch {
	case s.Unavailable:
		return "unavailable"
	case s.QueuedInteractiveKind != TaskKindUnknown:
		return s.QueuedInteractiveKind
	case s.ActiveKind != TaskKindUnknown:
		return s.ActiveKind
	case s.QueuedBackgroundKind != TaskKindUnknown:
		return s.QueuedBackgroundKind
	case s.Deferred:
		return "deferred"
	default:
		return "idle"
	}
}

func (s SchedulerStatus) DisplayQueuedCount() int {
	if s.QueuedInteractiveCount > 0 {
		return s.QueuedInteractiveCount
	}
	return s.QueuedBackgroundCount
}

type StatusReporter interface {
	AIStatus() SchedulerStatus
}

func mergeSchedulerStatus(dst, src SchedulerStatus) SchedulerStatus {
	if src.ActiveKind != TaskKindUnknown && (dst.ActiveKind == TaskKindUnknown || src.ActivePriority > dst.ActivePriority) {
		dst.ActiveKind = src.ActiveKind
		dst.ActivePriority = src.ActivePriority
	}
	if dst.QueuedInteractiveKind == TaskKindUnknown && src.QueuedInteractiveKind != TaskKindUnknown {
		dst.QueuedInteractiveKind = src.QueuedInteractiveKind
	}
	if dst.QueuedBackgroundKind == TaskKindUnknown && src.QueuedBackgroundKind != TaskKindUnknown {
		dst.QueuedBackgroundKind = src.QueuedBackgroundKind
	}
	dst.QueuedInteractiveCount += src.QueuedInteractiveCount
	dst.QueuedBackgroundCount += src.QueuedBackgroundCount
	dst.Deferred = dst.Deferred || src.Deferred
	dst.Unavailable = dst.Unavailable || src.Unavailable
	return dst
}

func IsUnavailableError(err error) bool {
	if err == nil {
		return false
	}
	if MissingModelInstallHint(err) != "" {
		return true
	}
	msg := strings.ToLower(err.Error())
	for _, needle := range []string{
		"connection refused",
		"dial tcp",
		"no such host",
		"i/o timeout",
		"context deadline exceeded",
		"client.timeout exceeded",
		"use of closed network connection",
		"eof",
		"ollama returned 404",
	} {
		if strings.Contains(msg, needle) {
			return true
		}
	}
	return false
}

func IsContextLengthError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "context len") ||
		strings.Contains(msg, "context length") ||
		strings.Contains(msg, "input length exceeds")
}
