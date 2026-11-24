package main

import (
	_ "embed"
	"fmt"
)

const projectTemplate = `# %s

## Overview

TODO: Describe the project.

## Goals

- TODO

## Non-Goals

- TODO

## Requirements

- REQ-001: TODO

## Architecture

TODO
`

const featureTemplate = `# %s

## Summary

TODO: One-paragraph summary.

## Motivation

TODO

## Requirements

Addresses: TODO (list REQ-xxx ids this feature implements)

## Design

TODO

## Acceptance Criteria

- [ ] TODO
`

const adrTemplate = `# %s

## Context

TODO: What is the issue that we're seeing that is motivating this decision?

## Decision

TODO: What is the change that we're proposing and/or doing?

## Consequences

TODO: What becomes easier or more difficult because of this change?
`

const ideaTemplate = `# %s

## Notes

TODO: what's the idea?
`

func templateFor(specType, title string) string {
	switch specType {
	case "project":
		return fmt.Sprintf(projectTemplate, title)
	case "adr":
		return fmt.Sprintf(adrTemplate, title)
	case "idea":
		return fmt.Sprintf(ideaTemplate, title)
	default:
		return fmt.Sprintf(featureTemplate, title)
	}
}

const agentsTemplate = `# Agent instructions

This repository uses spec for spec-driven development.

- Specs live in specs/ (project/, features/, adr/, imported/); untyped
  ideas live in specs/inbox/ until promoted (spec promote).
- Each spec is markdown with YAML frontmatter (type, status, depends_on).
- Valid statuses are configured in .spec/config.yml (statuses:).
- .spec/facts.yml holds definite project facts, referenced as {fact:key}.
- Run spec check to validate specs and spec verify to run configured checks.
- When implementing a spec, update its frontmatter status to implemented.
- Implemented specs are living documentation: if a change alters behavior
  another spec describes, update that spec in the same change.
- Run spec drift to find specs whose covered code changed since they were
  last updated; fix them, or spec verified <id> if still accurate.
- A spec-managed pre-push hook blocks pushes that change files outside
  specs/ without a spec update; an advisory commit-msg hook warns at
  commit time. Commit through spec commit with conventional-commit
  subjects: a non-feat: prefix (fix:, docs:, chore:, ...) or a
  'Spec-Exempt: <reason>' trailer (--exempt) attests spec-neutral work;
  --agent asks the configured agent to write the message and pick the
  attestation (commit_agent: true makes that the default, --no-agent
  overrides); SPEC_SKIP=1 git push is the last-resort bypass.
- A VERSION file opts the repo into release versioning: spec commit
  refreshes the <major>.<yy>.<mm>-build-<ddhhmm> stamp, and spec release
  stamps a VERSION-only commit, pushes, and tags v<stamp> — only where
  repo config sets release: true.
- A development role may be defined in .spec/role.md or selected via
  persona: in .spec/config.yml (shared library ~/.config/spec/personas/) —
  when set, adopt that persona and its conventions for all work here.
`

const claudeTemplate = `@AGENTS.md
`

// specSkill is seeded to .devin/skills/ and .claude/skills/ by `spec init`
// so agents working in the repo pick up the spec workflow.
//
//go:embed skills/spec/SKILL.md
var specSkill string

var requiredSections = map[string][]string{
	"project": {"Overview", "Requirements"},
	"feature": {"Summary", "Design", "Acceptance Criteria"},
	"adr":     {"Context", "Decision", "Consequences"},
}
