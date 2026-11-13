package main

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// finding is one problem in the report.
type finding struct {
	Severity string   `json:"severity"` // ERROR or WARN
	Code     string   `json:"code"`
	Package  string   `json:"package"`
	Tests    []string `json:"tests"`
	File     string   `json:"file,omitempty"`
	Line     int      `json:"line,omitempty"`
	Message  string   `json:"message"`
	Fix      string   `json:"fix"`
}

// report is the result of comparing a tree with its runs.
type report struct {
	Version       string    `json:"version"`
	Dir           string    `json:"dir"`
	Modules       int       `json:"modules"`
	Packages      int       `json:"packages"`
	Tests         int       `json:"tests"`
	Runs          []string  `json:"runs"`
	Ran           int       `json:"ran"`
	Skipped       int       `json:"skipped"`
	NeverRan      int       `json:"neverRan"`
	Ignored       int       `json:"ignored"`
	OutsideModule int       `json:"testFilesOutsideModules"`
	Findings      []finding `json:"findings"`
}

// merged is what all runs together did with one test.
type merged struct {
	ran, passed, skipped bool
	skipReason           string
	subtests, subSkipped int
	passedWithSubtests   bool
}

func check(t *tree, runs []*run, ignore []*regexp.Regexp) (*report, error) {
	rep := &report{Version: version, Dir: t.Root, Modules: len(t.Modules), OutsideModule: t.Outside}
	for _, r := range runs {
		rep.Runs = append(rep.Runs, r.Name)
	}
	// Which packages and modules appear in any run, and how.
	pkgSeen := map[string]bool{}
	pkgNoFiles := map[string]bool{}  // [no test files] in every run that had it
	pkgRanTests := map[string]bool{} // ran at least one test in some run
	pkgOutput := map[string][]string{}
	modSeen := map[string]bool{}
	results := map[string]*merged{} // package + "." + test
	matched := 0
	var outside []string
	for _, r := range runs {
		for path, p := range r.Packages {
			m, inTree := moduleFor(t, path)
			if !inTree {
				outside = append(outside, path)
				continue
			}
			matched++
			modSeen[m.Dir] = true
			pkgSeen[path] = true
			if p.NoTestFiles {
				if _, ok := pkgNoFiles[path]; !ok {
					pkgNoFiles[path] = true
				}
			} else {
				pkgNoFiles[path] = false
			}
			if len(p.Tests) > 0 {
				pkgRanTests[path] = true
			} else if len(p.Output) > 0 && pkgOutput[path] == nil {
				pkgOutput[path] = p.Output
			}
			for name, tr := range p.Tests {
				key := path + "." + name
				mg := results[key]
				if mg == nil {
					mg = &merged{}
					results[key] = mg
				}
				mg.ran = true
				switch {
				case tr.Passed:
					mg.passed = true
					mg.subtests += tr.Subtests
					mg.subSkipped += tr.SubSkipped
					if tr.Subtests > tr.SubSkipped {
						mg.passedWithSubtests = true
					}
				case tr.Outcome == "skip":
					mg.skipped = true
					if mg.skipReason == "" {
						mg.skipReason = tr.SkipReason
					}
				}
			}
		}
	}
	if len(outside) > 0 {
		sort.Strings(outside)
		if matched == 0 {
			return nil, fmt.Errorf("none of the packages in the runs is in %s (module %s), for example %s; the runs are from another project or the directory is wrong", t.Root, t.Modules[0].Path, outside[0])
		}
		return nil, fmt.Errorf("the runs tested %s that %s in %s (%s); point --dir at the root of the repository the runs tested", count(len(outside), "package"), plural(len(outside), "isn't", "aren't"), t.Root, listSome(outside))
	}
	if matched == 0 {
		return nil, fmt.Errorf("none of the packages in the runs is in %s (module %s); the runs are from another project or the directory is wrong", t.Root, t.Modules[0].Path)
	}

	inSource := map[string]bool{}
	var tests []*testFunc
	for _, tf := range t.Tests {
		inSource[tf.Package+"."+tf.Name] = true
		if matchesAny(ignore, tf.Package+"."+tf.Name) {
			rep.Ignored++
			continue
		}
		tests = append(tests, tf)
	}
	var missing []string
	for key := range results {
		if !inSource[key] {
			missing = append(missing, key)
		}
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		return nil, fmt.Errorf("the runs have %s that %s in %s (%s): the runs and the source are from different revisions, or a test file is incomplete. Check out the commit the runs tested", count(len(missing), "test"), plural(len(missing), "isn't", "aren't"), t.Root, listSome(missing))
	}
	if len(tests) == 0 {
		if rep.Ignored > 0 {
			return nil, fmt.Errorf("every test in %s matches --ignore, so nothing was checked", t.Root)
		}
		return nil, fmt.Errorf("no tests in %s: no Test, Fuzz, or Example-with-output functions in _test.go files", t.Root)
	}
	rep.Tests = len(tests)
	pkgs := map[string][]*testFunc{}
	var pkgOrder []string
	for _, tf := range tests {
		if _, ok := pkgs[tf.Package]; !ok {
			pkgOrder = append(pkgOrder, tf.Package)
		}
		pkgs[tf.Package] = append(pkgs[tf.Package], tf)
	}
	rep.Packages = len(pkgOrder)

	// Whole modules that no run tested.
	modDone := map[string]bool{}
	for _, path := range pkgOrder {
		ts := pkgs[path]
		m := ts[0].Module
		if modSeen[m.Dir] || modDone[m.Dir] {
			continue
		}
		modDone[m.Dir] = true
		var names []string
		var mpkgs []string
		for _, p := range pkgOrder {
			if pkgs[p][0].Module == m {
				mpkgs = append(mpkgs, p)
				for _, tf := range pkgs[p] {
					names = append(names, shortName(p, m.Path)+tf.Name)
				}
			}
		}
		rep.NeverRan += len(names)
		modFile := "go.mod"
		if m.Dir != "." {
			modFile = m.Dir + "/go.mod"
		}
		msg := fmt.Sprintf("no package of module %s was in any run: %s in %s never ran", m.Path, count(len(names), "test"), count(len(mpkgs), "package"))
		fix := "add a run of go test -json ./... in this module, and pass its output too"
		if m.Nested {
			msg += fmt.Sprintf(". It is a nested module (%s), and go test ./... in the parent module does not enter it", modFile)
			fix = fmt.Sprintf("run go test -json ./... in %s as its own CI step (or list it in go.work and test it from there), and pass that output too", m.Dir)
		}
		rep.Findings = append(rep.Findings, finding{
			Severity: "ERROR", Code: "module-not-run", Package: m.Path, Tests: names, File: modFile,
			Message: msg, Fix: fix,
		})
	}

	for _, path := range pkgOrder {
		ts := pkgs[path]
		m := ts[0].Module
		if !modSeen[m.Dir] {
			continue
		}
		if !pkgSeen[path] {
			rep.NeverRan += len(ts)
			f := finding{
				Severity: "ERROR", Code: "package-not-run", Package: path, Tests: names(ts), File: ts[0].File, Line: ts[0].Line,
				Message: fmt.Sprintf("the package is not in any run, so its %s never ran", count(len(ts), "test")),
				Fix:     "check the package pattern CI passes to go test (./... covers every package below the module root); a narrower pattern leaves packages out",
			}
			if lim := commonLimit(ts); lim != "" {
				f.Message = fmt.Sprintf("the package is not in any run, and every test file in it %s; go test ./... leaves out a package none of whose files match the build, so its %s never ran", lim, count(len(ts), "test"))
				f.Fix = constraintFix(ts[0])
			} else if allLimited(ts) {
				f.Message = fmt.Sprintf("the package is not in any run, and every test file in it has a build constraint or a GOOS/GOARCH file name (%s %s); go test ./... leaves out a package none of whose files match the build, so its %s never ran", ts[0].File, limitText(ts[0]), count(len(ts), "test"))
				f.Fix = "add a CI run for a platform or tags these files build on, and pass its output too"
			}
			rep.Findings = append(rep.Findings, f)
			continue
		}
		var notRun []*testFunc
		for _, tf := range ts {
			if mg := results[path+"."+tf.Name]; mg == nil || !mg.ran {
				notRun = append(notRun, tf)
			}
		}
		if len(notRun) > 0 {
			rep.NeverRan += len(notRun)
			rep.Findings = append(rep.Findings, notRunFindings(path, notRun, len(ts), pkgNoFiles[path], pkgRanTests[path], pkgOutput[path])...)
		}
		for _, tf := range ts {
			mg := results[path+"."+tf.Name]
			if mg == nil || !mg.ran {
				continue
			}
			switch {
			case !mg.passed && mg.skipped:
				rep.Skipped++
				reason := ""
				if mg.skipReason != "" {
					reason = fmt.Sprintf(": %q", mg.skipReason)
				}
				rep.Findings = append(rep.Findings, finding{
					Severity: "WARN", Code: "skipped", Package: path, Tests: []string{tf.Name}, File: tf.File, Line: tf.Line,
					Message: "skipped in every run" + reason,
					Fix:     "give one CI run what the test needs (an environment variable, a service, a flag such as without -short), or delete the test if it no longer applies",
				})
			case mg.passed && mg.subtests > 0 && !mg.passedWithSubtests:
				rep.Skipped++
				rep.Findings = append(rep.Findings, finding{
					Severity: "WARN", Code: "all-subtests-skipped", Package: path, Tests: []string{tf.Name}, File: tf.File, Line: tf.Line,
					Message: fmt.Sprintf("passed, but all %d of its subtests were skipped in every run", mg.subSkipped),
					Fix:     "find the t.Skip in the subtests and give one CI run what they need",
				})
			case mg.passed && tf.CallsRun && mg.subtests == 0:
				rep.Ran++
				rep.Findings = append(rep.Findings, finding{
					Severity: "WARN", Code: "no-subtests", Package: path, Tests: []string{tf.Name}, File: tf.File, Line: tf.Line,
					Message: "passed without running any subtest, although it calls t.Run: its table, or the files it loads its cases from, may be empty",
					Fix:     "check the loop that calls t.Run (an empty slice, a glob that matches no files, or a filter that drops every case), and fail the test when it finds no cases",
				})
			default:
				rep.Ran++
			}
		}
	}
	sort.SliceStable(rep.Findings, func(i, j int) bool {
		return sevRank(rep.Findings[i].Severity) > sevRank(rep.Findings[j].Severity)
	})
	return rep, nil
}

