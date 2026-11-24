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
  not specs yet — never applied; refined ones show on the board as "ready"
- `specs/archive/` — archived specs (same layout; `spec archive`/`unarchive`):
  deprecated/rejected specs and discarded ideas. History — read them for
  context, but they may be outdated; never implement them, and trust
  live specs and the code where they differ
- `.spec/config.yml` — agent, statuses, `verify:` commands
- `.spec/facts.yml` — public project facts, referenced in specs as `{fact:key}`
- `.spec/secrets.yml` — gitignored credential values, referenced as `{secret:key}`
- `.spec/reminders.yml` — recurring obligations (cadence + `last_done`),
  managed by `spec remind`; due items show in `spec status`, `spec check`,
  and your prompt context

## Workflow

1. **Orient first**: run `spec status` (spec counts, REQ→feature coverage,
   due reminders, configured agent) and read the project spec's
   Requirements section.
2. **Before changing behavior**, check for an existing feature spec:
   `spec search <topic>`.
3. **New behavior** → `spec new feature <title>` (or `spec draft feature
   <title>` for an LLM/heuristic draft) — specs are mandatory: no behavior
   change ships without a spec describing it. Set status
   `proposed`/`accepted` per repo convention. There is NO bug spec type:
   - code violates spec → just fix the code (no spec needed)
   - spec is wrong/ambiguous → edit the spec, or `spec new adr` if it's a real
     decision
   - bug exposed a missing requirement → add the REQ, write a feature spec
   - not sure what it is yet → `spec idea <title>`; `spec refine <IDEA-id>`
     suggests a type; then `spec promote <IDEA-id> [project|feature|adr]`
     (defaults to the suggestion; deletes the idea; the new spec gets
     `promoted_from`). Promotion is ALWAYS `spec promote` — never hand-set
     `status:`/`promoted_to:` on the idea file; `spec validate` rejects
     statuses outside `idea_statuses:`
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
8. **Pushes enforce it.** A spec-managed `pre-push` hook rejects pushes that
   change files outside `specs/` without a spec update; a `commit-msg`
   hook warns earlier, at commit time (advisory only — never blocks).
   Commit through
   `spec commit` — use conventional-commit subjects (`feat:`, `fix:`,
   `docs:`, `chore:`, …). The hook accepts a non-`feat:` prefix or a
   `Spec-Exempt: <reason>` trailer as attestation for spec-neutral work
   (`--exempt` adds one; `--agent` has the configured agent write the
   message and pick the attestation from the staged diff — `commit_agent:
   true` in config makes that the default, `--no-agent` opts one out).
   `feat:` —
   and anything unlabeled — means the push needs a `specs/` change.
   `SPEC_SKIP=1 git push` is the last-resort bypass, not a routine one.
9. **Releases are stamped, not numbered.** A `VERSION` file at the repo
   root opts the repo in: `spec commit` refreshes the
   `<major>.<yy>.<mm>-build-<ddhhmm>` stamp on every commit, and
   `spec release` (clean tree) stamps a VERSION-only `chore: bump`
   commit, pushes, and tags `v<stamp>` — the tag drives the release
   workflow. Bump the major by editing VERSION (`--major N` forces it);
   `version_stamp: false` manages VERSION by hand, and `release: true`
   in repo config is the second opt-in — without it `spec release`
   refuses, so stamping never implies releasing.

## Rules

- **Specs are mandatory.** Every change that adds or alters behavior must
  be described by a spec in the same change — a new feature spec, or an
  update to the existing spec that covers it. The only exception is a fix
  that restores behavior a spec already documents.
- Statuses come from `.spec/config.yml` (`statuses:`); default lifecycle is
  draft → proposed → accepted → implemented (plus deprecated/rejected).
- Declare inter-spec dependencies with `depends_on:` frontmatter; `spec graph`
  shows them and `spec validate` fails on unknown ids and cycles.
- Never invent definite values (domain, phone, company name) — use
  `{fact:key}`; add missing facts with `spec fact set key value`.
- `spec trace` shows REQ→feature coverage.
- Recurring obligations live in `.spec/reminders.yml`, not in spec
  statuses (a recurring task never finishes — it resets). When a spec
  mandates periodic work, register it:
  `spec remind add <title> --every <7d|30d|weekly|monthly|quarterly|yearly>
  [--spec <id>]`; mark it `spec remind done <id>` after doing it. Due
  reminders in your context are real repo obligations — do the task or
  flag it to the user, don't ignore it.
- `spec apply <id>` runs the configured agent on an accepted/proposed spec.
  With no ids it targets all accepted specs and asks first (`--yes` to skip,
  required when non-interactive). `spec apply --print` outputs the prompts.
- `spec apply` succeeds only if the spec's status reaches `implemented` and
  `verify:` passes — so when applying a spec, set its status to
  `implemented` only once the work is really done.
- A spec's type is fixed at creation (it sets id prefix and directory);
  `spec validate` fails on mismatches. To change type, create a new spec.

## Facts and credentials

Credentials (tokens, keys, passwords) always go in `.spec/secrets.yml` —
never in `facts.yml`, spec bodies, or skill files, all of which are
committed. Use `spec fact set --secret <key> <value>` to store one. In a
spec, `{secret:key}` stays literal. Agent context lists secret key names,
and `spec fact list` masks secret values. Retrieve the credential where it
is needed using `spec fact get <key>` or an environment variable granted
to the agent. `spec check` audits both types of reference and warns about
possible credential leaks. Secrets are protected by file permissions only:
anything that can run `spec fact get` can read them, so grant agents
credentials via `agent_env:` rather than letting them fetch freely.

A nonempty `agent_env:` list grants variables to agents launched by spec
alongside essentials such as `PATH`. List `NAME` to pass through an exported
variable or `NAME=key` to inject a secret fact. An unset or empty list allows
full inheritance. Skills installed into other tools receive those tools'
environments.

## Gotchas

- `spec verify` runs `verify:` commands as plain argv — no shell, so no
  pipes/redirects/globs; wrap those in a script. Quotes group arguments
  (`sh -c 'a b'`).
- All commands must run inside a spec-managed tree (`.spec/` is found by
  walking up from cwd — `spec init` creates it).
- `spec skill sync` installs skills as symlinks to their source — repo
  skills link to `.spec/skills/`, library skills to
  `~/.config/spec/skills/` — so harnesses always see current content.
  This `spec` skill is embedded in the binary and installed as a copy:
  it goes stale when the binary's embedded version changes. `spec skill`
  checks; `spec skill update` refreshes it.
- `spec skill` lists enabled skills (with descriptions) then the
  library's not-enabled entries; `spec skill which <name>` shows the
  resolved source and `spec skill diff <name>` diffs installed copies
  against it.
- `.spec/config.yml` documents every key; regenerate it with
  `spec config init repo --force`.
