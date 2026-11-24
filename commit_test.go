package main

import (
	"bytes"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
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

	// uncovered + auto message: chore: prefix attests — no trailer needed
	writeFile(t, st, "baz.go", "package x\n")
	if err := cmdCommit(&out, io.Discard, nil); err != nil {
		t.Fatal(err)
	}
	if m := lastCommitMsg(t, st); strings.Contains(m, "Spec-Exempt") || !strings.HasPrefix(m, "chore: update ") {
		t.Fatalf("want chore: summary, prefix is the attestation:\n%s", m)
	}

	// uncovered + non-feat: prefix: the prefix itself attests — no
	// trailer, no warning
	writeFile(t, st, "qux.go", "package x\n")
	errs.Reset()
	if err := cmdCommit(&out, &errs, []string{"-m", "fix: panic on empty config"}); err != nil {
		t.Fatal(err)
	}
	if m := lastCommitMsg(t, st); strings.Contains(m, "Spec-Exempt") {
		t.Fatalf("fix: prefix attests without a trailer:\n%s", m)
	}
	if errs.String() != "" {
		t.Fatalf("fix: prefix should commit silently:\n%s", errs.String())
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

// fakeCommitAgent installs an agent_cmd that saves its prompt and
// replies with the canned message.
func fakeCommitAgent(t *testing.T, st Store, output string) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("shell-script fake agent")
	}
	dir := t.TempDir()
	outFile, promptFile := filepath.Join(dir, "out.txt"), filepath.Join(dir, "prompt.txt")
	os.WriteFile(outFile, []byte(output), 0o644)
	fake := filepath.Join(dir, "agent.sh")
	os.WriteFile(fake, []byte("#!/bin/sh\ncat > '"+promptFile+"'\ncat '"+outFile+"'\n"), 0o755)
	writeFile(t, st, ".spec/config.yml", "agent_cmd: sh "+fake+"\n")
	return promptFile
}

// FEAT-038: --agent asks the configured agent for the message and,
// on uncovered commits, an honest Spec-Exempt reason.
func TestSpecCommitAgent(t *testing.T) {
	st := setupRepo(t)
	commit := gitInit(t, st)
	commit("baseline")
	var out, errs bytes.Buffer

	// agent emits a message carrying its own exemption → kept verbatim
	promptFile := fakeCommitAgent(t, st,
		"fix empty-config panic\n\nSpec-Exempt: restores REQ-017 documented behavior\n")
	writeFile(t, st, "foo.go", "package x\n")
	if err := cmdCommit(&out, &errs, []string{"--agent"}); err != nil {
		t.Fatal(err)
	}
	if m := lastCommitMsg(t, st); !strings.Contains(m, "fix empty-config panic") ||
		!strings.Contains(m, "Spec-Exempt: restores REQ-017 documented behavior") ||
		strings.Contains(m, "spec auto-commit") {
		t.Fatalf("want agent message verbatim:\n%s", m)
	}
	prompt, _ := os.ReadFile(promptFile)
	if !strings.Contains(string(prompt), "foo.go") ||
		!strings.Contains(string(prompt), "No spec under specs/") {
		t.Fatalf("prompt should carry the staged files and coverage state:\n%s", prompt)
	}

	// agent emits an unattested message on an uncovered commit → the
	// commit lands but warns the push will fail; spec doesn't stamp it
	fakeCommitAgent(t, st, "add experimental flag\n")
	writeFile(t, st, "bar.go", "package x\n")
	errs.Reset()
	if err := cmdCommit(&out, &errs, []string{"--agent"}); err != nil {
		t.Fatal(err)
	}
	if m := lastCommitMsg(t, st); !strings.Contains(m, "add experimental flag") ||
		strings.Contains(m, "Spec-Exempt") {
		t.Fatalf("want agent message, no rubber stamp:\n%s", m)
	}
	if !strings.Contains(errs.String(), "pre-push") {
		t.Fatalf("want push-block warning:\n%s", errs.String())
	}
	// an agent fix: prefix attests on its own — no trailer appended
	fakeCommitAgent(t, st, "fix: flaky test teardown\n")
	writeFile(t, st, "test.go", "package x\n")
	errs.Reset()
	if err := cmdCommit(&out, &errs, []string{"--agent"}); err != nil {
		t.Fatal(err)
	}
	if m := lastCommitMsg(t, st); strings.Contains(m, "Spec-Exempt") {
		t.Fatalf("fix: prefix attests without a trailer:\n%s", m)
	}
	if errs.String() != "" {
		t.Fatalf("fix: prefix should commit silently:\n%s", errs.String())
	}

	// -m beats --agent, noted on stderr
	writeFile(t, st, "baz.go", "package x\n")
	errs.Reset()
	if err := cmdCommit(&out, &errs, []string{"--agent", "-m", "explicit", "--exempt", "ok"}); err != nil {
		t.Fatal(err)
	}
	if m := lastCommitMsg(t, st); !strings.HasPrefix(m, "explicit") {
		t.Fatalf("want explicit message:\n%s", m)
	}
	if !strings.Contains(errs.String(), "--agent ignored") {
		t.Fatalf("want ignored note:\n%s", errs.String())
	}

	// covered commit + --agent: agent message used, no trailer
	fakeCommitAgent(t, st, "add FEAT-009 spec\n")
	writeFile(t, st, "specs/features/FEAT-009-x.md",
		"---\nid: FEAT-009\ntype: feature\nstatus: draft\n---\n# x\n")
	writeFile(t, st, "more.go", "package x\n")
	if err := cmdCommit(&out, &errs, []string{"--agent"}); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(lastCommitMsg(t, st), "Spec-Exempt") {
		t.Fatal("spec-covered commit should not attest")
	}
}

