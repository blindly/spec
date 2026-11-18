---
covers: [agent.go, commands.go, spec_test.go]
created: 2026-11-02
id: FEAT-017
status: implemented
title: Bounded agent prompts and bootstrap repo scan
type: feature
verified: b4e420a69259da3f82ca5556a41d034f7d8a8757
---

# Bounded agent prompts and bootstrap repo scan

## Summary

Two robustness fixes keep agent commands working in real repositories: a
prompt that exceeds the kernel's per-argument size limit is delivered to the
agent via stdin instead of crashing with E2BIG, and `spec bootstrap`
ignores dependency caches and build-output directories when scanning the
repo so vendored manifests can't flood the prompt or the heuristic spec.

## Motivation

`spec bootstrap` in a Node project failed with
`agent codex failed: fork/exec ... argument list too long` — the repo scan
collected every `package.json` under `node_modules`, producing a prompt far
larger than the 128 KiB a single argv entry may hold. The heuristic
fallback then wrote those vendored paths into the project spec.

## Requirements

Addresses: REQ-004

## Design

- `agent.runLive` substitutes `{prompt}` into argv only when the resulting
  argument fits within `maxArgBytes` (120 KiB, below the kernel limit; on
  Windows 30 KiB, since CreateProcess caps the whole command line at 32767
  chars). An oversized prompt drops its argv placeholder and is written to
  the agent's stdin — the same delivery used when `{prompt}` is absent.
- A `{prompt-file}` argv placeholder expands to the path of a temp file
  holding the prompt (removed after the run), for agents whose
  non-interactive mode cannot read a prompt from stdin — the `devin`
  (`--prompt-file`), `aider` (`--message-file`), and `vibe`
  (`--prompt-file`) adapters use it (`devin --print` errors when no
  prompt arrives, so the stdin fallback would reach it empty).
  `agent_cmd` may use it too.
- Adapters whose prompt is a positional argument or a boolean flag
  (claude, codex, opencode, cline, amp) fall back to stdin cleanly on
  overflow — those CLIs all read a piped prompt. `gemini` fuses the flag
  to the placeholder (`--prompt={prompt}`): an oversized prompt drops the
  whole argument, and bare `gemini` reads stdin as the prompt.
- `cmdBootstrap`'s repo walk skips `generatedDirs` (node_modules, vendor,
  target, dist, build, coverage, __pycache__, venv, Pods, …) alongside the
  existing dot-dir skip, and caps the manifest/doc lists at
  `maxDetectedFiles` (50) so a monorepo can't produce an unbounded section.
- `repoContext` now honors its `maxEntries` parameter, collapsing excess
  specs into a "… and N more specs" line. Public facts appear inline and
  secret fact keys appear with a retrieval hint. The secret fact file
  supplies keys only to prompt context.

## Acceptance Criteria

- [x] A prompt larger than one argv entry reaches the agent via stdin
  instead of failing with E2BIG (or, on Windows, the CreateProcess
  command-line cap)
- [x] Bootstrap skips `node_modules` and other generated dirs; vendored
  `package.json` files never appear in the spec or the prompt
- [x] Real top-level manifests and docs are still detected and listed
- [x] Small prompts still travel in argv as before
- [x] `devin`, `aider`, and `vibe` get the prompt via a file flag
  (`--prompt-file`/`--message-file`) at any size — stdin isn't a reliable
  prompt channel for them
- [x] Positional/bool-flag adapters (claude, codex, opencode, cline, amp)
  overflow to stdin; `gemini`'s `--prompt={prompt}` drops the whole arg on
  overflow so stdin becomes the prompt
- [x] Unit tests cover the stdin fallback, the `{prompt-file}` channel,
  and the generated-dir skip
