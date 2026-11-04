---
covers: [factdiscover.go, factdiscover_test.go, facts.go, help.go, main.go]
created: 2026-11-02
id: FEAT-015
status: implemented
title: spec fact discover
type: feature
verified: 73d57e6eb9a7a2d02674c2b4f143c16859812d73
---

# spec fact discover

## Summary

`spec fact discover` populates `.spec/facts.yml` instead of making the user
hunt for keys one at a time. It always seeds `{fact:<key>}` placeholders
found in spec bodies as empty entries. When an agent is configured, it also
asks the agent to scan the repository for definite project values —
domains, phone numbers, emails, project names, anything that doesn't
change — and proposes them as `key: value` candidates for confirmation.

## Motivation

Facts are meant to hold definite values so specs never inline them, but
there is no bridge from "repo full of real values" to a populated
`facts.yml`: today every key is added by hand with `spec fact set`, and
`spec check` fails on every placeholder that was never seeded. `discover`
does the finding; the human still decides what becomes a fact.

## Requirements

Addresses: REQ-004, REQ-006, REQ-009

## Design

- New `spec fact` subcommand; usage becomes
  `spec fact [list|get <key>|set <key> <value>|unset <key>|discover [--yes]]`.
- **Placeholder seeding (always, offline):** collect `{fact:<key>}`
  references from the same spec set `spec check` audits (`st.Specs()`,
  including archived specs) and append missing keys to `.spec/facts.yml`
  with empty values — so `spec check`'s `FAIL undefined fact reference`
  becomes the existing `WARN fact has an empty value` until filled.
- **Agent scan (agent configured):** resolve and run the configured agent
  the same way `spec draft` does (`resolveAgent`, `repoContext`,
  `agent_timeout`). The prompt asks for definite project values found in
  the repository — domains, phone numbers, emails, names — returned as
  `key: value` lines with where each was found.
- **Confirmation before write:** agent-proposed and heuristic candidates
  are printed (`key: value — source`) and written only after the user
  confirms; `--yes` accepts all, like `spec apply --yes`. Existing keys
  are never overwritten — a proposal for an already-defined key is shown
  as a conflict and skipped.
- **Heuristic fallback:** with `agent: none` (or an agent failure —
  warn and fall back, like `spec draft`), only placeholder seeding plus
  values derivable from structured files run: git remote host, `go.mod`
  module path, `package.json` name/homepage, `pyproject.toml` name.
  Fuzzy matches (phones, free-text domains) are left to the agent.
- Structured candidate keys are `git_remote_host`, `go_module`,
  `package_name`, `homepage`, and `python_project_name` (from `[project]`
  or `[tool.poetry]`). Agent lines use `key: value — source`; malformed
  nonempty responses warn and fall back. Seeding is unconditional even
  when value proposals are declined; seeded empty keys are preserved.
- Values are never silently invented: heuristics read only authoritative
  files; everything else is a confirmed proposal.
- `facts.yml` is rewritten atomically, preserving existing entries,
  comments, and order.
- Output summarizes keys seeded, candidates proposed, and what was
  written. Idempotent — a rerun proposes nothing already defined.
- The help text's Facts section and usage line mention `discover`.

## Acceptance Criteria

- [x] `{fact:<key>}` refs missing from facts.yml are appended with empty values, offline and agent-configured alike
- [x] With an agent configured, discover runs it and prints proposed `key: value` pairs with sources
- [x] Proposals are written only after confirmation or `--yes`; declining writes nothing
- [x] Existing keys are never overwritten; conflicting proposals are reported and skipped
- [x] With `agent: none`, structured sources (git remote, module manifests) still produce proposals
- [x] Agent failure warns and falls back to heuristics, matching `spec draft`
- [x] Comments, existing entries, and order in facts.yml are preserved; write is atomic
- [x] A second run seeds/proposes nothing already defined
- [x] Usage and help text list `discover`
- [x] Tests cover seeding, confirmation, conflicts, fallback, and idempotency (agent runs are mocked)