// FEAT-038: --agent without a configured agent fails visibly.
func TestSpecCommitAgentUnconfigured(t *testing.T) {
	st := setupRepo(t)
	gitInit(t, st)
	writeFile(t, st, "foo.go", "package x\n")
	err := cmdCommit(io.Discard, io.Discard, []string{"--agent"})
	if err == nil || !strings.Contains(err.Error(), "configured agent") {
		t.Fatalf("want configured-agent error: %v", err)
	}
}

// FEAT-039: a VERSION file opts a repo into stamping; spec commit
// refreshes it and spec release tags it.
var stampRe = regexp.MustCompile(`^1\.\d{2}\.\d{2}-build-\d{6}$`)

func TestSpecCommitStampsVersion(t *testing.T) {
	st := setupRepo(t)
	commit := gitInit(t, st)
	writeFile(t, st, "VERSION", "1.25.01-build-010000\n")
	writeFile(t, st, "a.txt", "x")
	commit("chore: baseline")

	writeFile(t, st, "a.txt", "y")
	var out, errs bytes.Buffer
	if err := cmdCommit(&out, &errs, []string{"-m", "chore: change"}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(st.Root, "VERSION"))
	if err != nil || !stampRe.MatchString(strings.TrimSpace(string(data))) {
		t.Fatalf("VERSION not stamped: %q %v", data, err)
	}
	if files := mustGit(st.Root, "show", "--name-only", "--format=", "HEAD"); !strings.Contains(files, "VERSION") {
		t.Fatalf("VERSION did not ride the commit: %s", files)
	}

	// version_stamp: false opts the repo back out
	cfg, _ := os.ReadFile(filepath.Join(st.Root, ".spec", "config.yml"))
	writeFile(t, st, ".spec/config.yml", string(cfg)+"\nversion_stamp: false\n")
	before := string(data)
	writeFile(t, st, "a.txt", "z")
	out.Reset()
	if err := cmdCommit(&out, &errs, nil); err != nil {
		t.Fatal(err)
	}
	if after, _ := os.ReadFile(filepath.Join(st.Root, "VERSION")); string(after) != before {
		t.Fatalf("version_stamp: false still stamped: %s", after)
	}
}

