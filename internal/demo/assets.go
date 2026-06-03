package demo

import (
	_ "embed"
	"net/url"
	"strings"
)

//go:embed assets/cc-by-sa.png
var demoCCBySABadgePNG []byte

//go:embed assets/color-chart-330px.png
var demoColorChartPNG []byte

//go:embed assets/bee-on-sunflower-330px.jpg
var demoBeeOnSunflowerJPG []byte

//go:embed assets/changing-landscape-960px.jpg
var demoChangingLandscapeJPG []byte

// RemoteImageAsset returns demo image bytes for remote-image reveal fixtures.
// It intentionally recognizes only Herald-owned demo URLs so demo mode can
// exercise user-triggered remote image reveal without internet access.
func RemoteImageAsset(rawURL string) ([]byte, string, bool) {
	u, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || !strings.EqualFold(u.Hostname(), "assets.herald.demo") {
		return nil, "", false
	}
	switch strings.Trim(u.Path, "/") {
	case "cc-by-sa.png":
		return demoCCBySABadgePNG, "image/png", true
	case "color-chart-330px.png":
		return demoColorChartPNG, "image/png", true
	case "bee-on-sunflower-330px.jpg":
		return demoBeeOnSunflowerJPG, "image/jpeg", true
	case "changing-landscape-960px.jpg":
		return demoChangingLandscapeJPG, "image/jpeg", true
	default:
		return nil, "", false
	}
}
