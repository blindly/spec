---
covers: [commands.go, agent.go]
created: 2026-11-01
depends_on: FEAT-001
id: FEAT-002
status: implemented
title: Report apply outcome accurately
type: feature
verified: d33abb029a0045278c35e44b2be31d705c95ee36
---

# Report apply outcome accurately

## Summary

`spec apply` currently prints "FEAT-001 done" whenever the agent process
exits 0, even if the agent did nothing. It also reports a timeout as a plain
failure, even when the agent finished the work before being killed. This
feature makes `apply` judge the outcome by the spec's status and the repo's
verify commands, not by the agent's exit code.

## Motivation

Two real runs showed the problem:

- codex reported it was blocked by its sandbox and changed nothing, and spec
  still printed `FEAT-001 done`.
- cline wrote the tests and set the spec to `implemented`, then was killed
  by the timeout, and spec reported only `timed out after 900s`.

## Requirements

Addresses: REQ-004

## Design

After each agent run, `apply` reloads the spec from disk and classifies the
result:

| Agent result | Spec status | Verify | Reported as |
|---|---|---|---|
| exit 0 | `implemented` | passes or not configured | **implemented** |
| exit 0 | `implemented` | fails | **verify failed** |
| exit 0 | anything else | — | **not implemented** ("agent finished but FEAT-001 is still accepted") |
| timeout | `implemented` | passes or not configured | **implemented (agent timed out after finishing)**, as a warning |
| timeout | anything else | — | **timed out** |
| other error | — | — | **failed** with the agent's error |

- Verify runs the `verify:` commands from config (same as `spec verify`)
  only when the spec reached `implemented`. `--no-verify` skips it.
- `apply` never changes spec status itself; it only reports.
- With multiple targets, `apply` continues after a failure, then prints a
  summary line per spec.
- Exit code is 0 only if every target is reported as implemented
  (including the timed-out-after-finishing case).
- Replace the unconditional "`<id>` done" message.

## Acceptance Criteria

- [x] An agent that exits 0 without setting `implemented` is reported as not implemented, and `apply` exits non-zero
- [x] An agent that sets `implemented` and passes verify is reported as implemented, exit 0
- [x] Failing verify after `implemented` is reported as verify failed, exit non-zero
- [x] A timed-out agent whose spec reached `implemented` is reported with a warning, exit 0
- [x] `--no-verify` skips the verify commands
- [x] Multiple targets: a failure doesn't stop later targets, and a per-spec summary is printed
- [x] "done" is never printed without the spec reaching `implemented`
- [x] Tests cover each row of the table with fake agents
