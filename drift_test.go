package main

import (
	"bytes"
	"fmt"
	"io"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// gitInit makes st.Root a git repo and returns a commit function that
// commits everything with a distinct, fixed date per commit.
func gitInit(t *testing.T, st Store) func(msg string) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	n := 0
	run := func(env []string, args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-c", "user.name=t", "-c", "user.email=t@t",
			"-c", "commit.gpgsign=false", "-c", "core.hooksPath=/dev/null"}, args...)...)
		cmd.Dir = st.Root
		cmd.Env = append(os.Environ(), env...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run(nil, "init", "-q")
	return func(msg string) {
		t.Helper()
		n++
		date := fmt.Sprintf("2020-01-%02dT12:00:00Z", n)
		run(nil, "add", "-A")
		run([]string{"GIT_AUTHOR_DATE=" + date, "GIT_COMMITTER_DATE=" + date}, "commit", "-q", "--allow-empty", "-m", msg)
	}
}

func writeFile(t *testing.T, st Store, rel, content string) {
	t.Helper()
	p := filepath.Join(st.Root, filepath.FromSlash(rel))
	os.MkdirAll(filepath.Dir(p), 0o755)
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func liveFeature(t *testing.T, st Store, title string, covers ...string) Spec {
	t.Helper()
	extra := map[string][]string{"status": {"implemented"}}
	if len(covers) > 0 {
		extra["covers"] = covers
	}
	s, err := st.Create("feature", title, "# "+title+"\n\n## Summary\n\ns\n\n## Design\n\nd\n\n## Acceptance Criteria\n\n- [x] x\n", extra)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func driftOf(t *testing.T, st Store, id string) *driftInfo {
	t.Helper()
	g := loadGit(st.Root)
	if g == nil {
		t.Fatal("not a git repo")
	}
	return g.drift(*st.Get(id))
}

func TestCoverMatches(t *testing.T) {
	cases := []struct {
		pattern, file string
		want          bool
	}{
		{"web.go", "web.go", true},
		{"web.go", "webby.go", false},
		{"internal/web/", "internal/web/x.go", true},
		{"internal/web/", "internal/webx/x.go", false},
		{"*.go", "web.go", true},
		{"cmd/*.go", "cmd/a.go", true},
		{"cmd/*.go", "cmd/sub/a.go", false},
	}
	for _, c := range cases {
		if got := coverMatches(c.pattern, c.file); got != c.want {
			t.Fatalf("%q vs %q: got %v", c.pattern, c.file, got)
		}
	}
}

func TestIsCodeFile(t *testing.T) {
	yes := []string{"web.go", "cmd/main.go", "src/app.ts", "lib/util.py", "go.mod"}
	no := []string{"README.md", "specs/features/FEAT-001-x.md", ".spec/config.yml", ".github/workflows/ci.yml",
		"web_test.go", "src/app.test.ts", "src/app.spec.js", "tests/test_x.py", "test_util.py", "VERSION", "go.sum", "notes.txt"}
	for _, f := range yes {
		if !isCodeFile(f) {
			t.Fatalf("%s should be code", f)
		}
	}
	for _, f := range no {
		if isCodeFile(f) {
			t.Fatalf("%s should not be code", f)
		}
	}
}

func TestDriftDetection(t *testing.T) {
	st := setupRepo(t)
	commit := gitInit(t, st)
	s := liveFeature(t, st, "board", "app.go", "lib/")
	writeFile(t, st, "app.go", "package app\n")
	writeFile(t, st, "lib/x.go", "package lib\n")
	commit("implement board")
	if d := driftOf(t, st, s.ID()); d != nil {
		t.Fatalf("spec committed with its code should be in sync: %+v", d)
	}

	// covered file changes later, without the spec → flagged
	writeFile(t, st, "app.go", "package app\n// changed\n")
	commit("change app")
	d := driftOf(t, st, s.ID())
	if d == nil || strings.Join(d.Files, ",") != "app.go" || len(d.Commits) != 1 || d.Commits[0].Subject != "change app" {
		t.Fatalf("expected drift on app.go from 'change app': %+v", d)
	}

	// spec updated in a later commit → in sync again
	os.WriteFile(st.Get(s.ID()).Path, []byte(dumpFrontmatter(s.Meta, s.Body+"\nupdated.\n")), 0o644)
	if driftOf(t, st, s.ID()) != nil {
		t.Fatal("a spec with uncommitted edits counts as being updated")
	}
	commit("update spec")
	if driftOf(t, st, s.ID()) != nil {
		t.Fatal("spec updated after the code change should be in sync")
	}

	// uncommitted change in a covered directory → flagged as uncommitted
	writeFile(t, st, "lib/y.go", "package lib\n")
	if d := driftOf(t, st, s.ID()); d == nil || len(d.Uncommitted) != 1 || d.Uncommitted[0] != "lib/y.go" {
		t.Fatalf("uncommitted covered change should flag: %+v", d)
	}
	// spec and code changed in the same commit → in sync
	os.WriteFile(st.Get(s.ID()).Path, []byte(dumpFrontmatter(s.Meta, s.Body+"\nupdated again.\n")), 0o644)
	commit("code and spec together")
	if driftOf(t, st, s.ID()) != nil {
		t.Fatal("same-commit spec update should be in sync")
	}
	// unrelated file → not flagged
	writeFile(t, st, "other.go", "package other\n")
	commit("unrelated")
	if driftOf(t, st, s.ID()) != nil {
		t.Fatal("uncovered file shouldn't flag")
	}
}

func TestSystemDriftScope(t *testing.T) {
	st := setupRepo(t)
	commit := gitInit(t, st)
	live := liveFeature(t, st, "live", "app.go")
	wip, _ := st.Create("feature", "wip", "# wip\n", map[string][]string{"status": {"accepted"}, "covers": {"app.go"}})
	bare := liveFeature(t, st, "bare")
	writeFile(t, st, "app.go", "package app\n")
	commit("init")
	writeFile(t, st, "app.go", "package app\n// v2\n")
	commit("v2")
	stale, uncovered := loadGit(st.Root).systemDrift(st, loadConfig(st.Root))
	if len(stale) != 1 || stale[0].Spec.ID() != live.ID() {
		t.Fatalf("only the system spec should be checked, got %d stale", len(stale))
	}
	if len(uncovered) != 1 || uncovered[0] != bare.ID() {
		t.Fatalf("uncovered: %v", uncovered)
	}
	_ = wip
}

func TestCoversInfer(t *testing.T) {
	st := setupRepo(t)
	commit := gitInit(t, st)
	s := liveFeature(t, st, "thing")
	writeFile(t, st, "thing.go", "package x\n")
	writeFile(t, st, "thing_test.go", "package x\n")
	writeFile(t, st, "README.md", "# r\n")
	commit("implement thing")
	writeFile(t, st, "later.go", "package x\n")
	commit("unrelated later")
	var out bytes.Buffer
	if err := cmdCovers(&out, []string{"--infer"}); err != nil {
		t.Fatal(err)
	}
	if got := st.Get(s.ID()).Field("covers"); len(got) != 1 || got[0] != "thing.go" {
		t.Fatalf("inferred covers: %v\n%s", got, out.String())
	}
	out.Reset()
	cmdCovers(&out, nil)
	if !strings.Contains(out.String(), s.ID()) || !strings.Contains(out.String(), "thing.go") {
		t.Fatalf("covers listing: %s", out.String())
	}
}

func TestVerifiedClearsDrift(t *testing.T) {
	st := setupRepo(t)
	commit := gitInit(t, st)
	s := liveFeature(t, st, "v", "app.go")
	writeFile(t, st, "app.go", "1")
	commit("init")
	writeFile(t, st, "app.go", "2")
	commit("change")
	if driftOf(t, st, s.ID()) == nil {
		t.Fatal("expected drift before verifying")
	}
	if err := cmdVerified(io.Discard, []string{s.ID()}); err != nil {
		t.Fatal(err)
	}
	if v := metaGet(st.Get(s.ID()).Meta, "verified", ""); len(v) < 7 {
		t.Fatalf("verified not stamped: %q", v)
	}
	commit("verify")
	if driftOf(t, st, s.ID()) != nil {
		t.Fatal("verified spec should be in sync")
	}
	writeFile(t, st, "app.go", "3")
	commit("change again")
	if driftOf(t, st, s.ID()) == nil {
		t.Fatal("drift should return when covered code changes after verifying")
	}
}

func TestDriftCommandOutputAndCheck(t *testing.T) {
	st := setupRepo(t)
	commit := gitInit(t, st)
	s := liveFeature(t, st, "out", "app.go")
	bare := liveFeature(t, st, "nocov")
	writeFile(t, st, "app.go", "1")
	commit("init")
	writeFile(t, st, "app.go", "2")
	commit("Change the app")
	var out bytes.Buffer
	if err := cmdDrift(strings.NewReader(""), &out, io.Discard, nil); err != nil {
		t.Fatal(err)
	}
	o := out.String()
	for _, want := range []string{s.ID() + "  may be stale — app.go changed", "Change the app", "No coverage, can't check (run spec covers --infer): " + bare.ID(), "1 spec(s) may be stale"} {
		if !strings.Contains(o, want) {
			t.Fatalf("drift output missing %q:\n%s", want, o)
		}
	}
	out.Reset()
	if err := cmdCheck(&out, io.Discard); err != nil {
		t.Fatalf("drift must not fail spec check: %v", err)
	}
	if !strings.Contains(out.String(), "WARN  "+s.ID()+" may be stale: app.go") {
		t.Fatalf("check should warn about drift:\n%s", out.String())
	}
}

func TestDriftWithoutGit(t *testing.T) {
	setupRepo(t)
	var out bytes.Buffer
	if err := cmdDrift(strings.NewReader(""), &out, io.Discard, nil); err != nil || !strings.Contains(out.String(), "needs git") {
		t.Fatalf("no-git drift: %v %s", err, out.String())
	}
	out.Reset()
	if err := cmdCheck(&out, io.Discard); err != nil || strings.Contains(out.String(), "stale") {
		t.Fatalf("check without git should be silent about drift: %v %s", err, out.String())
	}
}

// fakeReviewAgent replies with output and saves the prompt it received.
func fakeReviewAgent(t *testing.T, st Store, output string) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("shell-script fake agent")
	}
	dir := t.TempDir() // outside the repo so it isn't seen as a change
	outFile, promptFile := filepath.Join(dir, "out.txt"), filepath.Join(dir, "prompt.txt")
	os.WriteFile(outFile, []byte(output), 0o644)
	fake := filepath.Join(dir, "agent.sh")
	os.WriteFile(fake, []byte("#!/bin/sh\ncat > '"+promptFile+"'\ncat '"+outFile+"'\n"), 0o755)
	writeFile(t, st, ".spec/config.yml", "agent_cmd: sh "+fake+"\n")
	return promptFile
}

