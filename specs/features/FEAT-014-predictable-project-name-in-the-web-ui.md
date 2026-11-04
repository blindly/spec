---
covers: [help.go, web.go]
created: 2026-11-02
id: FEAT-014
status: implemented
title: Predictable project name in the web UI
type: feature
verified: 425321963e1a1691e1d9a56b9eebe3c8e91bae46
---

# Predictable project name in the web UI

## Summary

The web UI's project badge (and the hue of its accent color) used the project
spec's title, or else the repo's folder name. Folder names depend on where a
repo was cloned (`website-1`), so the same project could show different names
in different clones, and when the folder matched the branch name it looked
like the badge showed the branch. The name now comes from, in order: an
explicit `project_name` fact, the project spec's title, the git remote's repo
name, and only then the folder.

## Motivation

The badge should name the project the same way everywhere, and be easy to
set on purpose.

## Requirements

Addresses: REQ-007, REQ-009

## Design

- **Order:**
  1. `project_name` in `.spec/facts.yml` (`spec fact set project_name "…"`)
  2. the title of the first non-archived project spec
  3. the repo name from `remote.origin.url`, e.g.
     `git@github.com:acme/truecare-website.git` → `truecare-website`
  4. the folder name
- The remote is read once when `spec web` starts, not on every request.
- The accent hue is still derived from the chosen name.
- The help page's Facts section mentions `project_name`.

## Acceptance Criteria

- [x] A `project_name` fact wins over everything else
- [x] Without the fact, the project spec's title is used; archived project specs are ignored
- [x] Without either, the git remote's repo name is used (ssh, https, and local-path remotes, with or without `.git`)
- [x] Without a remote, the folder name is used
- [x] The remote is read once per server, not per request
- [x] Tests cover each step of the order and the remote URL parsing
