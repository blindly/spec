package main

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// fakeApplyAgent installs a shell agent that reads the apply prompt on stdin,
// and for spec ids listed in $FAKE_IMPLEMENT sets status: implemented. It
// then exits with $FAKE_EXIT, or sleeps $FAKE_SLEEP seconds (to hit the
// timeout). extraConfig is appended to .spec/config.yml.
func fakeApplyAgent(t *testing.T, st Store, extraConfig string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("shell-script fake agent")
	}
	fake := filepath.Join(st.Root, "fakeagent.sh")
	os.WriteFile(fake, []byte(`#!/bin/sh
id=$(sed -n 's/^Spec \([A-Z]*-[0-9]*\) (.*/\1/p' | head -n 1)
case " $FAKE_IMPLEMENT " in *" $id "*)
  for f in specs/*/"$id"-*.md; do
    sed 's/^status: .*/status: implemented/' "$f" > "$f.t" && mv "$f.t" "$f"
  done;;
esac
echo "fake agent worked on $id"
[ -n "$FAKE_SLEEP" ] && exec sleep "$FAKE_SLEEP"
exit ${FAKE_EXIT:-0}
`), 0o755)
	os.WriteFile(filepath.Join(st.Root, configDir, "config.yml"),
		[]byte("agent_cmd: sh "+fake+"\n"+extraConfig), 0o644)
	t.Setenv("FAKE_IMPLEMENT", "")
	t.Setenv("FAKE_SLEEP", "")
	t.Setenv("FAKE_EXIT", "")
}

func featureWithStatus(t *testing.T, st Store, title, status string) string {
	t.Helper()
	s, err := st.Create("feature", title, "# "+title+"\n\n## Summary\n\ns\n\n## Design\n\nd\n\n## Acceptance Criteria\n\n- [ ] x\n", map[string][]string{"status": {status}})
	if err != nil {
		t.Fatal(err)
	}
	return s.ID()
}

func setTerminal(t *testing.T, tty bool) {
	t.Helper()
	old := stdinIsTerminal
	stdinIsTerminal = func() bool { return tty }
	t.Cleanup(func() { stdinIsTerminal = old })
}

func runApply(ids []string, opts applyOpts, input string) (string, error) {
	var out bytes.Buffer
	err := cmdApply(strings.NewReader(input), &out, io.Discard, ids, opts)
	return out.String(), err
}

// --- FEAT-001: scope and confirmation ---

func TestApplyNoIdsSkipsProposed(t *testing.T) {
	st := setupRepo(t)
	fakeApplyAgent(t, st, "")
	acc := featureWithStatus(t, st, "approved", "accepted")
	prop := featureWithStatus(t, st, "not yet", "proposed")
	t.Setenv("FAKE_IMPLEMENT", acc+" "+prop)
	setTerminal(t, false)
	out, err := runApply(nil, applyOpts{Yes: true}, "")
	if err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	if !strings.Contains(out, "Applying "+acc) || strings.Contains(out, "Applying "+prop) {
		t.Fatalf("wrong scope:\n%s", out)
	}
	if st.Get(prop).Status() != "proposed" {
		t.Fatal("proposed spec was applied")
	}
}

func TestApplyNoIdsConfirmation(t *testing.T) {
	st := setupRepo(t)
	fakeApplyAgent(t, st, "")
	id := featureWithStatus(t, st, "confirm me", "accepted")
	t.Setenv("FAKE_IMPLEMENT", id)
	setTerminal(t, true)

	for _, answer := range []string{"n\n", "\n", "maybe\n", ""} {
		out, err := runApply(nil, applyOpts{}, answer)
		if err == nil || !strings.Contains(err.Error(), "aborted") {
			t.Fatalf("answer %q: expected abort, got %v", answer, err)
		}
		if !strings.Contains(out, id) || !strings.Contains(out, "[y/N]") {
			t.Fatalf("targets not listed before prompt:\n%s", out)
		}
		if strings.Contains(out, "Applying") || st.Get(id).Status() != "accepted" {
			t.Fatalf("agent ran despite answer %q", answer)
		}
	}
	out, err := runApply(nil, applyOpts{}, "yes\n")
	if err != nil || st.Get(id).Status() != "implemented" {
		t.Fatalf("yes should run the agent: %v\n%s", err, out)
	}
}

