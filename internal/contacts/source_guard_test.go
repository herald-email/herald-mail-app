package contacts

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDarwinImportUsesContactsFrameworkAPIOnly(t *testing.T) {
	dir := packageDir(t)
	darwinSource := readPackageFile(t, dir, "macos.go")

	forbidden := []string{
		"osascript",
		"tell application \"Contacts\"",
		"AddressBook",
		"ABAddressBook",
	}
	for _, token := range forbidden {
		if strings.Contains(darwinSource, token) {
			t.Fatalf("darwin contact import source contains forbidden token %q", token)
		}
	}

	required := []string{
		"Contacts.framework",
		"CNContactStore",
		"CNContactFetchRequest",
	}
	for _, token := range required {
		if !strings.Contains(darwinSource, token) {
			t.Fatalf("darwin contact import source is missing required token %q", token)
		}
	}
}

func packageDir(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("get working directory: %v", err)
	}
	return wd
}

func readPackageFile(t *testing.T, dir, name string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	return string(data)
}
