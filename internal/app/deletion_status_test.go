package app

import (
	"strings"
	"testing"

	"github.com/herald-email/herald-mail-app/internal/models"
)

func TestDeletionStatusNamesQueuedOperation(t *testing.T) {
	for _, key := range []string{"a", "e", "D"} {
		t.Run(key, func(t *testing.T) {
			m := makeSizedModel(t, 220, 50)
			m.activeTab = tabTimeline
			m.timeline.emails = timelineRangeEmails()
			m.deletionRequestCh = make(chan models.DeletionRequest, 4)
			m.deletionResultCh = make(chan models.DeletionResult, 4)
			m.updateTimelineTable()
			m.setFocusedPanel(panelTimeline)
			m = timelineKeyPress(t, m, keyRunes(key))
			req := readDeletionRequests(t, m.deletionRequestCh, 1)[0]
			archive := key != "D"
			if !m.deleting || req.IsArchive != archive {
				t.Fatalf("operation not queued correctly: deleting=%v request=%+v", m.deleting, req)
			}
			verb, other := "Deleting", "Archiving"
			if archive {
				verb, other = other, verb
			}
			for _, reconnecting := range []bool{false, true} {
				m.connectionLost = reconnecting
				for _, sender := range []string{"", "alice@example.com"} {
					m.deletionProgress.Sender = sender
					want := verb
					if reconnecting {
						want += " (reconnecting…)"
					}
					status := stripANSI(m.renderStatusBar())
					if !strings.Contains(status, want) || strings.Contains(status, other) || !strings.Contains(status, "0/1") {
						t.Fatalf("expected %q with 0/1 progress, got %q", want, status)
					}
				}
			}
			updated, _ := m.Update(models.DeletionResult{MessageID: req.MessageID, Folder: req.Folder, IsArchive: archive, DeletedCount: 1})
			m = updated.(*Model)
			if status := stripANSI(m.renderStatusBar()); strings.Contains(status, "Archiving") || strings.Contains(status, "Deleting") {
				t.Fatalf("completed operation still shown: %q", status)
			}
			m = timelineKeyPress(t, m, keyRunes("D"))
			readDeletionRequests(t, m.deletionRequestCh, 1)
			if status := stripANSI(m.renderStatusBar()); !strings.Contains(status, "Deleting") || strings.Contains(status, "Archiving") {
				t.Fatalf("next delete reused archive wording: %q", status)
			}
		})
	}
}
