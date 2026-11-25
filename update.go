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
	"os/exec"
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
	Tag        string         `json:"tag_name"`
	Draft      bool           `json:"draft"`
	Prerelease bool           `json:"prerelease"`
	Assets     []releaseAsset `json:"assets"`
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

// releaseTag resolves the target release. An explicit tag is strict —
// that release or an error. The default path walks recent releases
// newest→oldest and picks the first that carries the platform asset, so
// a broken/incomplete release can't strand updates; skipped tags are
// returned for reporting.
func releaseTag(ctx context.Context, requested, asset string) (*releaseInfo, []string, error) {
	if requested != "" {
		body, err := fetchURL(ctx, releasesAPI+"/tags/"+requested)
		if err != nil {
			return nil, nil, err
		}
		var rel releaseInfo
		if err := json.Unmarshal(body, &rel); err != nil {
			return nil, nil, err
		}
		if rel.Tag == "" {
			return nil, nil, fmt.Errorf("release response has no tag")
		}
		return &rel, nil, nil
	}
	body, err := fetchURL(ctx, releasesAPI+"?per_page=15")
	if err != nil {
		return nil, nil, err
	}
	var rels []releaseInfo
	if err := json.Unmarshal(body, &rels); err != nil {
		return nil, nil, err
	}
	var skipped []string
	for i := range rels {
		rel := &rels[i]
		if rel.Tag == "" || rel.Draft || rel.Prerelease {
			continue
		}
		if findAsset(rel, asset) == nil {
			skipped = append(skipped, rel.Tag)
			continue
		}
		return rel, skipped, nil
	}
	return nil, skipped, fmt.Errorf("no recent release carries asset %s", asset)
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
	name := "spec-" + runtime.GOOS + "-" + runtime.GOARCH
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	rel, skipped, err := releaseTag(ctx, requested, name)
	if err != nil {
		return err
	}
	fmt.Fprintln(out, "Latest release:", rel.Tag)
	for _, tag := range skipped {
		fmt.Fprintf(errOut, "note: skipped %s — no %s asset (incomplete release)\n", tag, name)
	}
	if version == "dev" {
		fmt.Fprintln(errOut, "note: this is a dev build; installing release over it")
	} else if "v"+version == rel.Tag {
		fmt.Fprintln(out, "Already up to date.")
		return nil
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
	if err := replaceFile(tmp, self); err != nil {
		os.Remove(tmp)
		return err
	}
	fmt.Fprintf(out, "Updated %s to %s\n", self, rel.Tag)
	resyncAfterUpdate(self, out, errOut)
	return nil
}

// resyncAfterUpdate re-runs `spec skill sync` through the just-installed
// binary so embedded-skill installs pick up the new version (this process
// still holds the old embedded copy). Repo sync when cwd is inside a
// spec-managed tree; global sync always. Best-effort — the update
// already succeeded, so failures are notes, not errors.
func resyncAfterUpdate(self string, out, errOut io.Writer) {
	if _, err := mustStore(); err == nil {
		run := exec.Command(self, "skill", "sync")
		run.Stdout, run.Stderr = out, errOut
		if err := run.Run(); err != nil {
			fmt.Fprintf(errOut, "note: 'spec skill sync' did not fully sync — run it to resolve\n")
		}
	}
	run := exec.Command(self, "skill", "sync", "--global")
	run.Stdout, run.Stderr = out, errOut
	if err := run.Run(); err != nil {
		fmt.Fprintf(errOut, "note: 'spec skill sync --global' did not fully sync — run it to resolve\n")
	}
	// regenerate completion scripts that are already installed — they
	// embed the old binary's command table; don't create ones the user
	// never installed
	for _, shell := range []string{"bash", "zsh", "fish"} {
		p, err := completionInstallPath(shell)
		if err != nil {
			continue
		}
		if _, err := os.Stat(p); err != nil {
			continue
		}
		run := exec.Command(self, "completion", shell, "--install")
		run.Stdout, run.Stderr = out, errOut
		if err := run.Run(); err != nil {
			fmt.Fprintf(errOut, "note: could not regenerate %s\n", p)
		}
	}
}
