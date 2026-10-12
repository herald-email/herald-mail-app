package backend

import "testing"

func TestDemoMovePreservesMessageInDestination(t *testing.T) {
	b := NewDemoBackend()
	before, err := b.GetTimelineEmails("INBOX")
	if err != nil || len(before) == 0 {
		t.Fatal("missing demo mail")
	}
	email := *before[0]
	if err := b.MoveEmail(email.MessageID, email.Folder, "Social"); err != nil {
		t.Fatal(err)
	}
	inbox, _ := b.GetTimelineEmails("INBOX")
	for _, e := range inbox {
		if e.MessageID == email.MessageID {
			t.Fatal("moved message still in inbox")
		}
	}
	destination, _ := b.GetTimelineEmails("Social")
	found := false
	for _, e := range destination {
		if e.MessageID == email.MessageID {
			found = true
			if e.Subject != email.Subject {
				t.Fatal("move lost message content")
			}
		}
	}
	if !found {
		t.Fatal("move deleted message instead of delivering to destination")
	}
	if err := b.MoveEmail(email.MessageID, "Social", "INBOX"); err != nil {
		t.Fatal(err)
	}
	inbox, _ = b.GetTimelineEmails("INBOX")
	if len(inbox) != len(before) {
		t.Fatalf("move back count=%d want %d", len(inbox), len(before))
	}
}
