//go:build darwin && cgo

package printing

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestMain(m *testing.M) {
	if handled, code, err := HandleHelper(os.Args[1:]); handled {
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
		}
		os.Exit(code)
	}
	os.Exit(m.Run())
}

func TestMacOSPrintSaveSmokeUsesNativePDFKitPages(t *testing.T) {
	if os.Getenv("HERALD_MACOS_PRINT_SMOKE") != "1" {
		t.Skip("set HERALD_MACOS_PRINT_SMOKE=1 to run the native macOS Save as PDF smoke test")
	}

	msg := findDemoMessage(t, "Step 5: View inline images in full screen")
	html, err := BuildHTMLDocument(Request{
		Email: &msg.Email,
		Body:  &msg.Body,
		Mode:  ModeRenderedMarkdown,
		Theme: ThemeGitHub,
	})
	if err != nil {
		t.Fatalf("BuildHTMLDocument error: %v", err)
	}
	inputPath, err := WriteTempHTML(html)
	if err != nil {
		t.Fatalf("WriteTempHTML error: %v", err)
	}
	defer os.Remove(inputPath)

	outputPath := filepath.Join(t.TempDir(), "step5-print-save.pdf")
	helper, err := os.Executable()
	if err != nil {
		t.Fatalf("locate test helper: %v", err)
	}
	cmd := exec.Command(helper,
		helperSubcommand,
		"--file", inputPath,
		"--title", "Herald native print smoke",
		"--save-pdf", outputPath,
	)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("native print save helper failed: %v\nstderr: %s", err, stderr.String())
	}

	pdf, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("read saved PDF: %v", err)
	}
	if len(pdf) < 100_000 {
		t.Fatalf("saved PDF is unexpectedly small: %d bytes", len(pdf))
	}
	if got := pdfPatternCount(pdf, `/Type\s*/Page\b`); got < 2 {
		t.Fatalf("saved PDF page count pattern = %d, want at least 2", got)
	}
	if got := pdfPatternCount(pdf, `/Length\s+11\b`); got != 0 {
		t.Fatalf("saved PDF contains %d blank-page content streams", got)
	}
	if got := pdfPatternCount(pdf, `/Subtype\s*/Image\b`); got < 4 {
		t.Fatalf("saved PDF image object count = %d, want at least 4", got)
	}
}

func TestMacOSPrintSaveV050RemoteNewsletterSmoke(t *testing.T) {
	if os.Getenv("HERALD_MACOS_PRINT_REMOTE_SMOKE") != "1" {
		t.Skip("set HERALD_MACOS_PRINT_REMOTE_SMOKE=1 to run the native macOS remote-image Save as PDF smoke test")
	}

	msg := findDemoMessage(t, "[PREVIEW] Herald v0.5.0 — Calendar, and multi-account arrive")
	outDir := strings.TrimSpace(os.Getenv("HERALD_PRINT_SMOKE_OUTDIR"))
	if outDir == "" {
		outDir = t.TempDir()
	} else if err := os.MkdirAll(outDir, 0o755); err != nil {
		t.Fatalf("create smoke output dir: %v", err)
	}
	helper, err := os.Executable()
	if err != nil {
		t.Fatalf("locate test helper: %v", err)
	}

	cases := []struct {
		name  string
		mode  Mode
		theme Theme
	}{
		{name: "v050-original-visual-remote", mode: ModeOriginalVisual, theme: ThemeOriginal},
		{name: "v050-rendered-markdown-swiss-remote", mode: ModeRenderedMarkdown, theme: ThemeSwiss},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			html, err := BuildHTMLDocument(Request{
				Email:             &msg.Email,
				Body:              &msg.Body,
				Mode:              tc.mode,
				Theme:             tc.theme,
				AllowRemoteImages: true,
			})
			if err != nil {
				t.Fatalf("BuildHTMLDocument error: %v", err)
			}
			htmlPath := filepath.Join(outDir, tc.name+".html")
			if err := os.WriteFile(htmlPath, []byte(html), 0o600); err != nil {
				t.Fatalf("write smoke HTML: %v", err)
			}
			pdfPath := filepath.Join(outDir, tc.name+"-pdfkit-save.pdf")
			cmd := exec.Command(helper,
				helperSubcommand,
				"--file", htmlPath,
				"--title", "Herald v0.5 newsletter remote image smoke",
				"--save-pdf", pdfPath,
			)
			var stderr bytes.Buffer
			cmd.Stderr = &stderr
			if err := cmd.Run(); err != nil {
				t.Fatalf("native print save helper failed: %v\nstderr: %s", err, stderr.String())
			}

			pdf, err := os.ReadFile(pdfPath)
			if err != nil {
				t.Fatalf("read saved PDF: %v", err)
			}
			if len(pdf) < 100_000 {
				t.Fatalf("saved PDF is unexpectedly small: %d bytes", len(pdf))
			}
			if got := pdfPatternCount(pdf, `/Type\s*/Page\b`); got < 2 {
				t.Fatalf("saved PDF page count pattern = %d, want at least 2", got)
			}
			if got := pdfPatternCount(pdf, `/Length\s+11\b`); got != 0 {
				t.Fatalf("saved PDF contains %d blank-page content streams", got)
			}
			if got := pdfPatternCount(pdf, `/Subtype\s*/Image\b`); got < 2 {
				t.Fatalf("saved PDF image object count = %d, want at least 2", got)
			}
			t.Logf("smoke HTML: %s", htmlPath)
			t.Logf("smoke PDF: %s", pdfPath)
		})
	}
}

func pdfPatternCount(pdf []byte, pattern string) int {
	return len(regexp.MustCompile(pattern).FindAll(pdf, -1))
}
