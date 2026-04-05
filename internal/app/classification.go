package app

import (
	"errors"

	tea "github.com/charmbracelet/bubbletea"
	"mail-processor/internal/logger"
	"mail-processor/internal/models"
)

func (m *Model) startClassification() tea.Cmd {
	folder := m.currentFolder
	ch := m.classifyCh // capture the current channel
	return func() tea.Msg {
		defer close(ch) // unblock the listener when we're done
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
			cat, err := m.classifier.Classify(email.Sender, email.Subject)
			if err != nil {
				logger.Warn("Classification failed for %s: %v", id, err)
				continue
			}
			_ = m.backend.SetClassification(id, cat)
			ch <- ClassifyProgressMsg{
				MessageID: id,
				Category:  cat,
				Done:      i + 1,
				Total:     total,
			}
		}
		return ClassifyDoneMsg{}
	}
}

// reclassifyEmailCmd re-classifies a single email and stores the result.
func (m *Model) reclassifyEmailCmd(email *models.EmailData) tea.Cmd {
	classifier := m.classifier // snapshot before goroutine
	b := m.backend
	messageID := email.MessageID
	sender := email.Sender
	subject := email.Subject
	return func() tea.Msg {
		if classifier == nil {
			return ReclassifyResultMsg{Err: errors.New("no AI classifier configured")}
		}
		cat, err := classifier.Classify(sender, subject)
		if err != nil {
			return ReclassifyResultMsg{MessageID: messageID, Err: err}
		}
		if setErr := b.SetClassification(messageID, cat); setErr != nil {
			return ReclassifyResultMsg{MessageID: messageID, Err: setErr}
		}
		return ReclassifyResultMsg{MessageID: messageID, Category: cat}
	}
}

// autoClassifyEmailCmd classifies a newly arrived email in the background and
// returns AutoClassifyResultMsg. Unlike reclassifyEmailCmd, it is a fire-and-
// forget background op triggered automatically on email arrival — no visible
// status update is set on success.
func (m *Model) autoClassifyEmailCmd(email *models.EmailData) tea.Cmd {
	classifier := m.classifier // snapshot
	b := m.backend
	messageID := email.MessageID
	sender := email.Sender
	subject := email.Subject
	return func() tea.Msg {
		cat, err := classifier.Classify(sender, subject)
		if err != nil {
			return AutoClassifyResultMsg{MessageID: messageID, Err: err}
		}
		_ = b.SetClassification(messageID, cat)
		return AutoClassifyResultMsg{MessageID: messageID, Category: string(cat)}
	}
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
