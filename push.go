package main

import (
	"fmt"
	"io"
	"os"
	"strings"
)

// cmdPush pushes the current branch after running the spec-coverage
// check in-process (FEAT-042) — the same rule the pre-push hook enforces,
// but native: no .git/hooks, no bash.
func cmdPush(out, errOut io.Writer) error {
	st, err := mustStore()
	if err != nil {
		return err
	}
	branch := strings.TrimSpace(mustGit(st.Root, "branch", "--show-current"))
	if branch == "" {
		return fmt.Errorf("detached HEAD — nothing to push")
	}
	if out2, err := gitOut(st.Root, "remote", "get-url", "origin"); err != nil {
		return fmt.Errorf("no origin remote: %s", out2)
	}
	if os.Getenv("SPEC_SKIP") == "1" {
		fmt.Fprintln(errOut, "note: SPEC_SKIP=1 — coverage check bypassed")
	} else {
		// push range: origin/<branch>..HEAD when that ref exists, else
		// every commit not on any remote — same rule the hook uses
		remote := "origin/" + branch
		var rangeSpec string
		if _, err := gitOut(st.Root, "show-ref", "--verify", "--quiet", "refs/remotes/"+remote); err == nil {
			rangeSpec = remote + "..HEAD"
		} else {
			rangeSpec = "HEAD --not --remotes"
		}
		if err := specCoverageGate(st.Root, rangeSpec, errOut); err != nil {
			return err
		}
	}
	if out2, err := gitOut(st.Root, "push", "origin", branch); err != nil {
		return fmt.Errorf("git push: %s", out2)
	}
	fmt.Fprintf(out, "pushed %s\n", branch)
	return nil
}

// specCoverageGate refuses a push range that changes files outside
// specs/ and .spec/ without a spec change, unless every new commit
// attests spec-neutrality (Spec-Exempt trailer or non-feat: prefix).
// SPEC_SKIP=1 bypasses — same contract as the pre-push hook.
func specCoverageGate(root, rangeSpec string, errOut io.Writer) error {
	changed := strings.Fields(mustGit(root, append([]string{"log", "--format=", "--name-only"}, strings.Fields(rangeSpec)...)...))
	outside, covered := false, false
	for _, f := range changed {
		switch {
		case strings.HasPrefix(f, "specs/"):
			covered = true
		case !strings.HasPrefix(f, ".spec/"):
			outside = true
		}
	}
	if !outside || covered {
		return nil
	}
	var unmarked []string
	for _, sha := range strings.Fields(mustGit(root, append([]string{"rev-list", "--no-merges"}, strings.Fields(rangeSpec)...)...)) {
		body := mustGit(root, "log", "-1", "--format=%B", sha)
		if strings.Contains(body, "\nSpec-Exempt:") || strings.HasPrefix(body, "Spec-Exempt:") {
			continue
		}
		subj, _, _ := strings.Cut(body, "\n")
		if nonFeatPrefix(strings.TrimSpace(subj)) {
			continue
		}
		unmarked = append(unmarked, strings.TrimSpace(mustGit(root, "log", "-1", "--format=%h %s", sha)))
	}
	if len(unmarked) == 0 {
		return nil
	}
	fmt.Fprintln(errOut, "spec push: code changed without a spec update, and these commits")
	fmt.Fprintln(errOut, "           lack a 'Spec-Exempt: <reason>' trailer or non-feat: prefix:")
	for _, c := range unmarked {
		fmt.Fprintln(errOut, "            "+c)
	}
	fmt.Fprintln(errOut, "           add or amend a spec under specs/ (see: spec new feature),")
	fmt.Fprintln(errOut, "           re-commit a defect fix with a Spec-Exempt: trailer or")
	fmt.Fprintln(errOut, "           non-feat: prefix, or SPEC_SKIP=1 spec push")
	return fmt.Errorf("push blocked — %d unattested commit(s)", len(unmarked))
}
