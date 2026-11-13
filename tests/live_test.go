package tests

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// liveModule is a small module whose tests pass. Each case below breaks it
// in one way and runs the real go command on it.
var liveModule = map[string]string{
	"go.mod":       "module example.com/live\n\ngo 1.22\n",
	"calc/calc.go": "package calc\n\nfunc Add(a, b int) int { return a + b }\n",
	"calc/calc_test.go": `package calc

import "testing"

func TestAdd(t *testing.T) {
	if Add(2, 3) != 5 {
		t.Fatal("Add")
	}
}
`,
	"other/other_test.go": `package other

import "testing"

func TestOther(t *testing.T) {}
`,
}

func liveCopy(t *testing.T, changes map[string]string) string {
	files := map[string]string{}
	for k, v := range liveModule {
		files[k] = v
	}
	for k, v := range changes {
		files[k] = v
	}
	return writeModule(t, files)
}

// TestRealFailedRuns feeds neverran the go test -json output of runs that
// failed in different ways. Each must be refused, alone and next to a
// passing run of the same module.
func TestRealFailedRuns(t *testing.T) {
	cases := map[string]struct {
		changes map[string]string
		args    []string
	}{
		"failing test":        {map[string]string{"other/other_test.go": "package other\n\nimport \"testing\"\n\nfunc TestOther(t *testing.T) { t.Fatal(\"boom\") }\n"}, nil},
		"build error in code": {map[string]string{"calc/calc.go": "package calc\n\nfunc Add(a, b int) int { return a + c }\n"}, nil},
		"build error in test": {map[string]string{"other/other_test.go": "package other\n\nimport \"testing\"\n\nfunc TestOther(t *testing.T) { x := 1 }\n"}, nil},
		"vet error":           {map[string]string{"other/other_test.go": "package other\n\nimport (\n\t\"fmt\"\n\t\"testing\"\n)\n\nfunc TestOther(t *testing.T) { _ = fmt.Sprintf(\"%d\", \"s\") }\n"}, nil},
		"panic":               {map[string]string{"other/other_test.go": "package other\n\nimport \"testing\"\n\nfunc TestOther(t *testing.T) { var m map[string]int; m[\"x\"] = 1 }\n"}, nil},
		"panic in TestMain":   {map[string]string{"other/main_test.go": "package other\n\nimport \"testing\"\n\nfunc TestMain(m *testing.M) { panic(\"setup failed\") }\n"}, nil},
		"TestMain exit 1":     {map[string]string{"other/main_test.go": "package other\n\nimport (\n\t\"os\"\n\t\"testing\"\n)\n\nfunc TestMain(m *testing.M) { os.Exit(1) }\n"}, nil},
		"timeout":             {map[string]string{"other/other_test.go": "package other\n\nimport (\n\t\"testing\"\n\t\"time\"\n)\n\nfunc TestOther(t *testing.T) { time.Sleep(5 * time.Second) }\n"}, []string{"-timeout", "1s"}},
		"os.Exit in a test":   {map[string]string{"other/other_test.go": "package other\n\nimport (\n\t\"os\"\n\t\"testing\"\n)\n\nfunc TestOther(t *testing.T) { os.Exit(0) }\n"}, nil},
	}
	good := liveCopy(t, nil)
	goodRun := writeFile(t, t.TempDir(), "good.json", goTestJSON(t, good, nil, "./..."))
	if r := runTool(t, "--dir", good, goodRun); r.code != 0 || !strings.Contains(r.stdout, "No problems: all 2 tests ran") {
		t.Fatalf("passing run: exit %d\n%s%s", r.code, r.stdout, r.stderr)
	}
	for name, c := range cases {
		dir := liveCopy(t, c.changes)
		out := goTestJSON(t, dir, nil, append(c.args, "./...")...)
		p := writeFile(t, t.TempDir(), "run.json", out)
		refused(t, name, "--dir", dir, p)
		refused(t, name+", next to a passing run", "--dir", good, goodRun, p)
	}
}

