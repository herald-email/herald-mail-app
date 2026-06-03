//go:build darwin && !cgo

package contacts

import "github.com/herald-email/herald-mail-app/internal/models"

// ImportFromAppleContacts is disabled on macOS builds without cgo because
// Contacts.framework access requires the native Objective-C API bridge.
func ImportFromAppleContacts() ([]models.ContactAddr, error) {
	return nil, nil
}
