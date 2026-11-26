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
	noAgent := fs.Bool("no-agent", false, "skip the agent even when commit_agent is set")
	noCheck := fs.Bool("no-check", false, "skip the configured pre_commit checks for this commit")
	runVerifyToo := fs.Bool("verify", false, "also run the configured verify commands before committing")
	if err := fs.Parse(args); err != nil {
		return err
	}
	st, err := mustStore()
	if err != nil {
		return err
	}
	cfg := loadConfig(st.Root)
	// FEAT-041: pre_commit checks gate the commit — run before stamping or
	// staging so a failure leaves the tree untouched.
	if !*noCheck && len(cfg.PreCommit) > 0 {
		if err := runChecks(st.Root, cfg, cfg.PreCommit, out, errOut); err != nil {
			return fmt.Errorf("pre-commit checks failed — fix, or --no-check to skip")
		}
	}
	if *runVerifyToo && len(cfg.Verify) > 0 {
		if err := runChecks(st.Root, cfg, cfg.Verify, out, errOut); err != nil {
			return fmt.Errorf("verify checks failed")
		}
	}
	// A VERSION file opts the repo into stamped releases — refresh it so
	// every commit carries a fresh build stamp (version_stamp: false in
	// config opts back out).
	if cfg.VersionStamp {
		if _, err := stampVersion(st.Root, 0); err != nil {
			return fmt.Errorf("VERSION: %w", err)
		}
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
	// explicit --agent always engages (errors when unconfigured); the
	// default engages only when an agent is actually configured, so
	// agent-less repos stay offline silently
	wantAgent := !*noAgent && (*useAgent || (cfg.CommitAgent && agentConfigured(cfg)))
	m := *msg
	switch {
	case m != "":
		if *useAgent {
			fmt.Fprintln(errOut, "note: --agent ignored, -m supplied")
		}
	case wantAgent:
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
	case specExemptMessage(m):
		// user or agent already attested — trailer or non-feat: prefix
	case *exempt != "":
		m += "\n\nSpec-Exempt: " + *exempt
	default:
		// unattested — explicit -m without a marker, or a generated
		// message (feat:, unprefixed) the agent declined to attest.
		// Commit anyway but warn: spec doesn't rubber-stamp.
		fmt.Fprintln(errOut, "note: no specs/ in this commit — the pre-push hook will block")
		fmt.Fprintln(errOut, "      the push. Re-commit with --exempt <reason>, a non-feat: prefix,")
		fmt.Fprintln(errOut, "      or include a spec.")
	}
	// --no-verify: spec commit already ran the pre_commit checks and its
	// own coverage gate — the hooks exist for the plain-git bypass path.
	res, err := gitOut(st.Root, "commit", "--no-verify", "-m", m)
	fmt.Fprint(out, res)
	if err != nil {
		return err
	}
	// push_on_commit: the repo wants every commit to go out — run the same
	// gated push `spec push` performs (push_command honored, gate refusal
	// leaves the commit in place).
	if cfg.PushOnCommit {
		return cmdPush(out, errOut)
	}
	return nil
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
- First line: conventional-commit format, at most 72 characters —
  "type: imperative summary" using one of feat, fix, docs, test, chore,
  refactor, perf, style, ci, build, revert; a body only when needed.
  "feat" means behavior was added or altered.
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
			"  fix restoring documented behavior, a refactor, docs, CI), its non-feat:\n" +
			"  prefix is the attestation — or end the message with a Spec-Exempt:\n" +
			"  <short reason> trailer for anything the type can't express. If it\n" +
			"  adds or alters behavior (feat:), neither applies — this repo expects\n" +
			"  a spec and the push will be rejected."
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

// specExemptMessage reports whether a commit message attests
// spec-neutrality: a Spec-Exempt trailer (explicit, with reason) or a
// non-feat conventional-commit prefix on the first line (the routine
// categories — a "feat" label declares behavior work and never
// attests; neither does a breaking "type!" marker). Same rule the
// spec-managed hooks apply.
func specExemptMessage(m string) bool {
	for _, line := range strings.Split(m, "\n") {
		if strings.HasPrefix(line, "Spec-Exempt:") {
			return true
		}
	}
	subject, _, _ := strings.Cut(m, "\n")
	return nonFeatPrefix(subject)
}

// nonFeatPrefixes are the conventional-commit types that attest
// spec-neutral work — everything not "feat".
var nonFeatPrefixes = []string{
	"fix", "docs", "test", "chore", "refactor",
	"perf", "style", "ci", "build", "revert",
}

func nonFeatPrefix(subject string) bool {
	for _, t := range nonFeatPrefixes {
		rest, ok := strings.CutPrefix(subject, t)
		if !ok {
			continue
		}
		// fix: ... · fix(scope): ... — but never fix!: / fix(scope)!:
		if strings.HasPrefix(rest, ":") {
			return true
		}
		if strings.HasPrefix(rest, "(") {
			if i := strings.Index(rest, ")"); i > 0 {
				return strings.HasPrefix(rest[i+1:], ":")
			}
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
	return fmt.Sprintf("chore: update %d %s (+%d -%d): %s", len(files), noun, adds, dels, shown)
}
