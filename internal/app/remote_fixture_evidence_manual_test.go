//go:build remote_fixture_evidence

package app

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/herald-email/herald-mail-app/internal/models"
	"github.com/herald-email/herald-mail-app/internal/testmail"
)

func TestRemoteHTMLFixtureEvidenceTTYD(t *testing.T) {
	if os.Getenv("HERALD_REMOTE_FIXTURE_EVIDENCE_TUI") != "1" {
		t.Skip("manual ttyd evidence harness")
	}

	m := newVirtualLabTimelinePreviewModel(t, testmail.ScenarioRemoteHTMLImages, 130, 48)
	m.previewImageMode = previewImageModeIterm2
	m.timeline.fullScreen = true
	m.timeline.bodyScrollOffset = 0
	m.clearTimelinePreviewDocumentCache()

	tty, err := os.OpenFile("/dev/tty", os.O_RDWR, 0)
	if err != nil {
		t.Fatalf("open /dev/tty: %v", err)
	}
	defer tty.Close()

	program := tea.NewProgram(&remoteFixtureEvidenceModel{model: m}, tea.WithInput(tty), tea.WithOutput(tty))
	if _, err := program.Run(); err != nil {
		t.Fatalf("run evidence TUI: %v", err)
	}
}

type remoteFixtureEvidenceModel struct {
	model *Model
}

func (m *remoteFixtureEvidenceModel) Init() tea.Cmd {
	return nil
}

func (m *remoteFixtureEvidenceModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.model.windowWidth = msg.Width
		m.model.windowHeight = msg.Height
		m.model.timeline.bodyWrappedLines = nil
		m.model.clearTimelinePreviewDocumentCache()
	case tea.KeyPressMsg:
		switch msg.String() {
		case "q", "ctrl+c", "esc":
			return m, tea.Quit
		case "j", "down":
			m.model.timeline.bodyScrollOffset++
			return m, m.model.timelineIterm2NativeImageRepaintCmd()
		case "k", "up":
			if m.model.timeline.bodyScrollOffset > 0 {
				m.model.timeline.bodyScrollOffset--
			}
			return m, m.model.timelineIterm2NativeImageRepaintCmd()
		case "ctrl+d", "pagedown":
			m.model.timeline.bodyScrollOffset += maxInt(1, m.model.windowHeight/2)
			return m, m.model.timelineIterm2NativeImageRepaintCmd()
		case "ctrl+u", "pageup":
			m.model.timeline.bodyScrollOffset -= maxInt(1, m.model.windowHeight/2)
			if m.model.timeline.bodyScrollOffset < 0 {
				m.model.timeline.bodyScrollOffset = 0
			}
			return m, m.model.timelineIterm2NativeImageRepaintCmd()
		case "o":
			m.model.timeline.remoteImageLoads = revealedFixtureRemoteImages(nil, m.model.timelineRemoteImages())
			m.model.timeline.remoteImageRevision++
			m.model.clearTimelinePreviewDocumentCache()
			return m, m.model.timelineIterm2NativeImageRepaintCmd()
		}
	}
	return m, nil
}

func (m *remoteFixtureEvidenceModel) View() tea.View {
	return m.model.buildView(m.model.renderFullScreenEmail())
}

func revealedFixtureRemoteImages(t *testing.T, remotes []previewRemoteImage) map[string]previewRemoteImageState {
	if t != nil {
		t.Helper()
	}
	states := make(map[string]previewRemoteImageState, len(remotes))
	for i, remote := range remotes {
		asset := fixtureAssetForRemote(remote, i)
		data, err := os.ReadFile(repoFixturePath(t, asset))
		if err != nil {
			if t != nil {
				t.Fatalf("read fixture evidence asset %s: %v", asset, err)
			}
			panic(err)
		}
		key := remote.Key
		if key == "" {
			key = remoteImageDocumentKey(remote.URL)
		}
		states[key] = previewRemoteImageState{
			Image: models.InlineImage{
				ContentID: key,
				MIMEType:  "image/png",
				Data:      data,
			},
		}
	}
	return states
}

func fixtureAssetForRemote(remote previewRemoteImage, index int) string {
	label := strings.ToLower(remoteImageDisplayLabel(remote))
	switch {
	case strings.Contains(label, "3-day") || strings.Contains(label, "calendar"):
		return filepath.Join("docs", "public", "screenshots", "calendar-three-day-command.png")
	case strings.Contains(label, "sidebar") || strings.Contains(label, "multi-account"):
		return filepath.Join("docs", "public", "screenshots", "timeline-main-list.png")
	case index == 0:
		return filepath.Join("assets", "favicon", "favicon-48x48.png")
	default:
		return filepath.Join("docs", "public", "screenshots", "timeline-main-list.png")
	}
}

func repoFixturePath(t *testing.T, rel string) string {
	if t != nil {
		t.Helper()
	}
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		if t != nil {
			t.Fatalf("runtime.Caller failed")
		}
		panic("runtime.Caller failed")
	}
	return filepath.Join(filepath.Dir(file), "..", "..", rel)
}
