package main

import (
	"fmt"
	"io"
	"os"
	"os/exec"
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
	cfg := loadConfig(st.Root)
	if os.Getenv("SPEC_SKIP") == "1" {
		fmt.Fprintln(errOut, "note: SPEC_SKIP=1 — coverage check bypassed")
	} else {
		// push range: <remote>/<branch>..HEAD when that ref exists, else
		// every commit not on any remote — same rule the hook uses. The
		// remote comes from push_command when it's a parseable `git push
		// <remote> ...`; anything else falls back to --not --remotes
		// (a safe superset — everything unpushed anywhere is checked).
		remote := gateRemote(cfg.PushCommand)
		var rangeSpec string
		if remote != "" {
			if _, err := gitOut(st.Root, "show-ref", "--verify", "--quiet",
				"refs/remotes/"+remote+"/"+branch); err == nil {
				rangeSpec = remote + "/" + branch + "..HEAD"
			}
		}
		if rangeSpec == "" {
			rangeSpec = "HEAD --not --remotes"
		}
		if err := specCoverageGate(st.Root, rangeSpec, errOut); err != nil {
			return err
		}
	}
	argv, err := pushArgv(cfg.PushCommand, branch)
	if err != nil {
		return err
	}
	if err := runArgv(st.Root, argv, errOut); err != nil {
		return err
	}
	fmt.Fprintf(out, "pushed %s\n", branch)
	return nil
}

// gateRemote extracts the remote name from a push_command shaped like
// `git push <remote> ...`; anything else returns "" (the gate falls back
// to checking commits not on any remote).
func gateRemote(pushCommand string) string {
	argv, err := splitArgs(pushCommand)
	if err != nil || len(argv) < 3 {
		return "origin"
	}
	if argv[0] == "git" && argv[1] == "push" && !strings.HasPrefix(argv[2], "-") {
		return argv[2]
	}
	return ""
}

// pushArgv renders the configured push_command into argv — {ref} and
// {branch} both expand to the ref being pushed (the current branch for
// `spec push` and release's branch push, the tag for release's tag push)
// — same substitution contract as tasks.py's TASKS_PUSH_COMMAND. Absent
// config means `git push origin <ref>`.
func pushArgv(pushCommand, ref string) ([]string, error) {
	tpl := strings.TrimSpace(pushCommand)
	if tpl == "" {
		return []string{"git", "push", "origin", ref}, nil
	}
	argv, err := splitArgs(tpl)
	if err != nil {
		return nil, fmt.Errorf("push_command: %w", err)
	}
	if len(argv) == 0 {
		return nil, fmt.Errorf("push_command is empty")
	}
	for i, a := range argv {
		a = strings.ReplaceAll(a, "{branch}", ref)
		a = strings.ReplaceAll(a, "{ref}", ref)
		argv[i] = a
	}
	return argv, nil
}

// runArgv executes a push command, printing its output on failure.
func runArgv(root string, argv []string, errOut io.Writer) error {
	cmd := exec.Command(argv[0], argv[1:]...)
	cmd.Dir = root
	b, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s: %s", strings.Join(argv, " "), b)
	}
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
