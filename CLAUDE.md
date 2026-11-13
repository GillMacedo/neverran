# neverran

Claude Code skill and Go CLI that compares a Go repository's tests with the `go test -json` output of its CI runs, and lists the tests that no run ran or that every run skipped.

## Architecture
`plugin/scripts/neverran.go` holds the CLI and the text report. `plugin/scripts/source.go` walks the tree the way the go command does and finds modules (`loadTree`, `readModulePath`) and the tests go test would run (`readTestFile`, `isTestName`, `hasOutputComment`). `plugin/scripts/run.go` reads `go test -json` strictly (`readRun`). `plugin/scripts/check.go` compares the two and makes the findings (`check`, `notRunFindings`). `plugin/skills/finding-go-tests-ci-never-ran/SKILL.md` tells Claude how to use it. See `docs/architecture.md`.

## Commands
- `make test`: gofmt, vet and tests, including `tests/`, which run the real go command on `examples/shop` and on modules the tests write
- `make lint`: plugin manifest validation (needs Claude Code)
- `make demo`: run on the saved example runs

## Conventions
- Standard library only. Read files only: never run the user's tests, connect to CI, or write files.
- Any input it can't fully read or trust exits 2 with nothing on stdout, whatever `--fail-on` says. So does a run that failed, a run from another project or revision, and a run that checked nothing. Add a case to `badRuns` or `TestBadTrees` in `tests/inputs_test.go`, or to `TestRealFailedRuns` in `tests/live_test.go`, for each new kind of input.
- Which functions count as tests must follow the go command (`cmd/go/internal/load/test.go`, `go/doc` for examples). Check new rules against `go test -list`.
- Tests must not assume the machine they run on: use `go env` or `runtime` for anything platform-specific.

## Do and don't
- Do run `make test` on Go 1.22 and on the current Go release before calling a change done. `GOTOOLCHAIN=go1.22.12 make test` works.
- Don't add `go.mod` requirements to the root module: the installed plugin has no module.
- Don't print "No problems" for a run that checked nothing.
- Don't regenerate `examples/shop/runs/*.json` without updating the README output; `tests/readme_test.go` compares them.

## Definition of done
`make test` and `make lint` pass, README output matches a real run, and `ROADMAP.md` and docs are updated when relevant.
