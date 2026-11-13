package tests

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// refused checks that neverran refuses: exit 2, nothing on stdout, a reason
// on stderr, under every --fail-on and both formats.
func refused(t *testing.T, label string, args ...string) {
	t.Helper()
	for _, failOn := range []string{"error", "warn", "never"} {
		for _, format := range []string{"text", "json"} {
			r := runTool(t, append([]string{"--fail-on", failOn, "--format", format}, args...)...)
			if r.code != 2 || r.stdout != "" || !strings.Contains(r.stderr, "can't trust the input") {
				t.Errorf("%s (--fail-on %s --format %s): exit %d, want 2 with empty stdout\nstdout: %s\nstderr: %s", label, failOn, format, r.code, r.stdout, r.stderr)
				return
			}
		}
	}
}

// badRuns are go test -json files neverran must refuse. Runs made by the
// go command itself (failed builds, failed tests, panics, timeouts) are in
// live_test.go.
func badRuns(t *testing.T) map[string]string {
	full := readFile(t, fullRun)
	lines := strings.SplitAfter(full, "\n")
	half := strings.Join(lines[:len(lines)/2], "")
	return map[string]string{
		"empty":                "",
		"blank lines":          "\n\n  \n",
		"text":                 "hello, this is not a test run\n",
		"binary":               "\x1f\x8b\x08\x00\x00\x00\x00\x00\x00\x03\xff\xfe\n",
		"html error page":      "<!DOCTYPE html>\n<html><head><title>404 Not Found</title></head></html>\n",
		"curl error":           "curl: (22) The requested URL returned error: 404\n",
		"other json":           `{"name":"report","tests":3}` + "\n",
		"json array":           `[{"Action":"start","Package":"example.com/shop/cart"}]` + "\n",
		"go test without json": "ok  \texample.com/shop/cart\t0.002s\nok  \texample.com/shop/payments\t0.002s\n",
		"go test -v":           "=== RUN   TestTotal\n--- PASS: TestTotal (0.00s)\nPASS\nok  \texample.com/shop/cart\t0.002s\n",
		"build errors":         "# example.com/shop/cart\ncart/cart.go:3:1: syntax error: non-declaration statement outside function body\n",
		"go command error":     "go: cannot find main module, but found .git/config in /src\n",
		"panic text":           "panic: runtime error: index out of range [3] with length 3\n\ngoroutine 1 [running]:\n",
		"cut mid-line":         full[:len(full)/2],
		"cut at a line break":  half,
		"no final line break":  strings.TrimSuffix(full, "\n"),
		"cut run then a whole run pasted after it": half + full,
		"json events without start":                strings.Join(filterLines(lines, func(l string) bool { return !strings.Contains(l, `"Action":"start"`) }), ""),
		"only go: downloading":                     "go: downloading golang.org/x/sync v0.8.0\n",
	}
}

func filterLines(lines []string, keep func(string) bool) []string {
	var out []string
	for _, l := range lines {
		if keep(l) {
			out = append(out, l)
		}
	}
	return out
}

func TestBadRuns(t *testing.T) {
	for name, data := range badRuns(t) {
		p := writeFile(t, t.TempDir(), "run.json", data)
		refused(t, name+", alone", "--dir", shopDir, p)
		refused(t, name+", next to good runs", "--dir", shopDir, fullRun, p, toolsRun)
	}
}

// TestRunFromAnotherProject: a real run of another module is refused, not
// read as "every test of this tree never ran" or as clean.
func TestRunFromAnotherProject(t *testing.T) {
	other := writeModule(t, map[string]string{
		"go.mod":      "module example.com/other\n\ngo 1.22\n",
		"o/o_test.go": "package o\n\nimport \"testing\"\n\nfunc TestO(t *testing.T) {}\n",
	})
	p := writeFile(t, t.TempDir(), "other.json", goTestJSON(t, other, nil, "./..."))
	refused(t, "run from another project", "--dir", shopDir, p)
	// The same run against its own tree is clean, so the run itself is fine.
	if r := runTool(t, "--dir", other, p); r.code != 0 {
		t.Errorf("own tree: exit %d\n%s%s", r.code, r.stdout, r.stderr)
	}
}

