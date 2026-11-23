package app

import (
	tea "charm.land/bubbletea/v2"
	"time"
)

const quitConfirmationWindow = 3 * time.Second
const quitConfirmationText = "Press Ctrl+C again within 3s to exit."

type quitConfirmationExpiredMsg struct{ deadline time.Time }

func (m *Model) quitConfirmationActive() bool {
	return !m.quitDeadline.IsZero() && time.Now().Before(m.quitDeadline)
}

func (m *Model) requestQuit(confirm bool) tea.Cmd {
	if confirm && m.quitConfirmationActive() {
		m.cleanup()
		return tea.Quit
	}
	m.quitDeadline = time.Now().Add(quitConfirmationWindow)
	deadline := m.quitDeadline
	return tea.Tick(quitConfirmationWindow, func(time.Time) tea.Msg {
		return quitConfirmationExpiredMsg{deadline: deadline}
	})
}

func (m *Model) renderQuitWarning() string {
	w := m.windowWidth
	if w <= 0 {
		w = 80
	}
	return m.theme.Severity.Caution.Style().Bold(true).Width(w).Padding(0, 1).
		Render(safeChromeLine(quitConfirmationText, max(0, w-2)))
}
