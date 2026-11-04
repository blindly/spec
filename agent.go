package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
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
	"codex":    {"codex", "exec", "--skip-git-repo-check", "--dangerously-bypass-approvals-and-sandbox", "{prompt}"},
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
		argv, err := splitArgs(cfg.AgentCmd)
		if err != nil {
			return nil, fmt.Errorf("agent_cmd: %w", err)
		}
		if len(argv) == 0 {
			return nil, fmt.Errorf("agent_cmd is empty")
		}
		return &agent{name: "custom", argv: argv}, nil
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

// agentTimeoutError is returned when the agent is killed by agent_timeout;
// apply checks for it because a timed-out agent may still have finished.
type agentTimeoutError struct {
	name string
	secs int
}

func (e *agentTimeoutError) Error() string {
	return fmt.Sprintf("agent %s timed out after %ds", e.name, e.secs)
}

// run sends prompt to the agent and returns its stdout. dir is the cwd.
func (a *agent) run(prompt, dir string, timeoutSecs int) (string, error) {
	return a.runLive(prompt, dir, timeoutSecs, nil)
}

// maxArgBytes guards against E2BIG ("argument list too long"): the kernel
// caps a single argv entry (Linux MAX_ARG_STRLEN is 128 KiB), so an
// oversized prompt must be delivered via stdin instead — same as an agent
// argv with no {prompt}.
const maxArgBytes = 120 * 1024

// runLive is run plus optional live streaming: when live is non-nil the
// agent's stdout is tee'd to it while also being captured for the caller.
func (a *agent) runLive(prompt, dir string, timeoutSecs int, live io.Writer) (string, error) {
	var argv []string
	usesPlaceholder := false
	for _, arg := range a.argv {
		if strings.Contains(arg, "{prompt}") {
			if sub := strings.ReplaceAll(arg, "{prompt}", prompt); len(sub) <= maxArgBytes {
				usesPlaceholder = true
				argv = append(argv, sub)
			}
		} else {
			argv = append(argv, arg)
		}
	}
	if len(argv) == 0 {
		return "", fmt.Errorf("agent %s has no executable in argv", a.name)
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
			return "", &agentTimeoutError{name: a.name, secs: timeoutSecs}
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
	// some agents glue the narration straight onto the heading with no
	// newline ("…context first.# Title"); cut at the heading anyway
	if !strings.HasPrefix(s, "#") {
		if m := gluedHeading.FindStringIndex(s); m != nil {
			s = s[m[0]+1:]
		}
	}
	return strings.TrimSpace(s) + "\n"
}

var typeSuggestionPattern = regexp.MustCompile(`(?m)^\s*SPEC-TYPE-SUGGESTION:\s*(\w+)\s*:?\s*(.*)$`)

// gluedHeading matches a markdown heading fused onto narration-ending
// punctuation without a newline, e.g. "…explore first.# Title".
var gluedHeading = regexp.MustCompile(`[.!?]# `)

// extractTypeSuggestion strips the refine agent's SPEC-TYPE-SUGGESTION line
// from its output and returns the suggested type and reason. The suggestion
// is dropped when it names an unknown type or the spec's current type.
func extractTypeSuggestion(output, current string) (rest, suggested, reason string) {
	m := typeSuggestionPattern.FindStringSubmatch(output)
	rest = typeSuggestionPattern.ReplaceAllString(output, "")
	if m == nil {
		return rest, "", ""
	}
	t := strings.ToLower(m[1])
	if !specTypes[t] || t == current {
		return rest, "", ""
	}
	return rest, t, strings.TrimSpace(m[2])
}

// readmeExcerpt returns the first ~maxBytes of the repo README, if any.
func readmeExcerpt(root string, maxBytes int) string {
	s, _ := findDoc(root, "README.md", "README.markdown", "README", "README.txt")
	if maxBytes > 0 && len(s) > maxBytes {
		s = strings.TrimSpace(s[:maxBytes]) + "\n…"
	}
	return s
}

// agentsExcerpt returns the repo's agent-instructions doc (AGENTS.md preferred,
// else CLAUDE.md) and the filename it came from.
func agentsExcerpt(root string) (string, string) {
	return findDoc(root, "AGENTS.md", "CLAUDE.md")
}

// findDoc returns the content and actual filename of the first non-empty file
// in root matching one of names, compared case-insensitively (README.MD,
// readme.md, … all match README.md). Earlier names take precedence.
func findDoc(root string, names ...string) (string, string) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return "", ""
	}
	for _, want := range names {
		for _, e := range entries {
			if e.IsDir() || !strings.EqualFold(e.Name(), want) {
				continue
			}
			data, err := os.ReadFile(filepath.Join(root, e.Name()))
			if err == nil && len(data) > 0 {
				return strings.TrimSpace(toUTF8(data)), e.Name()
			}
		}
	}
	return "", ""
}

