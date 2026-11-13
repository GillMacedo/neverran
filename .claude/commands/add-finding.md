Add a finding to neverran for: $ARGUMENTS

1. Make the gap with the real go command: write a small module in a test in `tests/live_test.go`, run `go test -json` on it, and check what the events show. Link the go command or `testing` documentation behind the behavior in the PR.
2. Add it to `plugin/scripts/check.go` with a code in kebab case, a message that says which tests didn't run and why, and a fix line that names the CI change.
3. Add the healthy run of the same module, which must stay quiet.
4. Add the code to the table in `README.md` and to the skill's list in `plugin/skills/finding-go-tests-ci-never-ran/SKILL.md`.
5. Run `make test`.
