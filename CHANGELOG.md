# Changelog

All notable changes are documented here, in [Keep a Changelog](https://keepachangelog.com/en/1.1.0/) format. This project uses [Semantic Versioning](https://semver.org/).

## [Unreleased]

## [0.1.0] - 2026-11-13

### Added
- `neverran` CLI and the `finding-go-tests-ci-never-ran` skill: compares a Go repository's tests with `go test -json` output and lists the tests no run ran or every run skipped.
- Findings: `module-not-run` (including nested modules that `go test ./...` skips), `package-not-run`, `file-not-built` (build tags and platform file names), `package-ran-no-tests` (a `TestMain` that exits before `m.Run`), `test-not-run`, `skipped`, `all-subtests-skipped` and `no-subtests`.
- Several run files or stdin, `--dir`, `--ignore`, `--format json` and `--fail-on error|warn|never`.
- Refuses with exit 2 on runs or source trees it can't trust, on failed runs, on runs from another project or revision, and on runs that checked nothing.
