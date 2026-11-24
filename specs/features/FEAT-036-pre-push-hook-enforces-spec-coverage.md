---
covers: [prepush.go, prepush_test.go, skill.go, commands.go, facts.go, templates.go, README.md, AGENTS.md]
created: 2026-11-23
id: FEAT-036
status: implemented
title: spec-managed pre-push hook enforces spec coverage
type: feature
verified: 2c4c411515eaa8d823ba314439233c70e0c6fd08
---

# spec-managed pre-push hook enforces spec coverage

## Summary

Every spec-managed repo gets a `pre-push` hook that blocks pushes
changing files outside `specs/` and `.spec/` without a spec change in
the same push — "behavior ships with its spec", enforced at the git
boundary rather than by convention alone.

## Motivation

The spec-driven rule ("a spec must exist before behavior changes") was
social. A generated hook makes it mechanical for every repo that runs
`spec init` or `spec skill sync`, with a clear remediation message.

## Requirements

Addresses: REQ-014

## Design

- **Spec-managed, not committed.** The hooks are embedded in the
  binary (`prepush.go`) and written to `.git/hooks/` by
  `ensureSpecHooks`, called from `syncSkills` — so `spec init`,
  `spec skill sync`, and `spec update` all install or refresh them.
  Nothing lives under version control; the clone's hooks are
  regenerated output like the skill installs.
- **Self-maintaining, non-destructive.** A `# managed by spec` marker
  distinguishes ours from a user's: foreign hooks are left alone with
  a note; stale managed hooks refresh; a current one is untouched.
  Non-git directories are a silent no-op.
- **Push-scoped diff.** The hook reads the ref updates git passes on
  stdin: `remote_sha..local_sha` per updated ref, and for new refs the
  commits not on any remote (`--not --remotes`). Deletions are skipped.
- **The rule.** If the union of changed paths contains anything outside
  `specs/` and `.spec/` (config and facts aren't specs) but no
  `specs/` path, the push fails — *unless* every new commit in the
  push carries a `Spec-Exempt: <reason>` trailer attesting a defect
  fix that restores documented behavior. The failure names the
  offending commits. Strict by design — docs, CI, and metadata changes
  also route through specs or an attestation; a `code_paths`-style
  allowlist is a possible later refinement if the strictness proves
  noisy.
- **Why a trailer.** Exemption is self-attestation the hook can't
  verify, so the mechanism's job is audibility plus deliberate
  friction: the trailer lives in `git log` forever and forces a
  written reason, unlike an env var that vanishes. Conventional-commit
  prefixes (`fix:`) were rejected — spec doesn't assume that
  convention and a type label doesn't attest spec-alignment.
- **Escape hatch.** `SPEC_SKIP=1 git push` skips the check entirely —
  last resort for spec-neutral bulk changes (typos, refactors,
  test-only sweeps), not a routine alternative to the trailer.
- **Why pre-push, not pre-commit.** The rule is "behavior ships with
  its spec in the same *push*" — a code commit followed by a spec
  commit is a valid pair, and at commit time nothing can distinguish
  "spec coming next" from "spec forgotten". Push is also the
  publication boundary: local WIP commits legitimately lack specs.
- **Advisory commit-msg hook.** A second managed hook warns (never
  blocks — always exit 0) when a commit changes files outside
  `specs/`/`.spec/` with no `specs/` path staged and no `Spec-Exempt:`
  trailer in the message: the early nudge that a bare blocking hook
  can't give without false positives. Enforcement stays at pre-push.
- **Observable.** `spec status`/`spec ps` print
  `Spec hooks: <name>: installed|outdated|missing|foreign` for each
  managed hook (non-git repos omit the line), and `spec check` WARNs
  on any unenforced state — missing, outdated, or foreign hooks all
  mean the rule isn't active.

## Acceptance Criteria

- [x] A push touching files outside `specs/`/`.spec/` without `specs/`
  exits non-zero unless every new commit carries `Spec-Exempt:`; the
  failure names the offending commits and the trailer
- [x] Pushes including a `specs/` change pass; spec-only pushes pass
- [x] New-ref pushes are handled via `--not --remotes`
- [x] `SPEC_SKIP=1` bypasses; foreign hooks are never clobbered
  (noted instead); stale managed hooks refresh on sync
- [x] The `commit-msg` hook warns on spec-less code commits without a
  trailer, exits 0 always, and stays quiet for covered or exempt
  commits
- [x] `spec status`/`ps` report each hook's state; `spec check` WARNs
  when the rule isn't enforced (missing, outdated, or foreign)
- [x] `AGENTS.md` template and README document the behavior
- [x] Tests cover install semantics and live enforcement via real
  git commits
