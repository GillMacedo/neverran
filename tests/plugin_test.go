package tests

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

const skillPath = "../plugin/skills/finding-go-tests-ci-never-ran/SKILL.md"

// TestInstalledPlugin copies plugin/ to a directory outside this module, the
// way Claude Code installs it, and builds it with the command the skill uses.
// The plugin has no go.mod, so it must build from its .go files alone.
func TestInstalledPlugin(t *testing.T) {
	dir := t.TempDir()
	root := filepath.Join(dir, "neverran")
	copyDir(t, "../plugin", root)
	skill, err := os.ReadFile(skillPath)
	if err != nil {
		t.Fatal(err)
	}
	const buildLine = `go build -o "${TMPDIR:-/tmp}/neverran" $(ls "${CLAUDE_PLUGIN_ROOT}"/scripts/*.go | grep -v _test.go)`
	if !strings.Contains(string(skill), buildLine) {
		t.Fatalf("SKILL.md doesn't contain the build line this test runs:\n%s", buildLine)
	}
	if runtime.GOOS == "windows" {
		t.Skip("the skill's build line is for a POSIX shell")
	}
	out := filepath.Join(dir, "tmp")
	if err := os.Mkdir(out, 0o755); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("sh", "-c", buildLine)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "CLAUDE_PLUGIN_ROOT="+root, "TMPDIR="+out, "GOFLAGS=")
	if b, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("building the installed plugin failed: %v\n%s", err, b)
	}
	run := exec.Command(filepath.Join(out, "neverran"), "--dir", shopDir, ciRun)
	run.Dir = ".."
	b, err := run.CombinedOutput()
	if ee, ok := err.(*exec.ExitError); !ok || ee.ExitCode() != 1 || !strings.Contains(string(b), "3 errors, 1 warning") {
		t.Fatalf("the installed plugin's binary gave %v:\n%s", err, b)
	}
}

func TestManifests(t *testing.T) {
	var plugin struct{ Name, Version string }
	var market struct {
		Name    string
		Plugins []struct{ Name, Source string }
	}
	for p, v := range map[string]any{"../plugin/.claude-plugin/plugin.json": &plugin, "../.claude-plugin/marketplace.json": &market} {
		data, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(data, v); err != nil {
			t.Fatalf("%s: %v", p, err)
		}
	}
	if plugin.Name != "neverran" || plugin.Version != "0.1.0" {
		t.Errorf("plugin.json: %+v", plugin)
	}
	if market.Name != "neverran-marketplace" || len(market.Plugins) != 1 || market.Plugins[0].Name != "neverran" || market.Plugins[0].Source != "./plugin" {
		t.Errorf("marketplace.json: %+v", market)
	}
	src, _ := os.ReadFile("../plugin/scripts/neverran.go")
	if !strings.Contains(string(src), `const version = "0.1.0"`) {
		t.Error("neverran.go version doesn't match plugin.json")
	}
	skill, _ := os.ReadFile(skillPath)
	if !strings.HasPrefix(string(skill), "---\nname: finding-go-tests-ci-never-ran\ndescription: ") {
		t.Error("SKILL.md front matter")
	}
	readme := readFile(t, "README.md")
	if !strings.Contains(readme, "neverran/plugin/scripts@v0.1.0") || !strings.Contains(readme, "neverran@neverran-marketplace") {
		t.Error("README install lines don't match the version or marketplace name")
	}
}
