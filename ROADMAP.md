# Roadmap

## 0.1.0 (this release)
- List Go tests that no `go test -json` run ran, or that every run skipped, with the reason and the CI change that would run them.

## Next
- Read GitHub Actions workflow files and print the `go test -json` commands to save, one per test step, so setting up the runs takes less work.
- Accept JUnit XML from `go-junit-report` and `gotestsum --junitfile`, for CI systems that keep only JUnit reports.
- A `--baseline` mode: compare with an earlier set of runs and report tests that ran then and don't now.
- Say which build tags each run used, read from the run's package list, so `file-not-built` can name the run that came closest.
- Report benchmarks that no `-bench` run ran, when the user asks for it.

## Not planned
- Running tests. neverran reads the output of runs.
- Connecting to CI providers to download logs or artifacts.
