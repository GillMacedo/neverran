---
name: finding-go-tests-ci-never-ran
description: Lists the Go tests in a repository that no CI run ran, by comparing the source tree with go test -json output. Finds nested modules that go test ./... doesn't enter, test files behind build tags (//go:build integration) or for other platforms (_windows_test.go), packages left out of the go test pattern, a TestMain that exits before m.Run so the package passes with nothing run, -run and -skip filters, tests skipped in every run (t.Skip when an environment variable is missing, testing.Short), subtests all skipped, and table tests that ran no cases. Use when CI is green but someone asks whether the tests really ran, when adding integration tests or a nested module, when setting up or changing a CI test step, and when a bug shipped that a test should have caught.
---

# Finding Go tests CI never ran

neverran reads the source tree (every `go.mod` and `_test.go` file) and one or more `go test -json` outputs, and lists each test that none of the runs ran, or that every run skipped. It reads files only: it doesn't run tests, and it needs no network. It needs Go 1.22 or newer.

## Get the inputs

1. **The source tree** at the commit CI tested: `--dir` (default `.`). Use the repository root, so nested modules are included.
2. **The `go test -json` output of every CI test step for that commit**, one file per step. Ask the user how CI runs tests, or read the workflow files (`.github/workflows/*.yml`, `Makefile`, scripts) to find each `go test` command, its directory, tags and environment.
   - If CI already saves `go test -json` output (for example as an artifact, or through `gotestsum --jsonfile`), use those files.
   - Otherwise run each CI command locally with `-json` added and its output redirected: `go test -json ./... > run-root.json`. Keep the same directory, `-tags`, `-run`, `-skip`, `-short` and environment variables as CI, or the result describes your machine instead of CI. Say which commands you ran.
   - Don't merge stderr into the file unless needed; neverran accepts `go: downloading` lines but refuses other text.

## Run it

Build it once, then run it with every run file:

    go build -o "${TMPDIR:-/tmp}/neverran" $(ls "${CLAUDE_PLUGIN_ROOT}"/scripts/*.go | grep -v _test.go)
    "${TMPDIR:-/tmp}/neverran" --dir . run-root.json run-tools.json

Add `--format json` for machine-readable output, `--fail-on warn` to fail on warnings too, and `--ignore 'REGEXP'` (matched against `import/path.TestName`) for tests the user says are meant to be left out. Use `-` to read one run from stdin.

## Read the exit code first

- **0:** nothing at or above `--fail-on` (default `error`). With no findings the last line is `No problems: all N tests ran and passed in at least one run.`
- **1:** findings at or above `--fail-on`.
- **2:** nothing was reported. The message says why: a run that is empty, cut off, not `go test -json` output (plain `go test` output, build errors, an error page), or a run where a build or a test failed; runs whose packages or tests aren't in `--dir` (another project, or another revision); a `go.mod` or `_test.go` file that can't be read; or no tests to check. Fix the input and run again. Never describe exit 2 as a clean result, and never treat a failed run as "the tests ran".

## Check each finding

- **ERROR `module-not-run`:** no package of a module was in any run. For a nested module (`tools/go.mod`), `go test ./...` in the parent doesn't enter it. Fix: a CI step that runs `go test ./...` inside that module.
- **ERROR `package-not-run`:** the module was tested but this package wasn't: the pattern was narrower than `./...`, or every file of the package is for another platform or behind tags.
- **ERROR `file-not-built`:** the test file has a `//go:build` constraint or a `_GOOS`/`_GOARCH` file name that no run's build matched. The fix names the `-tags` or platform to add.
- **ERROR `package-ran-no-tests`:** the package passed but ran none of its tests. Usually a `TestMain` that calls `os.Exit(0)` before `m.Run()` when a database URL or fixture isn't set; the message quotes what the package printed. `-run` or `-skip` patterns can do the same.
- **ERROR `test-not-run`:** the package ran, but these tests didn't: `-run`, `-skip`, or a sharding script.
- **WARN `skipped`:** skipped in every run; the message quotes the `t.Skip` reason.
- **WARN `all-subtests-skipped`:** the test passed, but every subtest skipped.
- **WARN `no-subtests`:** the test calls `t.Run` but ran no subtests: an empty table or a glob that matched no files.

## Report

1. Errors first: which tests never ran, why, and the CI change that would run them (a step, a tag, an environment variable, a platform).
2. Then warnings, with the skip reasons.
3. Say which runs you checked and how you got them (CI artifacts, or commands you ran locally that copy CI's).

## Don't

Don't say the tests are good because neverran found nothing: it checks that each test ran and passed somewhere, not what the tests assert. Use runs from the same commit as `--dir`: it refuses runs with tests the source doesn't have, and a test added to the source after the runs is reported as never run. Don't remove build constraints or skips on your own; ask the user, because some are deliberate.
