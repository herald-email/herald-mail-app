package backend

import (
	"github.com/herald-email/herald-mail-app/internal/demo"
	"github.com/herald-email/herald-mail-app/internal/models"
)

func seedDemoContacts() []models.ContactData {
	return demo.Contacts()
}

func seedDemoEmails() []*models.EmailData {
	return demo.Emails()
}
