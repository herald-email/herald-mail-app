//go:build !darwin

package contacts

import "mail-processor/internal/models"

// ImportFromAppleContacts is a no-op on non-macOS platforms.
func ImportFromAppleContacts() ([]models.ContactAddr, error) {
	return nil, nil
}
