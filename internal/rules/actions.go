package rules

import (
	"fmt"
	"net/http"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"mail-processor/internal/models"
)

// notify sends a desktop notification.
// On macOS uses osascript; on Linux uses notify-send; on other OS is a no-op.
func notify(title, body string) error {
	switch runtime.GOOS {
	case "darwin":
		script := fmt.Sprintf(`display notification %q with title %q`, body, title)
		return exec.Command("osascript", "-e", script).Run()
	case "linux":
		return exec.Command("notify-send", title, body).Run()
	default:
		return nil
	}
}

// webhook sends an HTTP POST to url. bodyTmpl is a Go template rendered with ctx.
// Uses a 10-second timeout.
func webhook(url, bodyTmpl string, headers map[string]string, ctx models.RuleContext) error {
	body := renderTemplate(bodyTmpl, ctx)
	req, err := http.NewRequest("POST", url, strings.NewReader(body))
	if err != nil {
		return fmt.Errorf("webhook: create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("webhook: send: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return fmt.Errorf("webhook: server returned %d", resp.StatusCode)
	}
	return nil
}

// runCommand executes a shell command with HERALD_* env vars set from ctx.
func runCommand(cmd string, ctx models.RuleContext) error {
	c := exec.Command("sh", "-c", cmd)
	c.Env = append(c.Environ(),
		"HERALD_SENDER="+ctx.Sender,
		"HERALD_DOMAIN="+ctx.Domain,
		"HERALD_SUBJECT="+ctx.Subject,
		"HERALD_CATEGORY="+ctx.Category,
		"HERALD_MESSAGE_ID="+ctx.MessageID,
		"HERALD_FOLDER="+ctx.Folder,
		"HERALD_PROMPT_RESULT="+ctx.PromptResult,
	)
	if out, err := c.CombinedOutput(); err != nil {
		return fmt.Errorf("run command: %w: %s", err, string(out))
	}
	return nil
}
