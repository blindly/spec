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

// FEAT-036: the managed hook installs executable, refreshes when stale,
// and never clobbers a foreign hook.
func TestPrePushHookInstall(t *testing.T) {
	st := setupRepo(t)
	gitInit(t, st) // setupRepo ran before .git existed
	var buf bytes.Buffer
	ensurePrePushHook(st.Root, &buf)
	hook := filepath.Join(st.Root, ".git", "hooks", "pre-push")
	data, err := os.ReadFile(hook)
	if err != nil {
		t.Fatalf("hook not installed: %v", err)
	}
	if !strings.Contains(string(data), prePushMarker) {
		t.Fatal("installed hook missing marker")
	}
	if fi, _ := os.Stat(hook); fi.Mode()&0o111 == 0 {
		t.Fatal("hook not executable")
	}
	// foreign hook left alone, noted
	custom := "#!/bin/sh\n# user's own hook\n"
	os.WriteFile(hook, []byte(custom), 0o755)
	buf.Reset()
	ensurePrePushHook(st.Root, &buf)
	data, _ = os.ReadFile(hook)
	if string(data) != custom || !strings.Contains(buf.String(), "foreign") {
		t.Fatalf("foreign hook mishandled: %q", buf.String())
	}
	// stale managed hook refreshes
	os.WriteFile(hook, []byte(prePushMarker+"\n# old\n"), 0o755)
	ensurePrePushHook(st.Root, io.Discard)
	data, _ = os.ReadFile(hook)
	if string(data) != prePushHook {
		t.Fatal("stale managed hook not refreshed")
	}
}

// FEAT-036: status reports hook state; check warns when unenforced.
func TestPrePushHookReporting(t *testing.T) {
	st := setupRepo(t)
	gitInit(t, st)
	hook := filepath.Join(st.Root, ".git", "hooks", "pre-push")

	var out bytes.Buffer
	cmdStatus(&out, io.Discard)
	if !strings.Contains(out.String(), "Pre-push hook: missing") {
		t.Fatalf("status should report missing hook:\n%s", out.String())
	}
	var check bytes.Buffer
	cmdCheck(&check, io.Discard)
	if !strings.Contains(check.String(), "WARN  pre-push hook missing") {
		t.Fatalf("check should warn on missing hook:\n%s", check.String())
	}

	ensurePrePushHook(st.Root, io.Discard)
	if got := prePushHookState(st.Root); got != "installed" {
		t.Fatalf("installed hook reports %q", got)
	}
	out.Reset()
	cmdStatus(&out, io.Discard)
	if !strings.Contains(out.String(), "Pre-push hook: installed") {
		t.Fatalf("status should report installed:\n%s", out.String())
	}
	check.Reset()
	if err := cmdCheck(&check, io.Discard); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(check.String(), "pre-push") {
		t.Fatalf("installed hook should not warn:\n%s", check.String())
	}

	os.WriteFile(hook, []byte(prePushMarker+"\n# old\n"), 0o755)
	if got := prePushHookState(st.Root); !strings.Contains(got, "outdated") {
		t.Fatalf("stale hook reports %q", got)
	}
	os.WriteFile(hook, []byte("#!/bin/sh\n# mine\n"), 0o755)
	if got := prePushHookState(st.Root); !strings.Contains(got, "foreign") {
		t.Fatalf("foreign hook reports %q", got)
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
	ensurePrePushHook(st.Root, &buf)
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
	// one unmarked commit in the push still fails, named in the output
	os.WriteFile(filepath.Join(st.Root, "more.go"), []byte("package x\n"), 0o644)
	commit("drive-by")
	code, errOut := push(rev("HEAD"), marked)
	if code == 0 || !strings.Contains(errOut, "Spec-Exempt") {
		t.Fatalf("unmarked commit should fail naming the trailer: %d\n%s", code, errOut)
	}
}
