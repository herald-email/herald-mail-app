package contacts

import (
	"strings"

	"mail-processor/internal/models"
)

// parseAppleScriptOutput parses the "Name|email\n" output from the AppleScript.
func parseAppleScriptOutput(output string) []models.ContactAddr {
	var result []models.ContactAddr
	for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "|", 2)
		if len(parts) != 2 {
			continue
		}
		name := strings.TrimSpace(parts[0])
		email := strings.TrimSpace(parts[1])
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
