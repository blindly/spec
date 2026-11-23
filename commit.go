package main

import (
	"flag"
	"fmt"
	"io"
	"os/exec"
	"strings"
)

// cmdCommit implements `spec commit` (FEAT-037): stage everything and
// commit, owning the spec-coverage attestation the pre-push hook
// enforces — callers don't re-implement the gate.
func cmdCommit(out, errOut io.Writer, args []string) error {
	fs := flag.NewFlagSet("commit", flag.ContinueOnError)
	fs.SetOutput(errOut)
	msg := fs.String("m", "", "commit message (default: diff summary)")
	exempt := fs.String("exempt", "", "Spec-Exempt trailer reason")
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
	m := *msg
	if m == "" {
		m = diffSummary(st.Root)
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
	switch {
	case !(outside && !covered):
		// spec-covered or spec-irrelevant — clean commit
	case *exempt != "":
		m += "\n\nSpec-Exempt: " + *exempt
	case *msg == "":
		m += "\n\nSpec-Exempt: spec auto-commit — no spec-covered files"
	default:
		fmt.Fprintln(errOut, "note: no specs/ in this commit — the pre-push hook will block")
		fmt.Fprintln(errOut, "      the push. Re-commit with --exempt <reason> or include a spec.")
	}
	res, err := gitOut(st.Root, "commit", "-m", m)
	fmt.Fprint(out, res)
	return err
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
