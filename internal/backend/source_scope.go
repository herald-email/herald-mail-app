package backend

import "github.com/herald-email/herald-mail-app/internal/models"

func withDefaultFolderSyncScope(event models.FolderSyncEvent) models.FolderSyncEvent {
	event.SourceID = models.NormalizeSourceID(event.SourceID, models.DefaultMailSourceID)
	event.AccountID = models.NormalizeAccountID(event.AccountID)
	return event
}

func withDefaultNewEmailsScope(notification models.NewEmailsNotification) models.NewEmailsNotification {
	notification.SourceID = models.NormalizeSourceID(notification.SourceID, models.DefaultMailSourceID)
	notification.AccountID = models.NormalizeAccountID(notification.AccountID)
	for _, email := range notification.Emails {
		if email == nil {
			continue
		}
		if email.SourceID == "" {
			email.SourceID = notification.SourceID
		}
		if email.AccountID == "" {
			email.AccountID = notification.AccountID
		}
		ref := email.MessageRef()
		email.SourceID = ref.SourceID
		email.AccountID = ref.AccountID
		email.LocalID = ref.LocalID
		email.UIDValidity = ref.UIDValidity
	}
	return notification
}
