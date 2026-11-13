# 0003: Match runs to source by import path and top-level test name

## Context
CI often runs `go test` several times: per module, with and without tags, on several platforms, or in shards. Each run's output names packages by import path and tests by name.

## Decision
Read every run, and match each package by import path and each top-level test by name. A test counts as run if any run passed it. Module paths come from each `go.mod`, so nested modules and `go.work` workspaces map the same way. Subtests are only counted, because their names come from run time.

## Consequences
Runs from different directories, tags and machines can be combined in one check. Tests generated in CI before testing aren't in the source, so such runs are refused as another revision; generating them first is the workaround.
