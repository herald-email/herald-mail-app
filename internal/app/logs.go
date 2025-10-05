package app

import (
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// LogEntry represents a single log entry
type LogEntry struct {
	Timestamp time.Time
	Level     string
	Message   string
}

// LogViewer manages log display in the TUI
type LogViewer struct {
	viewport viewport.Model
	logs     []LogEntry
	mutex    sync.RWMutex
	styles   LogStyles
}

// LogStyles contains styling for different log levels
type LogStyles struct {
	info  lipgloss.Style
	warn  lipgloss.Style
	error lipgloss.Style
	debug lipgloss.Style
}

// NewLogViewer creates a new log viewer
func NewLogViewer(width, height int) *LogViewer {
	vp := viewport.New(width, height)
	vp.YPosition = 0

	styles := LogStyles{
		info:  lipgloss.NewStyle().Foreground(lipgloss.Color("86")),  // Green
		warn:  lipgloss.NewStyle().Foreground(lipgloss.Color("214")), // Orange
		error: lipgloss.NewStyle().Foreground(lipgloss.Color("196")), // Red
		debug: lipgloss.NewStyle().Foreground(lipgloss.Color("241")), // Gray
	}

	return &LogViewer{
		viewport: vp,
		logs:     make([]LogEntry, 0),
		styles:   styles,
	}
}

// AddLog adds a new log entry
func (lv *LogViewer) AddLog(level, message string) {
	lv.mutex.Lock()
	defer lv.mutex.Unlock()

	entry := LogEntry{
		Timestamp: time.Now(),
		Level:     level,
		Message:   message,
	}

	lv.logs = append(lv.logs, entry)

	// Keep only last 1000 entries to prevent memory issues
	if len(lv.logs) > 1000 {
		lv.logs = lv.logs[len(lv.logs)-1000:]
	}

	lv.updateContent()
}

// updateContent refreshes the viewport content
func (lv *LogViewer) updateContent() {
	var content strings.Builder

	for _, entry := range lv.logs {
		timestamp := entry.Timestamp.Format("15:04:05")
		
		var styledLine string
		switch entry.Level {
		case "ERROR":
			styledLine = lv.styles.error.Render(timestamp + " ERROR: " + entry.Message)
		case "WARN":
			styledLine = lv.styles.warn.Render(timestamp + " WARN:  " + entry.Message)
		case "DEBUG":
			styledLine = lv.styles.debug.Render(timestamp + " DEBUG: " + entry.Message)
		default:
			styledLine = lv.styles.info.Render(timestamp + " INFO:  " + entry.Message)
		}

		content.WriteString(styledLine + "\n")
	}

	lv.viewport.SetContent(content.String())
	lv.viewport.GotoBottom()
}

// Init implements tea.Model
func (lv *LogViewer) Init() tea.Cmd {
	return nil
}

// Update handles viewport updates
func (lv *LogViewer) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	lv.viewport, cmd = lv.viewport.Update(msg)
	return lv, cmd
}

// View renders the log viewer
func (lv *LogViewer) View() string {
	lv.mutex.RLock()
	defer lv.mutex.RUnlock()
	return lv.viewport.View()
}

// SetSize updates the viewport size
func (lv *LogViewer) SetSize(width, height int) {
	lv.viewport.Width = width
	lv.viewport.Height = height
}

// GetLogCount returns the number of logs
func (lv *LogViewer) GetLogCount() int {
	lv.mutex.RLock()
	defer lv.mutex.RUnlock()
	return len(lv.logs)
}