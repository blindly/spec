package main

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Drift detection (FEAT-012): a system spec "may be stale" when files it
// covers changed after the spec was last updated or verified.

type gitCommit struct {
	SHA, Date, Subject string
	Files              []string // paths relative to the store root
}

// gitRepo is a snapshot of the history and working tree under a store root.
// Loading it runs a fixed handful of git commands, however many specs or
// commits there are.
type gitRepo struct {
	root    string
	head    string
	commits []gitCommit     // newest first; only files under specs/ or covered by a spec
	dirty   map[string]bool // uncommitted changes (tracked or untracked), relative to root
	// specChange["<sha>:<spec path>"] is true when that commit changed the
	// spec's content, not just its covers: line. dirtySpecChange is the same
	// for uncommitted edits.
	specChange      map[string]bool
	dirtySpecChange map[string]bool
}

// runGit runs git in root. With SPEC_TRACE set, each call's duration is
// printed to stderr, to diagnose slow git on a given machine.
func runGit(root string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = root
	start := time.Now()
	out, err := cmd.Output()
	if os.Getenv("SPEC_TRACE") != "" {
		fmt.Fprintf(os.Stderr, "[git %4dms] %s\n", time.Since(start).Milliseconds(), strings.Join(args, " "))
	}
	return out, err
}

// gitDirtyState returns HEAD and the uncommitted files under root; ok is
// false when root isn't in a git work tree (or git isn't installed).
func gitDirtyState(root string) (head string, dirty map[string]bool, ok bool) {
	if out, err := runGit(root, "rev-parse", "--is-inside-work-tree"); err != nil || strings.TrimSpace(string(out)) != "true" {
		return "", nil, false
	}
	dirty = map[string]bool{}
	if out, err := runGit(root, "rev-parse", "HEAD"); err == nil {
		head = strings.TrimSpace(string(out))
	}
	if head != "" {
		if out, err := runGit(root, "diff", "--name-only", "--relative", "HEAD"); err == nil {
			addLines(dirty, out)
		}
	}
	if out, err := runGit(root, "ls-files", "--others", "--exclude-standard"); err == nil {
		addLines(dirty, out)
	}
	return head, dirty, true
}

// coverPathspec turns a covers: entry into a git pathspec.
func coverPathspec(c string) string {
	if strings.ContainsAny(c, "*?[") && !strings.HasSuffix(c, "/") {
		return ":(glob)" + c
	}
	return c
}

// loadGit reads what drift and the system view need: history limited to
// specs/ and the files specs cover, plus the working tree. Reading the whole
// repo's history instead was ~90% of a /system page load.
func loadGit(root string) *gitRepo {
	paths := []string{specDir}
	seen := map[string]bool{}
	for _, s := range (Store{Root: root}).Specs() {
		for _, c := range s.Field("covers") {
			if !seen[c] {
				seen[c] = true
				paths = append(paths, coverPathspec(c))
			}
		}
	}
	return loadGitPaths(root, paths, false)
}

// loadGitFull lists commits that touched specs/ with every file each one
// changed — what spec covers --infer needs to discover code files.
func loadGitFull(root string) *gitRepo {
	return loadGitPaths(root, []string{specDir}, true)
}

func loadGitPaths(root string, paths []string, fullDiff bool) *gitRepo {
	head, dirty, ok := gitDirtyState(root)
	if !ok {
		return nil
	}
	g := &gitRepo{root: root, head: head, dirty: dirty, specChange: map[string]bool{}, dirtySpecChange: map[string]bool{}}
	if head == "" {
		return g
	}
	args := []string{"log", "--format=@@@%H%x09%cs%x09%s", "--name-only", "--relative"}
	if fullDiff {
		args = append(args, "--full-diff")
	}
	out, err := runGit(root, append(append(args, "--"), paths...)...)
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
	// one patch log over specs/ tells, for every commit, whether it changed
	// a spec's content or only its covers: line
	if out, err := runGit(root, "log", "-p", "--format=@@@%H", "--relative", "--", specDir); err == nil {
		for _, block := range strings.Split(string(out), "@@@")[1:] {
			sha, patch, _ := strings.Cut(block, "\n")
			for f, changed := range specPatchChanges(patch) {
				g.specChange[strings.TrimSpace(sha)+":"+f] = changed
			}
		}
	}
	if out, err := runGit(root, "diff", "--relative", "HEAD", "--", specDir); err == nil {
		g.dirtySpecChange = specPatchChanges(string(out))
	}
	return g
}

