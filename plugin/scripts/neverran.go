// Command neverran lists the Go tests in a source tree that no go test -json
// run ran: nested modules that go test ./... skips, files behind build tags,
// packages left out of the pattern, a TestMain that exits early, and tests
// that were skipped in every run.
package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"
)

const version = "0.1.0"

type multiFlag []string

func (m *multiFlag) String() string     { return strings.Join(*m, ",") }
func (m *multiFlag) Set(v string) error { *m = append(*m, v); return nil }

func main() {
	os.Exit(realMain(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}

const usage = `usage: neverran [--dir DIR] [--fail-on error|warn|never] [--format text|json] [--ignore REGEXP]... RUN.json...

Reads the output of one or more go test -json runs (use - for stdin) and lists
the tests in DIR (default .) that none of them ran, or that every run skipped.

Exit codes: 0 nothing at or above --fail-on, 1 findings, 2 input it can't
trust (an empty, cut-off, failed or non-JSON run, or a source tree it can't
read), nothing to check, or bad flags.
`

func realMain(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("neverran", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	dir := fs.String("dir", ".", "source tree to inventory")
	failOn := fs.String("fail-on", "error", "error, warn or never")
	format := fs.String("format", "text", "text or json")
	showVersion := fs.Bool("version", false, "print the version")
	var ignores multiFlag
	fs.Var(&ignores, "ignore", "regexp of package.TestName to leave out (repeatable)")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			fmt.Fprint(stdout, usage)
			return 0
		}
		fmt.Fprintf(stderr, "neverran: %v\n%s", err, usage)
		return 2
	}
	if *showVersion {
		fmt.Fprintf(stdout, "neverran %s\n", version)
		return 0
	}
	if *failOn != "error" && *failOn != "warn" && *failOn != "never" {
		fmt.Fprintf(stderr, "neverran: --fail-on must be error, warn or never, not %q\n", *failOn)
		return 2
	}
	if *format != "text" && *format != "json" {
		fmt.Fprintf(stderr, "neverran: --format must be text or json, not %q\n", *format)
		return 2
	}
	if fs.NArg() == 0 {
		fmt.Fprintf(stderr, "neverran: no go test -json output given\n%s", usage)
		return 2
	}
	var rxs []*regexp.Regexp
	for _, s := range ignores {
		rx, err := regexp.Compile(s)
		if err != nil {
			fmt.Fprintf(stderr, "neverran: --ignore %q: %v\n", s, err)
			return 2
		}
		rxs = append(rxs, rx)
	}
	refuse := func(err error) int {
		fmt.Fprintf(stderr, "neverran: can't trust the input, nothing was reported: %v\n", err)
		return 2
	}
	t, err := loadTree(*dir)
	if err != nil {
		return refuse(err)
	}
	var runs []*run
	usedStdin := false
	for _, name := range fs.Args() {
		var r io.Reader
		if name == "-" {
			if usedStdin {
				fmt.Fprintln(stderr, "neverran: - given twice")
				return 2
			}
			usedStdin = true
			r = stdin
			name = "stdin"
		} else {
			f, err := os.Open(name)
			if err != nil {
				return refuse(err)
			}
			defer f.Close()
			r = f
		}
		rn, err := readRun(name, r)
		if err != nil {
			return refuse(fmt.Errorf("%s: %v", name, err))
		}
		runs = append(runs, rn)
	}
	rep, err := check(t, runs, rxs)
	if err != nil {
		return refuse(err)
	}
	errorsN, warns := 0, 0
	for _, f := range rep.Findings {
		if f.Severity == "ERROR" {
			errorsN++
		} else {
			warns++
		}
	}
	if *format == "json" {
		if rep.Findings == nil {
			rep.Findings = []finding{}
		}
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		enc.Encode(rep)
	} else {
		printText(stdout, rep, errorsN, warns)
	}
	switch {
	case *failOn == "error" && errorsN > 0, *failOn == "warn" && errorsN+warns > 0:
		return 1
	}
	return 0
}

func printText(w io.Writer, rep *report, errorsN, warns int) {
	fmt.Fprintf(w, "neverran %s: %s, %s, %s with tests, %s; %s\n", rep.Version, rep.Dir,
		count(rep.Modules, "module"), count(rep.Packages, "package"), count(rep.Tests, "test"), count(len(rep.Runs), "run"))
	if rep.Ignored > 0 {
		fmt.Fprintf(w, "note: left out by --ignore: %s\n", count(rep.Ignored, "test"))
	}
	if rep.OutsideModule > 0 {
		fmt.Fprintf(w, "note: test files under no go.mod, not checked: %d\n", rep.OutsideModule)
	}
	for _, f := range rep.Findings {
		fmt.Fprintln(w)
		where := f.File
		if f.Line > 0 {
			where = fmt.Sprintf("%s:%d", f.File, f.Line)
		}
		fmt.Fprintf(w, "%s %s  %s  %s\n", f.Severity, f.Code, f.Package, where)
		fmt.Fprintf(w, "  %s\n", wrapList(f.Tests))
		fmt.Fprintf(w, "  %s\n", f.Message)
		fmt.Fprintf(w, "  fix: %s\n", f.Fix)
	}
	fmt.Fprintln(w)
	if len(rep.Findings) == 0 {
		fmt.Fprintf(w, "No problems: all %s ran and passed in at least one run.\n", count(rep.Tests, "test"))
		return
	}
	fmt.Fprintf(w, "%s, %s: %d of %d tests never ran, %d %s skipped everywhere\n",
		count(errorsN, "error"), count(warns, "warning"), rep.NeverRan, rep.Tests, rep.Skipped, plural(rep.Skipped, "was", "were"))
}

func wrapList(names []string) string {
	const max = 8
	if len(names) > max {
		return strings.Join(names[:max], " ") + fmt.Sprintf(" and %d more", len(names)-max)
	}
	return strings.Join(names, " ")
}
