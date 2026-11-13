// Package tests runs neverran as a binary, the way the skill does, on real
// go test -json output from the example module and from modules the tests
// write, and on broken input.
package tests

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

var (
	bin     string // the neverran binary
	tmpRoot string
)

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "neverran-tests")
	if err != nil {
		panic(err)
	}
	tmpRoot = dir
	bin = filepath.Join(dir, "neverran")
	cmd := exec.Command("go", "build", "-o", bin, "./plugin/scripts")
	cmd.Dir = ".."
	if out, err := cmd.CombinedOutput(); err != nil {
		fmt.Fprintf(os.Stderr, "building neverran: %v\n%s", err, out)
		os.Exit(1)
	}
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}

type result struct {
	code           int
	stdout, stderr string
}

// runTool runs neverran from the repository root.
func runTool(t *testing.T, args ...string) result {
	t.Helper()
	return runToolStdin(t, "", args...)
}

func runToolStdin(t *testing.T, stdin string, args ...string) result {
	t.Helper()
	cmd := exec.Command(bin, args...)
	cmd.Dir = ".."
	cmd.Stdin = strings.NewReader(stdin)
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	err := cmd.Run()
	code := 0
	if ee, ok := err.(*exec.ExitError); ok {
		code = ee.ExitCode()
	} else if err != nil {
		t.Fatal(err)
	}
	return result{code, out.String(), errb.String()}
}

type jsonFinding struct {
	Severity, Code, Package, File, Message, Fix string
	Line                                        int
	Tests                                       []string
}

type jsonReport struct {
	Modules, Packages, Tests, Ran, Skipped, NeverRan, Ignored int
	Findings                                                  []jsonFinding
}

func runJSON(t *testing.T, args ...string) (jsonReport, result) {
	t.Helper()
	r := runTool(t, append([]string{"--format", "json"}, args...)...)
	var rep jsonReport
	if r.code == 0 || r.code == 1 {
		if err := json.Unmarshal([]byte(r.stdout), &rep); err != nil {
			t.Fatalf("bad JSON (%v):\n%s", err, r.stdout)
		}
	}
	return rep, r
}

// codes lists "CODE package tests" for each finding, in order.
func codes(rep jsonReport) []string {
	var out []string
	for _, f := range rep.Findings {
		out = append(out, f.Code+" "+f.Package+" "+strings.Join(f.Tests, ","))
	}
	return out
}

// writeFile writes data to name under dir, creating directories.
func writeFile(t *testing.T, dir, name, data string) string {
	t.Helper()
	p := filepath.Join(dir, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

// writeModule writes a module from a map of file names to contents.
func writeModule(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, data := range files {
		writeFile(t, dir, name, data)
	}
	return dir
}

// copyDir copies a directory tree.
func copyDir(t *testing.T, from, to string) {
	t.Helper()
	err := filepath.WalkDir(from, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(from, p)
		target := filepath.Join(to, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, 0o644)
	})
	if err != nil {
		t.Fatal(err)
	}
}

// goTestJSON runs go test -json in dir and returns its stdout, whatever the
// exit code, so failing runs can be fed to neverran.
func goTestJSON(t *testing.T, dir string, env []string, args ...string) string {
	t.Helper()
	cmd := exec.Command("go", append([]string{"test", "-json", "-count=1"}, args...)...)
	cmd.Dir = dir
	cmd.Env = append(append(os.Environ(), "GOFLAGS=", "GOTOOLCHAIN=local", "GOWORK=off"), env...)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	_ = cmd.Run()
	return out.String()
}

func readFile(t *testing.T, p string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", p))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

const (
	shopDir  = "examples/shop"
	ciRun    = "examples/shop/runs/ci.json"
	fullRun  = "examples/shop/runs/full.json"
	toolsRun = "examples/shop/runs/tools.json"
)

// shopFindings is what the CI-style run of the example must report.
var shopFindings = []string{
	"module-not-run example.com/shop/tools lint.TestValidSKU,lint.ExampleValidSKU",
	"package-ran-no-tests example.com/shop/search TestFindIgnoresCase,TestFindNothing",
	"file-not-built example.com/shop/store TestSaveAndLoad,TestLoadCorruptFile",
	"skipped example.com/shop/payments TestRefundAgainstSandbox",
}

// shopFullEnv is what the full run sets so every example test runs.
var shopFullEnv = []string{"SHOP_PAYMENTS_SANDBOX=1", "SHOP_SEARCH_FIXTURES=testdata/products.txt"}
