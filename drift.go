package main

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
	"time"
)

// Drift detection (FEAT-012): a system spec "may be stale" when files it
// covers changed after the spec was last updated or verified.

type gitCommit struct {
	SHA, Date, Subject string
	Files              []string // paths relative to the store root
}

// gitRepo is a snapshot of the history and working tree under a store root.
type gitRepo struct {
	root    string
	head    string
	commits []gitCommit     // newest first
	dirty   map[string]bool // uncommitted changes (tracked or untracked), relative to root
}

func runGit(root string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = root
	return cmd.Output()
}

// loadGit reads the repo's history and dirty files; nil when root isn't in
// a git work tree (or git isn't installed).
func loadGit(root string) *gitRepo {
	if out, err := runGit(root, "rev-parse", "--is-inside-work-tree"); err != nil || strings.TrimSpace(string(out)) != "true" {
		return nil
	}
	g := &gitRepo{root: root, dirty: map[string]bool{}}
	if out, err := runGit(root, "rev-parse", "HEAD"); err == nil {
		g.head = strings.TrimSpace(string(out))
	}
	if g.head != "" {
		out, err := runGit(root, "log", "--format=@@@%H%x09%cs%x09%s", "--name-only", "--relative")
		if err != nil {
			return nil
		}
		for _, block := range strings.Split(string(out), "@@@")[1:] {
			lines := strings.Split(strings.TrimSpace(block), "\n")
			parts := strings.SplitN(lines[0], "\t", 3)
			if len(parts) < 3 {
				continue
			}
			c := gitCommit{SHA: parts[0], Date: parts[1], Subject: parts[2]}
			for _, f := range lines[1:] {
				if f = strings.TrimSpace(f); f != "" {
					c.Files = append(c.Files, f)
				}
			}
			g.commits = append(g.commits, c)
		}
		if out, err := runGit(root, "diff", "--name-only", "--relative", "HEAD"); err == nil {
			addLines(g.dirty, out)
		}
	}
	if out, err := runGit(root, "ls-files", "--others", "--exclude-standard"); err == nil {
		addLines(g.dirty, out)
	}
	return g
}

func addLines(set map[string]bool, out []byte) {
	sc := bufio.NewScanner(bytes.NewReader(out))
	for sc.Scan() {
		if l := strings.TrimSpace(sc.Text()); l != "" {
			set[l] = true
		}
	}
}

func (g *gitRepo) rel(p string) string {
	r, err := filepath.Rel(g.root, p)
	if err != nil {
		return p
	}
	return filepath.ToSlash(r)
}

// coverMatches reports whether a covers entry matches a repo-relative file:
// trailing "/" is a directory, glob characters make it a glob, else exact.
func coverMatches(pattern, file string) bool {
	switch {
	case strings.HasSuffix(pattern, "/"):
		return strings.HasPrefix(file, pattern)
	case strings.ContainsAny(pattern, "*?["):
		ok, _ := path.Match(pattern, file)
		return ok
	}
	return pattern == file
}

func coveredBy(covers []string, file string) bool {
	for _, c := range covers {
		if coverMatches(c, file) {
			return true
		}
	}
	return false
}

// isCodeFile filters out what isn't code a spec describes: specs and config,
// dot-directories, docs, lockfiles, and tests.
func isCodeFile(rel string) bool {
	rel = filepath.ToSlash(rel)
	if strings.HasPrefix(rel, specDir+"/") || strings.HasPrefix(rel, configDir+"/") {
		return false
	}
	segs := strings.Split(rel, "/")
	for _, s := range segs[:len(segs)-1] {
		if strings.HasPrefix(s, ".") || s == "test" || s == "tests" || s == "__tests__" {
			return false
		}
	}
	base := segs[len(segs)-1]
	if strings.HasPrefix(base, ".") || strings.HasPrefix(base, "test_") {
		return false
	}
	for _, t := range []string{"_test.", ".test.", ".spec."} {
		if strings.Contains(base, t) {
			return false
		}
	}
	switch strings.ToLower(filepath.Ext(base)) {
	case ".md", ".markdown", ".rst", ".txt":
		return false
	}
	switch base {
	case "VERSION", "LICENSE", "go.sum", "package-lock.json", "yarn.lock", "pnpm-lock.yaml", "Cargo.lock", "poetry.lock":
		return false
	}
	return true
}

