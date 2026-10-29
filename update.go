package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

var releasesAPI = "https://api.github.com/repos/blindly/spec/releases"

var updateHTTPClient = &http.Client{Timeout: 30 * time.Second}

type releaseAsset struct {
	Name string `json:"name"`
	URL  string `json:"browser_download_url"`
}

type releaseInfo struct {
	Tag    string         `json:"tag_name"`
	Assets []releaseAsset `json:"assets"`
}

func fetchURL(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "spec/"+version)
	resp, err := updateHTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("download %s: %s", url, resp.Status)
	}
	return io.ReadAll(io.LimitReader(resp.Body, 100<<20))
}

func releaseTag(ctx context.Context, requested string) (*releaseInfo, error) {
	url := releasesAPI + "/latest"
	if requested != "" {
		url = releasesAPI + "/tags/" + requested
	}
	body, err := fetchURL(ctx, url)
	if err != nil {
		return nil, err
	}
	var rel releaseInfo
	if err := json.Unmarshal(body, &rel); err != nil {
		return nil, err
	}
	if rel.Tag == "" {
		return nil, fmt.Errorf("release response has no tag")
	}
	return &rel, nil
}

func findAsset(rel *releaseInfo, name string) *releaseAsset {
	for i, a := range rel.Assets {
		if a.Name == name {
			return &rel.Assets[i]
		}
	}
	return nil
}

func cmdUpdate(out, errOut io.Writer, requested string) error {
	ctx := context.Background()
	rel, err := releaseTag(ctx, requested)
	if err != nil {
		return err
	}
	fmt.Fprintln(out, "Latest release:", rel.Tag)
	if version != "dev" && "v"+version == rel.Tag {
		fmt.Fprintln(out, "Already up to date.")
		return nil
	}
	name := "spec-" + runtime.GOOS + "-" + runtime.GOARCH
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	bin := findAsset(rel, name)
	if bin == nil {
		return fmt.Errorf("release %s has no asset %s", rel.Tag, name)
	}
	data, err := fetchURL(ctx, bin.URL)
	if err != nil {
		return err
	}
	// Verify against checksums.txt when the release provides one.
	if sums := findAsset(rel, "checksums.txt"); sums != nil {
		sumData, err := fetchURL(ctx, sums.URL)
		if err != nil {
			return err
		}
		want := ""
		for _, line := range strings.Split(string(sumData), "\n") {
			f := strings.Fields(line)
			if len(f) == 2 && f[1] == name {
				want = f[0]
			}
		}
		if want == "" {
			return fmt.Errorf("checksums.txt has no entry for %s", name)
		}
		sum := sha256.Sum256(data)
		got := hex.EncodeToString(sum[:])
		if got != want {
			return fmt.Errorf("checksum mismatch for %s: got %s, want %s", name, got, want)
		}
	}
	self, err := os.Executable()
	if err != nil {
		return err
	}
	self, err = filepath.EvalSymlinks(self)
	if err != nil {
		return err
	}
	tmp := self + ".new"
	if err := os.WriteFile(tmp, data, 0o755); err != nil {
		return err
	}
	// Windows refuses to replace a running exe in place; move it aside first.
	old := self + ".old"
	os.Remove(old)
	if err := os.Rename(self, old); err != nil {
		os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, self); err != nil {
		os.Rename(old, self) // restore
		return err
	}
	fmt.Fprintf(out, "Updated %s to %s\n", self, rel.Tag)
	return nil
}
