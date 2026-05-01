package app

import (
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/herald-email/herald-mail-app/internal/backend"
	"github.com/herald-email/herald-mail-app/internal/filesafe"
	"github.com/herald-email/herald-mail-app/internal/logger"
	"github.com/herald-email/herald-mail-app/internal/models"
)

func expandTilde(path string) string {
	if strings.HasPrefix(path, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, path[2:])
		}
	}
	return path
}

// saveAttachmentCmd returns a tea.Cmd that writes attachment data to destPath.
func saveAttachmentCmd(b backend.Backend, att *models.Attachment, destPath string) tea.Cmd {
	return func() tea.Msg {
		if err := b.SaveAttachment(att, destPath); err != nil {
			return AttachmentSavedMsg{Err: err}
		}
		return AttachmentSavedMsg{Filename: att.Filename, Path: destPath}
	}
}

func attachmentSaveCollision(path string) (suggested string, warning string, blocked bool) {
	suggested, exists, err := filesafe.SuggestIfExists(path)
	if err != nil {
		return path, fmt.Sprintf("Cannot check destination: %v", err), true
	}
	if !exists {
		return path, "", false
	}
	return suggested, fmt.Sprintf("%s already exists. Suggested: %s", path, suggested), true
}

// addAttachmentCmd reads a file from path and returns an AttachmentAddedMsg.
func addAttachmentCmd(path string) tea.Cmd {
	return func() tea.Msg {
		info, err := os.Stat(path)
		if err != nil {
			return AttachmentAddedMsg{Err: fmt.Errorf("cannot read %s: %w", path, err)}
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return AttachmentAddedMsg{Err: err}
		}
		return AttachmentAddedMsg{
			Attachment: models.ComposeAttachment{
				Path:     path,
				Filename: filepath.Base(path),
				Size:     info.Size(),
				Data:     data,
			},
		}
	}
}

// unsubscribeCmd attempts to unsubscribe via List-Unsubscribe header.
// RFC 8058: if List-Unsubscribe-Post is "List-Unsubscribe=One-Click" and an https URL exists,
// it does an HTTP POST. Otherwise it copies the URL or mailto address to the clipboard.
func unsubscribeCmd(body *models.EmailBody) tea.Cmd {
	return func() tea.Msg {
		raw := body.ListUnsubscribe
		if raw == "" {
			return UnsubscribeResultMsg{Err: fmt.Errorf("no List-Unsubscribe header")}
		}
		// Parse angle-bracket-delimited URIs: <https://...>, <http://...>, <mailto:...>
		var httpsURL, httpURL, mailtoAddr string
		parts := strings.Split(raw, ",")
		for _, part := range parts {
			part = strings.TrimSpace(part)
			if len(part) >= 2 && part[0] == '<' && part[len(part)-1] == '>' {
				part = part[1 : len(part)-1]
			}
			if strings.HasPrefix(part, "https://") && httpsURL == "" {
				httpsURL = part
			} else if strings.HasPrefix(part, "http://") && httpURL == "" {
				httpURL = part
			} else if strings.HasPrefix(part, "mailto:") && mailtoAddr == "" {
				mailtoAddr = part
			}
		}
		// One-click POST (RFC 8058)
		if body.ListUnsubscribePost == "List-Unsubscribe=One-Click" && httpsURL != "" {
			resp, err := http.Post(httpsURL, "application/x-www-form-urlencoded",
				strings.NewReader("List-Unsubscribe=One-Click"))
			if err != nil {
				return UnsubscribeResultMsg{Err: err}
			}
			resp.Body.Close()
			return UnsubscribeResultMsg{Method: "one-click", URL: httpsURL}
		}
		// Browser fallback: open HTTP/HTTPS URL in the system browser
		webURL := httpsURL
		if webURL == "" {
			webURL = httpURL
		}
		if webURL != "" {
			if err := openBrowserFn(webURL); err == nil {
				return UnsubscribeResultMsg{Method: "browser-opened", URL: webURL}
			}
			// fall through to clipboard on exec error
		}
		// Copy HTTPS URL to clipboard
		if httpsURL != "" {
			cmd := exec.Command("pbcopy")
			if runtime.GOOS == "linux" {
				if os.Getenv("WAYLAND_DISPLAY") != "" {
					cmd = exec.Command("wl-copy")
				} else {
					cmd = exec.Command("xclip", "-sel", "clip")
				}
			}
			cmd.Stdin = strings.NewReader(httpsURL)
			_ = cmd.Run()
			return UnsubscribeResultMsg{Method: "url-copied", URL: httpsURL}
		}
		// Copy mailto address to clipboard
		if mailtoAddr != "" {
			cmd := exec.Command("pbcopy")
			if runtime.GOOS == "linux" {
				if os.Getenv("WAYLAND_DISPLAY") != "" {
					cmd = exec.Command("wl-copy")
				} else {
					cmd = exec.Command("xclip", "-sel", "clip")
				}
			}
			cmd.Stdin = strings.NewReader(mailtoAddr)
			_ = cmd.Run()
			return UnsubscribeResultMsg{Method: "mailto-copied", URL: mailtoAddr}
		}
		return UnsubscribeResultMsg{Err: fmt.Errorf("no usable unsubscribe URI found")}
	}
}