func TestSpecRelease(t *testing.T) {
	st := setupRepo(t)
	commit := gitInit(t, st)
	writeFile(t, st, "VERSION", "9.25.01-build-010000\n")
	commit("chore: baseline")
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", st.Root}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("config", "user.name", "t")
	run("config", "user.email", "t@t")
	remote := filepath.Join(t.TempDir(), "r.git")
	run("init", "--bare", remote)
	run("remote", "add", "origin", remote)

	var out, errs bytes.Buffer
	// release is off until the repo opts in
	if err := cmdRelease(&out, &errs, nil); err == nil || !strings.Contains(err.Error(), "release: true") {
		t.Fatalf("want not-enabled refusal: %v", err)
	}
	cfg, _ := os.ReadFile(filepath.Join(st.Root, ".spec", "config.yml"))
	writeFile(t, st, ".spec/config.yml", string(cfg)+"\nrelease: true\n")
	commit("chore: enable release")

	// dirty tree refuses — the release commit must be VERSION-only
	writeFile(t, st, "a.txt", "uncommitted")
	if err := cmdRelease(&out, &errs, nil); err == nil || !strings.Contains(err.Error(), "spec commit first") {
		t.Fatalf("want dirty-tree refusal: %v", err)
	}
	run("clean", "-f", "a.txt")

	if err := cmdRelease(&out, &errs, nil); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(filepath.Join(st.Root, "VERSION"))
	stamp := strings.TrimSpace(string(data))
	if !regexp.MustCompile(`^9\.\d{2}\.\d{2}-build-\d{6}$`).MatchString(stamp) {
		t.Fatalf("VERSION not stamped: %q", stamp)
	}
	if msg := strings.TrimSpace(lastCommitMsg(t, st)); msg != "chore: bump "+stamp {
		t.Fatalf("release commit: %q", msg)
	}
	if files := strings.TrimSpace(mustGit(st.Root, "show", "--name-only", "--format=", "HEAD")); files != "VERSION" {
		t.Fatalf("release commit not VERSION-only: %s", files)
	}
	if !strings.Contains(mustGit(st.Root, "ls-remote", "--tags", "origin"), "refs/tags/v"+stamp) {
		t.Fatal("tag not pushed")
	}

	// a second release on the same minute re-tags — must refuse
	if err := cmdRelease(&out, &errs, nil); err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("want existing-tag refusal: %v", err)
	}
}

// FEAT-038: commit_agent: true makes the agent the default; --no-agent
// opts a single commit back to the offline summary.
func TestSpecCommitAgentDefault(t *testing.T) {
	st := setupRepo(t)
	commit := gitInit(t, st)
	commit("baseline")
	var out, errs bytes.Buffer

	promptFile := fakeCommitAgent(t, st, "fix: handle nil map\n")
	// fakeCommitAgent rewrote config — add the default-on key
	cfg, _ := os.ReadFile(filepath.Join(st.Root, ".spec", "config.yml"))
	writeFile(t, st, ".spec/config.yml", string(cfg)+"commit_agent: true\n")

	// bare commit uses the agent when commit_agent is set
	writeFile(t, st, "foo.go", "package x\n")
	if err := cmdCommit(&out, &errs, nil); err != nil {
		t.Fatal(err)
	}
	if m := lastCommitMsg(t, st); !strings.Contains(m, "fix: handle nil map") {
		t.Fatalf("want agent message by default:\n%s", m)
	}
	if prompt, _ := os.ReadFile(promptFile); len(prompt) == 0 {
		t.Fatal("agent was not invoked")
	}

	// --no-agent falls back to the offline summary
	writeFile(t, st, "bar.go", "package x\n")
	errs.Reset()
	if err := cmdCommit(&out, &errs, []string{"--no-agent"}); err != nil {
		t.Fatal(err)
	}
	if m := lastCommitMsg(t, st); !strings.HasPrefix(m, "chore: update") {
		t.Fatalf("--no-agent should use the summary:\n%s", m)
	}
}
