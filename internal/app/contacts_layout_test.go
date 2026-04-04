package app

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"mail-processor/internal/models"
)

// TestContactsTab_RightPanelFits verifies that the Contacts tab two-panel layout
// does not overflow the terminal width. Before the fix, rightW = width - leftW - 4
// was 2 chars too wide because each panel's outer visual width = Width(w) + 2 borders,
// so total = (leftW+2) + 2 (separator) + (rightW+2) = leftW+rightW+6, which with
// rightW = width-leftW-4 gives width+2 → 2 chars over the terminal edge.
//
// Fixed: rightW = width - leftW - 6.
func TestContactsTab_RightPanelFits(t *testing.T) {
	for _, w := range []int{80, 120, 160, 220} {
		b := &stubBackend{}
		m := New(b, nil, "", nil, false)
		m.loading = false
		m.activeTab = tabContacts
		m.contactsFiltered = []models.ContactData{
			{Email: "test@example.com", DisplayName: "Test User", Company: "Acme Corp", EmailCount: 5},
		}

		rendered := m.renderContactsTab(w, 40)
		lines := strings.Split(rendered, "\n")
		for i, line := range lines {
			// Count visible width (strip ANSI codes manually by checking rune count of non-escape chars).
			// A simpler proxy: if any line has more than w runes, it overflows.
			// We use a basic visible-rune counter that skips ANSI escape sequences.
			visible := visibleWidth(line)
			if visible > w {
				t.Errorf("w=%d: line %d visible width=%d exceeds terminal width %d: %q",
					w, i, visible, w, line)
			}
		}
	}
}

// TestContactsTab_RowsNoWrap verifies that each contact row in the left panel
// renders as exactly one line (not two) — i.e., the formatted line fits within
// the panel's inner content width and does not cause lipgloss to wrap it.
//
// Before the fix, maxNameW = leftW - 8 made the total line width (name + company + count)
// up to 90 chars, exceeding the panel inner width of leftW-1=76, causing wrapping.
//
// Fixed: maxNameW = leftW - 23 so the full line fits in leftW-1 chars.
func TestContactsTab_RowsNoWrap(t *testing.T) {
	b := &stubBackend{}
	m := New(b, nil, "", nil, false)
	m.loading = false
	m.activeTab = tabContacts
	// Contact with a long company name to maximise line width.
	m.contactsFiltered = []models.ContactData{
		{
			Email:       "newsletter@techweekly.example",
			DisplayName: "Tech Weekly",
			Company:     "Tech Weekly Magazine Publishers",
			EmailCount:  99,
		},
	}

	rendered := m.renderContactsTab(220, 50)
	lines := strings.Split(rendered, "\n")

	// Find the line that contains "Tech Weekly" (the contact name).
	// There must be exactly one such line; if the row wraps we'd see parts on multiple lines.
	nameLines := 0
	for _, line := range lines {
		if strings.Contains(stripANSIContacts(line), "Tech Weekly") {
			nameLines++
		}
	}
	if nameLines != 1 {
		t.Errorf("contact row appears on %d lines, want exactly 1 (row is wrapping)", nameLines)
	}
}

// visibleWidth returns the number of visible (non-ANSI) runes in s.
func visibleWidth(s string) int {
	inEscape := false
	count := 0
	for _, r := range s {
		if r == '\x1b' {
			inEscape = true
		}
		if inEscape {
			if r == 'm' {
				inEscape = false
			}
			continue
		}
		count++
	}
	return count
}

// TestContactsDetail_EmailRowsNoWrap verifies that recent email rows in the contact
// detail panel render on exactly one line. Before the fix, maxSubjW = rightW-14 made
// the formatted line exactly rightW chars wide, but the panel's inner content is
// rightW-1, causing the date to wrap onto the next line.
//
// Regression test: fixed by changing maxSubjW = rightW-14 → rightW-15.
func TestContactsDetail_EmailRowsNoWrap(t *testing.T) {
	b := &stubBackend{}
	m := New(b, nil, "", nil, false)
	m.loading = false
	m.activeTab = tabContacts

	c := models.ContactData{
		Email:       "delivery@usps.example.com",
		DisplayName: "USPS Informed Delivery",
		EmailCount:  4,
	}
	m.contactsFiltered = []models.ContactData{c}
	m.contactDetail = &c
	m.contactFocusPanel = 1
	m.contactDetailIdx = 0
	// Populate detail emails with a long subject that would previously cause wrapping.
	m.contactDetailEmails = []*models.EmailData{
		{
			MessageID: "msg-1",
			Subject:   "Your Daily Digest for Tue, 3/31 is ready to view",
			Sender:    "delivery@usps.example.com",
			Date:      time.Date(2026, 3, 31, 0, 0, 0, 0, time.UTC),
		},
	}

	rendered := m.renderContactsTab(220, 50)
	lines := strings.Split(rendered, "\n")

	// The date "2026-03-31" must appear complete on the same line as the subject.
	// Any wrapping — even one character — would split "2026-03-31" across two lines.
	fullDateOnLine := false
	for _, line := range lines {
		stripped := stripANSIContacts(line)
		if strings.Contains(stripped, "2026-03-31") && strings.Contains(stripped, "Digest") {
			fullDateOnLine = true
		}
	}
	if !fullDateOnLine {
		t.Error("date '2026-03-31' not found on same line as subject — email row is wrapping")
	}

	// Additionally check no line exceeds w=220.
	for i, line := range lines {
		if v := visibleWidth(line); v > 220 {
			t.Errorf("line %d visible width=%d exceeds terminal width 220", i, v)
		}
	}
}

