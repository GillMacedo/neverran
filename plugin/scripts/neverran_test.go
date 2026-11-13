package main

import (
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestIsTestName(t *testing.T) {
	cases := map[string]bool{
		"Test": true, "TestX": true, "Test_x": true, "Test1": true, "TestÄ": true,
		"Testx": false, "Testing": false, "test": false, "Tes": false,
	}
	for name, want := range cases {
		if got := isTestName(name, "Test"); got != want {
			t.Errorf("isTestName(%q) = %v, want %v", name, got, want)
		}
	}
}

func TestFileNameLimit(t *testing.T) {
	cases := map[string]string{
		"x_test.go":                 "",
		"linux_test.go":             "",
		"x_linux_test.go":           "linux",
		"x_windows_amd64_test.go":   "windows/amd64",
		"windows_amd64_test.go":     "amd64",
		"x_arm64_test.go":           "arm64",
		"x_unix_test.go":            "",
		"store_integration_test.go": "",
	}
	for name, want := range cases {
		if got := fileNameLimit(name); got != want {
			t.Errorf("fileNameLimit(%q) = %q, want %q", name, got, want)
		}
	}
}

func writeTemp(t *testing.T, name, data string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(p, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestReadModulePath(t *testing.T) {
	good := map[string]string{
		"module example.com/a\n\ngo 1.22\n":                                 "example.com/a",
		"// comment\nmodule \"example.com/q\" // trailing\n":                "example.com/q",
		"module example.com/b\n\nrequire (\n\tx.org/y v1.0.0\n)\n":          "example.com/b",
		"module example.com/c\ngo 1.24\ntoolchain go1.24.1\ntool x.org/t\n": "example.com/c",
		"module example.com/no-final-newline":                               "example.com/no-final-newline",
	}
	for src, want := range good {
		got, err := readModulePath(writeTemp(t, "go.mod", src))
		if err != nil || got != want {
			t.Errorf("readModulePath(%q) = %q, %v; want %q", src, got, err, want)
		}
	}
	bad := []string{
		"",
		"\n\n",
		"go 1.22\n",
		"module\n",
		"modu",
		"module example.com/a\nrequire (\n\tx.org/y v1.0.0\n",
		"module example.com/a\nmodule example.com/b\n",
		"<html><body>404</body></html>\n",
		"module a b\n",
		"module /abs\n",
		"\x00\x01\x02\xff\xfe",
	}
	for _, src := range bad {
		if got, err := readModulePath(writeTemp(t, "go.mod", src)); err == nil {
			t.Errorf("readModulePath(%q) = %q, want an error", src, got)
		}
	}
}

func TestReadTestFile(t *testing.T) {
	src := `//go:build integration && !windows

package p

import (
	"testing"
	tt "testing"
)

func TestA(t *testing.T) {
	for _, c := range []int{1} {
		t.Run("x", func(t *testing.T) { _ = c })
	}
}
func TestB(x *tt.T) {}
func Testlower(t *testing.T) {}
func TestWrongArgs(t *testing.T, n int) {}
func TestMain(m *testing.M) {}
func FuzzC(f *testing.F) {}
func BenchmarkD(b *testing.B) {}
func ExampleE() {
	// Output: hi
}
func ExampleF() {
	// no output comment, so go test only compiles it
}
func ExampleG() {
	// Unordered output:
	// a
}
func Example_h() {
	// Output:
	_ = 1
}
func (r R) TestMethod(t *testing.T) {}
type R struct{}
`
	p := writeTemp(t, "p_linux_test.go", src)
	tests, err := readTestFile(token.NewFileSet(), p, "p/p_linux_test.go")
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, tf := range tests {
		got = append(got, tf.Name)
		if tf.Constraint != "integration && !windows" || tf.FileOS != "linux" {
			t.Errorf("%s: constraint %q, file limit %q", tf.Name, tf.Constraint, tf.FileOS)
		}
		if tf.CallsRun != (tf.Name == "TestA") {
			t.Errorf("%s: CallsRun = %v", tf.Name, tf.CallsRun)
		}
	}
	// Example_h's output comment isn't the last thing in its body, but it is
	// the last comment, which is what go/doc looks at.
	want := "TestA TestB FuzzC ExampleE ExampleG Example_h"
	if strings.Join(got, " ") != want {
		t.Errorf("tests = %s, want %s", strings.Join(got, " "), want)
	}
}

func TestTagsIn(t *testing.T) {
	cases := map[string]string{
		"integration":                  "integration",
		"!(go1.27 && !http2legacy)":    "http2legacy",
		"linux && amd64":               "",
		"e2e || (slow && !windows)":    "e2e slow",
		"ignore":                       "",
		"unix && cgo && go1.22 && foo": "foo",
	}
	for expr, want := range cases {
		if got := strings.Join(tagsIn(expr), " "); got != want {
			t.Errorf("tagsIn(%q) = %q, want %q", expr, got, want)
		}
	}
}

func TestSkipReason(t *testing.T) {
	lines := []string{"=== RUN   TestX\n", "    x_test.go:12: DATABASE_URL not set\n", "--- SKIP: TestX (0.00s)\n"}
	if got := skipReason(lines); got != "DATABASE_URL not set" {
		t.Errorf("skipReason = %q", got)
	}
}

func TestCountRepeats(t *testing.T) {
	src := `{"Action":"start","Package":"a"}
{"Action":"run","Package":"a","Test":"TestX"}
{"Action":"pass","Package":"a","Test":"TestX"}
{"Action":"run","Package":"a","Test":"TestX"}
{"Action":"skip","Package":"a","Test":"TestX"}
{"Action":"pass","Package":"a"}
`
	r, err := readRun("count", strings.NewReader(src))
	if err != nil {
		t.Fatal(err)
	}
	if tr := r.Packages["a"].Tests["TestX"]; !tr.Passed || tr.Outcome != "skip" {
		t.Errorf("result %+v", tr)
	}
}

func TestReadRunRejects(t *testing.T) {
	start := `{"Action":"start","Package":"a"}` + "\n"
	cases := map[string]string{
		"empty":          "",
		"blank lines":    "\n\n",
		"no final break": start + `{"Action":"pass","Package":"a"}`,
		"not json":       "ok  \ta\t0.1s\n",
		"json no action": `{"Package":"a"}` + "\n",
		"array":          "[1,2]\n",
		"two values":     `{"Action":"start","Package":"a"} {"x":1}` + "\n",
		"open package":   start,
		"no start":       `{"Action":"run","Package":"a","Test":"TestX"}` + "\n",
		"failed":         start + `{"Action":"fail","Package":"a"}` + "\n",
		"build failed":   `{"ImportPath":"a [a.test]","Action":"build-fail"}` + "\n" + start + `{"Action":"fail","Package":"a","FailedBuild":"a [a.test]"}` + "\n",
		"restart":        start + start,
		"test no result": start + `{"Action":"run","Package":"a","Test":"TestX"}` + "\n" + `{"Action":"pass","Package":"a"}` + "\n",
		"no package":     `{"Action":"run","Test":"TestX"}` + "\n",
		"only downloads": "go: downloading x.org/y v1.0.0\n",
	}
	for name, src := range cases {
		if _, err := readRun(name, strings.NewReader(src)); err == nil {
			t.Errorf("%s: readRun accepted %q", name, src)
		}
	}
}
