package main

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

// update selection: the newest release may be a broken tag (workflow's
// VERSION check failed → no assets) — update must fall through to the
// newest release that actually carries the platform asset.
func TestReleaseTagSkipsIncompleteReleases(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `[
			{"tag_name":"v1.0.0-build-999999","assets":[]},
			{"tag_name":"v1.0.0-build-999998","draft":true,"assets":[{"name":"spec-linux-amd64","browser_download_url":"x"}]},
			{"tag_name":"v1.0.0-build-999997","assets":[{"name":"spec-linux-amd64","browser_download_url":"x"}]}
		]`)
	}))
	defer srv.Close()
	old := releasesAPI
	releasesAPI = srv.URL
	defer func() { releasesAPI = old }()

	rel, skipped, err := releaseTag(context.Background(), "", "spec-linux-amd64")
	if err != nil {
		t.Fatal(err)
	}
	if rel.Tag != "v1.0.0-build-999997" {
		t.Fatalf("want the first release carrying the asset, got %s", rel.Tag)
	}
	if len(skipped) != 1 || skipped[0] != "v1.0.0-build-999999" {
		t.Fatalf("want the empty release reported as skipped (draft is silent): %v", skipped)
	}
}

// An explicit tag request stays strict — that release or an error, no
// falling through to older releases.
func TestReleaseTagExplicitStrict(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"tag_name":"v1.0.0-build-999999","assets":[]}`)
	}))
	defer srv.Close()
	old := releasesAPI
	releasesAPI = srv.URL
	defer func() { releasesAPI = old }()

	rel, _, err := releaseTag(context.Background(), "v1.0.0-build-999999", "spec-linux-amd64")
	if err != nil {
		t.Fatal(err)
	}
	if rel.Tag != "v1.0.0-build-999999" {
		t.Fatalf("explicit tag should return exactly it, got %s", rel.Tag)
	}
}
