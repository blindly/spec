---
covers: [agent.go, commands.go, spec_test.go]
created: 2026-11-02
id: FEAT-017
status: implemented
title: Bounded agent prompts and bootstrap repo scan
type: feature
verified: 3309b8bba9d2844712d68ffbd7df96cae3fa5255
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
  argument fits within `maxArgBytes` (120 KiB, below the kernel limit). An
  oversized prompt drops its argv placeholder and is written to the agent's
  stdin — the same delivery used when `{prompt}` is absent.
- `cmdBootstrap`'s repo walk skips `generatedDirs` (node_modules, vendor,
  target, dist, build, coverage, __pycache__, venv, Pods, …) alongside the
  existing dot-dir skip, and caps the manifest/doc lists at
  `maxDetectedFiles` (50) so a monorepo can't produce an unbounded section.
- `repoContext` now honors its `maxEntries` parameter, collapsing excess
  specs into a "… and N more specs" line.

## Acceptance Criteria

- [x] A prompt larger than one argv entry reaches the agent via stdin
  instead of failing with E2BIG
- [x] Bootstrap skips `node_modules` and other generated dirs; vendored
  `package.json` files never appear in the spec or the prompt
- [x] Real top-level manifests and docs are still detected and listed
- [x] Small prompts still travel in argv as before
- [x] Unit tests cover the stdin fallback and the generated-dir skip