func TestApplyNoIdsNonInteractive(t *testing.T) {
	st := setupRepo(t)
	fakeApplyAgent(t, st, "")
	id := featureWithStatus(t, st, "ci run", "accepted")
	t.Setenv("FAKE_IMPLEMENT", id)
	setTerminal(t, false)
	if _, err := runApply(nil, applyOpts{}, ""); err == nil || !strings.Contains(err.Error(), "--yes") {
		t.Fatalf("expected refusal mentioning --yes, got %v", err)
	}
	if st.Get(id).Status() != "accepted" {
		t.Fatal("agent ran without confirmation")
	}
	if out, err := runApply(nil, applyOpts{Yes: true}, ""); err != nil || strings.Contains(out, "[y/N]") {
		t.Fatalf("--yes should run without prompting: %v\n%s", err, out)
	}
}

func TestApplyExplicitIdAndPrintNeverPrompt(t *testing.T) {
	st := setupRepo(t)
	fakeApplyAgent(t, st, "")
	id := featureWithStatus(t, st, "named", "proposed")
	featureWithStatus(t, st, "accepted one", "accepted")
	t.Setenv("FAKE_IMPLEMENT", id)
	setTerminal(t, false)
	if out, err := runApply([]string{id}, applyOpts{}, ""); err != nil || strings.Contains(out, "[y/N]") {
		t.Fatalf("explicit proposed id should run without prompting: %v\n%s", err, out)
	}
	if out, err := runApply(nil, applyOpts{Print: true}, ""); err != nil || strings.Contains(out, "[y/N]") || !strings.Contains(out, "apply prompt for") {
		t.Fatalf("--print should never prompt: %v\n%s", err, out)
	}
}

// --- FEAT-002: outcome reporting ---

func TestApplyOutcomes(t *testing.T) {
	cases := []struct {
		name              string
		verify, implement string
		sleep, exit       string
		opts              applyOpts
		wantOK            bool
		wantOut, notInOut string
	}{
		{name: "exit 0 but not implemented", wantOut: "not implemented: agent finished but FEAT-001 is still accepted"},
		{name: "implemented, no verify", implement: "FEAT-001", wantOK: true, wantOut: "FEAT-001: implemented"},
		{name: "implemented, verify passes", verify: "verify: [true]\n", implement: "FEAT-001", wantOK: true, wantOut: "FEAT-001: implemented"},
		{name: "implemented, verify fails", verify: "verify: [false]\n", implement: "FEAT-001", wantOut: "verify failed"},
		{name: "--no-verify skips failing verify", verify: "verify: [false]\n", implement: "FEAT-001", opts: applyOpts{NoVerify: true}, wantOK: true, wantOut: "FEAT-001: implemented", notInOut: "Verifying"},
		{name: "timeout after finishing", verify: "agent_timeout: 1\n", implement: "FEAT-001", sleep: "5", wantOK: true, wantOut: "warning: agent timed out after 1s"},
		{name: "timeout without finishing", verify: "agent_timeout: 1\n", sleep: "5", wantOut: "timed out after 1s (status still accepted)"},
		{name: "agent error", exit: "3", wantOut: "FEAT-001: failed:"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			st := setupRepo(t)
			fakeApplyAgent(t, st, c.verify)
			featureWithStatus(t, st, "outcome", "accepted")
			t.Setenv("FAKE_IMPLEMENT", c.implement)
			t.Setenv("FAKE_SLEEP", c.sleep)
			t.Setenv("FAKE_EXIT", c.exit)
			out, err := runApply([]string{"FEAT-001"}, c.opts, "")
			if (err == nil) != c.wantOK {
				t.Fatalf("ok=%v want %v (err %v)\n%s", err == nil, c.wantOK, err, out)
			}
			if !strings.Contains(out, c.wantOut) {
				t.Fatalf("missing %q:\n%s", c.wantOut, out)
			}
			if c.notInOut != "" && strings.Contains(out, c.notInOut) {
				t.Fatalf("unexpected %q:\n%s", c.notInOut, out)
			}
			if strings.Contains(out, " done") {
				t.Fatalf("old 'done' message printed:\n%s", out)
			}
		})
	}
}

func TestApplyMultipleTargetsContinueAndSummarize(t *testing.T) {
	st := setupRepo(t)
	fakeApplyAgent(t, st, "")
	a := featureWithStatus(t, st, "first fails", "accepted")
	b := featureWithStatus(t, st, "second works", "accepted")
	t.Setenv("FAKE_IMPLEMENT", b)
	out, err := runApply([]string{a, b}, applyOpts{}, "")
	if err == nil || !strings.Contains(err.Error(), "1 of 2") {
		t.Fatalf("expected 1 of 2 failure, got %v\n%s", err, out)
	}
	if st.Get(b).Status() != "implemented" {
		t.Fatalf("second spec didn't run after first failed:\n%s", out)
	}
	sum := out[strings.Index(out, "Summary:"):]
	if !strings.Contains(sum, a+"   not implemented") || !strings.Contains(sum, b+"   implemented") {
		t.Fatalf("bad summary:\n%s", sum)
	}
}
