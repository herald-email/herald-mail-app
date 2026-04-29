//go:build !darwin

package contacts

import "github.com/herald-email/herald-mail-app/internal/models"

// ImportFromAppleContacts is a no-op on non-macOS platforms.
func ImportFromAppleContacts() ([]models.ContactAddr, error) {
	return nil, nil
}
