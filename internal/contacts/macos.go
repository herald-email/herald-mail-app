//go:build darwin

package contacts

import (
	"os/exec"

	"github.com/herald-email/herald-mail-app/internal/models"
)

// ImportFromAppleContacts runs an AppleScript to list all contacts from macOS Contacts.app.
// Returns a slice of ContactAddr with names and email addresses.
// Returns nil, nil if Contacts.app is not accessible or returns no data.
func ImportFromAppleContacts() ([]models.ContactAddr, error) {
	script := `
tell application "Contacts"
    set output to ""
    repeat with p in every person
        set pName to name of p
        set pEmails to emails of p
        if (count of pEmails) > 0 then
            repeat with e in pEmails
                set output to output & pName & "|" & (value of e) & "\n"
            end repeat
        end if
    end repeat
    return output
end tell`

	cmd := exec.Command("osascript", "-e", script)
	out, err := cmd.Output()
	if err != nil {
		// Silently ignored: Contacts.app may require user permission grant or be unavailable in CI
		return nil, nil
	}

	return parseAppleScriptOutput(string(out)), nil
}
