---
covers: [commit.go, commit_test.go, agent.go, help.go, README.md]
created: 2026-11-23
depends_on: [FEAT-037, FEAT-036, FEAT-014]
id: FEAT-038
status: implemented
title: spec commit --agent — agent-generated commit messages
type: feature
verified: 9fa806bbbdd79b457ada723259099e2b194d8228
---

# spec commit --agent — agent-generated commit messages

## Summary

`spec commit --agent` asks the configured agent (`agent:`/`agent_cmd:`)
to write the commit message from the staged diff instead of the dumb
`update N files` summary. The agent may also supply an honest
`Spec-Exempt: <reason>` trailer when it judges the change spec-neutral.

## Motivation

FEAT-037's generated messages are diff summaries that say nothing, and
its auto `Spec-Exempt:` trailer is a rubber stamp — "no spec-covered
files" attests nothing about *why* no spec is needed. An agent that
reads the diff can write a real message and, for spec-neutral changes,
a real exemption reason. Pre-push stays dumb and offline: it greps the
trailer regardless of who wrote it.

## Requirements

Addresses: REQ-003 (agent optional; offline fallback preserved)

## Design

- **Opt-in flag.** `--agent` means a commit without `-m` is a network
  call; making it opt-in keeps bare `spec commit` instant and offline.
  You opted in per-commit, matching `drift --review`'s opt-in.
- **Message precedence.** `-m` > `--agent` > `diffSummary` fallback.
  `-m --agent` together: `-m` wins, noted on stderr.
- **`--agent` without a configured agent errors** rather than silently
  falling back — an explicit request that can't be honored should fail
  visibly. Agent failures likewise surface instead of committing a
  rubber-stamped summary.
- **Bounded prompt** (FEAT-017): staged file list + `--numstat` scale
  + the staged diff truncated at 48 KiB. `agent_timeout` applies.
- **Honest attestation.** The prompt names the coverage state and asks
  for conventional-commit format (`type: summary`; `feat` = behavior).
  When no `specs/` file is staged, a spec-neutral change attests via
  its non-`feat` prefix alone — or a `Spec-Exempt: <reason>` trailer
  for anything the type can't express. A `feat:` (or otherwise
  unattested) message on an uncovered commit is the agent's judgment
  that a spec is missing.
- **The judgment is surfaced, not overridden.** An unattested message
  commits with a stderr note that the push will fail — spec does not
  append a rubber-stamp trailer to a message it didn't attest. The
  diff-summary fallback is `chore:`-prefixed, so it attests itself
  silently.
- **Output hygiene.** Agent output is trimmed and a single outer code
  fence unwrapped; an empty result errors.
- **Layering unchanged.** No `VERSION`, no push, no tag — tasks.py
  keeps those; the flag flows through `spec commit` so `tasks.py
  commit` inherits it unchanged.

## Acceptance Criteria

- [x] `--agent` with a configured agent commits with the agent's
  message; the prompt carries the staged diff and file list
- [x] `--agent` with no configured agent fails visibly; agent errors
  propagate
- [x] An agent-emitted `Spec-Exempt:` line or non-`feat:` prefix
  satisfies the gate — no trailer appended
- [x] An uncovered commit whose agent message attests nothing commits
  with a push-block warning — no rubber-stamp trailer
- [x] `-m` beats `--agent` (noted on stderr); `--exempt` still appends
- [x] No `--agent` and no `-m` → unchanged summary behavior, fully
  offline
- [x] Tests cover agent message, emitted trailer, missing agent, and
  fallback paths
