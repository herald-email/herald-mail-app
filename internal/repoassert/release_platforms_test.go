package repoassert

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// Exercise the actual beta publication script with small archives, so a new
// platform cannot silently disappear between build artifacts and publication.
func TestReleasePackagesEveryPlatformInBetaChannel(t *testing.T) {
	data, err := os.ReadFile(repoPath(t, ".github/workflows/release.yml"))
	if err != nil {
		t.Fatal(err)
	}
	var workflow struct {
		Jobs map[string]struct {
			Strategy struct {
				Matrix struct {
					Include []struct {
						GOOS   string `yaml:"goos"`
						Arch   string `yaml:"arch"`
						Runner string `yaml:"runner"`
					} `yaml:"include"`
				} `yaml:"matrix"`
			} `yaml:"strategy"`
			Steps []struct {
				Name string `yaml:"name"`
				Run  string `yaml:"run"`
			} `yaml:"steps"`
		} `yaml:"jobs"`
	}
	if err := yaml.Unmarshal(data, &workflow); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "dist"), 0700); err != nil {
		t.Fatal(err)
	}
	platforms := map[string]string{"darwin-arm64": "macos-15", "darwin-amd64": "macos-15-intel", "linux-arm64": "ubuntu-24.04-arm", "linux-amd64": "ubuntu-24.04"}
	seen := map[string]bool{}
	for _, job := range workflow.Jobs {
		for _, entry := range job.Strategy.Matrix.Include {
			platform := entry.GOOS + "-" + entry.Arch
			if runner, ok := platforms[platform]; ok {
				if entry.Runner != runner {
					t.Fatalf("%s must build natively on %s, got %s", platform, runner, entry.Runner)
				}
				seen[platform] = true
			}
		}
	}
	for platform := range platforms {
		if !seen[platform] {
			t.Fatalf("release build missing %s", platform)
		}
		if err := os.WriteFile(filepath.Join(dir, "dist", "herald-v0.1.0-beta.1-"+platform+".tar.gz"), []byte(platform), 0600); err != nil {
			t.Fatal(err)
		}
	}
	var script string
	for _, step := range workflow.Jobs["release"].Steps {
		if step.Name == "Prepare beta-latest assets" {
			script = step.Run
		}
	}
	if script == "" {
		t.Fatal("missing beta asset preparation")
	}
	cmd := exec.Command("bash", "-euc", script)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GITHUB_REF_NAME=v0.1.0-beta.1", "GITHUB_SHA=fixture")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("prepare beta assets: %v\n%s", err, out)
	}
	manifest, err := os.ReadFile(filepath.Join(dir, "beta-channel", "beta-latest.json"))
	if err != nil {
		t.Fatal(err)
	}
	for platform := range platforms {
		asset := "herald-beta-latest-" + platform + ".tar.gz"
		content, err := os.ReadFile(filepath.Join(dir, "beta-channel", asset))
		if err != nil || string(content) != platform {
			t.Fatalf("%s missing or changed: %v", asset, err)
		}
		if !strings.Contains(string(manifest), `"`+asset+`"`) {
			t.Fatalf("manifest missing %s", asset)
		}
	}
	check := exec.Command("shasum", "-a", "256", "-c", "checksums-beta-latest.txt")
	check.Dir = filepath.Join(dir, "beta-channel")
	if out, err := check.CombinedOutput(); err != nil {
		t.Fatalf("verify beta checksums: %v\n%s", err, out)
	}
}
