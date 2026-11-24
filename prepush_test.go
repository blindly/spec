package main

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// FEAT-036: the managed hooks install executable, refresh when stale,
// and never clobber a foreign hook.
func TestPrePushHookInstall(t *testing.T) {
	st := setupRepo(t)
	gitInit(t, st) // setupRepo ran before .git existed
	var buf bytes.Buffer
	ensureSpecHooks(st.Root, &buf)
	for _, h := range specHookScripts {
		hook := filepath.Join(st.Root, ".git", "hooks", h.name)
		data, err := os.ReadFile(hook)
		if err != nil {
			t.Fatalf("%s hook not installed: %v", h.name, err)
		}
		if !strings.Contains(string(data), specHookMarker) {
			t.Fatalf("%s hook missing marker", h.name)
		}
		if fi, _ := os.Stat(hook); fi.Mode()&0o111 == 0 {
			t.Fatalf("%s hook not executable", h.name)
		}
	}
	// foreign hook left alone, noted
	hook := filepath.Join(st.Root, ".git", "hooks", "pre-push")
	custom := "#!/bin/sh\n# user's own hook\n"
	os.WriteFile(hook, []byte(custom), 0o755)
	buf.Reset()
	ensureSpecHooks(st.Root, &buf)
	data, _ := os.ReadFile(hook)
	if string(data) != custom || !strings.Contains(buf.String(), "foreign") {
		t.Fatalf("foreign hook mishandled: %q", buf.String())
	}
	// stale managed hook refreshes
	os.WriteFile(hook, []byte(specHookMarker+"\n# old\n"), 0o755)
	ensureSpecHooks(st.Root, io.Discard)
	data, _ = os.ReadFile(hook)
	if string(data) != prePushHook {
		t.Fatal("stale managed hook not refreshed")
	}
}

// FEAT-036: status reports hook state; check warns when unenforced.
func TestPrePushHookReporting(t *testing.T) {
	st := setupRepo(t)
	gitInit(t, st)
	hooksDir, _ := gitHooksDir(st.Root)

	var out bytes.Buffer
	cmdStatus(&out, io.Discard)
	for _, want := range []string{"pre-push: missing", "commit-msg: missing"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("status should report %q:\n%s", want, out.String())
		}
	}
	var check bytes.Buffer
	cmdCheck(&check, io.Discard)
	for _, want := range []string{"WARN  pre-push hook missing", "WARN  commit-msg hook missing"} {
		if !strings.Contains(check.String(), want) {
			t.Fatalf("check should warn %q:\n%s", want, check.String())
		}
	}

	ensureSpecHooks(st.Root, io.Discard)
	out.Reset()
	cmdStatus(&out, io.Discard)
	if !strings.Contains(out.String(), "pre-push: installed") ||
		!strings.Contains(out.String(), "commit-msg: installed") {
		t.Fatalf("status should report both hooks installed:\n%s", out.String())
	}
	check.Reset()
	if err := cmdCheck(&check, io.Discard); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(check.String(), "hook") {
		t.Fatalf("installed hooks should not warn:\n%s", check.String())
	}

	hook := filepath.Join(hooksDir, "pre-push")
	os.WriteFile(hook, []byte(specHookMarker+"\n# old\n"), 0o755)
	if got := specHooksSummary(st.Root); !strings.Contains(got, "pre-push: outdated") {
		t.Fatalf("stale hook reports %q", got)
	}
	os.WriteFile(hook, []byte("#!/bin/sh\n# mine\n"), 0o755)
	if got := specHooksSummary(st.Root); !strings.Contains(got, "pre-push: foreign") {
		t.Fatalf("foreign hook reports %q", got)
	}
}

// FEAT-036: commit-msg warns (never blocks) on spec-less code commits;
// silent for covered or exempt commits.
func TestCommitMsgHookWarning(t *testing.T) {
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("bash not installed")
	}
	st := setupRepo(t)
	gitInit(t, st)
	ensureSpecHooks(st.Root, io.Discard)
	hook := filepath.Join(st.Root, ".git", "hooks", "commit-msg")
	msg := filepath.Join(st.Root, ".git", "COMMIT_EDITMSG")

	run := func(message string) string {
		os.WriteFile(msg, []byte(message), 0o644)
		cmd := exec.Command("bash", hook, msg)
		cmd.Dir = st.Root
		var errs bytes.Buffer
		cmd.Stderr = &errs
		if err := cmd.Run(); err != nil {
			t.Fatalf("advisory hook must always exit 0: %v", err)
		}
		return errs.String()
	}

	os.WriteFile(filepath.Join(st.Root, "foo.go"), []byte("package x\n"), 0o644)
	exec.Command("git", "-C", st.Root, "add", "foo.go").Run()
	if out := run("wip\n"); !strings.Contains(out, "Spec-Exempt") {
		t.Fatalf("spec-less code commit should warn:\n%s", out)
	}
	if out := run("fix\n\nSpec-Exempt: restores documented behavior\n"); out != "" {
		t.Fatalf("exempt commit should stay quiet:\n%s", out)
	}
	if out := run("fix: panic on empty config\n"); out != "" {
		t.Fatalf("non-feat: prefix should stay quiet:\n%s", out)
	}
	if out := run("feat: undocumented knob\n"); !strings.Contains(out, "Spec-Exempt") {
		t.Fatalf("feat: without a spec should warn:\n%s", out)
	}
	os.WriteFile(filepath.Join(st.Root, "specs", "features", "FEAT-001-x.md"),
		[]byte("---\nid: FEAT-001\ntype: feature\nstatus: draft\n---\n# x\n"), 0o644)
	exec.Command("git", "-C", st.Root, "add", "specs/").Run()
	if out := run("add feature\n"); out != "" {
		t.Fatalf("spec-covered commit should stay quiet:\n%s", out)
	}
}

