---
covers: [push.go, commands.go, main.go, help.go, completion.go]
created: 2026-11-25
id: FEAT-042
status: implemented
title: spec push — Go-native push with coverage enforcement
type: feature
depends_on: [FEAT-036, FEAT-037]
---

# spec push — Go-native push with coverage enforcement

## Summary

`spec push` pushes the current branch to `origin` after running the
pre-push spec-coverage check **in-process**. It's the third leg of the
spec-owned flow (`spec commit` → `spec push` → `spec release`) and makes
enforcement independent of the installed bash hook — no `.git/hooks`, no
git-bash requirement, identical semantics on Windows.

## Design

- The check mirrors `pre-push` exactly: files changed in the push range
  outside `specs/`/`.spec/` with no `specs/` change are allowed only when
  every new commit attests spec-neutrality via a `Spec-Exempt:` trailer
  or a non-`feat:` conventional-commit prefix (merges exempt — content
  vetted via parents). `SPEC_SKIP=1` bypasses, same as the hook.
- Push range: `origin/<branch>..HEAD` when the remote ref exists, else
  commits not on any remote (`HEAD --not --remotes`) — same rule the
  hook uses for new branches/tags.
- On refusal, the error names the unattested commits and explains how to
  fix (update a spec, re-commit with a trailer/prefix, or SPEC_SKIP) —
  same guidance the hook prints.
- On pass, `git push origin <current-branch>`; detached HEAD or no
  remote is a clear error.
- `push_command:` in config replaces the push invocation for `spec push`
  and `spec release` (branch and tag pushes alike) — argv template, no
  shell, `{ref}`/`{branch}` expand to the ref being pushed. The gate
  reads the remote out of `git push <remote> ...` templates; anything
  else falls back to checking commits not on any remote.

## Acceptance Criteria

- [x] `spec push` pushes a spec-covered range without complaint
- [x] An uncovered push whose commits all carry trailers or non-feat prefixes pushes
- [x] An uncovered push with unattested commits refuses, naming them; `SPEC_SKIP=1` bypasses
- [x] The check runs without any installed git hook (works in repos where hooks are absent or bash is unavailable)
- [x] Detached HEAD and missing origin produce clear errors
- [x] `push_command: git push <remote> {ref}` redirects both `spec push` and `spec release` pushes; `{ref}`/`{branch}` expand to the pushed ref
