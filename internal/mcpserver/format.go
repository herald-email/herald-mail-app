package mcpserver

import (
	"fmt"

	"github.com/herald-email/herald-mail-app/internal/models"
)

func mcpMessageIDRef(email *models.EmailData) string {
	if email == nil || email.MessageID == "" {
		return "message_id=(missing)"
	}
	ref := email.MessageRef()
	return fmt.Sprintf("message_id=%s source_id=%s account_id=%s local_id=%s", ref.MessageID, ref.SourceID, ref.AccountID, ref.LocalID)
}
