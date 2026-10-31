---
name: spec
description: Spec-driven development with the `spec` CLI — how to work in repos that have a .spec/ directory (project/feature specs, ADRs, REQ traceability, verify)
allowed-tools:
  - read
  - exec
  - grep
  - glob
  - edit
---

This repository is spec-managed (has a `.spec/` dir). Specs are markdown files
with YAML frontmatter (id, type, status, depends_on) under `specs/`.

## Layout

- `specs/project/` — project spec(s); own the `REQ-xxx` requirement ids
- `specs/features/` — feature specs; reference REQs they implement
- `specs/adr/` — architecture decision records
- `specs/imported/` — imported docs
- `.spec/config.yml` — agent, statuses, `verify:` commands
- `.spec/facts.yml` — definite project facts, referenced in specs as `{fact:key}`

## Workflow

1. **Orient first**: run `spec status` (spec counts, REQ→feature coverage,
   configured agent) and read the project spec's Requirements section.
2. **Before changing behavior**, check for an existing feature spec:
   `spec search <topic>`.
3. **New behavior** → `spec new feature <title>` (or `spec draft feature
   <title>` for an LLM/heuristic draft). Set status `proposed`/`accepted` per
   repo convention. There is NO bug spec type:
   - code violates spec → just fix the code (no spec needed)
   - spec is wrong/ambiguous → edit the spec, or `spec new adr` if it's a real
     decision
   - bug exposed a missing requirement → add the REQ, write a feature spec
4. **Feature specs** must reference existing `REQ-xxx` ids on the
   "Addresses:" line — features citing undefined REQs fail `spec validate`.
5. **After implementing**: set frontmatter `status: implemented`, then run
   `spec validate`, `spec check`, and `spec verify`.

## Rules

- Statuses come from `.spec/config.yml` (`statuses:`); default lifecycle is
  draft → proposed → accepted → implemented (plus deprecated/rejected).
- Declare inter-spec dependencies with `depends_on:` frontmatter; `spec graph`
  shows them and `spec validate` fails on unknown ids and cycles.
- Never invent definite values (domain, phone, company name) — use
  `{fact:key}`; add missing facts with `spec fact set key value`.
- `spec check` audits `{fact:key}` references; `spec trace` shows REQ→feature
  coverage.
- `spec apply` executes the configured agent on accepted/proposed specs;
  `spec apply --print` outputs the prompts instead.

## Gotchas

- `spec verify` runs `verify:` commands as plain argv — no shell, so no
  pipes/redirects/globs; wrap those in a script.
- All commands must run inside a spec-managed tree (`.spec/` is found by
  walking up from cwd — `spec init` creates it).
- `.spec/config.yml` documents every key; regenerate it with
  `spec config init repo --force`.
