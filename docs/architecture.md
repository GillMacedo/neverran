# Architecture

neverran is one Go program in `plugin/scripts`, using only the standard library, so the installed plugin builds with `go build` and no module download.

## Flow

1. **Source tree (`source.go`).** `loadTree` walks `--dir` with the go command's rules: it skips `testdata`, `vendor`, and names starting with `.` or `_`. Each `go.mod` starts a module; `readModulePath` reads its module path and refuses a file that isn't a whole go.mod. A module inside another module's directory is marked nested. Each `_test.go` file is parsed with `go/parser`; `readTestFile` keeps the functions go test runs: `TestXxx(*testing.T)`, `FuzzXxx(*testing.F)`, and `Example` functions whose last comment starts with `Output:` or `Unordered output:` (the `go/doc` rule). It records the file's `//go:build` expression, any `_GOOS`/`_GOARCH` file name limit, and whether a test calls `t.Run` on its own `*testing.T`. The import path of each package is the module path plus the directory.
2. **Runs (`run.go`).** `readRun` reads `go test -json` line by line. Every line must be a whole JSON object with an `Action` (`go: downloading` lines are allowed). Each package must begin with `start` and end with `pass`, `skip` or `fail`. Any `build-fail`, failed package, open package at the end of the file, or package that starts again while open makes the run untrusted. For each package it keeps whether go test printed `[no test files]`, up to three lines the package printed outside tests, and for each top-level test its result, its `t.Skip` message and its subtest counts.
3. **Comparison (`check.go`).** `check` refuses runs with packages outside the tree or tests the source doesn't have. Then for each test in the source: if no package of its module ran, `module-not-run`; if the package didn't run, `package-not-run`; if the package ran and the test didn't, `file-not-built` when its file has a constraint or platform name, `package-ran-no-tests` when the package ran nothing, otherwise `test-not-run`. A test that ran is fine if it passed in some run, `skipped` if it only skipped, `all-subtests-skipped` if every subtest skipped, and `no-subtests` if it calls `t.Run` but ran none.
4. **Report (`neverran.go`).** Text or JSON, errors first. Exit 0, 1 or 2 by `--fail-on`, and 2 for every refusal.

## Why the go command's rules matter

A test the go command wouldn't run, such as `Testfoo` or an example without an output comment, must not be reported as never run. `tests/examples_test.go` compares neverran's inventory with `go test -list` for the example module.