// notRunFindings explains why tests in a package that was in a run didn't
// run, grouping tests from one constrained file.
func notRunFindings(path string, notRun []*testFunc, total int, noFiles, ranAny bool, output []string) []finding {
	var out []finding
	byFile := map[string][]*testFunc{}
	var order []string
	for _, tf := range notRun {
		if _, ok := byFile[tf.File]; !ok {
			order = append(order, tf.File)
		}
		byFile[tf.File] = append(byFile[tf.File], tf)
	}
	var plain []*testFunc
	for _, file := range order {
		ts := byFile[file]
		tf := ts[0]
		limit := limitText(tf)
		if limit == "" {
			plain = append(plain, ts...)
			continue
		}
		out = append(out, finding{
			Severity: "ERROR", Code: "file-not-built", Package: path, Tests: names(ts), File: tf.File, Line: tf.Line,
			Message: fmt.Sprintf("the file %s, and no run's build included it, so %s never ran", limit, count(len(ts), "test")),
			Fix:     constraintFix(tf),
		})
	}
	if len(plain) == 0 {
		return out
	}
	tf := plain[0]
	switch {
	case noFiles:
		out = append(out, finding{
			Severity: "ERROR", Code: "test-not-run", Package: path, Tests: names(plain), File: tf.File, Line: tf.Line,
			Message: fmt.Sprintf("go test reported [no test files] for this package, so %s never ran", count(len(plain), "test")),
			Fix:     "the run's build left out these files; check the package clause and build settings of the run",
		})
	case !ranAny && len(plain) == total:
		said := ""
		if len(output) > 0 {
			said = fmt.Sprintf("; it printed %q", strings.Join(output, " "))
		}
		out = append(out, finding{
			Severity: "ERROR", Code: "package-ran-no-tests", Package: path, Tests: names(plain), File: tf.File, Line: tf.Line,
			Message: fmt.Sprintf("the package passed without running any of its %s%s", count(total, "test"), said),
			Fix:     "a TestMain that returns or calls os.Exit before m.Run() does this, as do -run or -skip patterns that match nothing here; make TestMain fail, or call m.Run, when what the tests need is missing",
		})
	default:
		out = append(out, finding{
			Severity: "ERROR", Code: "test-not-run", Package: path, Tests: names(plain), File: tf.File, Line: tf.Line,
			Message: fmt.Sprintf("the package ran, but %s never ran", plural(len(plain), "this test", "these tests")),
			Fix:     "check the -run and -skip patterns of each run, and any sharding script that splits tests by name",
		})
	}
	return out
}

