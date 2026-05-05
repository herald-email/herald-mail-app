package app

import (
	"fmt"
	"strings"
	"testing"

	"github.com/herald-email/herald-mail-app/internal/models"
)

func TestCleanupConfigurationOverlaysRenderAsCompactModals(t *testing.T) {
	tests := []struct {
		name  string
		open  func(*Model) *Model
		title string
	}{
		{
			name: "automation rule editor",
			open: func(m *Model) *Model {
				updated, _ := m.Update(keyRunes("W"))
				return updated.(*Model)
			},
			title: "Automation Rule",
		},
		{
			name: "custom prompt editor",
			open: func(m *Model) *Model {
				updated, _ := m.Update(keyRunes("P"))
				return updated.(*Model)
			},
			title: "New Custom Prompt",
		},
		{
			name: "cleanup rule manager",
			open: func(m *Model) *Model {
				updated, _ := m.Update(keyRunes("C"))
				return updated.(*Model)
			},
			title: "Auto-Cleanup Rules",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			for _, size := range []struct {
				width  int
				height int
			}{
				{width: 220, height: 50},
				{width: 80, height: 24},
			} {
				t.Run(fmt.Sprintf("%dx%d", size.width, size.height), func(t *testing.T) {
					m := makeSizedModel(t, size.width, size.height)
					m.activeTab = tabCleanup
					m.stats = makeCleanupStats()
					m.updateSummaryTable()

					m = tc.open(m)

					rendered := m.View().Content
					assertFitsWidth(t, size.width, rendered)
					assertFitsHeight(t, size.height, rendered)
					assertOverlayKeepsCleanupBackdrop(t, rendered, tc.title)
				})
			}
		})
	}
}

func TestCleanupDryRunPreviewRendersAsCompactModal(t *testing.T) {
	for _, size := range []struct {
		width  int
		height int
	}{
		{width: 220, height: 50},
		{width: 80, height: 24},
	} {
		t.Run(fmt.Sprintf("%dx%d", size.width, size.height), func(t *testing.T) {
			m := makeSizedModel(t, size.width, size.height)
			m.activeTab = tabCleanup
			m.stats = makeCleanupStats()
			m.updateSummaryTable()
			m.ruleDryRunPreview = newCleanupDryRunPreview(sampleDryRunReport(models.RuleDryRunKindCleanup), models.RuleDryRunRequest{}, nil)

			rendered := m.View().Content
			assertFitsWidth(t, size.width, rendered)
			assertFitsHeight(t, size.height, rendered)
			assertOverlayKeepsCleanupBackdrop(t, rendered, "Cleanup Rules Preview")
		})
	}
}

func TestCleanupConfigurationOverlaysUseMinimumSizeGuard(t *testing.T) {
	tests := []struct {
		name string
		open func(*Model) *Model
	}{
		{
			name: "automation rule editor",
			open: func(m *Model) *Model {
				updated, _ := m.Update(keyRunes("W"))
				return updated.(*Model)
			},
		},
		{
			name: "custom prompt editor",
			open: func(m *Model) *Model {
				updated, _ := m.Update(keyRunes("P"))
				return updated.(*Model)
			},
		},
		{
			name: "cleanup rule manager",
			open: func(m *Model) *Model {
				updated, _ := m.Update(keyRunes("C"))
				return updated.(*Model)
			},
		},
		{
			name: "dry-run preview",
			open: func(m *Model) *Model {
				m.ruleDryRunPreview = newCleanupDryRunPreview(sampleDryRunReport(models.RuleDryRunKindCleanup), models.RuleDryRunRequest{}, nil)
				return m
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			m := makeSizedModel(t, 50, 15)
			m.activeTab = tabCleanup
			m = tc.open(m)

			rendered := m.View().Content
			assertFitsWidth(t, 50, rendered)
			stripped := stripANSI(rendered)
			if !strings.Contains(stripped, "Terminal too narrow") {
				t.Fatalf("expected minimum-size guard for %s, got:\n%s", tc.name, stripped)
			}
		})
	}
}

func assertOverlayKeepsCleanupBackdrop(t *testing.T, rendered, title string) {
	t.Helper()
	stripped := stripANSI(rendered)
	if !strings.Contains(stripped, title) {
		t.Fatalf("expected overlay title %q, got:\n%s", title, stripped)
	}
	lines := strings.Split(strings.TrimRight(stripped, "\n"), "\n")
	if len(lines) == 0 || !strings.Contains(lines[0], "Herald") {
		t.Fatalf("expected Cleanup backdrop chrome to remain visible above modal, got first line %q:\n%s", firstLine(lines), stripped)
	}
	titleRow := -1
	for i, line := range lines {
		if strings.Contains(line, title) {
			titleRow = i
			break
		}
	}
	if titleRow < 2 {
		t.Fatalf("expected %q to be centered below the top chrome, row=%d:\n%s", title, titleRow, stripped)
	}
}

func firstLine(lines []string) string {
	if len(lines) == 0 {
		return ""
	}
	return lines[0]
}
