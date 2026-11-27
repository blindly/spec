---
covers: [skill.go, config.go, commands.go, skill_test.go, spec_test.go, help.go, skills/spec/SKILL.md, README.md]
id: FEAT-045
type: feature
status: implemented
depends_on: [FEAT-044]
title: agent skill-dir maps, additive targets, and install-scope separation
---

## Summary

Three changes that make skill installs land where the configured
harnesses actually read:

1. **Codex joins the agent maps.** `agentSkillDirs`/`agentGlobalSkillDirs`
   gain `codex → .agents/skills` / `~/.agents/skills` (the standardized
   location; `~/.codex/skills` is deprecated upstream). Before this,
   `agent: codex` matched no map entry and the fallback installed into
   every known dir — Claude's and Devin's — while the configured agent
   got nothing.
2. **Targets are additive and agent-named.** Repo targets =
   dirs(`agent:` ∪ `skill_agents:`) ∪ `skill_targets:` raw dirs; global
   targets mirror that under `~/`. `skill_agents:` is a new
   `global+repo` config key (repo list replaces global — override, not
   union) that names harnesses so users never write dir paths for known
   agents. `skill_targets:`/`skill_global_targets:` remain as the raw-dir
   escape hatch for harnesses spec doesn't know. Defaults apply only
   when all three sources are empty.
3. **Install scopes are separated.** Repo sync installs the repo-scope
   set (`{spec}` ∪ `.spec/skills/` ∪ repo `skills:`) into repo targets;
   global sync installs `{spec}` ∪ global `skills:` into user dirs.
   Globally-enabled skills no longer duplicate into every repo — both
   real harnesses merge user-level and repo-level skill dirs, so the
   copy in `~/` is already visible from any repo.

## Design

- **Prune scans former targets.** `pruneSkillTargets` and the `local`
  listing scan the union of current targets ∪ every dir in the agent
  maps — a config change that removes a dir must not strand spec-made
  installs as invisible orphans. A provable install is removed unless
  it's resolved *and* in a current target — so former-target links die
  even when the skill stays enabled elsewhere. Provable means: link
  into `.spec/skills`/library, dir byte-identical to a library entry,
  or a copy equal to the embedded built-in. Differing dirs stay
  listed as locals.
- **Global rows report `~/` status.** `spec skill` still lists
  `library(global)` rows — the agent sees them via the user dirs — but
  the status column checks the global target paths. `spec check`
  failures there point at `spec skill sync --global`.
- **Accepted coupling**: `agent:` still means both "the apply
  subprocess" and "an agent whose skill dirs are targets". A repo that
  sets `agent:` for apply cannot exclude that agent's skill dir from
  repo targets — documented, not accidental.
- **`agent:` contributes its dir even when it's the only key set**;
  `skill_agents: [bogus]` names nothing known → `spec check` warns
  "no known skill dir" rather than failing.
- `.agents/skills` joined `defaultSkillTargets` — unconfigured repos
  get Codex/Cursor/OpenCode visibility for free.

## Acceptance Criteria

- [x] `agent: codex` resolves to `.agents/skills` / `~/.agents/skills`
  at both levels; it is not in the map → no shotgun installs
- [x] `skill_agents:` names extra agents; effective targets = agent ∪
  skill_agents ∪ raw dirs; repo `skill_agents:` overrides the global
  list
- [x] Repo sync does not install global-only library skills into repo
  targets; `spec skill` still lists them as `library(global)` with
  `~/`-dir status
- [x] Prune and local scanning cover former targets (every dir in the
  agent maps); a provable install survives only when resolved AND in a
  current target
- [x] Unknown `skill_agents` names warn in `spec check`, never fail
- [x] `spec config keys`, seed comments, README, help, and the spec
  skill document the additive model

## Deferred / known edges

- A local dir shadowing a globally-enabled name shows twice (enabled
  row + local row) — correct but unmarked.
- `skill_agent_dirs: {name: dir}` (teaching spec new agent conventions
  in config) is deferred — `skill_targets:` covers unknown harnesses.
