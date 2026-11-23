---
covers: [completion.go, main.go, help.go, completion_test.go]
created: 2026-11-18
id: FEAT-029
status: implemented
title: Shell completion
type: feature
---

# Shell completion

## Summary

`spec completion <bash|zsh|fish>` prints a shell completion script to
stdout. Once sourced (`source <(spec completion bash)`, or dropped into
the shell's completions dir), tab-completion offers command names,
subcommands, flags, and the dynamic arguments a command takes — spec
ids, skill names, persona names, fact keys.

## Motivation

The CLI has grown a large surface — ~30 commands, `spec skill` alone
carries a dozen subcommands — and none of it is discoverable at the
prompt. Completion is the standard answer (gh, kubectl, git).

## Requirements

Addresses: REQ-019

## Design

- New `spec completion <shell>` command — works repo-free (completions
  must work outside a spec tree). Unknown shell → usage error.
  (`spec __complete <kind>` and `completion` are dispatched in `run()`,
  not the command table — the scripts' `spec __complete` calls would
  otherwise create an init-time dependency cycle.)
- Completion data lives in one declarative table mapping each command to
  its subcommands, flags, and argument kind (`spec-id`, `skill`,
  `persona`, `fact-key`, `file`, none). Scripts are generated from that
  table so new commands get completion when they get a table entry.
- Static parts (commands, subs, flags) complete everywhere; dynamic
  parts query the running binary when they need repo state —
  `spec ls --paths`-style helpers or direct calls — and degrade to
  static completion outside a repo.
- Dynamic arguments: spec ids (`edit`, `view`, `apply`, `refine`,
  `archive`, `unarchive`, `discard`, `promote`, `covers`, `verified`,
  `drift`); skill names (`skill use|unuse|adopt|push|rm|edit|view|
  refine|which|diff`); persona names (`persona use|edit|refine|view`);
  fact keys (`fact get|unset`); spec types for `new`/`promote`;
  `global|repo` for `config init|edit`.
- Output is plain script to stdout, fully offline; the script reflects
  the binary that generated it, so it never drifts from the CLI.
- `--install` writes the script to the shell's conventional per-user
  completions dir (`~/.config/fish/completions/spec.fish`,
  `~/.local/share/bash-completion/completions/spec`, `~/.zfunc/_spec`
  — zsh has no standard dir, so the note tells the user to put
  `~/.zfunc` in `fpath`) and reports the path written.
- Documented in the main help and README (how to install per shell).

## Acceptance Criteria

- [x] `spec completion bash|zsh|fish` each print a working script; bad shell errors
- [x] Works outside a spec-managed repo (commands + static subs/flags still complete)
- [x] Inside a repo, spec ids complete after `spec view/edit/apply ` and skill names after `spec skill use `
- [x] Flag completion covers each command's flags (`--global`, `--force`, `--library`, `--local`, `--yes`, …)
- [x] Command list in the scripts comes from the command table, not a hardcoded second copy
- [x] Help/README show how to install for each shell
- [x] `--install` writes the script to the shell's user completions dir and reports the path
- [x] Tests: each shell's script generates and contains commands/subs; dynamic completers return ids and names in a temp repo
