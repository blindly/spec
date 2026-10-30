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
	resp, err := http.Get(srv.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if !strings.Contains(string(body), "FEAT-001") {
		t.Fatalf("index missing spec: %s", body)
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
	name, hue := projectIdent(st)
	if name != "Acme Thing" || hue < 0 || hue >= 360 {
		t.Fatalf("bad projectIdent: %q %d", name, hue)
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
