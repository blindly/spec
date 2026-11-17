---
covers: [skill.go, skill_test.go, config.go, commands.go, main.go, help.go, spec_test.go, templates.go]
created: 2026-11-16
id: FEAT-026
status: implemented
title: Global skill library with per-repo management
type: feature
---

# Global skill library with per-repo management

## Summary

A user-level skill library lives at `~/.config/spec/skills/<name>/` —
plain skill directories (`SKILL.md` plus any bundled files) that the user
may version and share with git; spec does not manage remotes. Repo-owned
skills live in `.spec/skills/<name>/`, spec's agent-agnostic namespace,
and are committed like any other file. Agent skill dirs (`.devin/skills/`,
`.claude/skills/`, …) are pure generated output: spec materializes the
resolved skill set into the dirs named by a `skill_targets:` config list.
A repo opts into library skills with a `skills: [names]` config list.
`spec skill` is the single command for all three layers.

## Motivation

Agent skills are currently scattered: `~/.claude/skills/`,
`~/.config/devin/skills/`, and per-repo `.devin/skills/` copies of unknown
freshness, with no tool knowing which copy came from where. No
agent-agnostic skill location exists — each agent CLI scans its own dir —
so spec provides its own repo layer (`.spec/skills/`) and generates the
per-agent copies. FEAT-020 gave personas a shared library with per-repo
opt-in; this applies the same model to skills, adjusted for the fact that
skills are consumed as files on disk, so the mechanism is install +
freshness-check — the machinery FEAT-025 already built for the single
embedded skill. The payoff: one known spot per scope, repo-owned skills
committed to git while generated installs stay untracked, repo-owned
skills distinguishable from managed copies by path, and spec handles
the correspondence bookkeeping instead of the user copying directories
around.

## Requirements

Addresses: REQ-017

## Design

