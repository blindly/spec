---
covers: commands.go
created: 2026-11-01
id: FEAT-001
status: implemented
title: Confirm apply scope
type: feature
verified: 5864386bd8432060071cefcfa6e28f462f119dfd
---

# Confirm apply scope

## Summary

Since `spec apply` now runs the agent by default, running it with no ids
starts an agent on every accepted or proposed spec, one after another. Each
run can take up to `agent_timeout` and edits code. This feature narrows the
default scope to approved specs and requires confirmation before starting
agent runs the user didn't name explicitly.

## Motivation

Before `--run` became the default, `spec apply` with no ids only printed
prompts and was harmless. Now a bare `spec apply` can launch several long,
code-editing agent runs, including on `proposed` specs nobody has approved.

## Requirements

Addresses: REQ-005, REQ-004

## Design

- **Default scope is `accepted` only.** With no ids, `apply` targets feature
  and project specs with status `accepted`. `proposed` specs are no longer
  picked up implicitly.
- **Explicit ids keep working.** `spec apply FEAT-001` runs on an accepted
  or proposed spec without confirmation, as today. Other statuses still
  need `--force`.
- **Confirm implicit runs.** With no ids (and not `--print`), `apply`
  lists the targets and their titles, then asks
  `Apply N spec(s) via <agent>? [y/N]`. Anything other than `y`/`yes` aborts
  with no agent started.
- **Non-interactive use.** If stdin is not a terminal, `apply` with no ids
  refuses with an error that suggests passing ids or `--yes`.
- **`--yes` flag** skips the confirmation (for scripts and CI).
- **`--print` is unchanged:** no confirmation, all accepted specs printed.
- Update `--help`, README, SKILL.md, and the web help page.

## Acceptance Criteria

- [x] `spec apply` with no ids ignores `proposed` specs
- [x] `spec apply` with no ids lists targets and asks for confirmation before running any agent
- [x] Answering anything but `y`/`yes` starts no agent and exits non-zero
- [x] `spec apply` with no ids and non-terminal stdin errors without running an agent, unless `--yes` is given
- [x] `spec apply --yes` runs without prompting
- [x] `spec apply FEAT-001` on an accepted or proposed spec runs without prompting
- [x] `spec apply --print` never prompts
- [x] Tests cover each case with a fake agent
- [x] Docs updated (help text, README, SKILL.md, web help)
