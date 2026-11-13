package tests

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
)

// TestSavedCIRun checks the saved CI-style run of the example: go test -json
// ./... at the module root, with no tags and no environment.
func TestSavedCIRun(t *testing.T) {
	rep, r := runJSON(t, "--dir", shopDir, ciRun)
	if r.code != 1 {
		t.Fatalf("exit %d, stderr %s", r.code, r.stderr)
	}
	if got := codes(rep); !reflect.DeepEqual(got, shopFindings) {
		t.Errorf("findings:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(shopFindings, "\n"))
	}
	if rep.Tests != 10 || rep.NeverRan != 6 || rep.Skipped != 1 || rep.Ran != 3 || rep.Modules != 2 || rep.Packages != 5 {
		t.Errorf("counts: %+v", rep)
	}
	for _, f := range rep.Findings {
		if f.Code == "package-ran-no-tests" && !strings.Contains(f.Message, "SHOP_SEARCH_FIXTURES not set") {
			t.Errorf("package-ran-no-tests should quote what TestMain printed: %s", f.Message)
		}
		if f.Code == "skipped" && !strings.Contains(f.Message, `"SHOP_PAYMENTS_SANDBOX not set"`) {
			t.Errorf("skipped should quote t.Skip: %s", f.Message)
		}
	}
}

// TestSavedFullRuns is the healthy case: every test ran, so it must stay quiet
// under every --fail-on.
func TestSavedFullRuns(t *testing.T) {
	for _, failOn := range []string{"error", "warn", "never"} {
		r := runTool(t, "--fail-on", failOn, "--dir", shopDir, fullRun, toolsRun)
		if r.code != 0 || !strings.Contains(r.stdout, "No problems: all 10 tests ran and passed in at least one run.") || r.stderr != "" {
			t.Errorf("--fail-on %s: exit %d\n%s%s", failOn, r.code, r.stdout, r.stderr)
		}
	}
	// One of the two runs alone is not enough.
	if r := runTool(t, "--dir", shopDir, fullRun); r.code != 1 || !strings.Contains(r.stdout, "module-not-run") {
		t.Errorf("full run without the tools module: exit %d\n%s", r.code, r.stdout)
	}
	if r := runTool(t, "--dir", shopDir, toolsRun); r.code != 1 || !strings.Contains(r.stdout, "module-not-run  example.com/shop  go.mod") {
		t.Errorf("tools run alone: exit %d\n%s", r.code, r.stdout)
	}
}

func TestFailOnAndFormats(t *testing.T) {
	cases := []struct {
		failOn string
		want   int
	}{{"error", 1}, {"warn", 1}, {"never", 0}}
	for _, c := range cases {
		for _, format := range []string{"text", "json"} {
			r := runTool(t, "--fail-on", c.failOn, "--format", format, "--dir", shopDir, ciRun)
			if r.code != c.want {
				t.Errorf("--fail-on %s --format %s: exit %d, want %d", c.failOn, format, r.code, c.want)
			}
			if strings.Contains(r.stdout, "No problems") {
				t.Errorf("--fail-on %s: printed a clean result for a run with gaps", c.failOn)
			}
		}
	}
	// Warnings only: ignore the tests behind the errors.
	args := []string{"--dir", shopDir, "--ignore", `^example\.com/shop/(tools/|search\.|store\.)`, ciRun}
	if r := runTool(t, args...); r.code != 0 || !strings.Contains(r.stdout, "WARN skipped") || !strings.Contains(r.stdout, "note: left out by --ignore: 7 tests") {
		t.Errorf("warnings only, default --fail-on: exit %d\n%s", r.code, r.stdout)
	}
	if r := runTool(t, append([]string{"--fail-on", "warn"}, args...)...); r.code != 1 {
		t.Errorf("warnings only, --fail-on warn: exit %d", r.code)
	}
	// Ignoring everything checks nothing, which isn't a clean result.
	if r := runTool(t, "--dir", shopDir, "--ignore", ".", ciRun); r.code != 2 || r.stdout != "" {
		t.Errorf("--ignore everything: exit %d\n%s", r.code, r.stdout)
	}
}

func TestStdin(t *testing.T) {
	r := runToolStdin(t, readFile(t, ciRun), "--dir", shopDir, "-")
	if r.code != 1 || !strings.Contains(r.stdout, "3 errors, 1 warning") {
		t.Errorf("stdin: exit %d\n%s%s", r.code, r.stdout, r.stderr)
	}
	if r := runToolStdin(t, "", "--dir", shopDir, "-"); r.code != 2 || r.stdout != "" {
		t.Errorf("empty stdin: exit %d\n%s", r.code, r.stdout)
	}
}

// TestLiveExample runs go test -json on a copy of the example with the Go
// running these tests, the way CI would, and checks that neverran gives the
// same findings as for the saved runs.
func TestLiveExample(t *testing.T) {
	if v, err := exec.Command("go", "version").Output(); err == nil {
		t.Logf("runs made with %s", strings.TrimSpace(string(v)))
	}
	dir := filepath.Join(t.TempDir(), "shop")
	copyDir(t, filepath.Join("..", shopDir), dir)
	runs := filepath.Join(t.TempDir(), "runs")
	os.Mkdir(runs, 0o755)
	write := func(name, data string) string {
		p := filepath.Join(runs, name)
		if err := os.WriteFile(p, []byte(data), 0o644); err != nil {
			t.Fatal(err)
		}
		return p
	}
	ci := write("ci.json", goTestJSON(t, dir, nil, "./..."))
	full := write("full.json", goTestJSON(t, dir, shopFullEnv, "-tags", "integration", "./..."))
	tools := write("tools.json", goTestJSON(t, filepath.Join(dir, "tools"), nil, "./..."))

	rep, r := runJSON(t, "--dir", dir, ci)
	if r.code != 1 {
		t.Fatalf("live CI run: exit %d %s", r.code, r.stderr)
	}
	if got := codes(rep); !reflect.DeepEqual(got, shopFindings) {
		t.Errorf("live CI run findings:\n%s", strings.Join(got, "\n"))
	}
	if r := runTool(t, "--dir", dir, full, tools); r.code != 0 || !strings.Contains(r.stdout, "No problems") {
		t.Errorf("live full runs: exit %d\n%s%s", r.code, r.stdout, r.stderr)
	}
}

// TestInventoryMatchesGoTestList compares neverran's view of which tests
// exist with go test -list, which asks the go command itself. With the
// integration tag, go test -list sees every file in the example, so the
// tests it lists are exactly the tests neverran counts.
func TestInventoryMatchesGoTestList(t *testing.T) {
	list := func(dir string, args ...string) []string {
		cmd := exec.Command("go", append([]string{"test", "-list", "."}, args...)...)
		cmd.Dir = dir
		// The search package's TestMain exits before listing anything
		// unless its fixtures are set.
		cmd.Env = append(append(os.Environ(), "GOFLAGS=", "GOTOOLCHAIN=local", "GOWORK=off"), shopFullEnv...)
		out, err := cmd.Output()
		if err != nil {
			t.Fatalf("go test -list: %v", err)
		}
		var names []string
		for _, l := range strings.Split(string(out), "\n") {
			if l == "" || strings.HasPrefix(l, "ok ") || strings.HasPrefix(l, "?") || strings.HasPrefix(l, "Benchmark") {
				continue
			}
			names = append(names, l)
		}
		return names
	}
	shop := filepath.Join("..", shopDir)
	got := append(list(shop, "-tags", "integration", "./..."), list(filepath.Join(shop, "tools"), "./...")...)
	sort.Strings(got)
	if len(got) != 10 {
		t.Fatalf("go test -list found %d tests: %v", len(got), got)
	}
	rep, _ := runJSON(t, "--dir", shopDir, ciRun)
	var names []string
	for _, f := range rep.Findings {
		if f.Severity == "ERROR" {
			names = append(names, f.Tests...)
		}
	}
	// Tests that ran in the CI run (from the JSON itself).
	for _, l := range strings.Split(strings.TrimSpace(readFile(t, ciRun)), "\n") {
		var ev struct{ Action, Test string }
		if err := json.Unmarshal([]byte(l), &ev); err != nil {
			t.Fatal(err)
		}
		if ev.Action == "run" && !strings.Contains(ev.Test, "/") {
			names = append(names, ev.Test)
		}
	}
	for i, n := range names {
		names[i] = n[strings.LastIndex(n, ".")+1:]
	}
	sort.Strings(names)
	if !reflect.DeepEqual(got, names) {
		t.Errorf("go test -list: %v\nneverran never-ran plus ran: %v", got, names)
	}
}
