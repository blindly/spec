package main

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestArchiveAndUnarchive(t *testing.T) {
	st := setupRepo(t)
	id := featureWithStatus(t, st, "old work", "deprecated")
	orig := st.Get(id).Path
	var out bytes.Buffer
	if err := cmdArchive(&out, []string{id}); err != nil {
		t.Fatal(err)
	}
	s := st.Get(id)
	want := filepath.Join(st.Root, specDir, "archive", "features", filepath.Base(orig))
	if s.Path != want || !s.Archived || metaGet(s.Meta, "archived", "") == "" {
		t.Fatalf("not archived correctly: %s archived=%v meta=%v", s.Path, s.Archived, s.Meta)
	}
	if _, err := os.Stat(orig); !os.IsNotExist(err) {
		t.Fatal("original file still present")
	}
	if !strings.Contains(out.String(), "Archived "+id+" → specs/archive/features/") {
		t.Fatalf("output: %s", out.String())
	}
	if err := cmdArchive(io.Discard, []string{id}); err == nil {
		t.Fatal("archiving twice should fail")
	}
	if err := cmdUnarchive(io.Discard, []string{id}); err != nil {
		t.Fatal(err)
	}
	s = st.Get(id)
	if s.Path != orig || s.Archived || metaGet(s.Meta, "archived", "") != "" {
		t.Fatalf("not restored: %s %v %v", s.Path, s.Archived, s.Meta)
	}
	if err := cmdUnarchive(io.Discard, []string{id}); err == nil {
		t.Fatal("unarchiving a live spec should fail")
	}
}

func TestArchiveEligibility(t *testing.T) {
	st := setupRepo(t)
	active := featureWithStatus(t, st, "in flight", "accepted")
	done := featureWithStatus(t, st, "done", "deprecated")
	if err := cmdArchive(io.Discard, []string{done, active}); err == nil || !strings.Contains(err.Error(), "--force") {
		t.Fatalf("archiving an accepted spec should need --force: %v", err)
	}
	if st.Get(done).Archived {
		t.Fatal("nothing should move when any id is ineligible")
	}
	if err := cmdArchive(io.Discard, []string{active, "--force"}); err != nil || !st.Get(active).Archived {
		t.Fatalf("--force should archive: %v", err)
	}
	// implemented specs are living documentation: not archivable by default
	live := featureWithStatus(t, st, "live", "implemented")
	if err := cmdArchive(io.Discard, []string{live}); err == nil || !strings.Contains(err.Error(), "living documentation") {
		t.Fatalf("archiving an implemented spec should explain why not: %v", err)
	}
	if st.Get(live).Archived {
		t.Fatal("implemented spec was archived")
	}
	if err := cmdArchive(io.Discard, []string{done}); err != nil {
		t.Fatalf("deprecated spec should archive without --force: %v", err)
	}
	rej := featureWithStatus(t, st, "nope", "rejected")
	if err := cmdArchive(io.Discard, []string{rej}); err != nil {
		t.Fatalf("rejected spec should archive without --force: %v", err)
	}
	newIdea(t, st, "live idea", nil)
	newIdea(t, st, "dead idea", map[string][]string{"status": {"discarded"}})
	if err := cmdArchive(io.Discard, []string{"IDEA-001"}); err == nil {
		t.Fatal("a new idea shouldn't be archivable without --force")
	}
	if err := cmdArchive(io.Discard, []string{"IDEA-002"}); err != nil {
		t.Fatalf("discarded idea should be archivable: %v", err)
	}
	if p := st.Get("IDEA-002").Path; !strings.Contains(filepath.ToSlash(p), "specs/archive/inbox/") {
		t.Fatalf("idea archived to wrong place: %s", p)
	}
}

func TestArchivedExcludedFromCommands(t *testing.T) {
	st := setupRepo(t)
	fakeApplyAgent(t, st, "")
	keep := featureWithStatus(t, st, "keep me", "accepted")
	gone := featureWithStatus(t, st, "gone", "accepted")
	if err := cmdArchive(io.Discard, []string{gone, "--force"}); err != nil {
		t.Fatal(err)
	}
	setTerminal(t, false)
	t.Setenv("FAKE_IMPLEMENT", keep+" "+gone)

	// apply: implicit scope skips archived; explicit id errors
	out, err := runApply(nil, applyOpts{Yes: true}, "")
	if err != nil || strings.Contains(out, "Applying "+gone) || !strings.Contains(out, "Applying "+keep) {
		t.Fatalf("implicit apply should skip archived: %v\n%s", err, out)
	}
	if _, err := runApply([]string{gone}, applyOpts{Force: true}, ""); err == nil || !strings.Contains(err.Error(), "unarchive") {
		t.Fatalf("explicit apply of archived spec should suggest unarchive: %v", err)
	}

	// status: separate line, not in counts
	var buf bytes.Buffer
	cmdStatus(&buf, io.Discard)
	if s := buf.String(); !strings.Contains(s, "Specs: 1\n") || !strings.Contains(s, "Archived: 1") {
		t.Fatalf("status:\n%s", s)
	}

	// roadmap: excluded
	buf.Reset()
	cmdRoadmap(&buf, io.Discard, "")
	if strings.Contains(buf.String(), gone) {
		t.Fatalf("roadmap should exclude archived:\n%s", buf.String())
	}
}

