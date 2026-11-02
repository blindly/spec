package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func setupWeb(t *testing.T) (Store, *httptest.Server, string) {
	t.Helper()
	st := setupRepo(t)
	token := "testtoken"
	srv := httptest.NewServer(webHandler(st, loadConfig(st.Root), token))
	t.Cleanup(srv.Close)
	return st, srv, token
}

func postForm(t *testing.T, url string, form url.Values) *http.Response {
	t.Helper()
	// don't follow redirects — we assert on them
	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}}
	resp, err := client.PostForm(url, form)
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

func TestWebIndexAndDetail(t *testing.T) {
	st, srv, _ := setupWeb(t)
	if err := cmdNew(io.Discard, io.Discard, "feature", "web test"); err != nil {
		t.Fatal(err)
	}
	resp, err := http.Get(srv.URL + "/board")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if !strings.Contains(string(body), "FEAT-001") {
		t.Fatalf("board missing spec: %s", body)
	}
	resp, err = http.Get(srv.URL + "/spec/FEAT-001")
	if err != nil {
		t.Fatal(err)
	}
	body, _ = io.ReadAll(resp.Body)
	resp.Body.Close()
	if !strings.Contains(string(body), "web test") {
		t.Fatalf("detail missing spec: %s", body)
	}
	resp, err = http.Get(srv.URL + "/spec/NOPE-999")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 404 {
		t.Fatalf("want 404, got %d", resp.StatusCode)
	}
	_ = st
}

