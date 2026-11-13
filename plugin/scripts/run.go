package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
)

// event is one line of go test -json output (cmd/test2json).
type event struct {
	Action      string
	Package     string
	Test        string
	Output      string
	ImportPath  string
	FailedBuild string
}

// testResult is what one run did with one top-level test.
type testResult struct {
	Outcome    string // pass, skip or fail: the worst result when -count repeats it
	Passed     bool   // passed at least once
	SkipReason string
	Subtests   int
	SubSkipped int
}

// pkgResult is what one run did with one package.
type pkgResult struct {
	Outcome     string // pass, skip or fail
	NoTestFiles bool
	Output      []string // lines the package printed outside any test
	Tests       map[string]*testResult
}

// run is one go test -json file.
type run struct {
	Name     string
	Packages map[string]*pkgResult
	Lines    int
}

// readRun reads go test -json output strictly. Anything that isn't a whole,
// passing run from Go 1.20 or newer is an error: an empty file, text that
// isn't test2json events, a run cut off before a package's result, and a run
// with a failed build or a failed test.
func readRun(name string, r io.Reader) (*run, error) {
	br := bufio.NewReader(r)
	out := &run{Name: name, Packages: map[string]*pkgResult{}}
	open := map[string]*pkgResult{}  // packages started and not finished
	running := map[string][]string{} // per package, output lines of each test
	var buildFails, failed []string
	lineNo := 0
	for {
		line, err := br.ReadBytes('\n')
		if len(line) == 0 && err == io.EOF {
			break
		}
		if err != nil && err != io.EOF {
			return nil, err
		}
		lineNo++
		if err == io.EOF {
			return nil, fmt.Errorf("line %d has no line break at the end; the file looks cut off", lineNo)
		}
		text := bytes.TrimRight(line, "\r\n")
		if len(bytes.TrimSpace(text)) == 0 {
			continue
		}
		if bytes.HasPrefix(text, []byte("go: downloading ")) {
			continue
		}
		if text[0] != '{' {
			return nil, fmt.Errorf("line %d is not go test -json output: %q%s", lineNo, clip(string(text), 70), notJSONHint(string(text)))
		}
		var ev event
		dec := json.NewDecoder(bytes.NewReader(text))
		if err := dec.Decode(&ev); err != nil {
			return nil, fmt.Errorf("line %d is not a whole JSON object (%v); the file may be cut off or not go test -json output", lineNo, err)
		}
		if dec.More() {
			return nil, fmt.Errorf("line %d holds more than one JSON value", lineNo)
		}
		if ev.Action == "" {
			return nil, fmt.Errorf("line %d is JSON without an Action field, so it isn't go test -json output", lineNo)
		}
		out.Lines++
		switch ev.Action {
		case "build-output":
			continue
		case "build-fail":
			buildFails = append(buildFails, ev.ImportPath)
			continue
		}
		if ev.Package == "" {
			if ev.ImportPath != "" {
				continue // build events from newer Go versions
			}
			return nil, fmt.Errorf("line %d: a %q event with no Package", lineNo, ev.Action)
		}
		p := open[ev.Package]
		if ev.Action == "start" {
			if p != nil {
				return nil, fmt.Errorf("line %d: package %s starts again before its first run finished; the first run looks cut off. Pass each run as its own file", lineNo, ev.Package)
			}
			p = out.Packages[ev.Package]
			if p == nil {
				p = &pkgResult{Tests: map[string]*testResult{}}
				out.Packages[ev.Package] = p
			}
			p.Outcome = ""
			open[ev.Package] = p
			continue
		}
		if p == nil {
			return nil, fmt.Errorf("line %d: a %q event for %s before its \"start\" event. neverran needs the complete output of go test -json from Go 1.20 or newer", lineNo, ev.Action, ev.Package)
		}
		if ev.Test == "" {
			switch ev.Action {
			case "output":
				if strings.Contains(ev.Output, "[no test files]") {
					p.NoTestFiles = true
				}
			case "pass", "skip", "fail":
				p.Outcome = ev.Action
				delete(open, ev.Package)
				if ev.Action == "fail" {
					if ev.FailedBuild != "" || strings.Contains(strings.Join(running[ev.Package+"\x00"], ""), "[build failed]") {
						buildFails = append(buildFails, ev.Package)
					} else {
						failed = append(failed, ev.Package)
					}
				}
				for t, tr := range p.Tests {
					if tr.Outcome == "" && ev.Action != "fail" {
						return nil, fmt.Errorf("package %s finished but test %s has no result; the output is incomplete", ev.Package, t)
					}
				}
			}
			if ev.Action == "output" {
				running[ev.Package+"\x00"] = append(running[ev.Package+"\x00"], ev.Output)
				if l := strings.TrimSpace(ev.Output); l != "" && !isFrame(l) && len(p.Output) < 3 {
					p.Output = append(p.Output, clip(l, 100))
				}
			}
			continue
		}
		top, sub, isSub := strings.Cut(ev.Test, "/")
		_ = sub
		tr := p.Tests[top]
		key := ev.Package + "\x00" + ev.Test
		switch ev.Action {
		case "run":
			if tr == nil {
				tr = &testResult{}
				p.Tests[top] = tr
			}
			if !isSub {
				tr.Outcome = ""
			}
			running[key] = nil
		case "output":
			running[key] = append(running[key], ev.Output)
		case "pass", "skip", "fail":
			if tr == nil {
				// A result without a run event happens for tests that
				// time out or for odd harnesses; record it anyway.
				tr = &testResult{}
				p.Tests[top] = tr
			}
			if isSub {
				tr.Subtests++
				if ev.Action == "skip" {
					tr.SubSkipped++
				}
			} else {
				tr.Outcome = mergeOutcome(tr.Outcome, ev.Action)
				if ev.Action == "pass" {
					tr.Passed = true
				}
				if ev.Action == "skip" && tr.SkipReason == "" {
					tr.SkipReason = skipReason(running[key])
				}
			}
			delete(running, key)
		}
	}
	if len(buildFails) > 0 {
		return nil, fmt.Errorf("the run has a failed build (%s). A package that doesn't build runs none of its tests; fix the build and check a passing run", listSome(buildFails))
	}
	if len(failed) > 0 {
		sort.Strings(failed)
		return nil, fmt.Errorf("the run failed (%s). A failed run may have stopped before some tests ran; check a passing run", listSome(failed))
	}
	if len(open) > 0 {
		var names []string
		for n := range open {
			names = append(names, n)
		}
		sort.Strings(names)
		return nil, fmt.Errorf("the run is cut off: %s started but has no result", listSome(names))
	}
	if len(out.Packages) == 0 {
		if out.Lines == 0 {
			if lineNo == 0 {
				return nil, fmt.Errorf("the file is empty")
			}
			return nil, fmt.Errorf("the file has no go test -json events")
		}
		return nil, fmt.Errorf("no package was tested in this run")
	}
	return out, nil
}

