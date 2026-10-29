package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
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
