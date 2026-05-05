package app

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/herald-email/herald-mail-app/internal/models"
)

// TestComposeCCBCCWidth_MatchesToField verifies that CC and BCC textinput
// fields are given the same width as the To field after a window resize.
// Regression test for the "tiny box" rendering bug where CC/BCC showed
// only a single character because their Width was never set.
func TestComposeCCBCCWidth_MatchesToField(t *testing.T) {
	b := &stubBackend{}
	m := New(b, nil, "", nil, false)
	// Simulate a window resize event — this triggers the width calculation
	// that sets composeTo.Width and composeSubject.Width.
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(*Model)

	if m.composeCC.Width() == 0 {
		t.Fatal("composeCC.Width is 0 — width was never set")
	}
	if m.composeCC.Width() != m.composeTo.Width() {
		t.Fatalf("CC width %d != To width %d", m.composeCC.Width(), m.composeTo.Width())
	}
	if m.composeBCC.Width() != m.composeTo.Width() {
		t.Fatalf("BCC width %d != To width %d", m.composeBCC.Width(), m.composeTo.Width())
	}
}

// TestComposeBodyHeight_FitsTerminal verifies that the compose body textarea
// height is calculated to leave room for all four header fields (To/CC/BCC/Subject),
// the divider, status line, and body borders — so the total compose view never
// overflows the terminal height and pushes the To: field off the top of the screen.
//
// Regression test for the overflow bug where composeBodyHeight used -10 (missing
// CC and BCC rows) instead of the correct -16, causing the To: field to be
// scrolled off the top in a 50-row terminal.
func TestComposeBodyHeight_FitsTerminal(t *testing.T) {
	for _, h := range []int{24, 40, 50, 80} {
		b := &stubBackend{}
		m := New(b, nil, "", nil, false)
		updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: h})
		m = updated.(*Model)

		// chrome = header(1) + tabbar(1) + statusbar(1) + divider(1) + wrapped keyhints(2) = 6
		// + panel border (2) = 8 total deduction
		tableHeight := h - 8

		// Fixed compose rows (excluding body content):
		//   To(3) + CC(3) + BCC(3) + Subject(3) = 12 field rows
		//   divider(1) + status(1) + body borders(2) = 4 overhead rows
		//   total fixed = 16
		const fixedRows = 16
		expectedBodyHeight := tableHeight - fixedRows
		if expectedBodyHeight < 3 {
			expectedBodyHeight = 3
		}

		got := m.composeBody.Height()
		if got != expectedBodyHeight {
			t.Errorf("h=%d: composeBody.Height()=%d, want %d (would overflow terminal by %d rows)",
				h, got, expectedBodyHeight, got-expectedBodyHeight)
		}
	}
}

// TestComposeAlt3_SwitchesToContacts verifies that Alt+3 switches from Compose
// to Contacts while plain "3" remains available as draft text.
//
// Regression test for the compose-safe command layer: global tab switching uses
// Alt chords when a Compose text field is focused.
func TestComposeAlt3_SwitchesToContacts(t *testing.T) {
	b := &stubBackend{}
	m := New(b, nil, "", nil, false)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(*Model)
	m.loading = false
	m.activeTab = tabCompose

	updated2, _ := m.Update(altKey('3'))
	m2 := updated2.(*Model)

	if m2.activeTab != tabContacts {
		t.Fatalf("pressing alt+3 in compose: activeTab=%d, want %d (tabContacts)", m2.activeTab, tabContacts)
	}
}

func TestComposeAutocomplete_DoesNotPushChromeOffscreen(t *testing.T) {
	contacts := []models.ContactData{
		{DisplayName: "Rowan Finch", Email: "rowan@protonmail.com"},
		{DisplayName: "Rowan Finch", Email: "rowan@proton.me"},
		{DisplayName: "Rowan Finch", Email: "rowan.finch@protonmail.com"},
		{DisplayName: "Rowan from Manager.dev", Email: "managerdotdev@mail.beehiiv.com"},
		{DisplayName: "Rowan Finch", Email: "rowan@pm.me"},
	}

	lineCount := func(rendered string) int {
		stripped := strings.TrimRight(stripANSI(rendered), "\n")
		if stripped == "" {
			return 0
		}
		return len(strings.Split(stripped, "\n"))
	}

	for _, tc := range []struct {
		name   string
		width  int
		height int
	}{
		{name: "wide", width: 220, height: 50},
		{name: "standard", width: 80, height: 24},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := makeSizedModel(t, tc.width, tc.height)
			m.activeTab = tabCompose
			m.composeField = 0
			m.composeTo.SetValue("rowan")
			updated, _ := m.Update(ContactSuggestionsMsg{Contacts: contacts})
			m = updated.(*Model)
			freezeComposeCursors(m)

			rendered := m.renderMainView()
			if got := lineCount(rendered); got > tc.height {
				t.Fatalf("compose autocomplete rendered %d lines at %dx%d, exceeding viewport height\n%s", got, tc.width, tc.height, stripANSI(rendered))
			}

			stripped := stripANSI(rendered)
			if !strings.Contains(stripped, "Herald") {
				t.Fatalf("expected compose chrome to remain visible, got:\n%s", stripped)
			}
			if !strings.Contains(stripped, "To:") {
				t.Fatalf("expected active To field to remain visible, got:\n%s", stripped)
			}
		})
	}
}