// TestRealNonJSONOutput feeds neverran what a CI step leaves behind when
// -json was forgotten or the go command failed before testing.
func TestRealNonJSONOutput(t *testing.T) {
	dir := liveCopy(t, nil)
	run := func(dir string, args ...string) string {
		cmd := exec.Command("go", args...)
		cmd.Dir = dir
		cmd.Env = append(cmd.Environ(), "GOFLAGS=", "GOTOOLCHAIN=local", "GOWORK=off")
		out, _ := cmd.CombinedOutput()
		return string(out)
	}
	broken := liveCopy(t, map[string]string{"calc/calc.go": "package calc\n\nfunc Add(a, b int) int { return a + c }\n"})
	noMod := t.TempDir()
	writeFile(t, noMod, "x_test.go", "package x\n")
	empty := writeModule(t, map[string]string{"go.mod": "module example.com/empty\n\ngo 1.22\n"})
	outputs := map[string]string{
		"go test without -json": run(dir, "test", "-count=1", "./..."),
		"go test -v":            run(dir, "test", "-v", "-count=1", "./..."),
		"go build failure":      run(broken, "build", "./..."),
		"go vet failure":        run(broken, "vet", "./..."),
		"no go.mod":             run(noMod, "test", "-json", "./..."),
		"no packages":           run(empty, "test", "-json", "./..."),
		"go test -json -list":   run(dir, "test", "-json", "-list", ".", "./..."),
	}
	for name, out := range outputs {
		p := writeFile(t, t.TempDir(), "run.txt", out)
		if name == "go test -json -list" {
			// Valid output in which no test ran: findings, never clean.
			r := runTool(t, "--dir", dir, p)
			if r.code != 1 || !strings.Contains(r.stdout, "package-ran-no-tests") {
				t.Errorf("%s: exit %d\n%s%s", name, r.code, r.stdout, r.stderr)
			}
			continue
		}
		refused(t, name, "--dir", dir, p)
	}
}