// specPatchChanges reads a unified diff and reports, per file, whether any
// changed line is something other than a covers: line.
func specPatchChanges(patch string) map[string]bool {
	out := map[string]bool{}
	file := ""
	for _, line := range strings.Split(patch, "\n") {
		switch {
		case strings.HasPrefix(line, "diff --git "):
			file = ""
		case strings.HasPrefix(line, "--- a/") && file == "":
			file = line[len("--- a/"):]
			out[file] = out[file] || false
		case strings.HasPrefix(line, "+++ b/"):
			file = line[len("+++ b/"):]
			out[file] = out[file] || false
		case strings.HasPrefix(line, "+++ ") || strings.HasPrefix(line, "--- "):
		case file != "" && (strings.HasPrefix(line, "+") || strings.HasPrefix(line, "-")):
			if !strings.HasPrefix(strings.TrimSpace(line[1:]), "covers:") {
				out[file] = true
			}
		}
	}
	return out
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

// contentChanged reports whether commit sha changed the spec's content, not
// just its covers: line (covers: is bookkeeping about the spec, not a change
// to what it says). Unknown commits count as content changes.
func (g *gitRepo) contentChanged(sha, rel string) bool {
	changed, ok := g.specChange[sha+":"+rel]
	return !ok || changed
}

// drift returns why s may be stale, or nil if it's in sync (or can't tell:
// no coverage, never committed, or its content is being edited).
func (g *gitRepo) drift(s Spec) *driftInfo {
	covers := s.Field("covers")
	rel := g.rel(s.Path)
	if len(covers) == 0 {
		return nil
	}
	if g.dirty[rel] {
		// untracked (new) spec, or uncommitted edits beyond covers: → being edited
		if changed, tracked := g.dirtySpecChange[rel]; !tracked || changed {
			return nil
		}
	}
	cp := -1
	verified := metaGet(s.Meta, "verified", "")
	for i, c := range g.commits {
		if verified != "" && strings.HasPrefix(c.SHA, verified) {
			cp = i
			break
		}
		if containsStr(c.Files, rel) && g.contentChanged(c.SHA, rel) {
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

// gitCache keeps a loaded gitRepo between web requests and reloads it only
// when the repo changes: a new HEAD, a different set of uncommitted files,
// or an edit to one of them.
type gitCache struct {
	mu   sync.Mutex
	key  string
	repo *gitRepo
}

func (c *gitCache) get(root string) *gitRepo {
	key := gitStateKey(root)
	c.mu.Lock()
	defer c.mu.Unlock()
	if key != "" && key == c.key {
		return c.repo
	}
	c.repo, c.key = loadGit(root), key
	return c.repo
}

// gitStateKey fingerprints HEAD plus the working tree (git status, and the
// size and mtime of each listed file). "" means "don't cache" (no git, or no
// commits yet).
func gitStateKey(root string) string {
	out, err := runGit(root, "rev-parse", "HEAD", "--show-toplevel")
	if err != nil {
		return ""
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(lines) < 2 {
		return ""
	}
	status, err := runGit(root, "status", "--porcelain=v1", "-uall")
	if err != nil {
		return ""
	}
	h := sha256.New()
	h.Write(out)
	h.Write(status)
	for _, l := range strings.Split(string(status), "\n") {
		if len(l) < 4 {
			continue
		}
		p := l[3:]
		if i := strings.Index(p, " -> "); i >= 0 {
			p = p[i+4:]
		}
		if fi, err := os.Stat(filepath.Join(lines[1], strings.Trim(p, `"`))); err == nil {
			fmt.Fprintf(h, "%s %d %d\n", p, fi.Size(), fi.ModTime().UnixNano())
		}
	}
	return hex.EncodeToString(h.Sum(nil))
}

// treeSnap fingerprints the working tree so spec apply can tell which files
// the agent changed: HEAD plus a content hash of every dirty file.
type treeSnap struct {
	head  string
	files map[string]string
}

func snapshotTree(root string) *treeSnap {
	head, dirty, ok := gitDirtyState(root) // working tree only; no history needed
	if !ok {
		return nil
	}
	s := &treeSnap{head: head, files: map[string]string{}}
	for f := range dirty {
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
