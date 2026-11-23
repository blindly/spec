package main

import (
	"bytes"
	"io"
	"os/exec"
	"strings"
	"testing"
)

func lastCommitMsg(t *testing.T, st Store) string {
	t.Helper()
	b, err := exec.Command("git", "-C", st.Root, "log", "-1", "--format=%B").Output()
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// FEAT-037: spec commit owns the Spec-Exempt attestation the hook needs.
func TestSpecCommit(t *testing.T) {
	st := setupRepo(t)
	commit := gitInit(t, st)
	commit("baseline")
	var out, errs bytes.Buffer

	// uncovered + explicit -m: commits, warns, no auto-attestation
	writeFile(t, st, "foo.go", "package x\n")
	if err := cmdCommit(&out, &errs, []string{"-m", "code only"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(errs.String(), "pre-push") {
		t.Fatalf("want hook warning:\n%s", errs.String())
	}
	if strings.Contains(lastCommitMsg(t, st), "Spec-Exempt") {
		t.Fatal("explicit -m must not auto-attest")
	}

	// uncovered + --exempt: trailer with the reason
	writeFile(t, st, "bar.go", "package x\n")
	errs.Reset()
	if err := cmdCommit(&out, &errs, []string{"-m", "fix", "--exempt", "restores documented behavior"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(lastCommitMsg(t, st), "Spec-Exempt: restores documented behavior") {
		t.Fatalf("want exempt trailer:\n%s", lastCommitMsg(t, st))
	}

	// uncovered + auto message: harness attests
	writeFile(t, st, "baz.go", "package x\n")
	if err := cmdCommit(&out, io.Discard, nil); err != nil {
		t.Fatal(err)
	}
	if m := lastCommitMsg(t, st); !strings.Contains(m, "Spec-Exempt: spec auto-commit") || !strings.HasPrefix(m, "update ") {
		t.Fatalf("want auto summary + auto attestation:\n%s", m)
	}

	// covered commit: no trailer
	writeFile(t, st, "specs/features/FEAT-009-x.md",
		"---\nid: FEAT-009\ntype: feature\nstatus: draft\n---\n# x\n")
	writeFile(t, st, "more.go", "package x\n")
	if err := cmdCommit(&out, io.Discard, nil); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(lastCommitMsg(t, st), "Spec-Exempt") {
		t.Fatal("spec-covered commit should not attest")
	}

	// clean tree → nothing to commit
	out.Reset()
	if err := cmdCommit(&out, io.Discard, nil); err != nil ||
		!strings.Contains(out.String(), "nothing to commit") {
		t.Fatalf("want nothing-to-commit: %v\n%s", err, out.String())
	}
}