func driftedSpec(t *testing.T) (Store, Spec, func(string)) {
	st := setupRepo(t)
	commit := gitInit(t, st)
	s := liveFeature(t, st, "reviewed", "app.go")
	writeFile(t, st, "app.go", "package app // old behavior\n")
	commit("init")
	writeFile(t, st, "app.go", "package app // NEW behavior\n")
	commit("change behavior")
	return st, s, commit
}

func TestDriftReviewAccurate(t *testing.T) {
	st, s, _ := driftedSpec(t)
	promptFile := fakeReviewAgent(t, st, "VERDICT: accurate\nREASON: the change doesn't affect this spec\n")
	var out bytes.Buffer
	if err := cmdDrift(strings.NewReader(""), &out, io.Discard, []string{"--review"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "ACCURATE — the change doesn't affect this spec") || metaGet(st.Get(s.ID()).Meta, "verified", "") == "" {
		t.Fatalf("accurate verdict should stamp verified:\n%s", out.String())
	}
	prompt, _ := os.ReadFile(promptFile)
	if !strings.Contains(string(prompt), "NEW behavior") || !strings.Contains(string(prompt), "VERDICT: accurate|stale") {
		t.Fatalf("review prompt should include the diff and format:\n%s", prompt)
	}
}

func TestDriftReviewStale(t *testing.T) {
	st, s, _ := driftedSpec(t)
	newBody := "# reviewed\n\n## Summary\n\nnow describes NEW behavior\n\n## Design\n\nd\n\n## Acceptance Criteria\n\n- [x] x\n"
	fakeReviewAgent(t, st, "VERDICT: stale\nREASON: behavior changed\n\n"+newBody)
	setTerminal(t, false)

	// not interactive, no --yes → shows the diff, doesn't apply
	var out bytes.Buffer
	cmdDrift(strings.NewReader(""), &out, io.Discard, []string{"--review", s.ID()})
	if !strings.Contains(out.String(), "STALE — behavior changed") || !strings.Contains(out.String(), "+now describes NEW behavior") || !strings.Contains(out.String(), "not applied") {
		t.Fatalf("stale verdict output:\n%s", out.String())
	}
	if strings.Contains(st.Get(s.ID()).Body, "NEW behavior") {
		t.Fatal("must not apply without confirmation")
	}
	// --yes applies and stamps verified
	out.Reset()
	cmdDrift(strings.NewReader(""), &out, io.Discard, []string{"--review", "--yes"})
	got := st.Get(s.ID())
	if !strings.Contains(got.Body, "now describes NEW behavior") || metaGet(got.Meta, "verified", "") == "" {
		t.Fatalf("--yes should apply the update:\n%s\n%s", out.String(), got.Body)
	}
	// interactive y also applies
	st2, s2, _ := driftedSpec(t)
	fakeReviewAgent(t, st2, "VERDICT: stale\nREASON: r\n\n"+newBody)
	setTerminal(t, true)
	out.Reset()
	cmdDrift(strings.NewReader("y\n"), &out, io.Discard, []string{"--review"})
	if !strings.Contains(st2.Get(s2.ID()).Body, "NEW behavior") {
		t.Fatalf("answering y should apply:\n%s", out.String())
	}
}

func TestApplyTracksCoverage(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell-script fake agent")
	}
	st := setupRepo(t)
	commit := gitInit(t, st)
	other := liveFeature(t, st, "other", "shared.go")
	target := featureWithStatus(t, st, "target", "accepted")
	writeFile(t, st, "shared.go", "package x\n")
	// the fake agent edits shared.go and new.go, then marks the target implemented
	dir := t.TempDir()
	fake := filepath.Join(dir, "agent.sh")
	os.WriteFile(fake, []byte(`#!/bin/sh
cat > /dev/null
echo "// edited" >> shared.go
echo "package x" > new.go
for f in specs/features/`+target+`-*.md; do sed 's/^status: .*/status: implemented/' "$f" > "$f.t" && mv "$f.t" "$f"; done
`), 0o755)
	writeFile(t, st, ".spec/config.yml", "agent_cmd: sh "+fake+"\n")
	commit("init")

	out, err := runApply([]string{target}, applyOpts{}, "")
	if err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	covers := strings.Join(st.Get(target).Field("covers"), ",")
	if !strings.Contains(covers, "shared.go") || !strings.Contains(covers, "new.go") || strings.Contains(covers, "specs/") {
		t.Fatalf("applied spec should cover the code files the agent changed, got %q\n%s", covers, out)
	}
	if !strings.Contains(out, "Other specs cover files this run changed") || !strings.Contains(out, other.ID()+" (shared.go)") {
		t.Fatalf("should warn about other specs covering changed files:\n%s", out)
	}
}

