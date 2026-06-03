package models

import "testing"

func TestMessageRefFromLegacyEmailUsesDefaultScope(t *testing.T) {
	email := EmailData{
		MessageID: "<abc@example.com>",
		UID:       42,
		Folder:    "INBOX",
	}

	ref := email.MessageRef()

	if ref.SourceID != DefaultMailSourceID {
		t.Fatalf("SourceID = %q, want %q", ref.SourceID, DefaultMailSourceID)
	}
	if ref.AccountID != DefaultAccountID {
		t.Fatalf("AccountID = %q, want %q", ref.AccountID, DefaultAccountID)
	}
	if ref.Folder != "INBOX" || ref.UID != 42 || ref.MessageID != "<abc@example.com>" {
		t.Fatalf("ref = %#v, want legacy folder/uid/message id preserved", ref)
	}
	if ref.LocalID == "" {
		t.Fatal("LocalID should be populated for scoped cache lookups")
	}
}

func TestMessageRefUsesExplicitEmailScope(t *testing.T) {
	email := EmailData{
		SourceID:  SourceID("work-mail"),
		AccountID: AccountID("work"),
		LocalID:   "mail:work-mail:work:INBOX:<abc@example.com>",
		MessageID: "<abc@example.com>",
		Folder:    "INBOX",
	}

	ref := email.MessageRef()

	if ref.SourceID != SourceID("work-mail") || ref.AccountID != AccountID("work") {
		t.Fatalf("ref scope = %q/%q, want work-mail/work", ref.SourceID, ref.AccountID)
	}
	if ref.LocalID != email.LocalID {
		t.Fatalf("LocalID = %q, want %q", ref.LocalID, email.LocalID)
	}
}

func TestCollectionRefCacheKeyIncludesScope(t *testing.T) {
	ref := CollectionRef{
		SourceID:     SourceID("work-mail"),
		AccountID:    AccountID("work"),
		Kind:         SourceKindMail,
		CollectionID: "INBOX",
	}

	if got, want := ref.CacheKey(), "mail:work-mail:work:INBOX"; got != want {
		t.Fatalf("CacheKey = %q, want %q", got, want)
	}
}

func TestEventRefFromLocalIDParsesCalendarScope(t *testing.T) {
	ref := EventRef{SourceID: "work-calendar", AccountID: "work", CalendarID: "primary", EventID: "planning", InstanceID: "instance-1"}.WithDefaults()

	got, ok := EventRefFromLocalID(ref.LocalID)
	if !ok {
		t.Fatalf("EventRefFromLocalID(%q) did not parse", ref.LocalID)
	}
	if got.SourceID != ref.SourceID || got.AccountID != ref.AccountID || got.CalendarID != ref.CalendarID || got.EventID != ref.EventID || got.InstanceID != ref.InstanceID || got.LocalID != ref.LocalID {
		t.Fatalf("parsed ref = %#v, want %#v", got, ref)
	}
}
