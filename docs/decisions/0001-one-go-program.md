# 0001: One Go program in a skill

## Context
The tool reads files and prints a report. Agents can run a program from a skill; nothing here needs a long-running server.

## Decision
Ship a Claude Code skill with one standard-library Go program in `plugin/scripts`, built by the skill with `go build`. No MCP server, no dependencies. Go users already have the toolchain, and `go/parser` and `go/build/constraint` read test files exactly.

## Consequences
The plugin has no `go.mod`, so the root module can't add requirements. The same program runs in CI with `go run github.com/GillMacedo/neverran/plugin/scripts@v0.1.0`.
