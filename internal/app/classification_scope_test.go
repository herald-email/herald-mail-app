package app

import (
	"testing"
	"time"

	"github.com/herald-email/herald-mail-app/internal/models"
)

type scopedClassificationBackend struct {
	stubBackend
	ref      models.MessageRef
	category string
}

func (b *scopedClassificationBackend) SetClassificationByRef(ref models.MessageRef, category string) error {
	b.ref = ref
	b.category = category
	return nil
}

func TestClassifyProgressMsgUsesScopedClassificationKey(t *testing.T) {
	email := &models.EmailData{
		SourceID:  "work-mail",
		AccountID: "work",
		MessageID: "duplicate-id",
		Sender:    "alice@example.com",
		Subject:   "Scoped",
		Date:      time.Now(),
		Folder:    "INBOX",
	}
	ref := email.MessageRef()
	m := makeReclassifyModel(&stubClassifier{category: "news"})
	m.timeline.emails = []*models.EmailData{email}

	result, _ := m.Update(ClassifyProgressMsg{
		MessageRef: ref,
		MessageID:  ref.MessageID,
		Category:   "news",
		Done:       1,
		Total:      1,
	})
	updated := result.(*Model)

	if updated.classifications[ref.LocalID] != "news" {
		t.Fatalf("scoped classification = %q, want news", updated.classifications[ref.LocalID])
	}
	if updated.classifications[ref.MessageID] != "news" {
		t.Fatalf("legacy classification = %q, want news", updated.classifications[ref.MessageID])
	}
}

func TestReclassifyEmailCmdStoresClassificationByRef(t *testing.T) {
	email := &models.EmailData{
		SourceID:  "work-mail",
		AccountID: "work",
		MessageID: "msg-42",
		Sender:    "boss@corp.com",
		Subject:   "Urgent: Q4 review",
		Folder:    "INBOX",
	}
	backend := &scopedClassificationBackend{}
	m := makeReclassifyModel(&stubClassifier{category: "imp"})
	m.backend = backend

	msg := m.reclassifyEmailCmd(email)().(ReclassifyResultMsg)
	if msg.Err != nil {
		t.Fatalf("unexpected error: %v", msg.Err)
	}
	want := email.MessageRef()
	if msg.MessageRef != want {
		t.Fatalf("MessageRef = %#v, want %#v", msg.MessageRef, want)
	}
	if backend.ref != want || backend.category != "imp" {
		t.Fatalf("backend stored ref/category = %#v/%q, want %#v/imp", backend.ref, backend.category, want)
	}
}
