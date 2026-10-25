package app

import (
	"bytes"
	"crypto/sha256"
	"strings"
	"testing"

	"github.com/herald-email/herald-mail-app/internal/config"
	"github.com/herald-email/herald-mail-app/internal/imagefx"
	"github.com/herald-email/herald-mail-app/internal/models"
)

func crtTestModel(t *testing.T) *Model {
	t.Helper()
	m := &Model{theme: ThemeByName("amber-crt"), previewImageMode: previewImageModeIterm2}
	m.timeline.body = &models.EmailBody{InlineImages: []models.InlineImage{{ContentID: "photo", MIMEType: "image/png", Data: tinyPNG(t, 120, 80)}}}
	return m
}
func TestCRTStylesAndConfigPrecedence(t *testing.T) {
	m := crtTestModel(t)
	if m.currentImageStyle() != imagefx.Amber {
		t.Fatal("theme default")
	}
	m.cfg = &config.Config{}
	m.cfg.Theme.ImageStyle = "original"
	if m.currentImageStyle() != imagefx.Original {
		t.Fatal("config override")
	}
	if e := m.SetPreviewImageStyle("burnt-amber-crt"); e != nil {
		t.Fatal(e)
	}
	if m.currentImageStyle() != imagefx.BurntAmber {
		t.Fatal("launch override")
	}
	if e := m.SetPreviewImageStyle("bad"); e == nil {
		t.Fatal("accepted invalid style")
	}
	for _, name := range []string{"amber-crt", "burnt-amber-crt"} {
		if ThemeByName(name).Name != name {
			t.Fatal("missing theme", name)
		}
	}
}
func TestCRTBackgroundCachePreservesGeometryAndCopyBytes(t *testing.T) {
	m := crtTestModel(t)
	original := m.timeline.body.InlineImages[0]
	before := append([]byte(nil), original.Data...)
	cmd := m.prepareStyledImages()
	if cmd == nil {
		t.Fatal("no worker")
	}
	if m.prepareStyledImages() != nil {
		t.Fatal("duplicate worker")
	}
	done := cmd().(styledImagesMsg)
	_, next := m.Update(done)
	if next != nil {
		t.Fatal("reprocessed completed image")
	}
	key := sha256.Sum256(original.Data)
	if len(m.styledImages.ready[key]) == 0 {
		t.Fatal("missing cache")
	}
	req := previewImageRenderRequest{Mode: previewImageModeIterm2, Image: original, InnerWidth: 100, AvailableRows: 18}
	baseline := renderPreviewImageBlock(req)
	got := m.renderStyledPreviewImage(req)
	if got.Rows != baseline.Rows || got.Width != baseline.Width {
		t.Fatal("geometry changed")
	}
	if bytes.Equal(got.NativeImage.Image.Data, original.Data) {
		t.Fatal("effect missing")
	}
	rows := layoutPreviewImageBlock(previewDocumentBlock{Kind: previewBlockInlineImage, Image: original}, previewLayoutOptions{ImageMode: previewImageModeIterm2, InnerWidth: 100, AvailableRows: 18, ImageRenderer: m.renderStyledPreviewImage})
	if !bytes.Equal(rows[0].Image.Data, before) || !bytes.Equal(m.timeline.body.InlineImages[0].Data, before) {
		t.Fatal("original copy/source mutated")
	}
	clipped, ok := renderClippedPreviewNativeImage(got.NativeImage, previewImageModeIterm2, 1, 1)
	if !ok || !strings.Contains(clipped, "1337") {
		t.Fatal("styled image clipping failed")
	}
	if m.prepareStyledImages() != nil {
		t.Fatal("repaint scheduled worker")
	}
}
func TestCRTStaleResultWaitsThenSchedulesCurrentStyle(t *testing.T) {
	m := crtTestModel(t)
	old := m.prepareStyledImages()
	m.SetPreviewImageStyle("burnt-amber-crt")
	if m.prepareStyledImages() != nil {
		t.Fatal("concurrent workers")
	}
	_, next := m.Update(old())
	if next == nil {
		t.Fatal("current style never scheduled")
	}
	if len(m.styledImages.ready) != 0 {
		t.Fatal("stale data installed")
	}
	_, again := m.Update(next())
	if again != nil || m.styledImages.style != imagefx.BurntAmber || len(m.styledImages.ready) == 0 {
		t.Fatal("current result missing")
	}
}
func TestCRTNoProcessingForFallbackTransports(t *testing.T) {
	for _, mode := range []previewImageMode{previewImageModeOff, previewImageModeLinks, previewImageModePlaceholder} {
		m := crtTestModel(t)
		m.previewImageMode = mode
		if m.prepareStyledImages() != nil {
			t.Fatal("processing fallback", mode)
		}
	}
}
func TestCRTFailureCachedAndImageCountBounded(t *testing.T) {
	m := crtTestModel(t)
	m.timeline.body.InlineImages = nil
	for i := 0; i < maxStyledPreviewImages+3; i++ {
		m.timeline.body.InlineImages = append(m.timeline.body.InlineImages, models.InlineImage{Data: []byte{byte(i)}})
	}
	cmd := m.prepareStyledImages()
	done := cmd().(styledImagesMsg)
	if len(done.ready) != maxStyledPreviewImages {
		t.Fatal("unbounded batch", len(done.ready))
	}
	m.Update(done)
	if m.prepareStyledImages() != nil {
		t.Fatal("retrying invalid image")
	}
}
func TestCRTSettingsRoundTrip(t *testing.T) {
	cfg := &config.Config{}
	cfg.Theme.ImageStyle = "original"
	s := NewSettings(SettingsModePanel, cfg)
	if s.imageStyle != "original" {
		t.Fatal("setting not loaded")
	}
	s.imageStyle = "burnt-amber-crt"
	if s.buildConfig().Theme.ImageStyle != "burnt-amber-crt" {
		t.Fatal("setting not saved")
	}
}

