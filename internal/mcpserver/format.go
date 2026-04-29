package mcpserver

import "mail-processor/internal/models"

func mcpMessageIDRef(email *models.EmailData) string {
	if email == nil || email.MessageID == "" {
		return "message_id=(missing)"
	}
	return "message_id=" + email.MessageID
}
