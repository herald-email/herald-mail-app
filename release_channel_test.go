package main

import (
	"bufio"
	"os/exec"
	"strings"
	"testing"
)

func TestReleaseChannelClassification(t *testing.T) {
	tests := []struct {
		name string
		tag  string
		want map[string]string
	}{
		{
			name: "beta prerelease updates beta latest",
			tag:  "v0.1.0-beta.1",
			want: map[string]string{
				"prerelease":         "true",
				"make_latest":        "false",
				"update_beta_latest": "true",
				"channel":            "beta",
			},
		},
		{
			name: "stable release becomes latest",
			tag:  "v0.1.0",
			want: map[string]string{
				"prerelease":         "false",
				"make_latest":        "true",
				"update_beta_latest": "false",
				"channel":            "stable",
			},
		},
		{
			name: "release candidate stays prerelease without beta alias",
			tag:  "v0.2.0-rc.1",
			want: map[string]string{
				"prerelease":         "true",
				"make_latest":        "false",
				"update_beta_latest": "false",
				"channel":            "prerelease",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := runReleaseChannelScript(t, tt.tag)
			for key, wantValue := range tt.want {
				if got[key] != wantValue {
					t.Fatalf("%s = %q, want %q; all outputs: %#v", key, got[key], wantValue, got)
				}
			}
		})
	}
}

func runReleaseChannelScript(t *testing.T, tag string) map[string]string {
	t.Helper()

	cmd := exec.Command("bash", ".github/scripts/release-channel.sh", tag)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("release-channel.sh %q failed: %v\n%s", tag, err, out)
	}

	values := map[string]string{}
	scanner := bufio.NewScanner(strings.NewReader(string(out)))
	for scanner.Scan() {
		line := scanner.Text()
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			t.Fatalf("output line %q is not key=value", line)
		}
		values[key] = value
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("scan output: %v", err)
	}
	return values
}
