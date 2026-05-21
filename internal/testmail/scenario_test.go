package testmail

import "testing"

func TestLoadScenarioParsesFixtureMetadata(t *testing.T) {
	names := []ScenarioName{
		ScenarioPlainThread,
		ScenarioCalendlyInvite,
		ScenarioNewsletterTable,
		ScenarioReceiptHTML,
		ScenarioMalformedCharset,
		ScenarioInlineCIDImage,
		ScenarioLongLinkTracking,
		ScenarioUnsubscribeHeaders,
	}

	for _, name := range names {
		t.Run(string(name), func(t *testing.T) {
			scenario, err := LoadScenario(name)
			if err != nil {
				t.Fatalf("LoadScenario(%q): %v", name, err)
			}
			if scenario.Name != name {
				t.Fatalf("scenario name = %q, want %q", scenario.Name, name)
			}
			if len(scenario.Messages) == 0 {
				t.Fatalf("scenario %q has no messages", name)
			}
			for _, msg := range scenario.Messages {
				if msg.Key == "" || msg.File == "" || msg.Account == "" || msg.Folder == "" {
					t.Fatalf("scenario %q has incomplete placement: %+v", name, msg)
				}
				if msg.Subject == "" || msg.MessageID == "" {
					t.Fatalf("scenario %q did not parse headers for %s: %+v", name, msg.File, msg)
				}
				if len(msg.Data) == 0 {
					t.Fatalf("scenario %q message %s has no data", name, msg.File)
				}
			}
		})
	}
}

func TestStartScenarioSeedsUnsubscribeHeaders(t *testing.T) {
	seeded := StartScenario(t, ScenarioUnsubscribeHeaders)
	want := map[string]struct {
		subject string
	}{
		"one-click": {subject: "Unsubscribe fixture one-click"},
		"mailto":    {subject: "Unsubscribe fixture mailto"},
		"no-header": {subject: "Unsubscribe fixture no header"},
	}

	if len(seeded.Refs) != len(want) {
		t.Fatalf("seeded refs = %d, want %d: %#v", len(seeded.Refs), len(want), seeded.Refs)
	}
	for key, expected := range want {
		ref, ok := seeded.Refs[key]
		if !ok {
			t.Fatalf("missing ref %q in %#v", key, seeded.Refs)
		}
		if ref.Account != DefaultAliceAddress || ref.Folder != "INBOX" || ref.UID == 0 {
			t.Fatalf("ref %q = %+v, want Alice INBOX nonzero UID", key, ref)
		}
		got := seeded.Lab.WaitForSubject(DefaultAliceAddress, "INBOX", expected.subject)
		if got.MessageID != ref.MessageID {
			t.Fatalf("ref %q message ID = %q, WaitForSubject returned %q", key, ref.MessageID, got.MessageID)
		}
	}
}

func TestStartScenarioSeedsPlainThreadAcrossTwoAccounts(t *testing.T) {
	seeded := StartScenario(t, ScenarioPlainThread)
	want := map[string]struct {
		account string
		folder  string
		subject string
	}{
		"original":    {account: DefaultAliceAddress, folder: "INBOX", subject: "Plain thread kickoff"},
		"reply-sent":  {account: DefaultAliceAddress, folder: "Sent", subject: "Re: Plain thread kickoff"},
		"reply-inbox": {account: DefaultBobAddress, folder: "INBOX", subject: "Re: Plain thread kickoff"},
	}

	if seeded.Name != ScenarioPlainThread {
		t.Fatalf("seeded name = %q, want %q", seeded.Name, ScenarioPlainThread)
	}
	if len(seeded.Refs) != len(want) {
		t.Fatalf("seeded refs = %d, want %d: %#v", len(seeded.Refs), len(want), seeded.Refs)
	}
	for key, expected := range want {
		ref, ok := seeded.Refs[key]
		if !ok {
			t.Fatalf("missing ref %q in %#v", key, seeded.Refs)
		}
		if ref.Account != expected.account || ref.Folder != expected.folder || ref.UID == 0 {
			t.Fatalf("ref %q = %+v, want account=%s folder=%s nonzero UID", key, ref, expected.account, expected.folder)
		}
		got := seeded.Lab.WaitForSubject(expected.account, expected.folder, expected.subject)
		if got.MessageID != ref.MessageID {
			t.Fatalf("ref %q message ID = %q, WaitForSubject returned %q", key, ref.MessageID, got.MessageID)
		}
	}
}
