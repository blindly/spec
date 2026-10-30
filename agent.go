package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
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
	"devin":    {"devin", "--print", "--permission-mode", "smart", "--", "{prompt}"},
	"gemini":   {"gemini", "-p", "{prompt}"},
	"codex":    {"codex", "exec", "{prompt}"},
	"aider":    {"aider", "--message", "{prompt}"},
	"opencode": {"opencode", "run", "{prompt}"},
	"amp":      {"amp", "-x", "{prompt}"},
	"cline":    {"cline", "{prompt}"},
	"vibe":     {"vibe", "--auto-approve", "-p", "{prompt}"},
}

// resolveAgent picks an agent per config. Returns (nil, nil) when no agent is
// configured, and (nil, err) when one is configured but unusable — callers can
// then warn or fail rather than silently degrading.
func resolveAgent(cfg Config) (*agent, error) {
	if cfg.AgentCmd != "" {
		return &agent{name: "custom", argv: strings.Fields(cfg.AgentCmd)}, nil
	}
	if cfg.Agent == "" || cfg.Agent == "none" {
		return nil, nil
	}
	argv, ok := knownAdapters[cfg.Agent]
	if !ok {
		return nil, fmt.Errorf("unknown agent %q (want one of: claude, devin, gemini, codex, aider, opencode, amp, cline, vibe)", cfg.Agent)
	}
	if _, err := exec.LookPath(argv[0]); err != nil {
		return nil, fmt.Errorf("agent %q configured but %q not found on PATH", cfg.Agent, argv[0])
	}
	return &agent{name: cfg.Agent, argv: argv}, nil
}

// run sends prompt to the agent and returns its stdout. dir is the cwd.
func (a *agent) run(prompt, dir string, timeoutSecs int) (string, error) {
	return a.runLive(prompt, dir, timeoutSecs, nil)
}

// runLive is run plus optional live streaming: when live is non-nil the
// agent's stdout is tee'd to it while also being captured for the caller.
func (a *agent) runLive(prompt, dir string, timeoutSecs int, live io.Writer) (string, error) {
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
	if live != nil {
		cmd.Stdout = io.MultiWriter(&out, live)
	} else {
		cmd.Stdout = &out
	}
	cmd.Stderr = &errOut
	if err := cmd.Run(); err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return "", fmt.Errorf("agent %s timed out after %ds", a.name, timeoutSecs)
		}
		return "", fmt.Errorf("agent %s failed: %v: %s", a.name, err, strings.TrimSpace(errOut.String()))
	}
	return out.String(), nil
}

// dumpAgentOutput writes captured agent output to w for diagnosing failures,
// truncated so a pathological response can't flood the terminal.
func dumpAgentOutput(w io.Writer, s string) {
	const max = 4096
	s = strings.TrimSpace(s)
	if len(s) > max {
		s = s[:max] + "\n… (truncated)"
	}
	fmt.Fprintf(w, "── agent output ──\n%s\n──\n", s)
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
	// drop leading prose before the first markdown heading — agents often
	// narrate ("I'll explore the repository…") before emitting the doc
	if i := strings.Index(s, "\n#"); i >= 0 || strings.HasPrefix(s, "#") {
		lines := strings.Split(s, "\n")
		for j, line := range lines {
			if trimmed := strings.TrimSpace(line); strings.HasPrefix(trimmed, "#") {
				s = strings.Join(lines[j:], "\n")
				break
			}
		}
	}
	return strings.TrimSpace(s) + "\n"
}

// readmeExcerpt returns the first ~maxBytes of the repo README, if any.
func readmeExcerpt(root string, maxBytes int) string {
	for _, name := range []string{"README.md", "README", "Readme.md", "readme.md"} {
		data, err := os.ReadFile(filepath.Join(root, name))
		if err == nil && len(data) > 0 {
			s := toUTF8(data)
			if maxBytes > 0 && len(s) > maxBytes {
				s = s[:maxBytes] + "\n…"
			}
			return strings.TrimSpace(s)
		}
	}
	return ""
}

// agentsExcerpt returns the repo's agent-instructions doc (AGENTS.md preferred,
// else CLAUDE.md) and the filename it came from.
func agentsExcerpt(root string) (string, string) {
	for _, name := range []string{"AGENTS.md", "CLAUDE.md", "agents.md", "Agents.md", "claude.md", "Claude.md"} {
		data, err := os.ReadFile(filepath.Join(root, name))
		if err == nil && len(data) > 0 {
			return strings.TrimSpace(toUTF8(data)), name
		}
	}
	return "", ""
}

// repoContext summarizes the repository for prompt grounding.
func repoContext(st Store, maxEntries int) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Repository root: %s\n\nExisting specs:\n", st.Root)
	for _, s := range st.Specs() {
		fmt.Fprintf(&b, "- %s (%s, %s): %s\n", s.ID(), s.Type(), s.Status(), s.Title())
	}
	if facts := loadFacts(st.Root); len(facts) > 0 {
		b.WriteString("\nProject facts (definite values — use these, do not invent):\n")
		for _, f := range facts {
			fmt.Fprintf(&b, "- %s: %s\n", f.Key, f.Value)
		}
	}
	if r := readmeExcerpt(st.Root, 2048); r != "" {
		b.WriteString("\nREADME excerpt:\n")
		b.WriteString(r)
		b.WriteString("\n")
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
- Write Acceptance Criteria as concrete, testable checks — each should be
  provable by a unit test or a command, not a judgment call.
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

const applyPromptFmt = `You are implementing a feature specification in this repository.

Spec %s (%s):

%s

Rules:
- Modify the codebase so the spec's Acceptance Criteria are met.
- Write unit tests covering the Acceptance Criteria — tests are the proof of done.
- Keep changes focused on this spec; do not rewrite unrelated code.
- Do not edit the spec files under specs/.
- When you are done, update the spec file's frontmatter status to "implemented".%s

%s`

const refinePromptFmt = `You are improving an existing %s specification (id %s, title %q).

Current body:

%s

Rules:
- Improve clarity and completeness; keep every existing "## Section" heading.
- Do not invent new REQ ids; only reference existing ones.
- Output ONLY the improved markdown body, no frontmatter.

%s`
