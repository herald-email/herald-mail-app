package app

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/herald-email/herald-mail-app/internal/testmail"
)

func TestVirtualLabTimelinePreviewSurfacesAtTerminalSizes(t *testing.T) {
	cases := []struct {
		name        testmail.ScenarioName
		sizes       []struct{ width, height int }
		want        []string
		forbidShown []string
		forbidRaw   []string
	}{
		{
			name: testmail.ScenarioCalendlyInvite,
			sizes: []struct{ width, height int }{
				{width: 120, height: 40},
				{width: 80, height: 24},
			},
			want: []string{
				"Product review",
				"Join meeting",
			},
		},
		{
			name: testmail.ScenarioNewsletterTable,
			sizes: []struct{ width, height int }{
				{width: 80, height: 24},
			},
			want: []string{
				"Weekly systems digest",
				"table-heavy newsletter",
				"Read in browser",
			},
		},
		{
			name: testmail.ScenarioInlineCIDImage,
			sizes: []struct{ width, height int }{
				{width: 80, height: 24},
			},
			want: []string{
				"Inline fixture image follows",
			},
			forbidShown: []string{"cid:chart-001@herald.test"},
		},
		{
			name: testmail.ScenarioLongLinkTracking,
			sizes: []struct{ width, height int }{
				{width: 80, height: 24},
			},
			want: []string{
				"Open safe fixture link",
			},
			forbidShown: []string{
				"redacted-fixture-token",
				"links.herald.test/path/with/a/very/long",
			},
			forbidRaw: []string{"utm_", "fbclid", "gclid", "mc_cid", "mc_eid"},
		},
		{
			name: testmail.ScenarioMalformedCharset,
			sizes: []struct{ width, height int }{
				{width: 80, height: 24},
			},
			want: []string{
				"unknown charset",
				"fall back without blanking",
			},
		},
	}

	for _, tc := range cases {
		for _, size := range tc.sizes {
			t.Run(fmt.Sprintf("%s/%dx%d", tc.name, size.width, size.height), func(t *testing.T) {
				m := newVirtualLabTimelinePreviewModel(t, tc.name, size.width, size.height)
				rendered := viewContent(m.View())
				assertFitsWidth(t, size.width, rendered)
				assertFitsHeight(t, size.height, rendered)
				assertVirtualLabTimelineChrome(t, rendered)

				visible := ansi.Strip(rendered)
				normalizedVisible := normalizePreviewText(visible)
				for _, want := range tc.want {
					if !strings.Contains(normalizedVisible, normalizePreviewText(want)) {
						t.Fatalf("%s %dx%d view missing %q:\n%s", tc.name, size.width, size.height, want, visible)
					}
				}
				for _, bad := range tc.forbidShown {
					if strings.Contains(normalizedVisible, normalizePreviewText(bad)) {
						t.Fatalf("%s %dx%d view visibly leaked %q:\n%s", tc.name, size.width, size.height, bad, visible)
					}
				}
				for _, bad := range tc.forbidRaw {
					if strings.Contains(rendered, bad) {
						t.Fatalf("%s %dx%d view raw output leaked %q:\n%q", tc.name, size.width, size.height, bad, rendered)
					}
				}
			})
		}
	}
}

func TestVirtualLabTimelinePreviewMinimumSizeGuardAndRecovery(t *testing.T) {
	m := newVirtualLabTimelinePreviewModel(t, testmail.ScenarioCalendlyInvite, 80, 24)

	updated, _ := m.Update(tea.WindowSizeMsg{Width: 50, Height: 15})
	m = updated.(*Model)
	guard := viewContent(m.View())
	assertFitsWidth(t, 50, guard)
	guardVisible := ansi.Strip(guard)
	if !strings.Contains(guardVisible, "Terminal too narrow") {
		t.Fatalf("expected minimum-size guard at 50x15, got:\n%s", guardVisible)
	}
	if strings.Contains(guardVisible, "Product review") {
		t.Fatalf("minimum-size guard should replace the preview body, got:\n%s", guardVisible)
	}

	updated, _ = m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = updated.(*Model)
	recovered := viewContent(m.View())
	assertFitsWidth(t, 80, recovered)
	assertFitsHeight(t, 24, recovered)
	assertVirtualLabTimelineChrome(t, recovered)

	recoveredVisible := ansi.Strip(recovered)
	normalized := normalizePreviewText(recoveredVisible)
	if !strings.Contains(normalized, normalizePreviewText("Product review")) {
		t.Fatalf("expected preview to recover after resize, got:\n%s", recoveredVisible)
	}
	if strings.Contains(recoveredVisible, "Terminal too narrow") {
		t.Fatalf("minimum-size guard should clear after resize, got:\n%s", recoveredVisible)
	}
}

func assertVirtualLabTimelineChrome(t *testing.T, rendered string) {
	t.Helper()
	visible := ansi.Strip(rendered)
	for _, want := range []string{"Herald", "Timeline", "INBOX", "?: help"} {
		if !strings.Contains(visible, want) {
			t.Fatalf("expected virtual-lab Timeline surface to include %q, got:\n%s", want, visible)
		}
	}
}
