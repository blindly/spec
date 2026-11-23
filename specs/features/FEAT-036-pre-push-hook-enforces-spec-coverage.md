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

- **Spec-managed, not committed.** The hook is embedded in the binary
  (`prepush.go`) and written to `.git/hooks/pre-push` by
  `ensurePrePushHook`, called from `syncSkills` — so `spec init`,
  `spec skill sync`, and `spec update` all install or refresh it.
  Nothing lives under version control; the clone's hook is regenerated
  output like the skill installs.
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
- **Observable.** `spec status`/`spec ps` print
  `Pre-push hook: installed|outdated|missing|foreign` (non-git repos
  omit the line), and `spec check` WARNs on any unenforced state —
  missing, outdated, or foreign hooks all mean the rule isn't active.

## Acceptance Criteria

- [x] A push touching files outside `specs/`/`.spec/` without `specs/`
  exits non-zero unless every new commit carries `Spec-Exempt:`; the
  failure names the offending commits and the trailer
- [x] Pushes including a `specs/` change pass; spec-only pushes pass
- [x] New-ref pushes are handled via `--not --remotes`
- [x] `SPEC_SKIP=1` bypasses; foreign pre-push hooks are never
  clobbered (noted instead); stale managed hooks refresh on sync
- [x] `spec status`/`ps` report the hook state; `spec check` WARNs
  when the rule isn't enforced (missing, outdated, or foreign)
- [x] `AGENTS.md` template and README document the behavior
- [x] Tests cover install semantics and live enforcement via real
  git commits