// TestRealGaps builds modules with each kind of gap and checks the finding
// against a real run, plus the matching healthy run that must stay quiet.
func TestRealGaps(t *testing.T) {
	t.Run("no-subtests", func(t *testing.T) {
		test := `package cases

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCases(t *testing.T) {
	files, _ := filepath.Glob(filepath.Join("testdata", os.Getenv("CASES_DIR"), "*.txt"))
	for _, f := range files {
		t.Run(filepath.Base(f), func(t *testing.T) {})
	}
}
`
		dir := writeModule(t, map[string]string{
			"go.mod":                    "module example.com/cases\n\ngo 1.22\n",
			"cases/cases_test.go":       test,
			"cases/testdata/real/a.txt": "a\n",
		})
		empty := writeFile(t, t.TempDir(), "run.json", goTestJSON(t, dir, []string{"CASES_DIR=wrong"}, "./..."))
		rep, r := runJSON(t, "--dir", dir, empty)
		if r.code != 0 || len(rep.Findings) != 1 || rep.Findings[0].Code != "no-subtests" {
			t.Errorf("empty table: exit %d %+v %s", r.code, rep.Findings, r.stderr)
		}
		if r := runTool(t, "--fail-on", "warn", "--dir", dir, empty); r.code != 1 {
			t.Errorf("empty table with --fail-on warn: exit %d", r.code)
		}
		full := writeFile(t, t.TempDir(), "run.json", goTestJSON(t, dir, []string{"CASES_DIR=real"}, "./..."))
		if r := runTool(t, "--fail-on", "warn", "--dir", dir, full); r.code != 0 || !strings.Contains(r.stdout, "No problems") {
			t.Errorf("table with cases: exit %d\n%s", r.code, r.stdout)
		}
	})
	t.Run("all-subtests-skipped and -short", func(t *testing.T) {
		test := `package slow

import "testing"

func TestSlow(t *testing.T) {
	for _, n := range []string{"a", "b"} {
		t.Run(n, func(t *testing.T) {
			if testing.Short() {
				t.Skip("slow")
			}
		})
	}
}

func TestShortSkip(t *testing.T) {
	if testing.Short() {
		t.Skip("skipped in -short mode")
	}
}
`
		dir := writeModule(t, map[string]string{"go.mod": "module example.com/slow\n\ngo 1.22\n", "slow/slow_test.go": test})
		short := writeFile(t, t.TempDir(), "run.json", goTestJSON(t, dir, nil, "-short", "./..."))
		rep, r := runJSON(t, "--dir", dir, short)
		got := strings.Join(codes(rep), "\n")
		if r.code != 0 || got != "all-subtests-skipped example.com/slow/slow TestSlow\nskipped example.com/slow/slow TestShortSkip" {
			t.Errorf("-short: exit %d\n%s", r.code, got)
		}
		long := writeFile(t, t.TempDir(), "run.json", goTestJSON(t, dir, nil, "./..."))
		if r := runTool(t, "--fail-on", "warn", "--dir", dir, short, long); r.code != 0 {
			t.Errorf("-short and a full run together: exit %d\n%s", r.code, r.stdout)
		}
	})
	t.Run("-run and -skip", func(t *testing.T) {
		dir := liveCopy(t, nil)
		p := writeFile(t, t.TempDir(), "run.json", goTestJSON(t, dir, nil, "-skip", "TestOther", "./..."))
		rep, r := runJSON(t, "--dir", dir, p)
		if r.code != 1 || strings.Join(codes(rep), "\n") != "package-ran-no-tests example.com/live/other TestOther" {
			t.Errorf("-skip: exit %d %v", r.code, codes(rep))
		}
		p = writeFile(t, t.TempDir(), "run.json", goTestJSON(t, dir, nil, "./calc"))
		rep, r = runJSON(t, "--dir", dir, p)
		if r.code != 1 || strings.Join(codes(rep), "\n") != "package-not-run example.com/live/other TestOther" {
			t.Errorf("one package: exit %d %v", r.code, codes(rep))
		}
	})
	t.Run("platform file", func(t *testing.T) {
		// A test file for a platform other than the one running the tests.
		other := "windows"
		if strings.Contains(goEnv(t, "GOOS"), "windows") {
			other = "linux"
		}
		dir := liveCopy(t, map[string]string{"calc/calc_" + other + "_test.go": "package calc\n\nimport \"testing\"\n\nfunc TestOnlyThere(t *testing.T) {}\n"})
		p := writeFile(t, t.TempDir(), "run.json", goTestJSON(t, dir, nil, "./..."))
		rep, r := runJSON(t, "--dir", dir, p)
		if r.code != 1 || len(rep.Findings) != 1 || rep.Findings[0].Code != "file-not-built" || !strings.Contains(rep.Findings[0].Message, "limits it to "+other) {
			t.Errorf("platform file: exit %d %+v", r.code, rep.Findings)
		}
	})
	t.Run("go.work", func(t *testing.T) {
		// A workspace that lists the nested module: go test ./... from the
		// root still covers only the root module's packages.
		dir := liveCopy(t, map[string]string{
			"go.work":         "go 1.22\n\nuse (\n\t.\n\t./sub\n)\n",
			"sub/go.mod":      "module example.com/live/sub\n\ngo 1.22\n",
			"sub/s/s_test.go": "package s\n\nimport \"testing\"\n\nfunc TestSub(t *testing.T) {}\n",
		})
		cmd := exec.Command("go", "test", "-json", "-count=1", "./...")
		cmd.Dir = dir
		cmd.Env = append(cmd.Environ(), "GOFLAGS=", "GOTOOLCHAIN=local")
		out, err := cmd.Output()
		if err != nil {
			t.Fatalf("go test in workspace: %v\n%s", err, out)
		}
		p := writeFile(t, t.TempDir(), "run.json", string(out))
		rep, r := runJSON(t, "--dir", dir, p)
		if r.code != 1 || len(rep.Findings) != 1 || rep.Findings[0].Code != "module-not-run" {
			t.Errorf("workspace: exit %d %v", r.code, codes(rep))
		}
		cmd = exec.Command("go", "test", "-json", "-count=1", "./...")
		cmd.Dir = filepath.Join(dir, "sub")
		cmd.Env = append(cmd.Environ(), "GOFLAGS=", "GOTOOLCHAIN=local")
		sub, err := cmd.Output()
		if err != nil {
			t.Fatal(err)
		}
		q := writeFile(t, t.TempDir(), "sub.json", string(sub))
		if r := runTool(t, "--dir", dir, p, q); r.code != 0 {
			t.Errorf("workspace with both runs: exit %d\n%s", r.code, r.stdout)
		}
	})
}

func goEnv(t *testing.T, name string) string {
	out, err := exec.Command("go", "env", name).Output()
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(string(out))
}