// driftInfo explains why a spec may be stale.
type driftInfo struct {
	Spec        Spec
	Checkpoint  string      // SHA the spec was last updated/verified at
	Files       []string    // covered files changed since the checkpoint
	Commits     []gitCommit // commits since the checkpoint touching covered files, newest first
	Uncommitted []string    // covered files with uncommitted changes
}

// drift returns why s may be stale, or nil if it's in sync (or can't tell:
// no coverage, never committed, or its file is being edited).
func (g *gitRepo) drift(s Spec) *driftInfo {
	covers := s.Field("covers")
	rel := g.rel(s.Path)
	if len(covers) == 0 || g.dirty[rel] {
		return nil
	}
	cp := -1
	verified := metaGet(s.Meta, "verified", "")
	for i, c := range g.commits {
		if (verified != "" && strings.HasPrefix(c.SHA, verified)) || containsStr(c.Files, rel) {
			cp = i
			break
		}
	}
	if cp < 0 {
		return nil
	}
	d := &driftInfo{Spec: s, Checkpoint: g.commits[cp].SHA}
	seen := map[string]bool{}
	for _, c := range g.commits[:cp] {
		hit := false
		for _, f := range c.Files {
			if coveredBy(covers, f) {
				hit = true
				if !seen[f] {
					seen[f] = true
					d.Files = append(d.Files, f)
				}
			}
		}
		if hit {
			d.Commits = append(d.Commits, c)
		}
	}
	for f := range g.dirty {
		if coveredBy(covers, f) {
			d.Uncommitted = append(d.Uncommitted, f)
			if !seen[f] {
				seen[f] = true
				d.Files = append(d.Files, f)
			}
		}
	}
	if len(d.Files) == 0 {
		return nil
	}
	return d
}

func containsStr(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

// systemDrift checks every system spec; it returns the possibly-stale ones
// and the ids of system specs with no coverage.
func (g *gitRepo) systemDrift(st Store, cfg Config) (stale []*driftInfo, uncovered []string) {
	for _, s := range st.Specs() {
		if !inSystem(cfg, s) {
			continue
		}
		if len(s.Field("covers")) == 0 {
			uncovered = append(uncovered, s.ID())
			continue
		}
		if d := g.drift(s); d != nil {
			stale = append(stale, d)
		}
	}
	return stale, uncovered
}

// inferCovers derives coverage from history: the code files changed in the
// commits that touched the spec, limited to files that still exist.
func (g *gitRepo) inferCovers(s Spec) []string {
	rel := g.rel(s.Path)
	var out []string
	seen := map[string]bool{}
	for i := len(g.commits) - 1; i >= 0; i-- { // oldest first, for a stable order
		c := g.commits[i]
		if !containsStr(c.Files, rel) {
			continue
		}
		for _, f := range c.Files {
			if seen[f] || !isCodeFile(f) {
				continue
			}
			seen[f] = true
			if _, err := os.Stat(filepath.Join(g.root, filepath.FromSlash(f))); err == nil {
				out = append(out, f)
			}
		}
	}
	return out
}

// treeSnap fingerprints the working tree so spec apply can tell which files
// the agent changed: HEAD plus a content hash of every dirty file.
type treeSnap struct {
	head  string
	files map[string]string
}

func snapshotTree(root string) *treeSnap {
	g := loadGit(root)
	if g == nil {
		return nil
	}
	s := &treeSnap{head: g.head, files: map[string]string{}}
	for f := range g.dirty {
		s.files[f] = fileHash(filepath.Join(root, filepath.FromSlash(f)))
	}
	return s
}

func fileHash(p string) string {
	data, err := os.ReadFile(p)
	if err != nil {
		return "missing"
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// changedSince lists files that differ between before and now: dirty files
// whose content changed, plus anything in commits the agent made.
func changedSince(root string, before *treeSnap) []string {
	after := snapshotTree(root)
	if before == nil || after == nil {
		return nil
	}
	set := map[string]bool{}
	for f, h := range after.files {
		if before.files[f] != h {
			set[f] = true
		}
	}
	for f := range before.files {
		if _, ok := after.files[f]; !ok {
			set[f] = true // was dirty, now clean: committed or reverted
		}
	}
	if before.head != "" && after.head != before.head {
		if out, err := runGit(root, "diff", "--name-only", "--relative", before.head, after.head); err == nil {
			addLines(set, out)
		}
	}
	var out []string
	for f := range set {
		out = append(out, f)
	}
	return out
}