// FEAT-036: the hook blocks pushes that touch files outside specs/
// without a spec change; SPEC_SKIP=1 bypasses.
func TestPrePushHookEnforcement(t *testing.T) {
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("bash not installed")
	}
	st := setupRepo(t)
	commit := gitInit(t, st)
	var buf bytes.Buffer
	ensureSpecHooks(st.Root, &buf)
	hook := filepath.Join(st.Root, ".git", "hooks", "pre-push")
	rev := func(ref string) string {
		b, err := exec.Command("git", "-C", st.Root, "rev-parse", ref).Output()
		if err != nil {
			t.Fatal(err)
		}
		return strings.TrimSpace(string(b))
	}
	push := func(local, remote string, env ...string) (int, string) {
		cmd := exec.Command("bash", hook)
		cmd.Dir = st.Root
		cmd.Stdin = strings.NewReader(
			fmt.Sprintf("refs/heads/m %s refs/heads/m %s\n", local, remote))
		cmd.Env = append(os.Environ(), env...)
		var errs bytes.Buffer
		cmd.Stderr = &errs
		code := 0
		if err := cmd.Run(); err != nil {
			if ee, ok := err.(*exec.ExitError); ok {
				code = ee.ExitCode()
			} else {
				t.Fatal(err)
			}
		}
		return code, errs.String()
	}

	commit("baseline")
	base := rev("HEAD")
	os.WriteFile(filepath.Join(st.Root, "foo.go"), []byte("package x\n"), 0o644)
	commit("code only")
	codeOnly := rev("HEAD")
	os.WriteFile(filepath.Join(st.Root, "specs", "features", "FEAT-001-x.md"),
		[]byte("---\nid: FEAT-001\ntype: feature\nstatus: draft\n---\n# x\n"), 0o644)
	commit("spec")

	if code, errOut := push(codeOnly, base); code == 0 {
		t.Fatalf("code-only push should fail: %s", errOut)
	} else if !strings.Contains(errOut, "SPEC_SKIP") {
		t.Fatalf("failure should mention the escape hatch:\n%s", errOut)
	}
	if code, _ := push(rev("HEAD"), base); code != 0 {
		t.Fatal("push including a spec change should pass")
	}
	if code, _ := push(codeOnly, base, "SPEC_SKIP=1"); code != 0 {
		t.Fatal("SPEC_SKIP=1 should bypass")
	}

	// a Spec-Exempt trailer attests a defect fix
	os.WriteFile(filepath.Join(st.Root, "fix.go"), []byte("package x\n"), 0o644)
	commit("defect fix\n\nSpec-Exempt: restores documented behavior")
	marked := rev("HEAD")
	if code, _ := push(marked, rev("HEAD~1")); code != 0 {
		t.Fatal("Spec-Exempt commit should pass")
	}
	// so does a non-feat conventional-commit prefix
	os.WriteFile(filepath.Join(st.Root, "fix2.go"), []byte("package x\n"), 0o644)
	commit("fix: panic on empty config")
	if code, _ := push(rev("HEAD"), marked); code != 0 {
		t.Fatal("fix: prefix should pass")
	}
	// feat: declares behavior work — it is never an attestation
	os.WriteFile(filepath.Join(st.Root, "feat.go"), []byte("package x\n"), 0o644)
	commit("feat: undocumented knob")
	feat := rev("HEAD")
	if code, errOut := push(feat, rev("HEAD~1")); code == 0 {
		t.Fatal("feat: commit without a spec should fail")
	} else if !strings.Contains(errOut, "Spec-Exempt") {
		t.Fatalf("failure should name the remedy:\n%s", errOut)
	}
	// a plain unmarked commit fails too, named in the output
	os.WriteFile(filepath.Join(st.Root, "more.go"), []byte("package x\n"), 0o644)
	commit("drive-by")
	code, errOut := push(rev("HEAD"), feat)
	if code == 0 || !strings.Contains(errOut, "drive-by") {
		t.Fatalf("unmarked commit should fail naming it: %d\n%s", code, errOut)
	}
}