func TestArchivedStillSearchableTraceableValid(t *testing.T) {
	st := setupRepo(t)
	st.Create("project", "proj", "# proj\n\n## Overview\n\no\n\n## Requirements\n\n- REQ-001: thing\n", map[string][]string{"status": {"implemented"}})
	body := "# old\n\n## Summary\n\nfindme\n\n## Requirements\n\nAddresses: REQ-001\n\n## Design\n\nd\n\n## Acceptance Criteria\n\n- [x] x\n"
	old, _ := st.Create("feature", "old", body, map[string][]string{"status": {"deprecated"}})
	// a live spec depends on the soon-archived one
	st.Create("feature", "new", "# new\n\n## Summary\n\ns\n\n## Design\n\nd\n\n## Acceptance Criteria\n\n- [ ] x\n", map[string][]string{"depends_on": {old.ID()}})
	if err := cmdArchive(io.Discard, []string{old.ID()}); err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	cmdSearch(&buf, io.Discard, "findme")
	if !strings.Contains(buf.String(), old.ID()) || !strings.Contains(buf.String(), "[archived]") {
		t.Fatalf("search:\n%s", buf.String())
	}
	buf.Reset()
	cmdTrace(&buf, io.Discard)
	if !strings.Contains(buf.String(), old.ID()+" (archived)") || !strings.Contains(buf.String(), "Coverage: 1/1") {
		t.Fatalf("trace:\n%s", buf.String())
	}
	buf.Reset()
	if err := cmdValidate(&buf, io.Discard); err != nil {
		t.Fatalf("archived specs (and depends_on to them) should validate: %v\n%s", err, buf.String())
	}
	// ids are never reused
	if s, _ := st.Create("feature", "next", "# n\n", nil); s.ID() != "FEAT-003" {
		t.Fatalf("nextID reused an archived id: %s", s.ID())
	}
}

func TestArchivedIdeaCantBePromotedOrRefined(t *testing.T) {
	st := setupRepo(t)
	newIdea(t, st, "dead", map[string][]string{"status": {"discarded"}})
	cmdArchive(io.Discard, []string{"IDEA-001"})
	if err := cmdPromote(io.Discard, []string{"IDEA-001", "feature", "--force"}); err == nil || !strings.Contains(err.Error(), "unarchive") {
		t.Fatalf("promote archived idea: %v", err)
	}
	if _, err := runRefine("IDEA-001", "", true); err == nil || !strings.Contains(err.Error(), "unarchive") {
		t.Fatalf("refine archived idea: %v", err)
	}
}

func TestWebArchive(t *testing.T) {
	st, srv, _ := setupWeb(t)
	gone := featureWithStatus(t, st, "gone feature", "rejected")
	featureWithStatus(t, st, "live feature", "accepted")
	newIdea(t, st, "gone idea", map[string][]string{"status": {"discarded"}})
	if err := cmdArchive(io.Discard, []string{gone, "IDEA-001"}); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{"/board", "/board?all=1"} {
		page := webGet(t, srv.URL+p)
		if strings.Contains(page, "gone feature") || !strings.Contains(page, "live feature") {
			t.Fatalf("%s should exclude archived specs:\n%s", p, page)
		}
		if !strings.Contains(page, `2 archived — <a href="/archive">view archive</a>`) {
			t.Fatalf("%s missing archive link:\n%s", p, page)
		}
	}
	if page := webGet(t, srv.URL+"/?all=1"); strings.Contains(page, "gone idea") {
		t.Fatal("inbox should exclude archived ideas, even with ?all=1")
	}
	page := webGet(t, srv.URL+"/archive")
	if !strings.Contains(page, `href="/spec/`+gone+`"`) || !strings.Contains(page, "gone idea") || strings.Contains(page, "live feature") {
		t.Fatalf("archive page:\n%s", page)
	}
	if page := webGet(t, srv.URL+"/spec/"+gone); !strings.Contains(page, `<a href="/archive">archived</a>`) {
		t.Fatalf("archived spec page should be reachable and marked:\n%s", page)
	}
}
