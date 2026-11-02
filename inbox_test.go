package main

import (
	"bytes"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func newIdea(t *testing.T, st Store, title string, extra map[string][]string) Spec {
	t.Helper()
	s, err := st.Create("idea", title, templateFor("idea", title), extra)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestIdeaCreate(t *testing.T) {
	st := setupRepo(t)
	if err := commands["idea"]([]string{"better", "search"}, io.Discard, io.Discard); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(st.Root, specDir, "inbox", "IDEA-001-better-search.md")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("idea file not created: %v", err)
	}
	s := st.Get("IDEA-001")
	if s.Type() != "idea" || s.Status() != "new" || !strings.Contains(string(data), "## Notes") {
		t.Fatalf("bad idea: type=%s status=%s\n%s", s.Type(), s.Status(), data)
	}
	if err := cmdValidate(io.Discard, io.Discard); err != nil {
		t.Fatalf("fresh idea should validate (no required sections): %v", err)
	}
}

func TestIdeaStatusesValidated(t *testing.T) {
	st := setupRepo(t)
	cfg := loadConfig(st.Root)
	if strings.Join(cfg.IdeaStatuses, ",") != "new,refined,discarded" {
		t.Fatalf("default idea_statuses: %v", cfg.IdeaStatuses)
	}
	newIdea(t, st, "fine", map[string][]string{"status": {"refined"}})
	if err := cmdValidate(io.Discard, io.Discard); err != nil {
		t.Fatalf("refined idea should validate: %v", err)
	}
	newIdea(t, st, "wrong", map[string][]string{"status": {"accepted"}})
	var buf bytes.Buffer
	if err := cmdValidate(&buf, io.Discard); err == nil || !strings.Contains(buf.String(), `unknown status "accepted"`) {
		t.Fatalf("spec status on an idea should fail: %v\n%s", err, buf.String())
	}
	// and the reverse: an idea status on a feature
	os.Remove(st.Get("IDEA-002").Path)
	featureWithStatus(t, st, "feat", "new")
	buf.Reset()
	if err := cmdValidate(&buf, io.Discard); err == nil || !strings.Contains(buf.String(), `unknown status "new"`) {
		t.Fatalf("idea status on a feature should fail: %v\n%s", err, buf.String())
	}
}

func TestDiscardIdea(t *testing.T) {
	st := setupRepo(t)
	newIdea(t, st, "meh", nil)
	if err := cmdDiscard(io.Discard, []string{"IDEA-001"}); err != nil {
		t.Fatal(err)
	}
	if s := st.Get("IDEA-001"); s.Status() != "discarded" {
		t.Fatalf("status %s", s.Status())
	}
	featureWithStatus(t, st, "not an idea", "draft")
	if err := cmdDiscard(io.Discard, []string{"FEAT-001"}); err == nil {
		t.Fatal("discarding a feature should fail")
	}
}

func TestPromoteIdea(t *testing.T) {
	st := setupRepo(t)
	idea := newIdea(t, st, "pick a db", nil)
	notes := "# pick a db\n\n## Notes\n\nPostgres or SQLite?\n\n## Options\n\n- pg\n- sqlite\n"
	os.WriteFile(idea.Path, []byte(dumpFrontmatter(idea.Meta, notes)), 0o644)
	// another spec depends on the idea
	dep, _ := st.Create("feature", "uses db", "# x\n", map[string][]string{"depends_on": {"IDEA-001"}})

	var out bytes.Buffer
	if err := cmdPromote(&out, []string{"IDEA-001", "adr"}); err != nil {
		t.Fatal(err)
	}
	adr := st.Get("ADR-001")
	if adr == nil {
		t.Fatalf("no ADR created:\n%s", out.String())
	}
	if adr.Field("promoted_from")[0] != "IDEA-001" || adr.Status() != "draft" || adr.Title() != "pick a db" {
		t.Fatalf("bad promoted meta: %v", adr.Meta)
	}
	ctx := adr.Body[strings.Index(adr.Body, "## Context"):strings.Index(adr.Body, "## Decision")]
	if !strings.Contains(ctx, "Postgres or SQLite?") || !strings.Contains(ctx, "### Options") || strings.Contains(ctx, "TODO") {
		t.Fatalf("notes not carried into Context (headings demoted):\n%s", adr.Body)
	}
	if strings.Contains(adr.Body, "## Notes") {
		t.Fatalf("idea Notes heading leaked:\n%s", adr.Body)
	}
	if _, err := os.Stat(idea.Path); !os.IsNotExist(err) {
		t.Fatal("idea file should be deleted after promote")
	}
	if got := st.Get(dep.ID()).Field("depends_on"); len(got) != 1 || got[0] != "ADR-001" {
		t.Fatalf("depends_on not rewritten: %v", got)
	}
	if !strings.Contains(out.String(), "IDEA-001 → ADR-001") {
		t.Fatalf("output: %s", out.String())
	}
}

func TestPromoteUsesSuggestedType(t *testing.T) {
	st := setupRepo(t)
	newIdea(t, st, "no hint", nil)
	if err := cmdPromote(io.Discard, []string{"IDEA-001"}); err == nil || !strings.Contains(err.Error(), "no suggested type") {
		t.Fatalf("expected no-suggestion error, got %v", err)
	}
	newIdea(t, st, "hinted", map[string][]string{"suggested_type": {"feature"}})
	if err := cmdPromote(io.Discard, []string{"IDEA-002"}); err != nil {
		t.Fatal(err)
	}
	if f := st.Get("FEAT-001"); f == nil || f.Title() != "hinted" {
		t.Fatal("suggested_type not used")
	}
	newIdea(t, st, "bad type", nil)
	if err := cmdPromote(io.Discard, []string{"IDEA-003", "idea"}); err == nil {
		t.Fatal("promoting to idea should fail")
	}
}

func TestPromoteDiscardedNeedsForce(t *testing.T) {
	st := setupRepo(t)
	newIdea(t, st, "dead", map[string][]string{"status": {"discarded"}})
	if err := cmdPromote(io.Discard, []string{"IDEA-001", "feature"}); err == nil || !strings.Contains(err.Error(), "--force") {
		t.Fatalf("expected --force error, got %v", err)
	}
	if err := cmdPromote(io.Discard, []string{"IDEA-001", "feature", "--force"}); err != nil {
		t.Fatal(err)
	}
}

func TestApplyRefusesIdeas(t *testing.T) {
	st := setupRepo(t)
	fakeApplyAgent(t, st, "")
	newIdea(t, st, "not a spec", nil)
	for _, opts := range []applyOpts{{}, {Force: true}} {
		if _, err := runApply([]string{"IDEA-001"}, opts, ""); err == nil || !strings.Contains(err.Error(), "promote") {
			t.Fatalf("apply on idea (force=%v) should fail suggesting promote: %v", opts.Force, err)
		}
	}
}

func TestStatusAndRoadmapSeparateIdeas(t *testing.T) {
	st := setupRepo(t)
	newIdea(t, st, "one", nil)
	newIdea(t, st, "two", map[string][]string{"status": {"refined"}})
	featureWithStatus(t, st, "real", "accepted")
	var buf bytes.Buffer
	cmdStatus(&buf, io.Discard)
	out := buf.String()
	if !strings.Contains(out, "Inbox: 2 idea(s) (1 new, 1 refined)") || !strings.Contains(out, "Specs: 1") || strings.Contains(out, "not in config") {
		t.Fatalf("status output:\n%s", out)
	}
	buf.Reset()
	cmdRoadmap(&buf, io.Discard, "")
	if strings.Contains(buf.String(), "IDEA-") {
		t.Fatalf("roadmap should exclude ideas:\n%s", buf.String())
	}
}

// --- web ---

func webGet(t *testing.T, u string) string {
	t.Helper()
	resp, err := http.Get(u)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	return string(b)
}

func TestWebInbox(t *testing.T) {
	st, srv, token := setupWeb(t)

	page := webGet(t, srv.URL+"/")
	if !strings.Contains(page, "Inbox empty") || !strings.Contains(page, `action="/idea"`) {
		t.Fatalf("empty inbox page:\n%s", page)
	}
	// capture via the inbox box
	resp := postForm(t, srv.URL+"/idea", url.Values{"token": {token}, "title": {"first thought"}})
	resp.Body.Close()
	if resp.StatusCode != 303 || st.Get("IDEA-001") == nil {
		t.Fatalf("capture failed: %d", resp.StatusCode)
	}
	newIdea(t, st, "second thought", map[string][]string{"suggested_type": {"adr"}, "suggested_reason": {"a choice"}})
	featureWithStatus(t, st, "board item", "accepted")

	page = webGet(t, srv.URL+"/")
	if !strings.Contains(page, "first thought") || !strings.Contains(page, "second thought") ||
		!strings.Contains(page, `action="/promote/IDEA-001"`) || !strings.Contains(page, `action="/discard/IDEA-001"`) {
		t.Fatalf("inbox list missing rows/actions:\n%s", page)
	}
	if strings.Contains(page, "board item") {
		t.Fatal("specs must not appear in the inbox")
	}
	if strings.Index(page, "second thought") > strings.Index(page, "first thought") {
		t.Fatal("inbox should list newest first")
	}
	if !strings.Contains(page, `<span class="badge" title="a choice">adr</span>`) || !strings.Contains(page, "<option selected>adr</option>") {
		t.Fatalf("suggested type badge / preselect missing:\n%s", page)
	}
	board := webGet(t, srv.URL+"/board")
	if !strings.Contains(board, "board item") || strings.Contains(board, "first thought") {
		t.Fatalf("board should show specs but never ideas:\n%s", board)
	}

	// discard hides by default, toggle shows
	resp = postForm(t, srv.URL+"/discard/IDEA-001", url.Values{"token": {token}})
	resp.Body.Close()
	if st.Get("IDEA-001").Status() != "discarded" {
		t.Fatal("web discard failed")
	}
	page = webGet(t, srv.URL+"/")
	if strings.Contains(page, "first thought") || !strings.Contains(page, "1 discarded") {
		t.Fatalf("discarded idea should be hidden behind a toggle:\n%s", page)
	}
	if page = webGet(t, srv.URL+"/?all=1"); !strings.Contains(page, "first thought") || !strings.Contains(page, "hide discarded") {
		t.Fatalf("?all=1 should show discarded:\n%s", page)
	}

	// promote redirects to the new spec's edit page and deletes the idea
	resp = postForm(t, srv.URL+"/promote/IDEA-002", url.Values{"token": {token}, "type": {"adr"}})
	resp.Body.Close()
	if resp.StatusCode != 303 || resp.Header.Get("Location") != "/edit/ADR-001" || st.Get("IDEA-002") != nil {
		t.Fatalf("web promote: %d %s", resp.StatusCode, resp.Header.Get("Location"))
	}

	// bad token rejected on every inbox write
	for _, p := range []string{"/idea", "/promote/IDEA-001", "/discard/IDEA-001"} {
		resp = postForm(t, srv.URL+p, url.Values{"token": {"wrong"}, "title": {"x"}, "type": {"feature"}})
		resp.Body.Close()
		if resp.StatusCode != 403 {
			t.Fatalf("%s with bad token: %d", p, resp.StatusCode)
		}
	}
}

// FEAT-013: the new spec form is for typed specs; ideas come from the inbox.
func TestWebNewFormTypedOnly(t *testing.T) {
	st, srv, token := setupWeb(t)
	page := webGet(t, srv.URL+"/new")
	if strings.Contains(page, `value="idea"`) || !strings.Contains(page, `<option value="feature" selected>feature</option>`) || !strings.Contains(page, `<a href="/">inbox</a> as an idea`) {
		t.Fatalf("form should offer project/feature/adr with feature selected and link the inbox:\n%s", page)
	}
	resp := postForm(t, srv.URL+"/new", url.Values{"token": {token}, "type": {"idea"}, "title": {"sneaky"}})
	resp.Body.Close()
	if resp.StatusCode == 303 || st.Get("IDEA-001") != nil {
		t.Fatal("posting an idea to /new should be rejected")
	}
}

// FEAT-013: refined ideas appear on the board as a "ready" column.
func TestWebBoardReadyColumn(t *testing.T) {
	st, srv, token := setupWeb(t)
	if page := webGet(t, srv.URL+"/board"); strings.Contains(page, "<h2>ready") {
		t.Fatal("no ready column without refined ideas")
	}
	newIdea(t, st, "raw thought", nil)
	newIdea(t, st, "dead thought", map[string][]string{"status": {"discarded"}})
	newIdea(t, st, "ready thought", map[string][]string{"status": {"refined"}, "suggested_type": {"adr"}, "suggested_reason": {"a choice"}})
	featureWithStatus(t, st, "wip", "proposed")
	board := webGet(t, srv.URL+"/board")
	ready, proposed := strings.Index(board, "<h2>ready <span class=\"count\">1</span>"), strings.Index(board, "<h2>proposed")
	if ready < 0 || proposed < 0 || ready > proposed {
		t.Fatalf("ready column should come first with one idea:\n%s", board)
	}
	if !strings.Contains(board, "ready thought") || strings.Contains(board, "raw thought") || strings.Contains(board, "dead thought") {
		t.Fatalf("only refined ideas belong on the board:\n%s", board)
	}
	if !strings.Contains(board, `action="/promote/IDEA-003"`) || !strings.Contains(board, "<option selected>adr</option>") || !strings.Contains(board, `title="a choice">adr</span>`) {
		t.Fatalf("ready card should have the suggested badge and a preselected promote form:\n%s", board)
	}
	resp := postForm(t, srv.URL+"/promote/IDEA-003", url.Values{"token": {token}, "type": {"adr"}})
	resp.Body.Close()
	if resp.StatusCode != 303 || resp.Header.Get("Location") != "/edit/ADR-001" {
		t.Fatalf("promote from the board: %d %s", resp.StatusCode, resp.Header.Get("Location"))
	}
	// only refined ideas left → still a board, not "no work in progress"
	os.Remove(st.Get("FEAT-001").Path)
	newIdea(t, st, "another ready", map[string][]string{"status": {"refined"}})
	if board := webGet(t, srv.URL+"/board"); strings.Contains(board, "No work in progress") || !strings.Contains(board, "another ready") {
		t.Fatalf("refined ideas alone should still show the board:\n%s", board)
	}
}

func TestWebEditIdeaStatusDropdown(t *testing.T) {
	st, srv, token := setupWeb(t)
	newIdea(t, st, "editable", nil)
	featureWithStatus(t, st, "feat", "draft")
	page := webGet(t, srv.URL+"/edit/IDEA-001")
	if !strings.Contains(page, "<option selected>new</option>") || !strings.Contains(page, "<option>refined</option>") || strings.Contains(page, "<option>accepted</option>") {
		t.Fatalf("idea edit dropdown should list idea statuses:\n%s", page)
	}
	page = webGet(t, srv.URL+"/edit/FEAT-001")
	if !strings.Contains(page, "<option>accepted</option>") || strings.Contains(page, "<option>refined</option>") {
		t.Fatalf("feature edit dropdown should list spec statuses:\n%s", page)
	}
	// saving an idea with an idea status works; with a spec status it doesn't
	resp := postForm(t, srv.URL+"/edit/IDEA-001", url.Values{"token": {token}, "status": {"refined"}, "body": {"# editable\n\n## Notes\n\nmore\n"}})
	resp.Body.Close()
	if resp.StatusCode != 303 || st.Get("IDEA-001").Status() != "refined" {
		t.Fatalf("idea save: %d %s", resp.StatusCode, st.Get("IDEA-001").Status())
	}
	resp = postForm(t, srv.URL+"/edit/IDEA-001", url.Values{"token": {token}, "status": {"accepted"}, "body": {"# x\n"}})
	resp.Body.Close()
	if st.Get("IDEA-001").Status() != "refined" {
		t.Fatal("spec status accepted on an idea")
	}
}
