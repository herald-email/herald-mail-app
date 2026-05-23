package app

import (
	"context"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/herald-email/herald-mail-app/internal/backend"
	"github.com/herald-email/herald-mail-app/internal/logger"
	"github.com/herald-email/herald-mail-app/internal/models"
)

const previewPrewarmLimit = 50

type previewPrewarmTarget struct {
	MessageID string
	Folder    string
	UID       uint32
}

func previewPrewarmTargets(emails []*models.EmailData, folder string, limit int) []previewPrewarmTarget {
	if limit <= 0 || len(emails) == 0 {
		return nil
	}
	targets := make([]previewPrewarmTarget, 0, min(limit, len(emails)))
	seen := make(map[string]bool, limit)
	for _, email := range emails {
		if email == nil || strings.TrimSpace(email.MessageID) == "" || email.UID == 0 {
			continue
		}
		if email.Folder != "" && folder != "" && email.Folder != folder {
			continue
		}
		if seen[email.MessageID] {
			continue
		}
		targetFolder := email.Folder
		if targetFolder == "" {
			targetFolder = folder
		}
		targets = append(targets, previewPrewarmTarget{
			MessageID: email.MessageID,
			Folder:    targetFolder,
			UID:       email.UID,
		})
		seen[email.MessageID] = true
		if len(targets) >= limit {
			break
		}
	}
	return targets
}

func (m *Model) startPreviewPrewarmerIfNeeded() tea.Cmd {
	if m == nil || m.demoMode || m.previewPrewarmActive || isVirtualAllMailOnlyFolder(m.currentFolder) {
		return nil
	}
	if _, ok := m.backend.(messagePreviewServiceBackend); !ok {
		if _, ok := m.backend.(previewCacheBackend); !ok {
			return nil
		}
		if _, ok := m.backend.(previewFetchBackend); !ok {
			return nil
		}
	}
	targets := previewPrewarmTargets(m.timeline.emails, m.currentFolder, previewPrewarmLimit)
	if len(targets) == 0 {
		return nil
	}
	m.previewPrewarmActive = true
	m.previewPrewarmDone = 0
	m.previewPrewarmTotal = len(targets)
	m.previewPrewarmWarmed = 0
	m.previewPrewarmSkipped = 0
	logger.Info("Preview cache: 0/%d warming folder=%s", len(targets), m.currentFolder)
	return m.runPreviewPrewarmNext(targets, m.currentFolder, m.backgroundWorkGeneration, 0, len(targets), 0, 0)
}

func (m *Model) runPreviewPrewarmNext(targets []previewPrewarmTarget, folder string, generation int64, done, total, warmed, skipped int) tea.Cmd {
	if len(targets) == 0 {
		return nil
	}
	serviceBackend, serviceOK := m.backend.(messagePreviewServiceBackend)
	cacheBackend, cacheOK := m.backend.(previewCacheBackend)
	fetchBackend, fetchOK := m.backend.(previewFetchBackend)
	if !serviceOK && (!cacheOK || !fetchOK) {
		return nil
	}
	target := targets[0]
	remaining := append([]previewPrewarmTarget(nil), targets[1:]...)
	return func() tea.Msg {
		nextDone := done + 1
		nextWarmed := warmed
		nextSkipped := skipped
		var resultErr error

		if serviceOK {
			result, fetchErr := serviceBackend.GetMessagePreview(context.Background(), models.MessageRef{
				MessageID: target.MessageID,
				Folder:    target.Folder,
				UID:       target.UID,
			}.WithDefaults(), backend.MessageReadIntent{ViewID: "timeline-prewarm"})
			if fetchErr != nil {
				resultErr = fetchErr
			} else if result.Body == nil {
				nextSkipped++
			} else if result.Source == backend.MessageReadSourceCache {
				nextSkipped++
			} else {
				nextWarmed++
			}
		} else {
			cached, err := cacheBackend.GetCachedPreviewBody(target.MessageID)
			if err != nil {
				logger.Debug("Preview cache prewarm lookup failed: folder=%s message_id=%s error=%v", target.Folder, target.MessageID, err)
			}
			if err == nil && cached != nil {
				nextSkipped++
			} else {
				body, fetchErr := fetchBackend.FetchPreviewBody(target.MessageID, target.Folder, target.UID)
				if fetchErr != nil {
					resultErr = fetchErr
				} else if body == nil {
					nextSkipped++
				} else if cacheErr := cacheBackend.CachePreviewBody(target.MessageID, body); cacheErr != nil {
					resultErr = cacheErr
				} else {
					nextWarmed++
				}
			}
		}

		if resultErr != nil {
			logger.Warn("Preview cache prewarm failed: folder=%s message_id=%s done=%d/%d error=%v", target.Folder, target.MessageID, nextDone, total, resultErr)
		} else {
			logger.Debug("Preview cache prewarm progress: folder=%s message_id=%s done=%d/%d warmed=%d skipped=%d", target.Folder, target.MessageID, nextDone, total, nextWarmed, nextSkipped)
		}
		return previewPrewarmMsg{
			Folder:     folder,
			Generation: generation,
			Remaining:  remaining,
			Done:       nextDone,
			Total:      total,
			Warmed:     nextWarmed,
			Skipped:    nextSkipped,
			Err:        resultErr,
		}
	}
}

func (m *Model) handlePreviewPrewarmMsg(msg previewPrewarmMsg) tea.Cmd {
	if msg.Generation != m.backgroundWorkGeneration || (msg.Folder != "" && msg.Folder != m.currentFolder) {
		logger.Debug("Preview cache prewarm stale: folder=%s generation=%d currentFolder=%s currentGeneration=%d", msg.Folder, msg.Generation, m.currentFolder, m.backgroundWorkGeneration)
		return nil
	}
	m.previewPrewarmDone = msg.Done
	m.previewPrewarmTotal = msg.Total
	m.previewPrewarmWarmed = msg.Warmed
	m.previewPrewarmSkipped = msg.Skipped
	if len(msg.Remaining) == 0 {
		m.previewPrewarmActive = false
		logger.Info("Preview cache: %d/%d warmed folder=%s skipped=%d", msg.Warmed, msg.Total, msg.Folder, msg.Skipped)
		return nil
	}
	return m.runPreviewPrewarmNext(msg.Remaining, msg.Folder, msg.Generation, msg.Done, msg.Total, msg.Warmed, msg.Skipped)
}
