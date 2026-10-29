package main

import "fmt"

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

var requiredSections = map[string][]string{
	"project": {"Overview", "Requirements"},
	"feature": {"Summary", "Design", "Acceptance Criteria"},
	"adr":     {"Context", "Decision", "Consequences"},
}