// repoContext summarizes the repository for prompt grounding. At most
// maxEntries specs are listed (<=0 means all); the rest collapse into a
// count so huge repos can't build an unbounded prompt.
func repoContext(st Store, maxEntries int) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Repository root: %s\n\nExisting specs (implemented = describes the current system):\n", st.Root)
	specs := st.Specs()
	more := 0
	if maxEntries > 0 && len(specs) > maxEntries {
		more = len(specs) - maxEntries
		specs = specs[:maxEntries]
	}
	for _, s := range specs {
		note := ""
		if s.Archived {
			note = ", archived — history, may be outdated"
		}
		fmt.Fprintf(&b, "- %s (%s, %s%s): %s\n", s.ID(), s.Type(), s.Status(), note, s.Title())
	}
	if more > 0 {
		fmt.Fprintf(&b, "- … and %d more specs\n", more)
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
	if role, _ := roleFor(st, loadConfig(st.Root)); role != "" {
		b.WriteString("\nDevelopment role (follow this persona and these conventions):\n")
		b.WriteString(role)
		b.WriteByte('\n')
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
- Keep the other specs accurate. Implemented specs are the living
  documentation of this system: if your change alters behavior that another
  spec under specs/ describes, update that spec's text in the same change so
  it matches what the system now does. Do not change other specs' status,
  id, or type, and leave specs/archive/ alone (it is history). For accepted
  ADRs, don't edit the original sections: add a dated entry under a
  "## Corrections" section at the end instead.
- When you are done, set this spec's frontmatter status to "implemented".
  spec checks that status to decide whether the work is finished, so leave
  it unchanged if you could not complete the spec.
- In your final message, list any other specs you updated and why.%s

%s`

const ideaRefinePromptFmt = `You are refining an untyped idea (id %s, title %q) from a spec inbox.

Current notes:

%s

Rules:
- Flesh out the notes: clarify the problem, the goal, and open questions. Keep
  it short and informal — this is not a spec yet, so don't add spec sections.
- Keep the "# <title>" heading and a "## Notes" section; output ONLY the
  markdown body, no frontmatter.
- Then decide which spec type this idea should become:
  - project: describes a whole system and its requirements (usually one per repo)
  - feature: behavior to build or change, with outcomes you could check off
  - adr: a decision between alternatives, worth recording why
- End your output with exactly one final line, after the body:
  SPEC-TYPE-SUGGESTION: <project|feature|adr>: <one-sentence reason>

%s`

const refinePromptFmt = `You are improving an existing %s specification (id %s, title %q).

Current body:

%s

Rules:
- Improve clarity and completeness; keep every existing "## Section" heading.
- Do not invent new REQ ids; only reference existing ones.
- Output ONLY the improved markdown body, no frontmatter.
- Keep it a %s spec. If the content clearly belongs to a different type
  (project: whole system + REQ ids; feature: behavior to build with checkable
  acceptance criteria; adr: a decision with rejected alternatives), add one
  final line, after the body:
  SPEC-TYPE-SUGGESTION: <project|feature|adr>: <one-sentence reason>
  Omit that line when the type fits.

%s`
