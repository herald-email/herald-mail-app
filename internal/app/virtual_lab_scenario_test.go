package app

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
	"github.com/herald-email/herald-mail-app/internal/cache"
	"github.com/herald-email/herald-mail-app/internal/imap"
	"github.com/herald-email/herald-mail-app/internal/models"
	"github.com/herald-email/herald-mail-app/internal/testmail"
)

func TestTimelinePreviewRendersVirtualMailScenarios(t *testing.T) {
	cases := []struct {
		name        testmail.ScenarioName
		want        []string
		forbidShown []string
		forbidRaw   []string
	}{
		{
			name: testmail.ScenarioCalendlyInvite,
			want: []string{
				"Product review",
				"Join meeting",
			},
		},
		{
			name: testmail.ScenarioNewsletterTable,
			want: []string{
				"Weekly systems digest",
				"table-heavy newsletter",
				"Read in browser",
			},
		},
		{
			name: testmail.ScenarioReceiptHTML,
			want: []string{
				"Item",
				"Fixture service",
				"$26.10",
				"View receipt",
			},
		},
		{
			name: testmail.ScenarioMalformedCharset,
			want: []string{
				"unknown charset",
				"fall back without blanking",
			},
		},
		{
			name: testmail.ScenarioInlineCIDImage,
			want: []string{
				"Inline fixture image follows",
			},
			forbidShown: []string{"cid:chart-001@herald.test"},
		},
		{
			name: testmail.ScenarioLongLinkTracking,
			want: []string{
				"Open safe fixture link",
			},
			forbidShown: []string{
				"redacted-fixture-token",
				"links.herald.test/path/with/a/very/long",
			},
			forbidRaw: []string{"utm_", "fbclid", "gclid", "mc_cid", "mc_eid"},
		},
	}

	for _, tc := range cases {
		t.Run(string(tc.name), func(t *testing.T) {
			email, body := fetchFirstScenarioPreviewBody(t, tc.name)
			m := makeSizedModel(t, 120, 40)
			m.activeTab = tabTimeline
			m.focusedPanel = panelPreview
			m.timeline.selectedEmail = email
			m.timeline.bodyMessageID = email.MessageID
			m.timeline.body = body

			rendered := m.renderEmailPreview()
			visible := ansi.Strip(rendered)
			normalizedVisible := normalizePreviewText(visible)
			for _, want := range tc.want {
				if !strings.Contains(normalizedVisible, normalizePreviewText(want)) {
					t.Fatalf("%s preview missing %q:\n%s", tc.name, want, visible)
				}
			}
			for _, bad := range tc.forbidShown {
				if strings.Contains(normalizedVisible, normalizePreviewText(bad)) {
					t.Fatalf("%s preview visibly leaked %q:\n%s", tc.name, bad, visible)
				}
			}
			for _, bad := range tc.forbidRaw {
				if strings.Contains(rendered, bad) {
					t.Fatalf("%s preview raw output leaked %q:\n%q", tc.name, bad, rendered)
				}
			}
		})
	}
}

func fetchFirstScenarioPreviewBody(t *testing.T, name testmail.ScenarioName) (*models.EmailData, *models.EmailBody) {
	t.Helper()
	seeded := testmail.StartScenario(t, name)
	var selected testmail.ScenarioMessage
	for _, msg := range seeded.Messages {
		if msg.Account == testmail.DefaultAliceAddress && msg.Folder == "INBOX" {
			selected = msg
			break
		}
	}
	if selected.Key == "" {
		t.Fatalf("scenario %q has no Alice INBOX message", name)
	}
	ref := seeded.Refs[selected.Key]
	alice := seeded.Lab.Account(testmail.DefaultAliceAddress)
	c, err := cache.New(filepath.Join(t.TempDir(), "preview-cache.db"))
	if err != nil {
		t.Fatalf("cache.New: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })
	client := imap.New(alice.Config(filepath.Join(t.TempDir(), "imap-cache.db")), "", c, make(chan models.ProgressInfo, 8))
	if err := client.Connect(); err != nil {
		t.Fatalf("connect virtual IMAP: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })
	body, err := client.FetchEmailBody(ref.UID, ref.Folder)
	if err != nil {
		t.Fatalf("FetchEmailBody(%s/%d): %v", ref.Folder, ref.UID, err)
	}
	return &models.EmailData{
		MessageID: ref.MessageID,
		UID:       ref.UID,
		Sender:    body.From,
		Subject:   body.Subject,
		Date:      time.Date(2026, 5, 20, 10, 0, 0, 0, time.UTC),
		Folder:    ref.Folder,
	}, body
}

func normalizePreviewText(s string) string {
	s = strings.NewReplacer(
		"│", " ",
		"┌", " ",
		"┐", " ",
		"└", " ",
		"┘", " ",
		"─", " ",
		"┼", " ",
	).Replace(s)
	return strings.ToLower(strings.Join(strings.Fields(s), " "))
}