// mergeOutcome keeps the worst result when -count runs a test more than once.
func mergeOutcome(old, new string) string {
	rank := map[string]int{"": 0, "pass": 1, "skip": 2, "fail": 3}
	if rank[new] > rank[old] {
		return new
	}
	return old
}

// skipReason picks the t.Skip message from a test's output: the indented
// lines, without the "=== RUN" and "--- SKIP" frames.
func skipReason(lines []string) string {
	var msg []string
	for _, l := range lines {
		t := strings.TrimSpace(l)
		if t == "" || strings.HasPrefix(t, "=== ") || strings.HasPrefix(t, "--- ") {
			continue
		}
		// "    foo_test.go:12: message"
		if i := strings.Index(t, ".go:"); i >= 0 {
			rest := t[i+4:]
			if j := strings.Index(rest, ": "); j >= 0 {
				t = rest[j+2:]
			}
		}
		msg = append(msg, t)
	}
	return clip(strings.Join(msg, " "), 120)
}

// isFrame reports whether a package output line is go test's own summary.
func isFrame(l string) bool {
	for _, p := range []string{"ok ", "ok\t", "PASS", "FAIL", "?   ", "=== ", "--- ", "coverage:"} {
		if strings.HasPrefix(l, p) {
			return true
		}
	}
	return false
}

func notJSONHint(line string) string {
	switch {
	case strings.HasPrefix(line, "ok ") || strings.HasPrefix(line, "ok\t") || strings.HasPrefix(line, "--- ") || strings.HasPrefix(line, "=== ") || strings.HasPrefix(line, "PASS") || strings.HasPrefix(line, "FAIL") || strings.HasPrefix(line, "?"):
		return "; this looks like go test output without -json. Run go test -json"
	case strings.HasPrefix(line, "#"):
		return "; this looks like build errors, so the run didn't build"
	case strings.HasPrefix(line, "go: "):
		return "; the go command failed before running tests"
	case strings.HasPrefix(line, "panic:"):
		return "; a test binary panicked"
	}
	return ""
}

func listSome(xs []string) string {
	seen := map[string]bool{}
	var uniq []string
	for _, x := range xs {
		if !seen[x] {
			seen[x] = true
			uniq = append(uniq, x)
		}
	}
	if len(uniq) > 3 {
		return strings.Join(uniq[:3], ", ") + fmt.Sprintf(" and %d more", len(uniq)-3)
	}
	return strings.Join(uniq, ", ")
}