// limitText describes a file's build constraint and file name limit.
func limitText(tf *testFunc) string {
	switch {
	case tf.Constraint != "" && tf.FileOS != "":
		return fmt.Sprintf("has //go:build %s and a name that limits it to %s", tf.Constraint, tf.FileOS)
	case tf.Constraint != "":
		return "has //go:build " + tf.Constraint
	case tf.FileOS != "":
		return "has a name that limits it to " + tf.FileOS
	}
	return ""
}

// commonLimit returns the limit shared by every test file of a package, or
// "" when some file has none or the files differ.
func commonLimit(ts []*testFunc) string {
	lim := limitText(ts[0])
	for _, tf := range ts[1:] {
		if limitText(tf) != lim {
			return ""
		}
	}
	return lim
}

func allLimited(ts []*testFunc) bool {
	for _, tf := range ts {
		if limitText(tf) == "" {
			return false
		}
	}
	return true
}

func constraintFix(tf *testFunc) string {
	tags := tagsIn(tf.Constraint)
	var oses []string
	for _, w := range tagRx.FindAllString(tf.Constraint, -1) {
		if (contains(knownOS, w) || contains(knownArch, w)) && !contains(oses, w) {
			oses = append(oses, w)
		}
	}
	switch {
	case len(tags) > 0 && tf.FileOS == "":
		return fmt.Sprintf("add a CI run with -tags %s and pass its output too, or remove the constraint if the tests should always run", strings.Join(tags, ","))
	case tf.FileOS != "":
		return fmt.Sprintf("add a CI run on %s (GOOS/GOARCH) and pass its output too", tf.FileOS)
	case len(oses) > 0 && !strings.Contains(tf.Constraint, "!"):
		return fmt.Sprintf("add a CI run on %s (GOOS/GOARCH) and pass its output too", strings.Join(oses, " or "))
	}
	return "add a CI run whose build satisfies the constraint, and pass its output too"
}

