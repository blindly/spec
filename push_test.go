package main

import (
	"bytes"
	"io"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// FEAT-042: spec push runs the coverage gate in-process before pushing.
func TestSpecPush(t *testing.T) {
	st := setupRepo(t)
	commit := gitInit(t, st)
	writeFile(t, st, "a.txt", "x")
	commit("chore: baseline")

	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", st.Root}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	remote := filepath.Join(t.TempDir(), "r.git")
	run("init", "--bare", remote)
	run("remote", "add", "origin", remote)

	var out, errs bytes.Buffer

	// uncovered + unattested commit → refused, names the commit
	writeFile(t, st, "foo.go", "package x\n")
	commit("code only")
	if err := cmdPush(&out, &errs); err == nil {
		t.Fatal("unattested uncovered push must refuse")
	}
	if !strings.Contains(errs.String(), "Spec-Exempt") {
		t.Fatalf("want the attestation guidance:\n%s", errs.String())
	}

	// SPEC_SKIP bypasses — same contract as the hook
	t.Setenv("SPEC_SKIP", "1")
	out.Reset()
	if err := cmdPush(&out, &errs); err != nil {
		t.Fatalf("SPEC_SKIP should push: %v\n%s", err, errs.String())
	}
	t.Setenv("SPEC_SKIP", "")

	// covered range: a specs/ change pushes without attestation
	writeFile(t, st, "specs/features/FEAT-009-x.md",
		"---\nid: FEAT-009\ntype: feature\nstatus: draft\n---\n# x\n")
	writeFile(t, st, "bar.go", "package x\n")
	commit("feat: new thing")
	out.Reset()
	if err := cmdPush(&out, &errs); err != nil {
		t.Fatalf("spec-covered push should pass: %v\n%s", err, errs.String())
	}
	if !strings.Contains(out.String(), "pushed") {
		t.Fatalf("want pushed confirmation:\n%s", out.String())
	}

	// non-feat: prefix attests — pushes even when uncovered
	writeFile(t, st, "baz.go", "package x\n")
	commit("fix: regression")
	if err := cmdPush(&out, io.Discard); err != nil {
		t.Fatalf("non-feat: prefix attests: %v", err)
	}

	// push_command redirects the push — {ref} expands to the branch
	backup := filepath.Join(t.TempDir(), "b.git")
	run("init", "--bare", backup)
	run("remote", "add", "backup", backup)
	writeFile(t, st, ".spec/config.yml", "push_command: git push backup {ref}\n")
	writeFile(t, st, "specs/features/FEAT-009-x.md",
		"---\nid: FEAT-009\ntype: feature\nstatus: draft\n---\n# y\n")
	commit("chore: another")
	if err := cmdPush(&out, io.Discard); err != nil {
		t.Fatalf("push_command should redirect: %v", err)
	}
	got := strings.TrimSpace(mustGit(st.Root, "ls-remote", "backup", "HEAD"))
	if got == "" {
		t.Fatal("push_command target did not receive the branch")
	}
}
