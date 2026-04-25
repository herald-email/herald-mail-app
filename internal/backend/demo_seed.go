package backend

import (
	"mail-processor/internal/demo"
	"mail-processor/internal/models"
)

func seedDemoContacts() []models.ContactData {
	return demo.Contacts()
}

func seedDemoEmails() []*models.EmailData {
	return demo.Emails()
}