func TestApplyPromptListsStaleSpecs(t *testing.T) {
	st, s, _ := driftedSpec(t)
	featureWithStatus(t, st, "next", "accepted")
	var buf bytes.Buffer
	if err := cmdApply(strings.NewReader(""), &buf, io.Discard, []string{"FEAT-002"}, applyOpts{Print: true}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "may already be out of date") || !strings.Contains(buf.String(), s.ID()+" (app.go)") {
		t.Fatalf("apply prompt should list possibly-stale specs:\n%s", buf.String())
	}
}

// newTestServer serves st's web UI and returns its base URL.
func newTestServer(t *testing.T, st Store) string {
	t.Helper()
	srv := httptest.NewServer(webHandler(st, loadConfig(st.Root), "t"))
	t.Cleanup(srv.Close)
	return srv.URL
}

func TestWebDriftBadge(t *testing.T) {
	st, s, _ := driftedSpec(t)
	srv := newTestServer(t, st)
	sys := webGet(t, srv+"/system")
	if !strings.Contains(sys, `<span class="badge stale" title="changed since last updated: app.go">may be stale</span>`) {
		t.Fatalf("system view badge missing:\n%s", sys)
	}
	page := webGet(t, srv+"/spec/"+s.ID())
	if !strings.Contains(page, "change behavior") || !strings.Contains(page, "spec verified "+s.ID()) {
		t.Fatalf("spec page drift note missing:\n%s", page)
	}
}
