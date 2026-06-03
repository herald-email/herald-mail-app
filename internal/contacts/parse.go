package contacts

import (
	"encoding/json"
	"strings"

	"github.com/herald-email/herald-mail-app/internal/models"
)

type importedContactRow struct {
	Name  string `json:"name"`
	Email string `json:"email"`
}

// parseContactsJSON parses contacts returned by the native macOS API bridge.
func parseContactsJSON(output string) []models.ContactAddr {
	output = strings.TrimSpace(output)
	if output == "" {
		return nil
	}
	var rows []importedContactRow
	if err := json.Unmarshal([]byte(output), &rows); err != nil {
		return nil
	}
	var result []models.ContactAddr
	for _, row := range rows {
		name := strings.TrimSpace(row.Name)
		email := strings.TrimSpace(row.Email)
		if email == "" {
			continue
		}
		result = append(result, models.ContactAddr{
			Name:  name,
			Email: email,
		})
	}
	return result
}
