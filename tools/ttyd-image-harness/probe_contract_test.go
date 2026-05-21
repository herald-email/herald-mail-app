package ttydimageharness

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

const demoImageSubject = "Step 5: View inline images in full screen"

func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
}

func readRepoFile(t *testing.T, rel string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(repoRoot(t), rel))
	if err != nil {
		t.Fatalf("read %s: %v", rel, err)
	}
	return string(data)
}

func TestProbeDefaultsTargetCurrentDemoImageSubject(t *testing.T) {
	probe := readRepoFile(t, "tools/ttyd-image-harness/probe.sh")
	for _, want := range []string{
		`PROBE_TARGET="${PROBE_TARGET:-demo-image-sampler}"`,
		`SEARCH_QUERY="${SEARCH_QUERY:-` + demoImageSubject + `}"`,
		`"probeTarget": process.env.PROBE_TARGET`,
		`"searchQuery": process.env.SEARCH_QUERY`,
	} {
		if !strings.Contains(probe, want) {
			t.Fatalf("probe.sh missing %q", want)
		}
	}
	if strings.Contains(probe, "Example 4") {
		t.Fatalf("probe.sh should not use stale Example 4 wording")
	}
}

func TestImageDemoDocsAndTapeUseCurrentSubject(t *testing.T) {
	files := []string{
		"README.md",
		"demos/image-preview.tape",
		"demos/generate-doc-media.sh",
		"docs/src/content/docs/demo-mode.md",
		"docs/src/content/docs/advanced/demo-gifs.md",
		"engineering/testplans/TUI_TESTPLAN.md",
		"VISION.md",
	}
	for _, rel := range files {
		t.Run(rel, func(t *testing.T) {
			content := readRepoFile(t, rel)
			if !strings.Contains(content, demoImageSubject) {
				t.Fatalf("%s missing current image subject %q", rel, demoImageSubject)
			}
			for _, stale := range []string{
				"Example 4",
				"Creative Commons image sampler for terminal previews",
			} {
				if strings.Contains(content, stale) {
					t.Fatalf("%s still contains stale image-demo wording %q", rel, stale)
				}
			}
		})
	}
}
