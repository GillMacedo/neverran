package tests

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestReadme runs every "$ " line in the README's indented code blocks from
// the repository root and compares its output with the lines that follow.
func TestReadme(t *testing.T) {
	data, err := os.ReadFile("../README.md")
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(string(data), "\n")
	found := 0
	for i := 0; i < len(lines); i++ {
		if !strings.HasPrefix(lines[i], "    $ ") {
			continue
		}
		cmdline := strings.TrimPrefix(lines[i], "    $ ")
		var want []string
		for j := i + 1; j < len(lines); j++ {
			l := lines[j]
			if l == "" && j+1 < len(lines) && strings.HasPrefix(lines[j+1], "    ") && !strings.HasPrefix(lines[j+1], "    $ ") {
				want = append(want, "")
				continue
			}
			if !strings.HasPrefix(l, "    ") || strings.HasPrefix(l, "    $ ") {
				break
			}
			want = append(want, strings.TrimPrefix(l, "    "))
		}
		found++
		if !strings.HasPrefix(cmdline, "go run ./plugin/scripts ") {
			t.Errorf("README command doesn't start with go run ./plugin/scripts: %s", cmdline)
			continue
		}
		cmd := exec.Command("sh", "-c", strings.Replace(cmdline, "go run ./plugin/scripts", bin, 1))
		cmd.Dir = ".."
		out, err := cmd.Output()
		if err != nil {
			t.Errorf("%s: %v", cmdline, err)
		}
		if got := strings.TrimRight(string(out), "\n"); got != strings.TrimRight(strings.Join(want, "\n"), "\n") {
			t.Errorf("%s: README output differs.\n--- got\n%s\n--- README\n%s", cmdline, got, strings.Join(want, "\n"))
		}
	}
	if found < 2 {
		t.Errorf("found %d commands in the README, expected at least 2", found)
	}
}

// TestReadmeRefusal checks the refusal message quoted in the README against
// a real go test run without -json.
func TestReadmeRefusal(t *testing.T) {
	cmd := exec.Command("go", "test", "-count=1", "./...")
	cmd.Dir = "../" + shopDir
	cmd.Env = append(os.Environ(), "GOFLAGS=", "GOTOOLCHAIN=local", "GOWORK=off")
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("go test: %v\n%s", err, out)
	}
	dir := t.TempDir()
	writeFile(t, dir, "plain.txt", string(out))
	shop, err := filepath.Abs("../" + shopDir)
	if err != nil {
		t.Fatal(err)
	}
	run := exec.Command(bin, "--dir", shop, "plain.txt")
	run.Dir = dir
	var stderr strings.Builder
	run.Stderr = &stderr
	stdout, err := run.Output()
	if ee, ok := err.(*exec.ExitError); !ok || ee.ExitCode() != 2 || len(stdout) != 0 {
		t.Fatalf("exit %v, stdout %q", err, stdout)
	}
	// The README shows the line from a run where cart took 0.002s; the time
	// varies, so compare up to it.
	const prefix = `neverran: can't trust the input, nothing was reported: plain.txt: line 1 is not go test -json output: "ok  \texample.com/shop/cart\t`
	if !strings.HasPrefix(stderr.String(), prefix) || !strings.Contains(stderr.String(), "this looks like go test output without -json. Run go test -json") {
		t.Errorf("stderr: %s", stderr.String())
	}
	readme := readFile(t, "README.md")
	if !strings.Contains(readme, `    neverran: can't trust the input, nothing was reported: plain.txt: line 1 is not go test -json output: "ok  \texample.com/shop/cart\t0.002s"; this looks like go test output without -json. Run go test -json`) {
		t.Error("README's refusal example changed; update this test")
	}
}