// badTrees returns copies of the example with one thing broken. The healthy
// runs are used with each, so a clean result would be the failure.
func TestBadTrees(t *testing.T) {
	mutate := func(f func(dir string)) string {
		dir := filepath.Join(t.TempDir(), "shop")
		copyDir(t, filepath.Join("..", shopDir), dir)
		f(dir)
		return dir
	}
	write := func(name, data string) func(string) {
		return func(dir string) { writeFile(t, dir, name, data) }
	}
	cart := readFile(t, shopDir+"/cart/cart_test.go")
	trees := map[string]string{
		"empty go.mod":                mutate(write("go.mod", "")),
		"go.mod garbage":              mutate(write("go.mod", "<html>404</html>\n")),
		"go.mod cut mid-block":        mutate(write("go.mod", "module example.com/shop\n\ngo 1.22\n\nrequire (\n\tgolang.org/x/sync v0.8.0\n")),
		"go.mod cut mid-word":         mutate(write("go.mod", "modu")),
		"go.mod of another module":    mutate(write("go.mod", "module example.com/elsewhere\n\ngo 1.22\n")),
		"test file garbage":           mutate(write("cart/cart_test.go", "\x00\x01garbage\xff\n")),
		"test file empty":             mutate(write("cart/cart_test.go", "")),
		"test file cut mid-func":      mutate(write("cart/cart_test.go", cart[:len(cart)/2])),
		"test file is build output":   mutate(write("cart/cart_test.go", "# example.com/shop/cart\ncart/cart_test.go:5:2: undefined: Total\n")),
		"test file cut after imports": mutate(write("cart/cart_test.go", cart[:strings.Index(cart, "func TestTotal")])),
		"test file deleted":           mutate(func(dir string) { os.Remove(filepath.Join(dir, "store", "store_test.go")) }),
		"no test files at all": mutate(func(dir string) {
			filepath.Walk(dir, func(p string, info os.FileInfo, err error) error {
				if strings.HasSuffix(p, "_test.go") {
					os.Remove(p)
				}
				return nil
			})
		}),
		"empty directory":         t.TempDir(),
		"missing directory":       filepath.Join(t.TempDir(), "missing"),
		"a file, not a directory": writeFile(t, t.TempDir(), "file.txt", "x\n"),
	}
	for name, dir := range trees {
		refused(t, name, "--dir", dir, fullRun, toolsRun)
	}
}

// TestCutRuns cuts the saved runs at every byte. A cut run must never give a
// clean result: neverran refuses it, or (for a cut that lands exactly between
// two packages, which go test -json can't show) reports the packages it lost
// as errors.
func TestCutRuns(t *testing.T) {
	for _, run := range []string{fullRun, ciRun} {
		data := readFile(t, run)
		dir := t.TempDir()
		undetected := 0
		for i := 0; i < len(data); i++ {
			p := writeFile(t, dir, "cut.json", data[:i])
			args := []string{"--dir", shopDir, p}
			if run == fullRun {
				args = append(args, toolsRun)
			}
			r := runTool(t, args...)
			switch {
			case r.code == 2 && r.stdout == "":
			case r.code == 1 && strings.Contains(r.stdout, "ERROR ") && !strings.Contains(r.stdout, "No problems"):
				undetected++
			default:
				t.Fatalf("%s cut at byte %d of %d: exit %d\n%s%s", run, i, len(data), r.code, r.stdout, r.stderr)
			}
		}
		t.Logf("%s: %d byte cuts, %d reported as lost packages rather than refused", run, len(data), undetected)
	}
}

// TestCutTestFiles cuts each test file of the example at every byte and runs
// neverran with the healthy runs. The only acceptable clean result is a cut
// that removed nothing but trailing white space.
func TestCutTestFiles(t *testing.T) {
	files := []string{"cart/cart_test.go", "store/store_integration_test.go", "search/search_test.go", "tools/lint/lint_test.go"}
	for _, name := range files {
		dir := filepath.Join(t.TempDir(), "shop")
		copyDir(t, filepath.Join("..", shopDir), dir)
		data := readFile(t, shopDir+"/"+name)
		for i := 0; i < len(data); i++ {
			writeFile(t, dir, name, data[:i])
			r := runTool(t, "--dir", dir, fullRun, toolsRun)
			if r.code == 2 && r.stdout == "" {
				continue
			}
			if r.code == 0 && strings.TrimSpace(data[i:]) == "" {
				continue
			}
			t.Fatalf("%s cut at byte %d of %d: exit %d\n%s%s", name, i, len(data), r.code, r.stdout, r.stderr)
		}
	}
}

func TestBadFlags(t *testing.T) {
	cases := [][]string{
		{},
		{"--dir", shopDir},
		{"--fail-on", "maybe", ciRun},
		{"--format", "xml", ciRun},
		{"--ignore", "(", ciRun},
		{"--nope", ciRun},
		{"-", "-"},
	}
	for _, args := range cases {
		if r := runTool(t, args...); r.code != 2 || r.stdout != "" {
			t.Errorf("%v: exit %d\n%s", args, r.code, r.stdout)
		}
	}
	if r := runTool(t, "--version"); r.code != 0 || r.stdout != "neverran 0.1.0\n" {
		t.Errorf("--version: %d %q", r.code, r.stdout)
	}
}
