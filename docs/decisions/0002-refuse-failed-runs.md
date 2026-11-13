# 0002: Refuse failed, cut-off and mismatched runs

## Context
Two earlier tools in this family passed their own tests and still reported a failed build, or an unreadable report, as all clear. Here the danger is a run that stopped early: a panic, a timeout or a build failure means later tests never started, and a reader could take the report as "everything that should run, ran".

## Decision
Exit 2 with nothing on stdout, whatever `--fail-on` says, when a run is empty, isn't `go test -json`, is cut off (no final line break, a package without its result, a package starting again while open), lacks the `start` events Go 1.20 added, has a failed build or a failed test, tests packages outside `--dir`, or has tests the source doesn't have. Also when the source tree has an unreadable `go.mod` or `_test.go` file or no tests, and when `--ignore` leaves nothing.

## Consequences
neverran can't be used on a failing CI run; fix the failure first. A run cut exactly between two packages can't be seen in `go test -json`; the lost packages are reported as errors, so the result is never clean. Tests cut both saved runs at every byte to check this.