// createHideFutureMailCmd enables the Hide Future Mail sender rule.
func createHideFutureMailCmd(b backend.Backend, sender string) tea.Cmd {
	return func() tea.Msg {
		err := b.SoftUnsubscribeSender(sender, "")
		return SoftUnsubResultMsg{Sender: sender, Err: err}
	}
}

// markReadCmd fires and forgets — marks the email as read on IMAP and in cache.
func markReadCmd(b backend.Backend, messageID, folder string) tea.Cmd {
	return func() tea.Msg {
		if err := b.MarkRead(messageID, folder); err != nil {
			logger.Warn("markReadCmd failed for %s: %v", messageID, err)
		}
		return nil
	}
}

// markUnreadCmd fires and forgets; marks the email as unread on IMAP and in cache.
func markUnreadCmd(b backend.Backend, messageID, folder string) tea.Cmd {
	return func() tea.Msg {
		if err := b.MarkUnread(messageID, folder); err != nil {
			logger.Warn("markUnreadCmd failed for %s: %v", messageID, err)
		}
		return nil
	}
}

// toggleStarCmd toggles the \Flagged IMAP flag and returns a StarResultMsg.
func (m *Model) toggleStarCmd(email *models.EmailData) tea.Cmd {
	b := m.backend
	messageID := email.MessageID
	folder := email.Folder
	starred := !email.IsStarred
	return func() tea.Msg {
		var err error
		if starred {
			err = b.MarkStarred(messageID, folder)
		} else {
			err = b.UnmarkStarred(messageID, folder)
		}
		if err != nil {
			return StarResultMsg{MessageID: messageID, Err: err}
		}
		return StarResultMsg{MessageID: messageID, Starred: starred}
	}
}

// cacheUnsubscribeHeadersCmd stores List-Unsubscribe headers in the cache.
func cacheUnsubscribeHeadersCmd(b backend.Backend, messageID, listUnsub, listUnsubPost string) tea.Cmd {
	return func() tea.Msg {
		if err := b.UpdateUnsubscribeHeaders(messageID, listUnsub, listUnsubPost); err != nil {
			logger.Warn("cacheUnsubscribeHeadersCmd failed for %s: %v", messageID, err)
		}
		return nil
	}
}

// copyToClipboard returns a tea.Cmd that writes text to the system clipboard.
// Tries pbcopy (macOS), wl-copy (Wayland), then xclip (X11). Failures are
// logged and silently dropped so the TUI keeps running.
func copyToClipboard(text string) tea.Cmd {
	return func() tea.Msg {
		var cmd *exec.Cmd
		switch runtime.GOOS {
		case "darwin":
			cmd = exec.Command("pbcopy")
		default:
			if os.Getenv("WAYLAND_DISPLAY") != "" {
				cmd = exec.Command("wl-copy")
			} else {
				cmd = exec.Command("xclip", "-sel", "clip")
			}
		}
		cmd.Stdin = strings.NewReader(text)
		if err := cmd.Run(); err != nil {
			logger.Warn("clipboard copy failed: %v", err)
		}
		return nil
	}
}
