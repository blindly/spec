---
covers: [help.go, main.go, spec_test.go]
created: 2026-11-18
id: FEAT-028
status: implemented
title: Per-command help
type: feature
verified: 1753f80038be75249b0777d513bb30848de17789
---

# Per-command help

## Summary

`spec <command> --help` (equivalently `spec help <command>`) prints that
command's usage, description, subcommands, and flags — e.g.
`spec skill --help` shows the whole skill block instead of today's
`usage: spec skill --help <name>` error.

## Motivation

Command options are documented in the main `spec` help, but nothing
per-command exists: `run()` intercepts `--help` only at position 0, so
`spec skill --help` reaches `cmdSkill` as a positional and fails
confusingly. Users expect `cmd --help` to work everywhere.

## Requirements

Addresses: REQ-018

## Design

- `spec <command> --help` or `spec <command> -h` prints that command's
  help; `spec help <command>` does the same. `spec help` / `spec --help`
  keep printing the full help.
- Dispatch handles it centrally in `run()` — before the command runs —
  so every command gets it for free and subcommand parsers (like
  `cmdSkill`'s) never see `--help` as a positional.
- Each command's help text covers its usage lines, what it does, its
  subcommands, and its flags — drawn from a single source shared with
  the main help so the two can't diverge (one block per command, the
  main help assembles them).
- `--help` after an unknown command behaves like an unknown command;
  `spec help <nonsense>` errors the same way.
- Subcommand-level help (`spec skill use --help`) is out of scope —
  `--help` anywhere past the command prints that command's help.

## Acceptance Criteria

- [x] `spec skill --help` prints the skill usage block (subcommands + flags), exit 0
- [x] `spec help skill` prints the same block
- [x] Every command in the command table has a help entry; `--help` never reaches subcommand arg parsers
- [x] `spec help` and bare `spec` still print full help; unknown command + `--help` stays an error
- [x] Main help and per-command help come from the same source
- [x] Tests cover `--help`/`-h`/`help <cmd>` on a few commands, the unknown-command error, and skill output containing its subcommands
