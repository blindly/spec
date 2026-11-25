---
covers: [config.go, commit.go, prepush.go, commands.go, main.go, help.go, completion.go]
created: 2026-11-25
id: FEAT-041
status: implemented
title: Pre-commit checks
type: feature
depends_on: [FEAT-037, FEAT-036]
---

# Pre-commit checks

## Summary

`pre_commit:` in `.spec/config.yml` declares cheap checks (`ruff check`,
`biome check`, `tsc --noEmit`, `gofmt -l`) that `spec commit` runs before
staging — and a spec-managed `pre-commit` git hook runs for plain
`git commit` too. The full suite stays in `verify:`, which `spec commit`
can additionally run on demand with `--verify`.

## Design

- `pre_commit: [cmd, ...]` parses like `verify:` — argv lists, no shell,
  quotes group args; merged config (repo overrides global).
- `spec commit` runs every `pre_commit` command before stamping VERSION
  or staging. Any failure prints the command's output and aborts the
  commit with exit non-zero — nothing is staged, nothing stamped.
  `--no-check` skips the pass for one commit; `--verify` also runs the
  `verify:` list for a commit that wants the heavy checks.
- `spec precheck` runs the `pre_commit` list standalone — the entry point
  the git hook calls, and a way to test checks without committing.
- A spec-managed `pre-commit` hook execs `spec precheck`. If the `spec`
  binary isn't on PATH it warns and passes (fail open): the enforced
  path is `spec commit`; a missing binary must not brick commits.
- Foreign pre-commit hooks are never clobbered — same marker contract as
  the other managed hooks.
- Distinction from `verify:` is deliberate: `pre_commit` is fast checks
  worth paying per commit; `verify` is the heavyweight suite (tests) run
  by `spec verify`, post-apply, or `spec commit --verify`.

## Acceptance Criteria

- [x] `spec commit` runs configured `pre_commit` commands and aborts on failure, naming the failing command
- [x] `--no-check` skips `pre_commit` for one commit
- [x] `--verify` additionally runs the `verify:` list
- [x] `spec precheck` runs the list standalone; exit non-zero on failure; friendly message when unconfigured
- [x] The managed `pre-commit` hook execs `spec precheck`, warns and passes when `spec` is not on PATH, and never clobbers a foreign hook
- [x] `pre_commit` parses merged-config like `verify:` (argv lists, no shell)
