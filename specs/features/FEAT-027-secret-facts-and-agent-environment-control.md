---
covers: [facts.go, factcheck.go, factdiscover.go, agent.go, config.go, commands.go, facts_test.go, skills/spec/SKILL.md, help.go, main.go, README.md, .spec/config.yml, .gitignore]
created: 2026-11-17
id: FEAT-027
status: implemented
title: Secret facts and agent environment control
type: feature
verified: e851abc4ba6364c1fd794768e5737c96f870dbab
---

# Secret facts and agent environment control

## Summary

Facts gain a secret tier: `.spec/secrets.yml` holds credential values in
the same flat `key: value` format as `facts.yml` but is gitignored and
never expanded into committed files or agent prompts. Committed text
references secrets with a distinct `{secret:<key>}` placeholder that
never expands — `{fact:<key>}` keeps its single meaning of baking a
non-secret value in. `spec fact` manages both tiers through one key
namespace (`set --secret`, `get`, masked `list`), and a new `agent_env:`
config allowlist controls which environment variables the agent
subprocess sees — including `NAME=key` entries that inject a secret
fact's value under an env var name. Skills and scripts retrieve
credentials at point of use (`spec fact get`, `$VAR`) instead of
carrying values in committed text.

## Motivation

Skills need credentials such as API tokens and deploy keys. Before this
feature, the committed facts file was the only place to store project
values, and `repoContext` included those values in agent prompts.
Agent subprocesses also inherited the entire shell environment.
Storing credentials locally and granting environment variables explicitly
reduces exposure through generated text, agent prompts, and subprocess
inheritance.

## Requirements

Addresses: REQ-009, REQ-004

## Design

- **`.spec/secrets.yml`** — same parser and write discipline as
  `facts.yml` (flat `key: value`, comments, `writeFileAtomic`). It must
  never be committed: `spec init` (and every repo `spec skill sync`)
  appends `/.spec/secrets.yml` to
  `.gitignore` alongside the skill-target entries, and `spec check`
  FAILs when a `secrets.yml` exists but is not ignored by git (tracked
  or would-be-committed). Values are file-permission protected only —
  `0600` on write.
- **One namespace, two tiers.** `loadFacts` gains a secret view: a key
  defined in `secrets.yml` is a *secret fact*. `spec fact get <key>`
  resolves the merged namespace and prints the raw value to stdout —
  this is the sanctioned point-of-use retrieval for scripts and agents.
  `spec fact set --secret <key> <value>` writes to `secrets.yml`
  and moves any existing public definition out of `facts.yml`. Without
  `--secret`, new keys go to `facts.yml` and existing keys stay in their
  tier. `unset` removes all definitions of the key. `spec fact list` shows secret
  keys with the value masked as `<secret>`. A key defined in both files
  is a `check` warning (the secret wins).
- **Two placeholder spellings.** `{fact:<key>}` keeps a single meaning:
  bake a non-secret value into the file at `Create`. `{secret:<key>}`
  names a secret fact and never expands — it stays literal in committed
  output, where it documents the credential's existence without
  exposing the value. `spec check` resolves `{secret:<key>}` against
  `secrets.yml` (undefined key → FAIL, same as `{fact:}`) and FAILs a
  `{fact:<key>}` that names a secret key with a "use `{secret:…}`"
  hint — the expanding syntax can never touch a secret value, so no
  substitution code path can leak one.
- **Prompts name, never show.** `repoContext` keeps listing plain facts
  inline and lists secret keys under a separate heading — "Secret facts
  (values withheld — fetch with `spec fact get <key>` when needed)" —
  so the agent learns a credential exists and how to obtain it, without
  the value entering the prompt (and the transcripts/logs it lands in).
- **`agent_env:` config allowlist.** New optional `config.yml` list.
  Unset or empty: current behavior (agent inherits everything). When
  set, the agent subprocess receives only the listed entries plus the
  minimal set spec itself needs to run the CLI (`PATH`, `HOME`,
  `TMPDIR`, and platform equivalents). Two entry forms:
  - `NAME` — pass `NAME` through from spec's own environment, if set
  - `NAME=fact_key` — inject the value of secret fact `fact_key` as
    environment variable `NAME`; unknown keys fail the agent run with a
    clear error before the subprocess starts
- **Leak detection in `spec check`.** WARN on: a secret-looking value
  committed in `facts.yml` (known token shapes — `ghp_`, `sk-`, `AKIA`,
  `-----BEGIN`, plus a high-entropy heuristic), and literal secret
  values — matching the token patterns or equal to a `secrets.yml`
  value — appearing in spec bodies or `.spec/skills/` files.
  `{secret:<key>}` placeholders in committed text are expected and
  never flagged. Warnings only — scanning is heuristic, never a FAIL on
  its own (beyond the secrets-not-ignored rule).
- **`spec fact discover`** flags candidates matching the token
  patterns: they are proposed as secrets, and `--yes` writes them to
  `secrets.yml` instead of `facts.yml`.
- **Out of scope.** OS keychain/`op`-style external secret backends,
  per-skill credential declarations, and a localhost auth-injecting
  proxy — `NAME=key` injection and `spec fact get` cover the same jobs
  with no new machinery. The env allowlist governs only agents spec
  itself launches; skills installed into other tools' dirs inherit
  those tools' environments, which is documented in the spec skill.

## Acceptance Criteria

- [x] `spec fact set --secret k v` writes `.spec/secrets.yml` mode 0600;
  `get k` prints the raw value; `list` masks it as `<secret>`; `unset`
  removes it; `set`/`unset` operate on whichever file holds the key
- [x] `spec init` and repo `spec skill sync` gitignore `/.spec/secrets.yml`; `spec check` FAILs
  when a `secrets.yml` exists unignored, and WARNs on duplicate keys
  across the two files
- [x] `{secret:<key>}` never expands into files and counts as a defined
  reference when the key exists in `secrets.yml`; a `{fact:<key>}`
  naming a secret key FAILs `spec check` with a `{secret:…}` hint; an
  undefined `{secret:<key>}` FAILs like an undefined `{fact:<key>}`
- [x] Agent prompts (`repoContext`) contain secret key names and the
  `spec fact get` retrieval hint, never the values; plain facts are
  unchanged
- [x] `agent_env: [A, B=k]` restricts the agent environment to the
  essentials plus `A` passed through and `B` set to secret fact `k`'s
  value; unset `agent_env` preserves full inheritance; an unknown
  `=key` fails before the agent runs
- [x] `spec check` WARNs on secret-looking literals in `facts.yml`,
  spec bodies, and `.spec/skills/` files
- [x] `spec fact discover` proposes secret-looking candidates as
  secrets; `--yes` writes them to `secrets.yml`
- [x] Unit tests cover: secrets.yml load/set/unset/masked list,
  gitignore enforcement, placeholder preservation + defined-reference
  check, prompt withholding, `agent_env` filtering/injection/error, and
  the literal-scan warnings
- [x] The spec skill's facts section documents the secret tier; REQ-009
  and REQ-004 text updated to describe shipped behavior