// TestContactsDetail_EmojiSubjectNoWrap verifies that email subjects containing
// wide characters (emoji) do not cause the date column to wrap onto the next line.
// Wide characters like 🔔 occupy 2 terminal columns but count as 1 rune, so
// rune-based truncation/padding (the old approach) would make the line 1+ columns
// too wide for each emoji present. Fixed by using ansi.StringWidth-aware padding.
func TestContactsDetail_EmojiSubjectNoWrap(t *testing.T) {
	b := &stubBackend{}
	m := New(b, nil, "", nil, false)
	m.loading = false
	m.activeTab = tabContacts

	c := models.ContactData{
		Email:       "alerts@quicken.com",
		DisplayName: "Quicken Simplifi",
		EmailCount:  2,
	}
	m.contactsFiltered = []models.ContactData{c}
	m.contactDetail = &c
	m.contactFocusPanel = 1
	m.contactDetailIdx = 0
	m.contactDetailEmails = []*models.EmailData{
		{
			MessageID: "msg-1",
			Subject:   "🔔 See your bills & income this month",
			Sender:    "alerts@quicken.com",
			Date:      time.Date(2025, 10, 3, 0, 0, 0, 0, time.UTC),
		},
		{
			MessageID: "msg-2",
			Subject:   "🔔 2 transactions need your attention",
			Sender:    "alerts@quicken.com",
			Date:      time.Date(2025, 9, 25, 0, 0, 0, 0, time.UTC),
		},
	}

	rendered := m.renderContactsTab(220, 50)
	lines := strings.Split(rendered, "\n")

	// Both dates must appear complete on the same line as their emoji subject.
	date1OnLine := false
	date2OnLine := false
	for _, line := range lines {
		s := stripANSIContacts(line)
		if strings.Contains(s, "2025-10-03") && strings.Contains(s, "bills") {
			date1OnLine = true
		}
		if strings.Contains(s, "2025-09-25") && strings.Contains(s, "transactions") {
			date2OnLine = true
		}
	}
	if !date1OnLine {
		t.Error("emoji subject 1: date '2025-10-03' not on same line as subject — wide-char wrapping")
	}
	if !date2OnLine {
		t.Error("emoji subject 2: date '2025-09-25' not on same line as subject — wide-char wrapping")
	}

	// No line should exceed terminal width.
	for i, line := range lines {
		if v := visibleWidth(line); v > 220 {
			t.Errorf("line %d visible width=%d exceeds terminal width 220", i, v)
		}
	}
}

// TestContactsEnter_OpensInlinePreview verifies that pressing Enter on an email
// in the Contacts detail panel opens an inline preview within the Contacts tab
// (not jumping to Timeline).
func TestContactsEnter_OpensInlinePreview(t *testing.T) {
	b := &stubBackend{}
	m := New(b, nil, "", nil, false)
	m.loading = false
	m.windowWidth = 220
	m.windowHeight = 50
	m.activeTab = tabContacts
	m.contactFocusPanel = 1

	emails := []*models.EmailData{
		{MessageID: "msg-1", Subject: "First email", Sender: "a@example.com", Date: time.Now().Add(-3 * time.Hour)},
		{MessageID: "msg-2", Subject: "Second email", Sender: "b@example.com", Date: time.Now().Add(-2 * time.Hour)},
		{MessageID: "msg-3", Subject: "Third email", Sender: "c@example.com", Date: time.Now().Add(-1 * time.Hour)},
	}
	c := models.ContactData{Email: "a@example.com", DisplayName: "Test", EmailCount: 3}
	m.contactDetail = &c
	m.contactDetailEmails = emails
	m.contactDetailIdx = 1 // user has selected the second email (msg-2)

	// Press Enter: should open inline preview, staying on Contacts tab.
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(*Model)

	if m.activeTab != tabContacts {
		t.Fatalf("activeTab=%d after Enter, want tabContacts(%d)", m.activeTab, tabContacts)
	}
	if m.contactPreviewEmail == nil || m.contactPreviewEmail.MessageID != "msg-2" {
		t.Fatalf("contactPreviewEmail=%v, want msg-2", m.contactPreviewEmail)
	}
	if !m.contactPreviewLoading {
		t.Error("contactPreviewLoading should be true while body is fetching")
	}
}

// stripANSIContacts removes ANSI escape sequences from s (local to contacts tests).
// TestContactsTab_HeightFits verifies that the Contacts tab panel height does not
// exceed the available space, which would push the header off-screen. Before the fix,
// contentH = height - 6 forgot the 2 border lines lipgloss adds, making total output
// 2 lines taller than the terminal.
func TestContactsTab_HeightFits(t *testing.T) {
	for _, h := range []int{24, 40, 50} {
		b := &stubBackend{}
		m := New(b, nil, "", nil, false)
		m.loading = false
		m.activeTab = tabContacts
		m.contactsFiltered = []models.ContactData{
			{Email: "test@example.com", DisplayName: "Test User", Company: "Acme Corp", EmailCount: 5},
		}

		rendered := m.renderContactsTab(220, h)
		lines := strings.Split(rendered, "\n")
		// renderMainView adds 6 lines of chrome around this content:
		// header(1) + tab bar(1) + blank(1) + trailing newline(1) + status bar(1) + key hints(1).
		// So renderContactsTab output must fit in h - 6 lines.
		maxLines := h - 6
		if len(lines) > maxLines {
			t.Errorf("h=%d: renderContactsTab produced %d lines, max allowed %d (would push header off-screen)",
				h, len(lines), maxLines)
		}
	}
}

func stripANSIContacts(s string) string {
	var out strings.Builder
	inEscape := false
	for _, r := range s {
		if r == '\x1b' {
			inEscape = true
		}
		if inEscape {
			if r == 'm' {
				inEscape = false
			}
			continue
		}
		out.WriteRune(r)
	}
	return out.String()
}