- **Layers.**
  - Library: `~/.config/spec/skills/<name>/` — a plain directory; the
    user can `git init` it to version/share. spec never touches remotes.
  - Repo: `.spec/skills/<name>/` — repo-owned skills, committed to the
    repo's git. Local skills live here; so do forks adopted from the
    library. Agent-agnostic by construction.
  - Installed: the resolved target dirs — generated output only,
    gitignored (`spec init` appends the resolved targets to
    `.gitignore`; a stale committed copy would differ from some other
    machine's resolved source and block `sync` without `--force`).
    spec creates, refreshes, and removes dirs there to match resolution; it
    never treats them as source.
- **Built-in entry.** The embedded `specSkill` (FEAT-025) is the built-in
  library entry for the name `spec`, used when no `.spec/skills/` or
  library entry provides it, so `spec init` keeps working with an empty
  library. `spec` is implicitly enabled in every spec-managed repo — it
  is part of the resolved set without appearing in `.spec/skills/` or
  `skills:` — so `check`/`sync` cover it like any managed skill, and
  `adopt spec` forks the bundled skill into the repo for per-repo
  tailoring. `spec init` runs `sync`: seeding the spec skill is just the
  resolved set (built-in `spec` plus any global `skills:` baseline)
  materialized into the target dirs.
- **Selection.** `skills: [a, b]` opts the repo into library skills.
  Unlike every other key, `skills:` **unions** across levels — a repo's
  list extends the global baseline rather than replacing it (global
  entries still reach repos that set their own list). `use`/`unuse`
  write only the repo-level list; `use --global`/`unuse --global` write
  the global list and sync the user-level dirs. The list output labels
  each managed skill `library(repo)` or `library(global)` by which
  level enabled it (repo wins when both list it).
  Install targets resolve as: `skill_targets: [dirs]` config when set
  (explicit override — for multi-agent repos, since `agent:` is
  single-valued); else the skill dir of the configured `agent:` from a
  built-in agent→dir table (`claude` → `.claude/skills`, `devin` →
  `.devin/skills`; agents without a known skill dir contribute none);
  else `.devin/skills` + `.claude/skills` (FEAT-025 status quo, for
  repos with no agent configured). A repo that wants no claude dirs
  sets `agent: devin` or trims `skill_targets` — nothing installs for
  agents the repo doesn't use.
- **Resolution.** For a given name, first source wins:
  `.spec/skills/<name>` → `skills:` entry → library file → built-in
  (`spec` only). A `.spec/skills/<name>` always shadows the library —
  this is the fork, distinguished by path like `.spec/role.md` shadows a
  persona. `sync` writes the resolved tree to every `skill_targets` dir.
- **States**, derived from path membership and byte-compare (recursive
  over the dir tree) — no manifest, no bookkeeping:
  - `local` — in `.spec/skills/` only
  - `managed` — in `skills:`, installed copies match the resolved source
  - `stale` — in `skills:` or `.spec/skills/`, an installed copy differs
    from its source (includes hand-edited generated output — always an
    accident: sync refuses to clobber without `--force`)
  - `shadowed` — in both `.spec/skills/` and `skills:` (repo copy wins;
    reported so the user can `unuse` or delete the repo copy)
  - `missing` — in `skills:` or `.spec/skills/`, not installed in a
    target dir
  - `orphaned` — in `skills:` but the library entry is gone (reported,
    never deleted)
  - `available` — in the library, not enabled in this repo
- **Commands** (`spec skill`):
  - *(no args)* — list every skill known to the repo with source and
    status. Outside a spec-managed repo it lists the library instead
    (built-in, `library(global)`, `available`); `view`, `new --library`,
    `import --library`, `edit`, and `refine` also run repo-free since
    they only touch user-level state.
  - `view <name>` — print the effective `SKILL.md` plus a stderr note
    naming its source (like `spec role`).
  - `new <name>` — scaffold `.spec/skills/<name>/SKILL.md` with
    `name`/`description` frontmatter, open the editor. `--library`
    creates in the library instead.
  - `edit <name>` — open the effective source in the editor (repo copy
    first, else library); `--library` forces the library file.
  - `refine <name>` — improve a skill with the configured agent. Library
    skills are refined repo-blind into the library file (same contract
    as `spec persona refine`, adapted to keep valid SKILL.md
    frontmatter); repo skills get repo context like `spec role refine`.
  - `--local` on `edit`/`refine` operates on `.spec/skills/<name>` with
    repo context, first ensuring a repo-owned copy exists: an implicit
    `adopt` when managed (copies the source into `.spec/skills/`, drops
    the `skills:` entry), an implicit copy when library-only. Mirrors
    how `spec role refine` forks the persona into `.spec/role.md`.
  - `use <name>` / `unuse <name>` — add/remove the name in the repo's
    `skills:` and sync. `use` on a globally enabled name reports it
    (no redundant repo entry); `unuse` on one points at `--global`.
    `unuse` removes installed copies only when byte-equal to the
    source; a differing copy errors and suggests `adopt` (keep bytes)
    or `--force` (discard). `--global` variants edit the global
    `skills:` list and the user-level agent dirs instead.
  - `adopt <name>` — copy the resolved source (or the installed copy if
    it was hand-edited, preserving the edits) into `.spec/skills/<name>/`
    and drop the `skills:` entry — a managed skill becomes a repo-owned
    fork.
  - `rm <name>` — delete a repo-owned skill and its generated installs
    (removed only while they still match the source; a differing copy
    needs `--force`). On a managed skill it points at `unuse` — "not
    enabled here" is `unuse`, "gone" is `rm`. `--library` deletes the
    library entry; enabled repos then report it `orphaned`.
  - `push <name>` — copy `.spec/skills/<name>/` to the library. Creates
    the entry when absent; no-op when identical; when the library entry
    exists and differs, requires `--force` (the shared copy may serve
    other repos).
  - `import <name|path>` — copy a skill directory into `.spec/skills/`.
    A bare name searches the known skill dirs (`.devin/skills/`,
    `.claude/skills/`, `~/.claude/skills/`, `~/.config/devin/skills/`);
    a path is copied verbatim. A source inside the repo is a migration:
    it is removed after copying (`git rm` when tracked, plain `rm`
    otherwise) and sync regenerates the installed copy; `~/` sources
    are copied, never removed. With no operand, imports every foreign
    skill dir found in the agent dirs (names already resolved are
    skipped as generated copies); same-named dirs that differ are
    reported and skipped for explicit import. `--library` imports into
    the library — and, like `new --library`, needs no spec-managed repo
    (the library is a plain directory).
  - `check` — print per-skill status; exit non-zero when any resolved
    skill is missing or stale, so it can gate `verify:` (same contract
    as FEAT-025's `check`).
  - `sync` — regenerate all resolved skills in every `skill_targets`
    dir. Refuses to overwrite installed copies that differ from their
    source without `--force`; reports each action. `--global` applies
    the same operations to the user-level agent dirs using the globally
    resolved set (generalizes FEAT-025's `--global`).
- **Replaces:** `spec skill check|update` from FEAT-025 — `check` is kept
  and `update` is subsumed by `sync` (`update` may remain as an alias).
- **Names** match the persona name pattern (no separators or traversal).
  Copies are whole directory trees.
- `spec status` reports a one-line skills summary (local/managed/stale
  counts).

## Acceptance Criteria

- [x] Library skills live in `~/.config/spec/skills/<name>/`; the dir
  works as a plain git repo without spec involvement
- [x] Repo-owned skills live in `.spec/skills/<name>/` and shadow the
  library by path — no config needed, never pushed unless asked
- [x] `skills:` config opts a repo into library skills — unions
  global∪repo so a repo list extends the baseline; install targets
  come from `skill_targets:` when set, else the configured `agent:`'s
  skill dir, else `.devin/skills` + `.claude/skills` — nothing installs
  for agents the repo doesn't use
- [x] `spec skill` lists every skill with source and status; all states
  (local/managed/stale/shadowed/missing/orphaned/available) are derived
  from path membership and byte-compare alone; managed skills are
  labeled `library(repo)`/`library(global)` by enabling level
- [x] `spec skill use/unuse` manage the repo config entry and installed
  copies (`--global` variants manage the global entry and user-level
  dirs); `unuse` never deletes a differing copy
- [x] `spec skill adopt` copies the resolved source into `.spec/skills/`
  and drops the config entry; `push` publishes a repo skill to the
  library with the `--force` overwrite contract; `rm` deletes a repo
  skill and its installs (clobber-guarded, `--library` for library
  entries) while `unuse` stays the "not enabled here" path
- [x] `spec skill import` copies a skill dir from a known location or
  explicit path into `.spec/skills/` (or library with `--library`);
  in-repo sources are moved (`git rm`/`rm`), `~/` sources copied; bare
  `import` migrates every foreign dir in the agent dirs and skips
  resolved names and diverging duplicates
- [x] `spec skill check` exits non-zero on missing/stale skills; `sync`
  regenerates installs in every `skill_targets` dir and refuses to
  clobber differing copies without `--force`; `--global` covers the
  user-level dirs
- [x] The embedded spec skill works as a built-in library entry; `spec
  init` runs `sync` and seeds `.gitignore` with the resolved target
  dirs (generated installs stay untracked); FEAT-025's freshness check
  keeps working
- [x] `refine` is repo-blind for library skills and repo-aware for repo
  skills; `--local` on `edit`/`refine` ensures a `.spec/skills/` copy
  (implicit `adopt` when managed, implicit copy when library-only) and
  operates on it with repo context
- [x] `spec status` shows the skills summary
- [x] Unit tests cover resolution/shadowing, use/unuse, adopt, push,
  import, sync clobber protection, orphaned skills, and the built-in
  fallback
