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
- `specs/inbox/` — untyped ideas (`IDEA-xxx`, statuses new/refined/discarded);
  not specs yet — never applied, not on the board
- `specs/archive/` — archived specs (same layout; `spec archive`/`unarchive`):
  deprecated/rejected specs and discarded ideas. History — read them for
  context, but they may be outdated; never implement them, and trust
  live specs and the code where they differ
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
   - not sure what it is yet → `spec idea <title>`; `spec refine <IDEA-id>`
     suggests a type; then `spec promote <IDEA-id> [project|feature|adr]`
     (defaults to the suggestion; deletes the idea; the new spec gets
     `promoted_from`)
4. **Feature specs** must reference existing `REQ-xxx` ids on the
   "Addresses:" line — features citing undefined REQs fail `spec validate`.
5. **After implementing**: set frontmatter `status: implemented`, then run
   `spec validate`, `spec check`, and `spec verify`.
6. **Keep specs current.** Implemented specs are the living documentation of
   the system, not finished history. If your change alters behavior another
   spec describes, update that spec's text in the same change (don't change
   its status). If a spec no longer describes the system at all, mark it
   `deprecated` — don't leave it silently stale. Accepted ADRs are the
   exception: don't edit their original sections — add a dated entry under
   `## Corrections` at the end (or, if the decision itself changed, write a
   new ADR that supersedes it).
7. **Check for drift.** `spec drift` lists specs whose `covers:` files
   changed since they were last updated. Before finishing, fix any your work
   made stale; if a flagged spec is still accurate, `spec verified <id>`.
   Commit spec updates together with the code they describe.

## Rules

- Statuses come from `.spec/config.yml` (`statuses:`); default lifecycle is
  draft → proposed → accepted → implemented (plus deprecated/rejected).
- Declare inter-spec dependencies with `depends_on:` frontmatter; `spec graph`
  shows them and `spec validate` fails on unknown ids and cycles.
- Never invent definite values (domain, phone, company name) — use
  `{fact:key}`; add missing facts with `spec fact set key value`.
- `spec check` audits `{fact:key}` references; `spec trace` shows REQ→feature
  coverage.
- `spec apply <id>` runs the configured agent on an accepted/proposed spec.
  With no ids it targets all accepted specs and asks first (`--yes` to skip,
  required when non-interactive). `spec apply --print` outputs the prompts.
- `spec apply` succeeds only if the spec's status reaches `implemented` and
  `verify:` passes — so when applying a spec, set its status to
  `implemented` only once the work is really done.
- A spec's type is fixed at creation (it sets id prefix and directory);
  `spec validate` fails on mismatches. To change type, create a new spec.

## Gotchas

- `spec verify` runs `verify:` commands as plain argv — no shell, so no
  pipes/redirects/globs; wrap those in a script. Quotes group arguments
  (`sh -c 'a b'`).
- All commands must run inside a spec-managed tree (`.spec/` is found by
  walking up from cwd — `spec init` creates it).
- `.spec/config.yml` documents every key; regenerate it with
  `spec config init repo --force`.
