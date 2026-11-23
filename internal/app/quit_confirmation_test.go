package app

import (
	tea "charm.land/bubbletea/v2"
	"strings"
	"testing"
	"time"
)

func TestQuitRequiresSecondCtrlC(t *testing.T) {
	m := makeSizedModel(t, 80, 24)
	key := tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl}
	_, _ = m.Update(key)
	if !strings.Contains(stripANSI(m.renderStatusBar()), "Ctrl+C again") {
		t.Fatal("first Ctrl+C must show confirmation warning")
	}
	_, cmd := m.Update(key)
	if !commandIsQuit(cmd) {
		t.Fatal("second Ctrl+C must quit")
	}
}

func TestQuitConfirmationExpiresAndIgnoresOldTimer(t *testing.T) {
	m := makeSizedModel(t, 80, 24)
	_ = m.requestQuit(false)
	old := m.quitDeadline
	m.quitDeadline = time.Now().Add(-time.Second)
	_ = m.requestQuit(true)
	if !m.quitConfirmationActive() {
		t.Fatal("expired press must rearm")
	}
	_, _ = m.Update(quitConfirmationExpiredMsg{deadline: old})
	if !m.quitConfirmationActive() {
		t.Fatal("old timer cleared new warning")
	}
	_, _ = m.Update(quitConfirmationExpiredMsg{deadline: m.quitDeadline})
	if m.quitConfirmationActive() {
		t.Fatal("timer must clear warning")
	}
}

func TestQuitConfirmationAcrossScreensAndCancellation(t *testing.T) {
	for _, screen := range []string{"timeline", "compose", "calendar", "help", "settings", "small"} {
		t.Run(screen, func(t *testing.T) {
			m := makeSizedModel(t, 80, 24)
			switch screen {
			case "compose":
				m.activeTab = tabCompose
			case "calendar":
				m.activeTab = tabCalendar
			case "help":
				m.showHelp = true
			case "settings":
				m.openSettingsPanel()
			case "small":
				m.windowWidth, m.windowHeight = 50, 15
			}
			key := tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl}
			_, _ = m.Update(key)
			if !strings.Contains(stripANSI(m.View().Content), quitConfirmationText) {
				t.Fatal("warning missing from screen")
			}
			_, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
			if m.quitConfirmationActive() {
				t.Fatal("another key must cancel")
			}
			_, _ = m.Update(key)
			if !m.quitConfirmationActive() {
				t.Fatal("next Ctrl+C must rearm")
			}
		})
	}
}
