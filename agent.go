package main

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// agent is a resolved LLM CLI invocation: argv may contain {prompt} (replaced by the
// prompt); otherwise the prompt is written to stdin.
type agent struct {
	name string
	argv []string
}

// knownAdapters maps a name to argv; {prompt} is the prompt placeholder.
var knownAdapters = map[string][]string{
	"claude":   {"claude", "-p", "{prompt}"},
	"gemini":   {"gemini", "-p", "{prompt}"},
	"codex":    {"codex", "exec", "{prompt}"},
	"aider":    {"aider", "--message", "{prompt}"},
	"opencode": {"opencode", "run", "{prompt}"},
	"amp":      {"amp", "-x", "{prompt}"},
}

// autoDetectOrder is the preference order when agent: auto.
var autoDetectOrder = []string{"claude", "gemini", "codex", "aider", "opencode", "amp"}

// resolveAgent picks an agent per config. Returns nil when disabled or none found.
func resolveAgent(cfg Config) *agent {
	if cfg.Agent == "none" {
		return nil
	}
	if cfg.AgentCmd != "" {
		return &agent{name: "custom", argv: strings.Fields(cfg.AgentCmd)}
	}
	names := autoDetectOrder
	if cfg.Agent != "auto" && cfg.Agent != "" {
		names = []string{cfg.Agent}
	}
	for _, name := range names {
		argv, ok := knownAdapters[name]
		if !ok {
			continue
		}
		if _, err := exec.LookPath(argv[0]); err == nil {
			return &agent{name: name, argv: argv}
		}
	}
	return nil
}

// run sends prompt to the agent and returns its stdout. dir is the cwd.
func (a *agent) run(prompt, dir string, timeoutSecs int) (string, error) {
	var argv []string
	usesPlaceholder := false
	for _, arg := range a.argv {
		if strings.Contains(arg, "{prompt}") {
			usesPlaceholder = true
			argv = append(argv, strings.ReplaceAll(arg, "{prompt}", prompt))
		} else {
			argv = append(argv, arg)
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeoutSecs)*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
	cmd.Dir = dir
	if !usesPlaceholder {
		cmd.Stdin = strings.NewReader(prompt)
	}
	var out, errOut bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errOut
	if err := cmd.Run(); err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return "", fmt.Errorf("agent %s timed out after %ds", a.name, timeoutSecs)
		}
		return "", fmt.Errorf("agent %s failed: %v: %s", a.name, err, strings.TrimSpace(errOut.String()))
	}
	return out.String(), nil
}

// bodyFromAgentOutput extracts a markdown body from agent output: strips any
// leading frontmatter block and surrounding code fences.
func bodyFromAgentOutput(out string) string {
	s := strings.TrimSpace(out)
	if strings.HasPrefix(s, "---") {
		if _, body := parseFrontmatter(s); body != "" {
			s = strings.TrimSpace(body)
		}
	}
	// unwrap a single outer ``` fence
	if strings.HasPrefix(s, "```") {
		if i := strings.Index(s, "\n"); i >= 0 {
			s = s[i+1:]
		}
		if j := strings.LastIndex(s, "```"); j >= 0 {
			s = s[:j]
		}
	}
	return strings.TrimSpace(s) + "\n"
}

// repoContext summarizes the repository for prompt grounding.
func repoContext(st Store, maxEntries int) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Repository root: %s\n\nExisting specs:\n", st.Root)
	for _, s := range st.Specs() {
		fmt.Fprintf(&b, "- %s (%s, %s): %s\n", s.ID(), s.Type(), s.Status(), s.Title())
	}
	b.WriteString("\nProject requirements:\n")
	for _, s := range st.Specs() {
		if s.Type() != "project" {
			continue
		}
		for _, line := range strings.Split(s.Body, "\n") {
			if reqPattern.FindString(line) != "" {
				fmt.Fprintf(&b, "- %s\n", strings.TrimSpace(line))
			}
		}
	}
	return b.String()
}

const draftPromptFmt = `You are writing a feature specification for a spec-managed repository.

Write a feature spec titled %q using EXACTLY this structure (markdown body only, no frontmatter):

%s

Rules:
- Fill in every section with concrete content; no TODO placeholders.
- In "Requirements" reference only REQ-xxx ids listed below under "Addresses:".
- Output ONLY the markdown body.

%s`

const bootstrapPromptFmt = `You are writing a project specification for an existing repository.

Write a project spec titled %q using EXACTLY this structure (markdown body only, no frontmatter):

%s

Rules:
- Fill every section with content inferred from the repository context below.
- Define requirements as "- REQ-001: ..." lines under "## Requirements".
- Output ONLY the markdown body.

Repository context:
%s`

const refinePromptFmt = `You are improving an existing %s specification (id %s, title %q).

Current body:

%s

Rules:
- Improve clarity and completeness; keep every existing "## Section" heading.
- Do not invent new REQ ids; only reference existing ones.
- Output ONLY the improved markdown body, no frontmatter.

%s`
