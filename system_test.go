package main

import (
	"io"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func TestInSystem(t *testing.T) {
	cfg := defaultConfig()
	cases := []struct {
		typ, status string
		archived    bool
		want        bool
	}{
		{"feature", "implemented", false, true},
		{"project", "implemented", false, true},
		{"feature", "accepted", false, false},
		{"feature", "deprecated", false, false},
		{"adr", "accepted", false, true},
		{"adr", "proposed", false, false},
		{"adr", "deprecated", false, false},
		{"imported", "accepted", false, true},
		{"imported", "rejected", false, false},
		{"idea", "refined", false, false},
		{"feature", "implemented", true, false},
	}
	for _, c := range cases {
		s := Spec{Meta: map[string][]string{"type": {c.typ}, "status": {c.status}}, Archived: c.archived}
		if got := inSystem(cfg, s); got != c.want {
			t.Fatalf("%s/%s archived=%v: got %v", c.typ, c.status, c.archived, got)
		}
	}
}

func TestWebBoardSystemSplit(t *testing.T) {
	st, srv, _ := setupWeb(t)
	adr := "# x\n\n## Context\n\nc\n\n## Decision\n\nd\n\n## Consequences\n\nq\n"
	st.Create("project", "the project", "# p\n\n## Overview\n\no\n\n## Requirements\n\n- REQ-001: x\n", map[string][]string{"status": {"implemented"}})
	live := featureWithStatus(t, st, "live feature", "implemented")
	featureWithStatus(t, st, "wip feature", "accepted")
	st.Create("adr", "standing decision", adr, map[string][]string{"status": {"accepted"}})
	st.Create("adr", "open decision", adr, map[string][]string{"status": {"proposed"}})
	st.Create("imported", "style guide", "# g\n", map[string][]string{"status": {"accepted"}})
	st.Create("feature", "depends on live", "# d\n", map[string][]string{"status": {"implemented"}, "depends_on": {live}})
	featureWithStatus(t, st, "old feature", "deprecated")
	gone := featureWithStatus(t, st, "gone feature", "rejected")
	if err := cmdArchive(io.Discard, []string{gone}); err != nil {
		t.Fatal(err)
	}
	newIdea(t, st, "some idea", nil)

	board := webGet(t, srv.URL+"/board")
	for _, want := range []string{"wip feature", "open decision"} {
		if !strings.Contains(board, want) {
			t.Fatalf("board missing work in progress %q:\n%s", want, board)
		}
	}
	// by link, not title: the project's title is also the nav badge
	for _, not := range []string{live, "PROJ-001", "ADR-001", "DOC-001", "IDEA-001"} {
		if strings.Contains(board, `href="/spec/`+not+`"`) {
			t.Fatalf("board should not show %q:\n%s", not, board)
		}
	}
	if !strings.Contains(board, "5 spec(s) in the system") {
		t.Fatalf("board should count system specs:\n%s", board)
	}

	sys := webGet(t, srv.URL+"/system")
	for _, want := range []string{`href="/spec/PROJ-001"`, "live feature", "standing decision", "style guide", "depends on live"} {
		if !strings.Contains(sys, want) {
			t.Fatalf("system missing %q:\n%s", want, sys)
		}
	}
	for _, not := range []string{"wip feature", "open decision", "old feature", "gone feature", "some idea"} {
		if strings.Contains(sys, not) {
			t.Fatalf("system should not show %q:\n%s", not, sys)
		}
	}
	// grouped in order Project, Features, Decisions, Imported docs
	p, f, d, i := strings.Index(sys, "<h2>Project"), strings.Index(sys, "<h2>Features"), strings.Index(sys, "<h2>Decisions"), strings.Index(sys, "<h2>Imported docs")
	if p < 0 || f < 0 || d < 0 || i < 0 || !(p < f && f < d && d < i) {
		t.Fatalf("sections missing or out of order:\n%s", sys)
	}
	if !strings.Contains(sys, `<a href="/spec/`+live+`">`+live+`</a></td></tr>`) {
		t.Fatalf("depends_on not linked:\n%s", sys)
	}
	if !strings.Contains(sys, time.Now().Format("2006-01-02")) {
		t.Fatalf("last-changed date missing (mtime fallback outside git):\n%s", sys)
	}
}

func TestWebSystemEmptyAndNav(t *testing.T) {
	_, srv, _ := setupWeb(t)
	page := webGet(t, srv.URL+"/system")
	if !strings.Contains(page, "Nothing in the system yet") {
		t.Fatalf("empty system page:\n%s", page)
	}
	if !strings.Contains(page, `<a href="/">inbox</a><a href="/board">board</a><a href="/system">system</a><a href="/new">new spec</a>`) {
		t.Fatalf("nav order:\n%s", page)
	}
}

func TestLastChangedUsesGit(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	st := setupRepo(t)
	committed := featureWithStatus(t, st, "committed", "implemented")
	git := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-c", "user.name=t", "-c", "user.email=t@t", "-c", "commit.gpgsign=false"}, args...)...)
		cmd.Dir = st.Root
		cmd.Env = append(os.Environ(), "GIT_COMMITTER_DATE=2020-01-02T03:04:05Z", "GIT_AUTHOR_DATE=2020-01-02T03:04:05Z")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	git("init", "-q")
	git("add", ".")
	git("commit", "-q", "-m", "init")
	untracked := featureWithStatus(t, st, "untracked", "implemented")
	dates := lastChanged(st, loadGit(st.Root), st.Specs())
	if got := dates[st.Get(committed).Path]; got != "2020-01-02" {
		t.Fatalf("committed spec: got %q, want git commit date", got)
	}
	if got := dates[st.Get(untracked).Path]; got != time.Now().Format("2006-01-02") {
		t.Fatalf("untracked spec should fall back to mtime, got %q", got)
	}
}
