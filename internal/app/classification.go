package app

import (
	"errors"

	tea "charm.land/bubbletea/v2"
	"github.com/herald-email/herald-mail-app/internal/ai"
	"github.com/herald-email/herald-mail-app/internal/logger"
	"github.com/herald-email/herald-mail-app/internal/models"
)

func (m *Model) startClassification() tea.Cmd {
	folder := m.currentFolder
	ch := m.classifyCh // capture the current channel
	classifier := ai.WithTaskKind(ai.WithPriority(m.classifier, ai.PriorityBackground), ai.TaskKindClassification)
	return func() tea.Msg {
		defer close(ch) // unblock the listener when we're done
		if classifier == nil {
			return ClassifyDoneMsg{}
		}
		ids, err := m.backend.GetUnclassifiedIDs(folder)
		if err != nil || len(ids) == 0 {
			return ClassifyDoneMsg{}
		}
		total := len(ids)
		for i, id := range ids {
			email, err := m.backend.GetEmailByID(id)
			if err != nil {
				continue
			}
			cat, err := classifier.Classify(email.Sender, email.Subject)
			if err != nil {
				logger.Warn("Classification failed for %s: %v", id, err)
				continue
			}
			ref := email.MessageRef()
			setClassification(m.backend, ref, id, cat)
			ch <- ClassifyProgressMsg{
				MessageRef: ref,
				MessageID:  id,
				Category:   cat,
				Done:       i + 1,
				Total:      total,
			}
		}
		return ClassifyDoneMsg{}
	}
}

func (m *Model) startClassificationIfNeeded() tea.Cmd {
	if m.loading || m.classifying || m.classifier == nil || m.timelineIsReadOnlyDiagnostic() {
		return nil
	}
	m.classifying = true
	m.classifyDone = 0
	m.classifyTotal = 0
	m.classifyCh = make(chan ClassifyProgressMsg, 50)
	return tea.Batch(m.startClassification(), m.listenForClassification())
}

// reclassifyEmailCmd re-classifies a single email and stores the result.
func (m *Model) reclassifyEmailCmd(email *models.EmailData) tea.Cmd {
	classifier := ai.WithTaskKind(ai.WithPriority(m.classifier, ai.PriorityUserAction), ai.TaskKindClassification) // snapshot before goroutine
	b := m.backend
	messageID := email.MessageID
	ref := email.MessageRef()
	sender := email.Sender
	subject := email.Subject
	return func() tea.Msg {
		if classifier == nil {
			return ReclassifyResultMsg{Err: errors.New("no AI classifier configured")}
		}
		cat, err := classifier.Classify(sender, subject)
		if err != nil {
			return ReclassifyResultMsg{MessageRef: ref, MessageID: messageID, Err: err}
		}
		if setErr := setClassification(b, ref, messageID, cat); setErr != nil {
			return ReclassifyResultMsg{MessageRef: ref, MessageID: messageID, Err: setErr}
		}
		return ReclassifyResultMsg{MessageRef: ref, MessageID: messageID, Category: cat}
	}
}

// autoClassifyEmailCmd classifies a newly arrived email in the background and
// returns AutoClassifyResultMsg. Unlike reclassifyEmailCmd, it is a fire-and-
// forget background op triggered automatically on email arrival — no visible
// status update is set on success.
func (m *Model) autoClassifyEmailCmd(email *models.EmailData) tea.Cmd {
	classifier := ai.WithTaskKind(ai.WithPriority(m.classifier, ai.PriorityBackground), ai.TaskKindClassification) // snapshot
	b := m.backend
	messageID := email.MessageID
	ref := email.MessageRef()
	sender := email.Sender
	subject := email.Subject
	return func() tea.Msg {
		if classifier == nil {
			return AutoClassifyResultMsg{MessageRef: ref, MessageID: messageID, Err: errors.New("no AI classifier configured")}
		}
		cat, err := classifier.Classify(sender, subject)
		if err != nil {
			return AutoClassifyResultMsg{MessageRef: ref, MessageID: messageID, Err: err}
		}
		_ = setClassification(b, ref, messageID, cat)
		return AutoClassifyResultMsg{MessageRef: ref, MessageID: messageID, Category: string(cat)}
	}
}

func setClassification(b interface {
	SetClassification(string, string) error
}, ref models.MessageRef, messageID, category string) error {
	if scoped, ok := b.(interface {
		SetClassificationByRef(models.MessageRef, string) error
	}); ok && (ref.MessageID != "" || ref.LocalID != "") {
		return scoped.SetClassificationByRef(ref, category)
	}
	return b.SetClassification(messageID, category)
}

func classificationKeys(ref models.MessageRef, messageID string) []string {
	if ref.MessageID == "" {
		ref.MessageID = messageID
	}
	if ref.MessageID != "" || ref.LocalID != "" {
		ref = ref.WithDefaults()
	}
	keys := make([]string, 0, 2)
	if ref.LocalID != "" {
		keys = append(keys, ref.LocalID)
	}
	if ref.MessageID != "" && ref.MessageID != ref.LocalID {
		keys = append(keys, ref.MessageID)
	}
	return keys
}

func (m *Model) setClassificationKeys(ref models.MessageRef, messageID, category string) {
	if m.classifications == nil {
		m.classifications = make(map[string]string)
	}
	for _, key := range classificationKeys(ref, messageID) {
		m.classifications[key] = category
	}
}

func (m *Model) classificationForEmail(email *models.EmailData) string {
	if m.classifications == nil || email == nil {
		return ""
	}
	ref := email.MessageRef()
	if ref.LocalID != "" {
		if category := m.classifications[ref.LocalID]; category != "" {
			return category
		}
	}
	return m.classifications[email.MessageID]
}

// listenForClassification waits for the next classification result.
// Returns ClassifyDoneMsg when the channel is closed (classification finished).
func (m *Model) listenForClassification() tea.Cmd {
	ch := m.classifyCh // capture so it survives a channel replacement
	return func() tea.Msg {
		msg, ok := <-ch
		if !ok {
			return ClassifyDoneMsg{} // channel closed — classification is done
		}
		return msg
	}
}

// loadClassifications fetches existing AI tags from cache for the current folder
func (m *Model) loadClassifications() {
	if m.timelineIsReadOnlyDiagnostic() {
		m.classifications = make(map[string]string)
		return
	}
	tags, err := m.backend.GetClassifications(m.currentFolder)
	if err != nil {
		logger.Warn("Failed to load classifications: %v", err)
		return
	}
	for id, cat := range tags {
		m.classifications[id] = cat
	}
}

// handleComposeKey handles all key input when on the compose tab
