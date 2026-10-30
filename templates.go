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

func templateFor(specType, title string) string {
	switch specType {
	case "project":
		return fmt.Sprintf(projectTemplate, title)
	case "adr":
		return fmt.Sprintf(adrTemplate, title)
	default:
		return fmt.Sprintf(featureTemplate, title)
	}
}

const agentsTemplate = `# Agent instructions

This repository uses spec for spec-driven development.

- Specs live in specs/ (project/, features/, adr/, imported/).
- Each spec is markdown with YAML frontmatter (type, status, depends_on).
- Valid statuses are configured in .spec/config.yml (statuses:).
- .spec/facts.yml holds definite project facts, referenced as {fact:key}.
- Run spec check to validate specs and spec verify to run configured checks.
- When implementing a spec, update its frontmatter status to implemented.
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