func TestWebBoardHidesFinished(t *testing.T) {
	st := setupRepo(t)
	body := "# x\n\n## Summary\n\ns\n\n## Design\n\nd\n\n## Acceptance Criteria\n\n- [ ] x\n"
	for title, status := range map[string]string{"active one": "accepted", "live one": "implemented", "dead one": "rejected"} {
		if _, err := st.Create("feature", title, body, map[string][]string{"status": {status}}); err != nil {
			t.Fatal(err)
		}
	}
	get := func(srv *httptest.Server, path string) string {
		resp, err := http.Get(srv.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return string(b)
	}
	srv := httptest.NewServer(webHandler(st, loadConfig(st.Root), "t"))
	defer srv.Close()

	// the board is work in progress: implemented specs live in /system
	// (FEAT-011), no-longer-valid ones (rejected) are hidden by default
	page := get(srv, "/board")
	if !strings.Contains(page, "active one") || strings.Contains(page, "live one") || strings.Contains(page, "dead one") {
		t.Fatalf("default board should show only work in progress:\n%s", page)
	}
	if !strings.Contains(page, "1 finished spec(s) hidden") || !strings.Contains(page, `href="/board?all=1"`) {
		t.Fatalf("missing hidden count / show link:\n%s", page)
	}
	if !strings.Contains(page, `1 spec(s) in the system — <a href="/system">view system</a>`) {
		t.Fatalf("missing link to the system view:\n%s", page)
	}
	page = get(srv, "/board?all=1")
	if !strings.Contains(page, "dead one") || !strings.Contains(page, "hide finished") || strings.Contains(page, "live one") {
		t.Fatalf("?all=1 should show finished (not system) specs with a hide link:\n%s", page)
	}

	// custom done_statuses still honored; everything left finished →
	// "Nothing in progress" plus the show link
	os.WriteFile(filepath.Join(st.Root, configDir, "config.yml"), []byte("done_statuses: [accepted, rejected]\n"), 0o644)
	srv2 := httptest.NewServer(webHandler(st, loadConfig(st.Root), "t"))
	defer srv2.Close()
	if page = get(srv2, "/board"); !strings.Contains(page, "Nothing in progress") || !strings.Contains(page, "2 finished spec(s) hidden") {
		t.Fatalf("custom done_statuses / all-finished board:\n%s", page)
	}
}

func TestWebProjectBranding(t *testing.T) {
	st, srv, _ := setupWeb(t)
	if _, err := st.Create("project", "Acme Thing", "# t\n\n## Overview\n\no\n\n## Requirements\n\n- REQ-001: x\n", nil); err != nil {
		t.Fatal(err)
	}
	resp, err := http.Get(srv.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	s := string(body)
	if !strings.Contains(s, `class="proj"`) || strings.Count(s, "Acme Thing") < 2 {
		t.Fatalf("missing project name in nav/title: %.300s", s)
	}
	if !strings.Contains(s, "hsl(") {
		t.Fatal("no per-project accent color")
	}
	name, hue := projectIdent(st, "")
	if name != "Acme Thing" || hue < 0 || hue >= 360 {
		t.Fatalf("bad projectIdent: %q %d", name, hue)
	}
}

// FEAT-014: project_name fact → project spec title → git remote → folder.
func TestProjectNameOrder(t *testing.T) {
	st := setupRepo(t)
	if got := projectName(st, ""); got != filepath.Base(st.Root) {
		t.Fatalf("fallback should be the folder, got %q", got)
	}
	if got := projectName(st, "remote-repo"); got != "remote-repo" {
		t.Fatalf("remote should beat the folder, got %q", got)
	}
	proj, _ := st.Create("project", "Spec Title", "# p\n\n## Overview\n\no\n\n## Requirements\n\n- REQ-001: x\n", map[string][]string{"status": {"deprecated"}})
	if got := projectName(st, "remote-repo"); got != "Spec Title" {
		t.Fatalf("project spec title should beat the remote, got %q", got)
	}
	if err := cmdArchive(io.Discard, []string{proj.ID()}); err != nil {
		t.Fatal(err)
	}
	if got := projectName(st, "remote-repo"); got != "remote-repo" {
		t.Fatalf("archived project spec should be ignored, got %q", got)
	}
	if err := setFact(st.Root, "project_name", "Explicit Name"); err != nil {
		t.Fatal(err)
	}
	if got := projectName(st, "remote-repo"); got != "Explicit Name" {
		t.Fatalf("project_name fact should win, got %q", got)
	}
}

func TestRepoNameFromURL(t *testing.T) {
	for in, want := range map[string]string{
		"git@github.com:acme/truecare-website.git":  "truecare-website",
		"https://github.com/acme/truecare-website":  "truecare-website",
		"https://github.com/acme/truecare-website/": "truecare-website",
		"ssh://git@host:2222/acme/repo.git":         "repo",
		"/srv/git/local-repo.git":                   "local-repo",
		`C:\repos\win-repo`:                         "win-repo",
	} {
		if got := repoNameFromURL(in); got != want {
			t.Fatalf("%q: got %q want %q", in, got, want)
		}
	}
}

func TestWebCreateSpec(t *testing.T) {
	st, srv, token := setupWeb(t)
	form := url.Values{"token": {token}, "type": {"feature"}, "title": {"created via web"}}
	resp := postForm(t, srv.URL+"/new", form)
	resp.Body.Close()
	if resp.StatusCode != 303 || !strings.Contains(resp.Header.Get("Location"), "/spec/FEAT-001") {
		t.Fatalf("create redirect: %d %s", resp.StatusCode, resp.Header.Get("Location"))
	}
	if st.Get("FEAT-001") == nil {
		t.Fatal("spec not created")
	}
	// bad type rejected
	form = url.Values{"token": {token}, "type": {"evil"}, "title": {"x"}}
	resp = postForm(t, srv.URL+"/new", form)
	resp.Body.Close()
	if resp.StatusCode == 303 {
		t.Fatal("invalid type accepted")
	}
}

func TestWebEditSpec(t *testing.T) {
	st, srv, token := setupWeb(t)
	if err := cmdNew(io.Discard, io.Discard, "feature", "edit me"); err != nil {
		t.Fatal(err)
	}
	newBody := "# edit me\n\n## Summary\n\nedited.\n\n## Design\n\nd.\n\n## Acceptance Criteria\n\n- [ ] x\n"
	form := url.Values{"token": {token}, "type": {"feature"}, "status": {"accepted"}, "body": {newBody}}
	resp := postForm(t, srv.URL+"/edit/FEAT-001", form)
	resp.Body.Close()
	if resp.StatusCode != 303 {
		t.Fatalf("edit redirect: %d", resp.StatusCode)
	}
	data, _ := os.ReadFile(st.Get("FEAT-001").Path)
	if !strings.Contains(string(data), "status: accepted") || !strings.Contains(string(data), "edited.") {
		t.Fatalf("file not updated:\n%s", data)
	}
}

func TestWebAgents(t *testing.T) {
	st, srv, _ := setupWeb(t)
	get := func() string {
		resp, err := http.Get(srv.URL + "/agents")
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return string(body)
	}
	// seeded AGENTS.md renders
	if body := get(); !strings.Contains(body, "Agent instructions") {
		t.Fatalf("agents page missing seeded AGENTS.md: %s", body)
	}
	// falls back to CLAUDE.md
	os.Remove(filepath.Join(st.Root, "AGENTS.md"))
	if body := get(); !strings.Contains(body, "@AGENTS.md") {
		t.Fatalf("agents page missing CLAUDE.md fallback: %s", body)
	}
	// neither → friendly message
	os.Remove(filepath.Join(st.Root, "CLAUDE.md"))
	if body := get(); !strings.Contains(body, "No AGENTS.md or CLAUDE.md") {
		t.Fatalf("agents page missing empty state: %s", body)
	}
}

// The page heading is only added when the document has none of its own, so
// README/AGENTS.md headings aren't shown twice.
func TestWebDocPagesNoDuplicateHeading(t *testing.T) {
	st, srv, _ := setupWeb(t)
	os.WriteFile(filepath.Join(st.Root, "README.md"), []byte("# My project\n\nhello\n"), 0o644)
	if page := webGet(t, srv.URL+"/readme"); strings.Contains(page, "<h1>README</h1>") || !strings.Contains(page, "# My project") {
		t.Fatalf("readme with its own heading shouldn't get a second one:\n%s", page)
	}
	os.WriteFile(filepath.Join(st.Root, "README.md"), []byte("no heading here\n"), 0o644)
	if page := webGet(t, srv.URL+"/readme"); !strings.Contains(page, "<h1>README</h1>") {
		t.Fatalf("readme without a heading should get the page heading:\n%s", page)
	}
	// seeded AGENTS.md opens with "# Agent instructions"
	page := webGet(t, srv.URL+"/agents")
	if strings.Contains(page, "<h1>AGENTS.md</h1>") || !strings.Contains(page, `<p class="muted">from AGENTS.md</p>`) {
		t.Fatalf("agents page should show the doc's own heading plus a source caption:\n%s", page)
	}
}

// The spec page shows the title once: in the page heading (with the id), not
// again from the body's own "# " heading.
func TestWebSpecPageNoDuplicateTitle(t *testing.T) {
	st, srv, _ := setupWeb(t)
	st.Create("feature", "Board view", "# Board view\n\n## Summary\n\ns\n", nil)
	page := webGet(t, srv.URL+"/spec/FEAT-001")
	if !strings.Contains(page, "<h1>FEAT-001 — Board view</h1>") || strings.Contains(page, "# Board view") {
		t.Fatalf("title should appear once, in the page heading:\n%s", page)
	}
	if !strings.Contains(page, "## Summary") {
		t.Fatalf("rest of the body missing:\n%s", page)
	}
	// body heading edited to differ from frontmatter title → body wins
	st.Create("feature", "Old title", "# New title\n\nbody\n", nil)
	if page := webGet(t, srv.URL+"/spec/FEAT-002"); !strings.Contains(page, "<h1>FEAT-002 — New title</h1>") || strings.Contains(page, "# New title") {
		t.Fatalf("body heading should be used once:\n%s", page)
	}
	// no body heading → frontmatter title
	st.Create("feature", "Only meta", "just text\n", nil)
	if page := webGet(t, srv.URL+"/spec/FEAT-003"); !strings.Contains(page, "<h1>FEAT-003 — Only meta</h1>") || !strings.Contains(page, "just text") {
		t.Fatalf("frontmatter title fallback:\n%s", page)
	}
}

func TestWebReadmeCaseInsensitive(t *testing.T) {
	st, srv, _ := setupWeb(t)
	os.WriteFile(filepath.Join(st.Root, "README.MD"), []byte("# Shouty readme\n"), 0o644)
	resp, err := http.Get(srv.URL + "/readme")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if !strings.Contains(string(body), "Shouty readme") {
		t.Fatalf("README.MD not found: %.300s", body)
	}
}

func TestWebHelp(t *testing.T) {
	st, srv, _ := setupWeb(t)
	resp, err := http.Get(srv.URL + "/help")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	s := string(body)
	if !strings.Contains(s, "{fact:") || !strings.Contains(s, "No facts defined") {
		t.Fatalf("help page missing content: %.300s", s)
	}
	// defined facts are listed
	if err := setFact(st.Root, "domain", "example.com"); err != nil {
		t.Fatal(err)
	}
	resp, err = http.Get(srv.URL + "/help")
	if err != nil {
		t.Fatal(err)
	}
	body, _ = io.ReadAll(resp.Body)
	resp.Body.Close()
	if !strings.Contains(string(body), "{fact:domain}") {
		t.Fatalf("help page missing defined fact: %.300s", body)
	}
}

func TestWebEditTypeIsFixed(t *testing.T) {
	st, srv, token := setupWeb(t)
	if err := cmdNew(io.Discard, io.Discard, "feature", "fixed type"); err != nil {
		t.Fatal(err)
	}
	resp, err := http.Get(srv.URL + "/edit/FEAT-001")
	if err != nil {
		t.Fatal(err)
	}
	page, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if strings.Contains(string(page), `name="type"`) || !strings.Contains(string(page), "<strong>feature</strong>") {
		t.Fatalf("edit page should show type as text, not a field: %.500s", page)
	}
	// a submitted type is ignored; required sections checked against feature
	featBody := "# fixed type\n\n## Summary\n\ns\n\n## Design\n\nd\n\n## Acceptance Criteria\n\n- [ ] x\n"
	resp = postForm(t, srv.URL+"/edit/FEAT-001",
		url.Values{"token": {token}, "type": {"adr"}, "status": {"proposed"}, "body": {featBody}})
	resp.Body.Close()
	if resp.StatusCode != 303 {
		t.Fatalf("save failed: %d", resp.StatusCode)
	}
	s := st.Get("FEAT-001")
	if s.Type() != "feature" || s.Status() != "proposed" {
		t.Fatalf("type changed or status not saved: %s %s", s.Type(), s.Status())
	}
	// an adr-shaped body missing feature sections is rejected, even with type=adr
	adrBody := "# x\n\n## Context\n\nc\n\n## Decision\n\nd\n\n## Consequences\n\nq\n"
	resp = postForm(t, srv.URL+"/edit/FEAT-001",
		url.Values{"token": {token}, "type": {"adr"}, "status": {"proposed"}, "body": {adrBody}})
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if !strings.Contains(string(body), "missing required section") {
		t.Fatalf("sections not checked against current type: %.300s", body)
	}
}

func TestWebEditRejectsBadInput(t *testing.T) {
	_, srv, token := setupWeb(t)
	if err := cmdNew(io.Discard, io.Discard, "feature", "keep sections"); err != nil {
		t.Fatal(err)
	}
	// bad token → 403
	resp := postForm(t, srv.URL+"/edit/FEAT-001",
		url.Values{"token": {"wrong"}, "type": {"feature"}, "status": {"draft"}, "body": {"x"}})
	resp.Body.Close()
	if resp.StatusCode != 403 {
		t.Fatalf("bad token: %d", resp.StatusCode)
	}
	// invalid status → re-render (200), no save
	resp = postForm(t, srv.URL+"/edit/FEAT-001",
		url.Values{"token": {token}, "type": {"feature"}, "status": {"bogus"}, "body": {"x"}})
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("bad status: %d", resp.StatusCode)
	}
	// missing required section → 200 with error, no save
	resp = postForm(t, srv.URL+"/edit/FEAT-001",
		url.Values{"token": {token}, "type": {"feature"}, "status": {"draft"}, "body": {"# x\n\n## Summary\n\ns\n"}})
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if !strings.Contains(string(body), "missing required section") {
		t.Fatalf("missing-section check bypassed: %s", body)
	}
}
