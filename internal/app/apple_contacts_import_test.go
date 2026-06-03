package app

import (
	"errors"
	"testing"

	"github.com/herald-email/herald-mail-app/internal/models"
)

type appleContactsRecordingBackend struct {
	stubBackend
	upserted  []models.ContactAddr
	direction string
}

func (b *appleContactsRecordingBackend) UpsertContacts(addrs []models.ContactAddr, direction string) error {
	b.upserted = append([]models.ContactAddr(nil), addrs...)
	b.direction = direction
	return nil
}

func TestImportAppleContactsSkipsSystemImporterInDemoMode(t *testing.T) {
	oldImporter := importAppleContactsFromSystem
	defer func() { importAppleContactsFromSystem = oldImporter }()

	called := false
	importAppleContactsFromSystem = func() ([]models.ContactAddr, error) {
		called = true
		return []models.ContactAddr{{Name: "Real Contact", Email: "real@example.com"}}, nil
	}

	backend := &appleContactsRecordingBackend{}
	m := New(backend, nil, "demo@demo.local", nil, false)
	m.demoMode = true

	msg := m.importAppleContacts()()
	imported, ok := msg.(AppleContactsImportedMsg)
	if !ok {
		t.Fatalf("message type = %T, want AppleContactsImportedMsg", msg)
	}
	if imported.Count != 0 {
		t.Fatalf("imported count = %d, want 0 in demo mode", imported.Count)
	}
	if called {
		t.Fatal("demo mode called the real Apple Contacts importer")
	}
	if len(backend.upserted) != 0 {
		t.Fatalf("demo mode upserted contacts: %+v", backend.upserted)
	}
}

func TestImportAppleContactsUpsertsSystemContactsInNormalMode(t *testing.T) {
	oldImporter := importAppleContactsFromSystem
	defer func() { importAppleContactsFromSystem = oldImporter }()

	importAppleContactsFromSystem = func() ([]models.ContactAddr, error) {
		return []models.ContactAddr{
			{Name: "Alex Api", Email: "alex@example.com"},
			{Name: "Casey Framework", Email: "casey@example.com"},
		}, nil
	}

	backend := &appleContactsRecordingBackend{}
	m := New(backend, nil, "user@example.com", nil, false)

	msg := m.importAppleContacts()()
	imported, ok := msg.(AppleContactsImportedMsg)
	if !ok {
		t.Fatalf("message type = %T, want AppleContactsImportedMsg", msg)
	}
	if imported.Count != 2 {
		t.Fatalf("imported count = %d, want 2", imported.Count)
	}
	if backend.direction != "from" {
		t.Fatalf("upsert direction = %q, want from", backend.direction)
	}
	if got := len(backend.upserted); got != 2 {
		t.Fatalf("upserted contacts = %d, want 2", got)
	}
	if backend.upserted[0].Email != "alex@example.com" {
		t.Fatalf("first upserted contact = %+v", backend.upserted[0])
	}
}

func TestImportAppleContactsImporterErrorIsNonFatal(t *testing.T) {
	oldImporter := importAppleContactsFromSystem
	defer func() { importAppleContactsFromSystem = oldImporter }()

	importAppleContactsFromSystem = func() ([]models.ContactAddr, error) {
		return nil, errors.New("contacts access denied")
	}

	backend := &appleContactsRecordingBackend{}
	m := New(backend, nil, "user@example.com", nil, false)

	msg := m.importAppleContacts()()
	imported, ok := msg.(AppleContactsImportedMsg)
	if !ok {
		t.Fatalf("message type = %T, want AppleContactsImportedMsg", msg)
	}
	if imported.Count != 0 {
		t.Fatalf("imported count = %d, want 0 on importer error", imported.Count)
	}
	if len(backend.upserted) != 0 {
		t.Fatalf("upserted contacts despite importer error: %+v", backend.upserted)
	}
}
