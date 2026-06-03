package app

import (
	"os"
	"os/exec"
	"strings"

	tea "charm.land/bubbletea/v2"
)

const composeExternalEditorShortcut = "ctrl+x"

type composeExternalEditorFinishedMsg struct {
	Body string
	Err  error
}

func composeExternalEditorCommand() (string, []string) {
	for _, env := range []string{"VISUAL", "EDITOR"} {
		if editor := strings.TrimSpace(os.Getenv(env)); editor != "" {
			parts := strings.Fields(editor)
			if len(parts) > 0 {
				return parts[0], parts[1:]
			}
		}
	}
	return "nano", nil
}

func (m *Model) openComposeExternalEditorCmd() tea.Cmd {
	body := m.composeBody.Value()
	editor, args := composeExternalEditorCommand()
	tmpFile, err := os.CreateTemp("", "herald-compose-*.md")
	if err != nil {
		return func() tea.Msg {
			return composeExternalEditorFinishedMsg{Err: err}
		}
	}

	path := tmpFile.Name()
	if _, err = tmpFile.WriteString(body); err == nil {
		err = tmpFile.Close()
	} else {
		_ = tmpFile.Close()
	}
	if err != nil {
		_ = os.Remove(path)
		return func() tea.Msg {
			return composeExternalEditorFinishedMsg{Err: err}
		}
	}

	cmd := exec.Command(editor, append(args, path)...)
	return tea.ExecProcess(cmd, func(runErr error) tea.Msg {
		if runErr != nil {
			_ = os.Remove(path)
			return composeExternalEditorFinishedMsg{Err: runErr}
		}
		content, readErr := os.ReadFile(path)
		_ = os.Remove(path)
		if readErr != nil {
			return composeExternalEditorFinishedMsg{Err: readErr}
		}
		return composeExternalEditorFinishedMsg{Body: string(content)}
	})
}

func (m *Model) applyComposeExternalEditorResult(msg composeExternalEditorFinishedMsg) {
	if msg.Err != nil {
		m.composeStatus = "Editor failed: " + msg.Err.Error()
		return
	}
	m.composeBody.SetValue(msg.Body)
	m.composeField = composeFieldBody
	m.composeBody.Focus()
	m.composeAIInput.Blur()
	m.composeAIResponse.Blur()
	m.composeStatus = "Compose body updated from editor"
	m.refreshComposeLayout()
}
