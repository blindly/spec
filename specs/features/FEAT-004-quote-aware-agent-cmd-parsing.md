---
covers: [args.go, agent.go, config.go]
created: 2026-11-01
id: FEAT-004
status: implemented
title: Quote-aware agent_cmd parsing
type: feature
verified: 8bb4cbfd3650920472fae258012b1050c1712619
---

# Quote-aware agent_cmd parsing

## Summary

`agent_cmd` and `verify:` commands are split into arguments with
`strings.Fields`, which splits on every space. A quoted argument such as
`mycli --model 'gpt 5' {prompt}` becomes `'gpt` and `5'`. This feature adds
shell-style quote handling when splitting, without running a shell.

## Motivation

Users can't pass arguments that contain spaces to a custom agent or verify
command. The docs already say verify commands run as plain argv with no
shell, so quoting is the only way to express those arguments.

## Requirements

Addresses: REQ-004

## Design

- Add a `splitArgs(string) ([]string, error)` helper that follows POSIX
  shell word-splitting for quotes only:
  - single quotes keep everything literally
  - double quotes keep spaces and allow `\"` and `\\` escapes
  - a backslash outside quotes escapes the next character
  - no variable expansion, globbing, pipes, or redirects
- Use it for `agent_cmd` (in `resolveAgent`) and for each `verify:` command.
- `{prompt}` is still substituted inside any argument, quoted or not.
- An unterminated quote is a config error, reported when the agent is
  resolved or before verify runs, naming the offending command.

## Acceptance Criteria

- [x] `mycli --model 'gpt 5' {prompt}` yields `[mycli --model "gpt 5" <prompt>]`
- [x] Double quotes with `\"` escapes are handled
- [x] A backslash outside quotes escapes the next character
- [x] An unterminated quote returns a clear error and runs nothing
- [x] Plain commands without quotes split exactly as before
- [x] `verify:` commands use the same splitting
- [x] Unit tests cover `splitArgs`, plus one end-to-end agent_cmd test
