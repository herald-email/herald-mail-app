package demo

import (
	"context"
	"strings"
	"time"

	"mail-processor/internal/ai"
)

// AI is a deterministic offline AI client for demo mode.
type AI struct {
	embeddingModel string
}

var _ ai.AIClient = (*AI)(nil)

// NewAI returns an offline deterministic AI client for demo mode.
func NewAI() *AI {
	return &AI{embeddingModel: "demo-topic-vectors"}
}

func (d *AI) Chat(messages []ai.ChatMessage) (string, error) {
	last := ""
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == "user" {
			last = strings.TrimSpace(messages[i].Content)
			break
		}
	}
	if last == "" {
		return "Demo AI is ready. Ask about cleanup, priority mail, or search topics.", nil
	}
	return "Demo AI: I found the most relevant mailbox context for \"" + last + "\". Northstar Cloud and Forgepoint Labs are the highest-priority demo threads because they mention budget risk, infrastructure changes, and release review.", nil
}

func (d *AI) ChatWithTools(messages []ai.ChatMessage, tools []ai.Tool) (string, []ai.ToolCall, error) {
	reply, err := d.Chat(messages)
	return reply, nil, err
}

func (d *AI) Classify(sender, subject string) (ai.Category, error) {
	time.Sleep(15 * time.Millisecond)
	return CategoryFor(sender, subject), nil
}

func (d *AI) Embed(text string) ([]float32, error) {
	return VectorForText(text), nil
}

func (d *AI) SetEmbeddingModel(model string) {
	if strings.TrimSpace(model) != "" {
		d.embeddingModel = strings.TrimSpace(model)
	}
}

func (d *AI) GenerateQuickReplies(sender, subject, bodyPreview string) ([]string, error) {
	name := sender
	if idx := strings.Index(name, "<"); idx > 0 {
		name = strings.TrimSpace(name[:idx])
	}
	if fields := strings.Fields(name); len(fields) > 0 {
		name = fields[0]
	} else {
		name = "there"
	}
	return []string{
		"Thanks, " + name + ". I will review this today.",
		"Received. Please send any follow-up details when ready.",
		"Thanks for the heads up. I will take a look and circle back.",
	}, nil
}

func (d *AI) EnrichContact(email string, subjects []string) (string, []string, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	for _, contact := range Contacts() {
		if strings.ToLower(contact.Email) == email {
			return contact.Company, append([]string(nil), contact.Topics...), nil
		}
	}
	return "", []string{"demo contact"}, nil
}

func (d *AI) HasVisionModel() bool {
	return false
}

func (d *AI) DescribeImage(ctx context.Context, imageBytes []byte, mimeType string) (string, error) {
	return "Demo image placeholder with no external vision model required.", nil
}

func (d *AI) Ping() error {
	return nil
}
