package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os/exec"
	"strings"
)

// cmdCommit implements `spec commit` (FEAT-037): stage everything and
// commit, owning the spec-coverage attestation the pre-push hook
// enforces — callers don't re-implement the gate. --agent (FEAT-038)
// asks the configured agent to write the message — and an honest
// Spec-Exempt reason — from the staged diff.
func cmdCommit(out, errOut io.Writer, args []string) error {
	fs := flag.NewFlagSet("commit", flag.ContinueOnError)
	fs.SetOutput(errOut)
	msg := fs.String("m", "", "commit message (default: diff summary)")
	exempt := fs.String("exempt", "", "Spec-Exempt trailer reason")
	useAgent := fs.Bool("agent", false, "ask the configured agent to write the message")
	if err := fs.Parse(args); err != nil {
		return err
	}
	st, err := mustStore()
	if err != nil {
		return err
	}
	if out2, err := gitOut(st.Root, "add", "-A"); err != nil {
		return fmt.Errorf("git add: %s", out2)
	}
	files := strings.Fields(mustGit(st.Root, "diff", "--cached", "--name-only"))
	if len(files) == 0 {
		fmt.Fprintln(out, "nothing to commit")
		return nil
	}
	// same gate as the pre-push hook: files outside specs/ and .spec/
	// staged without any specs/ file → attestation needed
	outside, covered := false, false
	for _, f := range files {
		switch {
		case strings.HasPrefix(f, "specs/"):
			covered = true
		case !strings.HasPrefix(f, ".spec/"):
			outside = true
		}
	}
	m := *msg
	switch {
	case m != "":
		if *useAgent {
			fmt.Fprintln(errOut, "note: --agent ignored, -m supplied")
		}
	case *useAgent:
		generated, err := agentCommitMessage(st, files, covered)
		if err != nil {
			return err
		}
		m = generated
	default:
		m = diffSummary(st.Root)
	}
	switch {
	case !(outside && !covered):
		// spec-covered or spec-irrelevant — clean commit
	case hasSpecExempt(m):
		// user or agent already wrote the trailer
	case *exempt != "":
		m += "\n\nSpec-Exempt: " + *exempt
	case *msg == "":
		// the tool wrote the message (agent or summary) → auto-attest
		m += "\n\nSpec-Exempt: spec auto-commit — no spec-covered files"
		if *useAgent {
			fmt.Fprintln(errOut, "note: agent did not attest spec-neutrality —")
			fmt.Fprintln(errOut, "      committed with the generic exemption; this may need a spec")
		}
	default:
		fmt.Fprintln(errOut, "note: no specs/ in this commit — the pre-push hook will block")
		fmt.Fprintln(errOut, "      the push. Re-commit with --exempt <reason> or include a spec.")
	}
	res, err := gitOut(st.Root, "commit", "-m", m)
	fmt.Fprint(out, res)
	return err
}

// commitDiffBound caps the staged diff sent to the agent (FEAT-017).
const commitDiffBound = 48 * 1024

const commitPromptFmt = `You are writing the git commit message for a staged change in a spec-managed repository.

Staged files:
%s

Change scale:
%s

Staged diff%s:
%s

Rules:
- First line: imperative summary, at most 72 characters; a body only when needed.
- Output ONLY the commit message — no code fences, no commentary.
%s`

// agentCommitMessage asks the configured agent to write the message —
// and, on uncovered commits, an honest Spec-Exempt reason when the
// change is genuinely spec-neutral. Errors when no agent is configured
// (explicit opt-in that can't be honored must fail visibly) or when the
// agent returns nothing usable.
func agentCommitMessage(st Store, files []string, covered bool) (string, error) {
	cfg := loadConfig(st.Root)
	ag, err := resolveAgent(cfg)
	if err != nil {
		return "", err
	}
	if ag == nil {
		return "", errors.New("--agent needs a configured agent (set agent: or agent_cmd: in .spec/config.yml)")
	}
	diff := mustGit(st.Root, "diff", "--cached")
	note := ""
	if len(diff) > commitDiffBound {
		diff = diff[:commitDiffBound]
		note = " (truncated to first 48 KiB)"
	}
	var coverage string
	if covered {
		coverage = "- A spec under specs/ is staged — do not add any trailer."
	} else {
		coverage = "- No spec under specs/ is staged. If the change is spec-neutral (a defect\n" +
			"  fix restoring documented behavior, a refactor, docs, CI), end the message\n" +
			"  with one trailer line: Spec-Exempt: <short reason>. If it adds or alters\n" +
			"  behavior, do NOT add the trailer — this repo expects a spec."
	}
	prompt := fmt.Sprintf(commitPromptFmt,
		strings.Join(files, "\n"),
		mustGit(st.Root, "diff", "--cached", "--numstat"),
		note, diff, coverage)
	raw, err := ag.run(prompt, st.Root, cfg.AgentTimeout)
	if err != nil {
		return "", err
	}
	m := cleanCommitMessage(raw)
	if m == "" {
		return "", errors.New("agent returned an empty message")
	}
	return m, nil
}

// cleanCommitMessage trims agent output and unwraps a single outer
// code fence.
func cleanCommitMessage(out string) string {
	s := strings.TrimSpace(out)
	if strings.HasPrefix(s, "```") {
		if i := strings.Index(s, "\n"); i >= 0 {
			s = s[i+1:]
		}
		if j := strings.LastIndex(s, "```"); j >= 0 {
			s = s[:j]
		}
	}
	return strings.TrimSpace(s)
}

// hasSpecExempt reports whether the message already carries a
// Spec-Exempt trailer — same grep the pre-push hook performs.
func hasSpecExempt(m string) bool {
	for _, line := range strings.Split(m, "\n") {
		if strings.HasPrefix(line, "Spec-Exempt:") {
			return true
		}
	}
	return false
}

// gitOut runs a git command in dir and returns combined output.
func gitOut(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	b, err := cmd.CombinedOutput()
	return string(b), err
}

// mustGit is gitOut for queries that must succeed (fatals otherwise —
// callers are inside a known git repo).
func mustGit(dir string, args ...string) string {
	out, err := gitOut(dir, args...)
	if err != nil {
		return ""
	}
	return out
}

// diffSummary summarizes staged changes: "update N files (+a -d): x, y, +k more".
func diffSummary(dir string) string {
	out := mustGit(dir, "diff", "--cached", "--numstat")
	adds, dels := 0, 0
	var files []string
	for _, line := range strings.Split(out, "\n") {
		parts := strings.Split(line, "\t")
		if len(parts) != 3 {
			continue
		}
		var a, d int
		if _, err := fmt.Sscanf(parts[0], "%d", &a); err == nil {
			adds += a
			fmt.Sscanf(parts[1], "%d", &d)
			dels += d
		}
		files = append(files, parts[2])
	}
	if len(files) == 0 {
		return "update"
	}
	shown := strings.Join(files[:min(3, len(files))], ", ")
	if len(files) > 3 {
		shown += fmt.Sprintf(", +%d more", len(files)-3)
	}
	noun := "file"
	if len(files) != 1 {
		noun += "s"
	}
	return fmt.Sprintf("update %d %s (+%d -%d): %s", len(files), noun, adds, dels, shown)
}