func TestGreenCRTThemeAndSettings(t *testing.T) {
	for _, name := range []string{"vault-green-crt", "dark-olive-crt"} {
		m := crtTestModel(t)
		m.theme = ThemeByName(name)
		if m.theme.Name != name || string(m.currentImageStyle()) != name {
			t.Fatal("missing theme/style", name)
		}
		cfg := &config.Config{}
		cfg.Theme.ImageStyle = name
		s := NewSettings(SettingsModePanel, cfg)
		if s.buildConfig().Theme.ImageStyle != name {
			t.Fatal("lost preference", name)
		}
		cmd := m.prepareStyledImages()
		if cmd == nil {
			t.Fatal("no worker", name)
		}
		m.Update(cmd())
		if len(m.styledImages.ready) == 0 {
			t.Fatal("no result", name)
		}
		m.SetPreviewImageStyle("original")
		if m.currentImageStyle() != imagefx.Original {
			t.Fatal("original override", name)
		}
	}
}

func TestGloomSettingsAndBackgroundWorker(t *testing.T) {
	m := crtTestModel(t)
	cfg := &config.Config{}
	cfg.Theme.ImageStyle = "gloom-crt"
	s := NewSettings(SettingsModePanel, cfg)
	m.SetConfig(s.buildConfig())
	if m.currentImageStyle() != imagefx.Gloom {
		t.Fatal("preference lost")
	}
	cmd := m.prepareStyledImages()
	if cmd == nil {
		t.Fatal("missing worker")
	}
	m.Update(cmd())
	if len(m.styledImages.ready) == 0 || m.prepareStyledImages() != nil {
		t.Fatal("missing cached output")
	}
}

func TestAnalogSettingsAndBackgroundCache(t *testing.T) {
	for _, name := range []string{"old-color-tv", "worn-vhs", "haunted-broadcast"} {
		m := crtTestModel(t)
		cfg := &config.Config{}
		cfg.Theme.ImageStyle = name
		s := NewSettings(SettingsModePanel, cfg)
		m.SetConfig(s.buildConfig())
		if string(m.currentImageStyle()) != name {
			t.Fatal("lost style", name)
		}
		cmd := m.prepareStyledImages()
		if cmd == nil {
			t.Fatal("missing worker", name)
		}
		m.Update(cmd())
		if len(m.styledImages.ready) == 0 || m.prepareStyledImages() != nil {
			t.Fatal("missing cached result", name)
		}
	}
}
