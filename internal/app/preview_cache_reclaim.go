package app

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/herald-email/herald-mail-app/internal/models"
)

func (m *Model) handlePreviewCacheReclaimKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd, bool) {
	switch shortcutKey(msg) {
	case "y", "Y":
		policy := m.previewCacheReclaimPolicy
		if policy == "" {
			policy = m.previewCacheReclaimEstimate.Policy
		}
		m.pendingPreviewCacheReclaim = false
		m.statusMessage = "Reclaiming offline cache storage..."
		type previewCacheReclaimer interface {
			ReclaimOfflineCacheStorage(string) (models.PreviewCacheReclaimResult, error)
		}
		manager, ok := m.backend.(previewCacheReclaimer)
		if !ok {
			m.statusMessage = "Offline cache reclaim unavailable for this backend."
			return m, nil, true
		}
		return m, func() tea.Msg {
			result, err := manager.ReclaimOfflineCacheStorage(policy)
			return PreviewCacheReclaimMsg{Policy: policy, Result: result, Err: err}
		}, true
	case "n", "N", "esc":
		m.pendingPreviewCacheReclaim = false
		m.statusMessage = "Offline cache reclaim cancelled."
		return m, nil, true
	default:
		return m, nil, true
	}
}

func (m *Model) renderPreviewCacheReclaimOverlayView(backdrop string) string {
	w := m.windowWidth
	if w <= 0 {
		w = 80
	}
	h := m.windowHeight
	if h <= 0 {
		h = 24
	}
	return overlayCentered(backdrop, m.renderPreviewCacheReclaimPanel(), w, h)
}

func (m *Model) renderPreviewCacheReclaimPanel() string {
	estimate := m.previewCacheReclaimEstimate
	policy := m.previewCacheReclaimPolicy
	if policy == "" {
		policy = estimate.Policy
	}
	if policy == "" {
		policy = "lightweight"
	}

	lines := []string{
		defaultTheme.Setup.Title.Style().Render("Reclaim offline cache storage"),
		fmt.Sprintf("Policy: %s", policy),
		fmt.Sprintf("Estimate: %s -> %s (%s removable)",
			formatPreviewCacheBytes(estimate.CurrentBytes),
			formatPreviewCacheBytes(estimate.EstimatedAfterBytes),
			formatPreviewCacheBytes(estimate.ReclaimableBytes),
		),
		fmt.Sprintf("Rows: %d scanned, %d with reclaimable bytes", estimate.RowsScanned, estimate.RowsReclaimable),
		"Preview text, headers, and attachment metadata stay cached.",
		"Run prune and SQLite compaction?",
		"y: reclaim  n/Esc: cancel",
	}

	content := strings.Join(lines, "\n")
	return lipgloss.NewStyle().
		Width(68).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(defaultTheme.Setup.Border.ForegroundColor()).
		Padding(1, 2).
		Render(content)
}

func formatPreviewCacheBytes(n int64) string {
	if n < 0 {
		n = 0
	}
	const (
		kib = 1024
		mib = 1024 * kib
		gib = 1024 * mib
	)
	switch {
	case n >= gib:
		return fmt.Sprintf("%.1f GiB", float64(n)/float64(gib))
	case n >= mib:
		return fmt.Sprintf("%.1f MiB", float64(n)/float64(mib))
	case n >= kib:
		return fmt.Sprintf("%.1f KiB", float64(n)/float64(kib))
	default:
		return fmt.Sprintf("%d B", n)
	}
}
