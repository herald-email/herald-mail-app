package app

import (
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/herald-email/herald-mail-app/internal/models"
)

const (
	defaultSyncFlushCount = 100
	defaultSyncFlushDelay = 500 * time.Millisecond
)

type SyncEventMsg struct {
	Event models.FolderSyncEvent
}

type SyncFlushMsg struct {
	Folder     string
	Generation int64
}

type SyncHydratedMsg struct {
	Folder        string
	Generation    int64
	Stats         map[string]*models.SenderStats
	Emails        []*models.EmailData
	Err           error
	FinishLoading bool
	StatusMessage string
}

type syncAccumulatorAction struct {
	ArmTimer bool
	FlushNow bool
}

type syncAccumulator struct {
	folder       string
	generation   int64
	pendingCount int
	timerArmed   bool
	flushCount   int
	flushDelay   time.Duration
}

func newSyncAccumulator(flushCount int, flushDelay time.Duration) syncAccumulator {
	if flushCount <= 0 {
		flushCount = defaultSyncFlushCount
	}
	if flushDelay <= 0 {
		flushDelay = defaultSyncFlushDelay
	}
	return syncAccumulator{
		flushCount: flushCount,
		flushDelay: flushDelay,
	}
}

func (a *syncAccumulator) reset(folder string, generation int64) {
	a.folder = folder
	a.generation = generation
	a.pendingCount = 0
	a.timerArmed = false
}

func (a *syncAccumulator) flushAction() syncAccumulatorAction {
	a.pendingCount = 0
	a.timerArmed = false
	return syncAccumulatorAction{FlushNow: true}
}

func (a *syncAccumulator) observe(event models.FolderSyncEvent) syncAccumulatorAction {
	if event.Generation <= 0 {
		return syncAccumulatorAction{}
	}
	if event.Generation != a.generation || event.Folder != a.folder {
		a.reset(event.Folder, event.Generation)
	}

	switch event.Phase {
	case models.SyncPhaseSyncStarted:
		return syncAccumulatorAction{}
	case models.SyncPhaseSnapshotReady:
		return a.flushAction()
	case models.SyncPhaseRowsCached, models.SyncPhaseCountsUpdated:
		delta := event.EventCount
		if delta <= 0 {
			delta = 1
		}
		a.pendingCount += delta
		if a.pendingCount >= a.flushCount {
			return a.flushAction()
		}
		if !a.timerArmed {
			a.timerArmed = true
			return syncAccumulatorAction{ArmTimer: true}
		}
		return syncAccumulatorAction{}
	case models.SyncPhaseComplete, models.SyncPhaseError:
		return a.flushAction()
	default:
		return syncAccumulatorAction{}
	}
}

func (a *syncAccumulator) shouldFlush(msg SyncFlushMsg) bool {
	if !a.timerArmed {
		return false
	}
	if msg.Generation != a.generation || msg.Folder != a.folder {
		return false
	}
	a.pendingCount = 0
	a.timerArmed = false
	return true
}

func scheduleSyncFlush(folder string, generation int64, delay time.Duration) tea.Cmd {
	if delay <= 0 {
		delay = defaultSyncFlushDelay
	}
	return tea.Tick(delay, func(time.Time) tea.Msg {
		return SyncFlushMsg{Folder: folder, Generation: generation}
	})
}
