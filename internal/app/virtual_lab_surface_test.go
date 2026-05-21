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

func TestVirtualLabInlineCIDFullScreenImageModes(t *testing.T) {
	cases := []struct {
		name          string
		mode          PreviewImageMode
		wantVisible   []string
		forbidVisible []string
		wantRaw       []string
		forbidRaw     []string
	}{
		{
			name:        "local links",
			mode:        PreviewImageModeLinks,
			wantVisible: []string{"Inline fixture image follows", "fixture chart", "open image 1"},
			forbidVisible: []string{
				"cid:chart-001@herald.test",
			},
			wantRaw: []string{"\x1b]8;;http://127.0.0.1:"},
			forbidRaw: []string{
				"\x1b]1337;File=",
			},
		},
		{
			name:        "placeholder",
			mode:        PreviewImageModePlaceholder,
			wantVisible: []string{"Inline fixture image follows", "Image: fixture chart"},
			forbidVisible: []string{
				"open image",
				"127.0.0.1",
				"cid:chart-001@herald.test",
			},
			forbidRaw: []string{
				"\x1b]1337;File=",
				"\x1b]8;;http://127.0.0.1:",
			},
		},
		{
			name:        "forced iterm2",
			mode:        PreviewImageModeIterm2,
			wantVisible: []string{"Inline fixture image follows"},
			forbidVisible: []string{
				"open image",
				"127.0.0.1",
			},
			wantRaw: []string{"\x1b]1337;File="},
			forbidRaw: []string{
				"\x1b]8;;http://127.0.0.1:",
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := newVirtualLabTimelinePreviewModel(t, testmail.ScenarioInlineCIDImage, 120, 40)
			m.SetPreviewImageMode(tc.mode)
			m.timeline.fullScreen = true
			m.timeline.bodyScrollOffset = 0

			rendered := viewContent(m.View())
			visible := ansi.Strip(rendered)
			normalizedVisible := normalizePreviewText(visible)
			for _, want := range tc.wantVisible {
				if !strings.Contains(normalizedVisible, normalizePreviewText(want)) {
					t.Fatalf("%s full-screen preview missing visible %q:\n%s", tc.name, want, visible)
				}
			}
			for _, bad := range tc.forbidVisible {
				if strings.Contains(normalizedVisible, normalizePreviewText(bad)) {
					t.Fatalf("%s full-screen preview visibly leaked %q:\n%s", tc.name, bad, visible)
				}
			}
			for _, want := range tc.wantRaw {
				if !strings.Contains(rendered, want) {
					t.Fatalf("%s full-screen preview missing raw %q:\n%q", tc.name, want, rendered)
				}
			}
			for _, bad := range tc.forbidRaw {
				if strings.Contains(rendered, bad) {
					t.Fatalf("%s full-screen preview emitted forbidden raw %q:\n%q", tc.name, bad, rendered)
				}
			}
		})
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
