# neverran: find the Go tests your CI never ran

[![CI](https://github.com/GillMacedo/neverran/actions/workflows/ci.yml/badge.svg)](https://github.com/GillMacedo/neverran/actions/workflows/ci.yml) [![MIT license](https://img.shields.io/badge/license-MIT-blue)](LICENSE)

**A Claude Code skill and Go CLI for Go developers and CI maintainers. It compares your repository's tests with the `go test -json` output of your CI runs and lists every test that no run ran, or that every run skipped: nested modules that `go test ./...` doesn't enter, files behind `//go:build integration` or for other platforms, packages left out of the pattern, a `TestMain` that exits before `m.Run`, and `t.Skip` calls that fire every time.**

A green `go test ./...` says the tests that ran passed. It doesn't say which tests ran. Go leaves tests out without a word: `./...` stops at a nested `go.mod`, so a `tools/` module is never tested; a file with `//go:build integration` isn't compiled without `-tags integration`; a `TestMain` that calls `os.Exit(0)` when a database URL is missing makes the package print `ok` with nothing run; a test that calls `t.Skip` when an environment variable isn't set is skipped on every CI run, and the summary still says `ok`. neverran reads the source tree and the runs, and names each test that fell through, with the reason and the change that would run it.

It reads files only. It doesn't run your tests, connect to CI, or use the network.

## What you can ask Claude

- "CI is green. Did our integration tests actually run?"
- "Which Go tests in this repo has CI never run?"
- "I added a `tools/` module with its own go.mod. Is CI testing it?"
- "Here is the go test -json output from CI. Which tests were skipped, and why?"
- "We split tests across shards with -run. Did every test land in a shard?"

## Install

As a Claude Code plugin:

    claude plugin marketplace add GillMacedo/neverran
    claude plugin install neverran@neverran-marketplace

Or run it with Go 1.22 or newer. It has no dependencies outside this repository:

    go test -json ./... > run.json
    go run github.com/GillMacedo/neverran/plugin/scripts@v0.1.0 --dir . run.json

## Example

`examples/shop` is a made-up module with four common gaps: a nested `tools` module, an integration test file behind `//go:build integration`, a `search` package whose `TestMain` exits when its fixtures aren't configured, and a payments test that skips without a sandbox variable. `examples/shop/runs/ci.json` is the real output of the usual CI step, `go test -json ./...` at the module root. `--fail-on never` keeps the exit code at 0 for this demo; without it, the run exits 1:

    $ go run ./plugin/scripts --fail-on never --dir examples/shop examples/shop/runs/ci.json
    neverran 0.1.0: examples/shop, 2 modules, 5 packages with tests, 10 tests; 1 run

    ERROR module-not-run  example.com/shop/tools  tools/go.mod
      lint.TestValidSKU lint.ExampleValidSKU
      no package of module example.com/shop/tools was in any run: 2 tests in 1 package never ran. It is a nested module (tools/go.mod), and go test ./... in the parent module does not enter it
      fix: run go test -json ./... in tools as its own CI step (or list it in go.work and test it from there), and pass that output too

    ERROR package-ran-no-tests  example.com/shop/search  search/search_test.go:29
      TestFindIgnoresCase TestFindNothing
      the package passed without running any of its 2 tests; it printed "SHOP_SEARCH_FIXTURES not set, skipping search tests"
      fix: a TestMain that returns or calls os.Exit before m.Run() does this, as do -run or -skip patterns that match nothing here; make TestMain fail, or call m.Run, when what the tests need is missing

    ERROR file-not-built  example.com/shop/store  store/store_integration_test.go:10
      TestSaveAndLoad TestLoadCorruptFile
      the file has //go:build integration, and no run's build included it, so 2 tests never ran
      fix: add a CI run with -tags integration and pass its output too, or remove the constraint if the tests should always run

    WARN skipped  example.com/shop/payments  payments/payments_test.go:14
      TestRefundAgainstSandbox
      skipped in every run: "SHOP_PAYMENTS_SANDBOX not set"
      fix: give one CI run what the test needs (an environment variable, a service, a flag such as without -short), or delete the test if it no longer applies

    3 errors, 1 warning: 6 of 10 tests never ran, 1 was skipped everywhere

`runs/full.json` is the same module run with `-tags integration` and both environment variables set, and `runs/tools.json` is `go test -json ./...` inside `tools`. Together they run every test, and neverran stays quiet:

    $ go run ./plugin/scripts --dir examples/shop examples/shop/runs/full.json examples/shop/runs/tools.json
    neverran 0.1.0: examples/shop, 2 modules, 5 packages with tests, 10 tests; 2 runs

    No problems: all 10 tests ran and passed in at least one run.

Add `--format json` for machine-readable output. `examples/README.md` has the commands that made each run.

## In CI

Save the output of each test step, then check them together in a last step:

    - run: go test -json ./... > run-root.json
    - run: go test -json -tags integration ./... > run-integration.json
    - run: cd tools && go test -json ./... > ../run-tools.json
    - run: go run github.com/GillMacedo/neverran/plugin/scripts@v0.1.0 run-root.json run-integration.json run-tools.json

A test only needs to run in one of the files. `-json` doesn't change what `go test` does or its exit code. `gotestsum --jsonfile` writes the same events, so its file works too.

## What it reads

- **The source tree (`--dir`, default `.`):** every `go.mod`, and every `_test.go` file in it, skipping `testdata`, `vendor`, and directories starting with `.` or `_`, as the go command does. A test is a `Test` or `Fuzz` function with the right signature, or an `Example` with an output comment, by the go command's rules. Build constraints and `_GOOS`/`_GOARCH` file names are read to explain why a file was left out.
- **Runs (one or more files, or `-` for stdin):** the output of `go test -json` from Go 1.20 or newer. Each file is one `go test` invocation. Packages and tests are matched by import path and name, so runs from different directories, modules, tags and machines can be mixed.

## What it reports

| Code | Severity | Meaning |
|---|---|---|
| `module-not-run` | ERROR | No package of this module was in any run. For a nested module, `go test ./...` in the parent doesn't enter it |
| `package-not-run` | ERROR | The module was tested, but this package wasn't in any run: the pattern was narrower, or every file in it is for another platform or behind tags |
| `file-not-built` | ERROR | The test file has a `//go:build` constraint or a platform file name that no run's build matched. The fix names the tag or platform |
| `package-ran-no-tests` | ERROR | The package passed without running any test: a `TestMain` that exits before `m.Run`, or `-run`/`-skip` that matched nothing in it. It quotes what the package printed |
| `test-not-run` | ERROR | The package ran, but these tests didn't: `-run`, `-skip` or a sharding script left them out |
| `skipped` | WARN | Skipped in every run that reached it. The `t.Skip` message is quoted |
| `all-subtests-skipped` | WARN | Passed, but every subtest skipped in every run |
| `no-subtests` | WARN | Calls `t.Run` but ran no subtest: an empty table, or a glob that matched no files |

A test counts as run when at least one run passed it. `--ignore REGEXP` (repeatable) leaves out tests whose `import/path.TestName` matches, and the header says how many.

## Exit codes

| Code | Meaning |
|---|---|
| 0 | Nothing at or above `--fail-on`, which is `error` by default. Use `--fail-on warn` to fail on warnings too, or `never`. |
| 1 | Findings at or above `--fail-on`. |
| 2 | The input can't be trusted, nothing could be checked, or the flags are wrong. Stdout is empty. |

## When it refuses

neverran exits 2 and prints only the reason when it can't trust what it read, whatever `--fail-on` says. For example, a run saved without `-json`:

    (cd examples/shop && go test ./... > ../../plain.txt)
    go run ./plugin/scripts --dir examples/shop plain.txt

prints this on stderr. The program exits 2 (under `go run`, Go adds a line `exit status 2` and itself exits 1):

    neverran: can't trust the input, nothing was reported: plain.txt: line 1 is not go test -json output: "ok  \texample.com/shop/cart\t0.002s"; this looks like go test output without -json. Run go test -json

For runs it does the same for an empty file, text, an HTML or curl error, build or vet errors, a go command error, a file cut off mid-line or before a package's result, a file without a final line break, events without the `start` event Go 1.20 added, and any run in which a build or a test failed, including panics, timeouts, and a `TestMain` that exits 1. A failed run may have stopped before some tests ran, so it can't show what CI covers. It also refuses runs that test packages outside `--dir` or tests that aren't in the source, which means the runs come from another project or another commit. For the source tree: a missing or empty directory, no `go.mod`, a `go.mod` that is empty, cut off or not a go.mod, a `_test.go` file that doesn't parse, and a tree with no tests. A single bad file among good ones fails the whole run.

One kind of cut can't be seen: `go test -json` has no end marker, so a run cut exactly after one package's last line, with no package left open, looks complete. The packages it lost are then reported as `module-not-run` or `package-not-run` errors, so the result is never clean.

## Limits

- **It needs the runs.** neverran reads `go test -json` output; it doesn't read workflow files to work out what CI would run. Run it on files CI saved, or on runs made locally with the same commands, tags and environment as CI.
- **Ran is not tested.** A test that ran and passed counts, whatever it checks. neverran doesn't measure coverage or assertions.
- **Top-level tests.** Subtests are created at run time, so neverran checks them only as counts: all skipped, or none at all when the test calls `t.Run`.
- **Benchmarks** are left out, because `go test` runs them only with `-bench`.
- **Generated or deleted files.** Tests in files that CI generates before testing aren't in the source tree, so a run that has them is refused as being from another revision. Generate them before running neverran.
- **Same commit.** Use runs from the commit in `--dir`. A test added to the source after the runs is reported as never run.

## FAQ

**Why doesn't `go test ./...` run the tests in a subdirectory with its own go.mod?** A directory with a `go.mod` is a separate module, and `./...` only matches packages of the current module. Run `go test ./...` inside each module, or list them in a loop in CI.

**How do I run tests behind `//go:build integration`?** Pass `-tags integration` to `go test`. Without it, Go doesn't compile the file, and `go test` says nothing about it.

**Why does my package print `ok` when its tests didn't run?** A `TestMain` that calls `os.Exit(0)` or returns before `m.Run()` ends the test binary with success. `-run` or `-skip` patterns that match nothing in a package also end in `ok`, with `[no tests to run]` after it in recent Go versions. neverran reports both as `package-ran-no-tests`.

**How do I see which Go tests were skipped in CI?** Save `go test -json` output and pass it to neverran: it lists each test skipped in every run, with the `t.Skip` message.

**Does it work with gotestsum?** Yes: `gotestsum --jsonfile run.json` saves the `go test -json` events. The format was tested with output from `go test -json`, not from gotestsum.

**Is it an MCP server?** No. It's a skill with one Go program, so `go run` or `go build` is all it needs.

## Tested on

- **Real `go test -json` output from Go 1.22.12 and Go 1.27.3,** on linux/arm64. The tests run `go test -json` on a copy of `examples/shop` with the Go running them, and check that the findings match the saved runs; CI runs them on linux/amd64 with Go 1.22 and the current release.
- **Against the go command's own list:** a test compares the tests neverran finds in `examples/shop` with `go test -list` output. By hand, on golang.org/x/net at e44ac12 with Go 1.27.3 and `-short`, `go test -list` found 953 tests, the run had 953, and neverran counted 1,330 in the source: 916 ran, 37 skipped every time, and 377 never ran. The 377 are in two packages built only on solaris, darwin or the BSDs, and 11 http2 test files with `//go:build !(go1.27 && !http2legacy)`, which Go 1.27 leaves out unless `-tags http2legacy` is set.
- **go.uber.org/zap at 4892335, by hand:** `go test -json ./...` at the root left out 14 tests in two nested modules (`exp` and `zapgrpc/internal/test`), which zap's own Makefile tests separately. With a run per module, as the Makefile does, the only finding was `TestWindowsPaths` in `sink_windows_test.go`, which runs only on Windows.
- **github.com/jackc/pgx at 3843a26, by hand:** without a database, `go test -json ./...` fails, and neverran refused the run with exit 2.
- **Gaps made with the real go command:** `-short`, `-run`, `-skip`, a single package pattern, a platform file chosen from `go env GOOS`, a `go.work` workspace, an empty table test, and the healthy run for each that must stay quiet.
- **Input it can't trust:** tests pass 19 kinds of bad run (alone and next to good runs), 9 kinds of failed run made by the go command (a failing test, build and vet errors, panics, a timeout, `TestMain` exiting 1, `os.Exit` in a test), 6 kinds of non-JSON go command output, and 15 kinds of bad source tree, under every `--fail-on` value and both formats, and check for exit 2 with nothing on stdout. Others cut both saved runs at every byte (a cut is refused, or reported as lost packages, never clean) and the example's test files at every byte (refused, unless only trailing white space was cut).
- **The installed plugin:** a test copies `plugin/` out of the repository and builds it with the skill's own command.

Not tested: macOS and Windows; `go test -json` output saved by GitHub Actions or gotestsum (only output from the go command itself); repositories with generated test files; a fresh Claude Code session using the skill. The `go run ...@v0.1.0` lines work only once the repository is published.

## Contributing

Contributions are welcome, from people and from people working with AI agents. See [CONTRIBUTING.md](CONTRIBUTING.md). Good first issues are [listed here](https://github.com/GillMacedo/neverran/issues?q=is%3Aissue+is%3Aopen+label%3A%22good+first+issue%22).

### How pull requests work

1. **Open a PR.** Fork the repo, work on a branch, and keep it to one change. Write a title that says what changes for users, because it becomes the changelog line: `feat:` goes under Added and `fix:` under Fixed.
2. **Automatic check.** Code changes need tests, and `make test` must pass. Changes over 30 lines need a linked open issue (`Closes #<number>`), and a PR can have at most 800 changed lines. If something is missing, a comment tells you what to fix.
3. **Review.** Claude reviews each PR automatically. Code PRs also get a security review and are tested in a sandbox. The review then merges the PR or asks for changes, and a small problem may become a follow-up issue instead. A PR with no new commit 14 days after a change request is closed, and you can reopen it. PRs that change CI, agent settings, npm or Python dependencies, vendored code or build files can't be merged this way, and a comment says so. The same goes for anything the security review doesn't clear.
4. **Release.** A merge that changes what users install becomes a new version within about two hours, with a GitHub Release and, if the project has one, its npm package. Docs and CI changes wait for the next release.

## AI disclosure

This project was largely generated with Claude Code and reviewed by its maintainer.

## About me

I'm Giulliano Macedo, and I build developer tools at GoTech Solutions. Follow along or get in touch:

- GitHub: [GillMacedo](https://github.com/GillMacedo)
- LinkedIn: [giulliano](https://www.linkedin.com/in/giulliano/)
- X: [@giumacedo](https://x.com/giumacedo)

If `neverran` saved you some prompts, a star helps others find it.

---

Built by [Giulliano Macedo](https://github.com/GillMacedo), GoTech Solutions · [LinkedIn](https://www.linkedin.com/in/giulliano/) · [X](https://x.com/giumacedo)