var tagRx = regexp.MustCompile(`[A-Za-z0-9_.]+`)

// tagsIn lists the custom tags in a constraint, leaving out GOOS, GOARCH,
// go1.N, cgo and "ignore".
func tagsIn(expr string) []string {
	var tags []string
	seen := map[string]bool{}
	for _, w := range tagRx.FindAllString(expr, -1) {
		if contains(knownOS, w) || contains(knownArch, w) || strings.HasPrefix(w, "go1.") || w == "cgo" || w == "ignore" || w == "unix" || w == "gc" || w == "gccgo" || seen[w] {
			continue
		}
		seen[w] = true
		tags = append(tags, w)
	}
	return tags
}

// moduleFor finds the tree's module that owns an import path: the module
// with the longest matching path.
func moduleFor(t *tree, path string) (*module, bool) {
	if m, ok := t.Packages[path]; ok {
		return m, true
	}
	var best *module
	for _, m := range t.Modules {
		if path == m.Path || strings.HasPrefix(path, m.Path+"/") {
			if best == nil || len(m.Path) > len(best.Path) {
				best = m
			}
		}
	}
	return best, best != nil
}

func matchesAny(rxs []*regexp.Regexp, s string) bool {
	for _, rx := range rxs {
		if rx.MatchString(s) {
			return true
		}
	}
	return false
}

func shortName(pkg, mod string) string {
	rest := strings.TrimPrefix(strings.TrimPrefix(pkg, mod), "/")
	if rest == "" {
		return ""
	}
	return rest + "."
}

func names(ts []*testFunc) []string {
	var out []string
	for _, tf := range ts {
		out = append(out, tf.Name)
	}
	return out
}

func count(n int, word string) string {
	if n == 1 {
		return "1 " + word
	}
	return fmt.Sprintf("%d %ss", n, word)
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

func sevRank(s string) int {
	if s == "ERROR" {
		return 2
	}
	return 1
}
