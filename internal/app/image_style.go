package app

import (
	"crypto/sha256"
	"sort"

	tea "charm.land/bubbletea/v2"
	"github.com/herald-email/herald-mail-app/internal/imagefx"
	"github.com/herald-email/herald-mail-app/internal/models"
)

const maxStyledPreviewImages = 8
const maxStyledPreviewBytes = 16 * 1024 * 1024

type styledImageState struct {
	body       *models.EmailBody
	revision   int
	style      imagefx.Style
	generation uint64
	running    bool
	complete   bool
	ready      map[[32]byte][]byte
}
type styledImagesMsg struct {
	generation uint64
	ready      map[[32]byte][]byte
}

func (m *Model) SetPreviewImageStyle(value string) error {
	style, err := imagefx.Parse(value)
	if err != nil {
		return err
	}
	m.imageStyleOverride = string(style)
	m.clearTimelinePreviewDocumentCache()
	return nil
}
func (m *Model) currentImageStyle() imagefx.Style {
	value := m.imageStyleOverride
	if value == "" && m.cfg != nil {
		value = m.cfg.Theme.ImageStyle
	}
	style, err := imagefx.Parse(value)
	if err != nil {
		return imagefx.Original
	}
	return imagefx.Resolve(style, m.theme.Name)
}

// Update schedules presentation work after state transitions, never from View.
// One job at a time per model bounds decoding even during rapid navigation.
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if done, ok := msg.(styledImagesMsg); ok {
		m.styledImages.running = false
		if done.generation == m.styledImages.generation {
			m.styledImages.ready = done.ready
			m.styledImages.complete = true
			m.clearTimelinePreviewDocumentCache()
		}
		return m, m.prepareStyledImages()
	}
	model, cmd := m.update(msg)
	return model, tea.Batch(cmd, m.prepareStyledImages())
}
func (m *Model) prepareStyledImages() tea.Cmd {
	style := m.currentImageStyle()
	mode := m.currentPreviewImageMode()
	if mode != previewImageModeIterm2 && mode != previewImageModeKitty {
		style = imagefx.Original
	}
	s := &m.styledImages
	if s.body != m.timeline.body || s.revision != m.timeline.remoteImageRevision || s.style != style {
		s.body = m.timeline.body
		s.revision = m.timeline.remoteImageRevision
		s.style = style
		s.generation++
		s.ready = nil
		s.complete = false
		m.clearTimelinePreviewDocumentCache()
	}
	if s.running || s.complete || s.body == nil || style == imagefx.Original {
		return nil
	}
	images := append([]models.InlineImage(nil), s.body.InlineImages...)
	keys := make([]string, 0, len(m.timeline.remoteImageLoads))
	for key := range m.timeline.remoteImageLoads {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		images = append(images, m.timeline.remoteImageLoads[key].Image)
	}
	// Filter metadata before copying the bounded job input. Oversize/unsupported
	// inputs keep the same fallback as Original and are never retried on repaint.
	inputs := make(map[[32]byte][]byte)
	for _, im := range images {
		if len(inputs) >= maxStyledPreviewImages {
			break
		}
		if len(im.Data) == 0 || len(im.Data) > imagefx.MaxInputBytes {
			continue
		}
		key := sha256.Sum256(im.Data)
		inputs[key] = im.Data
	}
	s.running = true
	generation := s.generation
	return func() tea.Msg {
		ready := make(map[[32]byte][]byte, len(inputs))
		total := 0
		for key, data := range inputs {
			result, err := imagefx.Transform(data, style)
			if err != nil || total+len(result) > maxStyledPreviewBytes {
				ready[key] = nil
				continue
			}
			ready[key] = result
			total += len(result)
		}
		return styledImagesMsg{generation: generation, ready: ready}
	}
}
func (m *Model) renderStyledPreviewImage(req previewImageRenderRequest) previewImageRenderResult {
	if (req.Mode == previewImageModeIterm2 || req.Mode == previewImageModeKitty) && m.styledImages.style == m.currentImageStyle() && m.currentImageStyle() != imagefx.Original {
		req.RasterData = m.styledImages.ready[sha256.Sum256(req.Image.Data)]
	}
	// Original stays visible until the background result is ready, and remains the
	// fallback for unsupported or budget-limited images. Copy metadata is untouched.
	return renderPreviewImageBlock(req)
}
