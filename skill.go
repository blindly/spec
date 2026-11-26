package main

import (
	"bytes"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

// Skills live in three layers:
//   - library:   <global config dir>/skills/<name>/ — shared, opt-in via skills:
//   - repo:      <root>/.spec/skills/<name>/ — repo-owned (local + forks)
//   - installed: <root>/<target>/<name>/ per resolved target dir — generated
//
// Provenance is derived from path membership and byte-compare; there is no
// manifest. The embedded specSkill is the built-in library entry for the
// name "spec", which is implicitly enabled in every spec-managed repo.

// agentSkillDirs maps agent names to their repo-level skill dir; agents not
// listed have no known skill-dir convention.
var agentSkillDirs = map[string]string{
	"claude": ".claude/skills",
	"devin":  ".devin/skills",
}

// agentGlobalSkillDirs maps agent names to their user-level skill dir
// relative to the home directory.
var agentGlobalSkillDirs = map[string]string{
	"claude": ".claude/skills",
	"devin":  filepath.Join(".config", "devin", "skills"),
}

var defaultSkillTargets = []string{".devin/skills", ".claude/skills"}

func skillLibraryDir() string {
	p := globalConfigPath()
	if p == "" {
		return ""
	}
	return filepath.Join(filepath.Dir(p), "skills")
}

func repoSkillDir(root, name string) string {
	return filepath.Join(root, configDir, "skills", name)
}

func librarySkillDir(name string) string {
	return filepath.Join(skillLibraryDir(), name)
}

func isDir(p string) bool {
	st, err := os.Stat(p)
	return err == nil && st.IsDir()
}

func skillTargets(cfg Config) []string {
	if len(cfg.SkillTargets) > 0 {
		return cfg.SkillTargets
	}
	if d, ok := agentSkillDirs[cfg.Agent]; ok {
		return []string{d}
	}
	return defaultSkillTargets
}

// skillFiles returns a skill's contents keyed by slash-relative path.
// dir == "" reads the embedded built-in spec skill. A missing dir is
// (nil, nil), not an error.
func skillFiles(dir string) (map[string][]byte, error) {
	if dir == "" {
		return map[string][]byte{"SKILL.md": []byte(specSkill)}, nil
	}
	files := map[string][]byte{}
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(dir, p)
		if err != nil {
			return err
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		files[filepath.ToSlash(rel)] = data
		return nil
	})
	if os.IsNotExist(err) {
		return nil, nil
	}
	return files, err
}

func skillFilesEqual(a, b map[string][]byte) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if !bytes.Equal(b[k], v) {
			return false
		}
	}
	return true
}

// writeSkillFiles replaces dir's contents with files.
func writeSkillFiles(dir string, files map[string][]byte) error {
	if err := os.RemoveAll(dir); err != nil {
		return err
	}
	for rel, data := range files {
		p := filepath.Join(dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			return err
		}
		if err := writeFileAtomic(p, data, 0o644); err != nil {
			return err
		}
	}
	return nil
}

// skillLinkTarget returns the symlink target for installing src at dst,
// or false when the kind must be copied (built-in — embedded, no source
// dir). repo skills link relatively when the install target is inside
// the repo (a repo move doesn't break it); targets outside root get an
// absolute link — a ../ chain to an external dir breaks if either side
// moves. library skills link absolutely.
func skillLinkTarget(root, dst string, src skillSource) (string, bool) {
	if src.dir == "" {
		return "", false
	}
	absSrc, err := filepath.Abs(src.dir)
	if err != nil {
		return "", false
	}
	if src.kind != "repo" {
		return absSrc, true
	}
	absDstDir, err := filepath.Abs(filepath.Dir(dst))
	if err != nil {
		return "", false
	}
	absRoot, err := filepath.Abs(root)
	if err != nil || !withinRoot(absRoot, absDstDir) {
		return absSrc, true
	}
	rel, err := filepath.Rel(absDstDir, absSrc)
	if err != nil {
		return "", false
	}
	return rel, true
}

// skillInstallStatus reports the state at dst for the resolved source:
// "current" (right link, or matching copy for kinds that can't link),
// "missing", "relink" (link to another source), "copied" (real dir
// matching the source — upgradeable to a link), or "stale" (differs).
func skillInstallStatus(root, dst string, src skillSource, srcFiles map[string][]byte) string {
	target, linkable := skillLinkTarget(root, dst, src)
	li, err := os.Lstat(dst)
	if err != nil {
		return "missing"
	}
	if linkable && li.Mode()&os.ModeSymlink != 0 {
		if t, _ := os.Readlink(dst); t == target {
			return "current"
		}
		return "relink"
	}
	installed, err := skillFiles(dst)
	if err != nil || installed == nil {
		return "stale"
	}
	if skillFilesEqual(installed, srcFiles) {
		if linkable {
			return "copied"
		}
		return "current"
	}
	return "stale"
}

// installSkill materializes the install at dst: a symlink to the source
// dir when the kind links, else a file copy. Link failures (Windows
// privileges, cross-device) fall back to copy. Returns "linked" or
// "copied" — what was actually written.
func installSkill(root, dst string, src skillSource, srcFiles map[string][]byte) (string, error) {
	if target, ok := skillLinkTarget(root, dst, src); ok {
		if err := os.RemoveAll(dst); err != nil {
			return "", err
		}
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return "", err
		}
		if err := os.Symlink(target, dst); err == nil {
			return "linked", nil
		}
	}
	return "copied", writeSkillFiles(dst, srcFiles)
}

// generatedLink reports whether dst is a symlink pointing inside a skill
// source root (.spec/skills/ or the library) — one spec created and may
// remove. Links elsewhere are foreign and left alone.
func generatedLink(st Store, dst string) bool {
	t, err := os.Readlink(dst)
	if err != nil {
		return false
	}
	if !filepath.IsAbs(t) {
		t = filepath.Join(filepath.Dir(dst), t)
	}
	t = filepath.Clean(t)
	for _, root := range []string{repoSkillDir(st.Root, ""), skillLibraryDir()} {
		if root == "" {
			continue
		}
		if strings.HasPrefix(t, filepath.Clean(root)+string(filepath.Separator)) {
			return true
		}
	}
	return false
}

func skillDirNames(dir string) []string {
	entries, _ := os.ReadDir(dir)
	var names []string
	for _, e := range entries {
		if e.IsDir() {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	return names
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

// skillSource is the resolved source for a skill name; dir == "" means the
// embedded built-in.
type skillSource struct {
	dir  string
	kind string // "repo" | "library" | "built-in"
}

// resolveSkill applies the install-time chain: .spec/skills/<name> →
// skills: entry → library → built-in (spec only). A name in skills: whose
// library entry is gone resolves to no source (orphaned).
func resolveSkill(st Store, cfg Config, name string) skillSource {
	if d := repoSkillDir(st.Root, name); isDir(d) {
		return skillSource{d, "repo"}
	}
	if contains(cfg.Skills, name) {
		if d := librarySkillDir(name); isDir(d) {
			return skillSource{d, "library"}
		}
		return skillSource{}
	}
	if name == "spec" {
		return skillSource{"", "built-in"}
	}
	return skillSource{}
}

// readableSkill resolves for view/edit/refine: .spec/skills → library →
// built-in, ignoring skills: membership (a library skill needn't be
// enabled to be read or refined).
func readableSkill(st Store, name string) skillSource {
	if d := repoSkillDir(st.Root, name); isDir(d) {
		return skillSource{d, "repo"}
	}
	if d := librarySkillDir(name); isDir(d) {
		return skillSource{d, "library"}
	}
	if name == "spec" {
		return skillSource{"", "built-in"}
	}
	return skillSource{}
}

// skillNames resolves every skill name known to the repo: .spec/skills
// entries, skills: entries, and the implicit built-in spec.
func skillNames(st Store, cfg Config) []string {
	set := map[string]bool{"spec": true}
	for _, n := range skillDirNames(filepath.Join(st.Root, configDir, "skills")) {
		set[n] = true
	}
	for _, n := range cfg.Skills {
		set[n] = true
	}
	var names []string
	for n := range set {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// bestSkillFiles returns the content a repo-owned copy of name should start
// from: an installed copy that diverges from the resolved source (preserving
// hand edits), else the resolved source itself.
func bestSkillFiles(st Store, cfg Config, name string) (map[string][]byte, error) {
	src := resolveSkill(st, cfg, name)
	if src.dir == "" && src.kind == "" {
		// orphaned or unknown: fall back to an installed copy (resolve a
		// leftover link first — WalkDir won't descend a symlinked root)
		for _, t := range skillTargets(cfg) {
			p := filepath.Join(st.Root, t, name)
			if li, err := os.Lstat(p); err == nil && li.Mode()&os.ModeSymlink != 0 {
				if tg, err := filepath.EvalSymlinks(p); err == nil {
					p = tg
				}
			}
			if files, _ := skillFiles(p); files != nil {
				return files, nil
			}
		}
		return nil, fmt.Errorf("no skill %q", name)
	}
	srcFiles, err := skillFiles(src.dir)
	if err != nil {
		return nil, err
	}
	for _, t := range skillTargets(cfg) {
		dst := filepath.Join(st.Root, t, name)
		if li, err := os.Lstat(dst); err == nil && li.Mode()&os.ModeSymlink != 0 {
			continue // a link can't diverge from its source
		}
		installed, err := skillFiles(dst)
		if err != nil {
			return nil, err
		}
		if installed != nil && !skillFilesEqual(installed, srcFiles) {
			return installed, nil
		}
	}
	return srcFiles, nil
}

// writeSkillsConfig replaces the skills: list in the config file at path;
// an empty list drops the key.
func writeSkillsConfig(path string, skills []string) error {
	v := ""
	if len(skills) > 0 {
		v = "[" + strings.Join(skills, ", ") + "]"
	}
	return setConfigKeyIn(path, "skills", v)
}

func stringSet(list []string) map[string]bool {
	set := make(map[string]bool, len(list))
	for _, s := range list {
		set[s] = true
	}
	return set
}

func withoutSkill(skills []string, name string) []string {
	var out []string
	for _, s := range skills {
		if s != name {
			out = append(out, s)
		}
	}
	return out
}

// skillDesc returns the skill's one-line description from its SKILL.md
// frontmatter; dir == "" reads the embedded built-in.
func skillDesc(dir string) string {
	var data string
	if dir == "" {
		data = specSkill
	} else {
		b, err := os.ReadFile(filepath.Join(dir, "SKILL.md"))
		if err != nil {
			return ""
		}
		data = string(b)
	}
	meta, _ := parseFrontmatter(data)
	d, _, _ := strings.Cut(metaGet(meta, "description", ""), "\n")
	const max = 80
	if len(d) > max {
		d = d[:max-1] + "…"
	}
	return d
}

// lintSkillDir reports how a skill dir falls short of the SKILL.md
// contract: missing file, frontmatter name present but not matching
// the directory, missing description. name: is optional — the dir is
// the identity; a set-but-different name is drift. Notes, never
// hard failures.
func lintSkillDir(name, dir string) []string {
	data, err := os.ReadFile(filepath.Join(dir, "SKILL.md"))
	if err != nil {
		return []string{"no SKILL.md"}
	}
	meta, _ := parseFrontmatter(string(data))
	var problems []string
	if n := metaGet(meta, "name", ""); n != "" && n != name {
		problems = append(problems, fmt.Sprintf("SKILL.md name %q != dir %q", n, name))
	}
	if metaGet(meta, "description", "") == "" {
		problems = append(problems, "SKILL.md has no description:")
	}
	return problems
}

// repoRegistryPath is the advisory list of known spec-managed repos —
// one absolute root per line, under the global config dir.
func repoRegistryPath() string {
	p := globalConfigPath()
	if p == "" {
		return ""
	}
	return filepath.Join(filepath.Dir(p), "repos")
}

// registerRepo records root in the registry; best-effort and idempotent.
// Entries whose repo no longer holds a .spec dir are pruned on write.
func registerRepo(root string) {
	p := repoRegistryPath()
	if p == "" {
		return
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return
	}
	raw, _ := os.ReadFile(p)
	var lines []string
	for _, l := range strings.Fields(string(raw)) {
		if isDir(filepath.Join(l, configDir)) {
			lines = append(lines, l)
		}
	}
	if !contains(lines, abs) {
		lines = append(lines, abs)
	}
	content := strings.Join(lines, "\n") + "\n"
	if string(raw) == content {
		return
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return
	}
	os.WriteFile(p, []byte(content), 0o644)
}

// registeredRepos returns the registry entries that still hold a spec
// dir — stale entries are silently pruned from the result.
func registeredRepos() []string {
	data, err := os.ReadFile(repoRegistryPath())
	if err != nil {
		return nil
	}
	var out []string
	for _, line := range strings.Fields(string(data)) {
		if isDir(filepath.Join(line, configDir)) {
			out = append(out, line)
		}
	}
	return out
}

// reposUsingSkill returns registered repo roots whose repo-level
// skills: enables name.
func reposUsingSkill(name string) []string {
	var out []string
	for _, r := range registeredRepos() {
		if contains(configFileSkills(repoConfigPath(r)), name) {
			out = append(out, r)
		}
	}
	return out
}

// ensureRepoSkill makes .spec/skills/<name> exist: implicit adopt for a
// managed skill (drops the skills: entry), implicit copy for a library-only
// or built-in skill. Returns the repo skill dir.
func ensureRepoSkill(st Store, cfg Config, name string) (string, error) {
	dst := repoSkillDir(st.Root, name)
	if isDir(dst) {
		return dst, nil
	}
	files, err := bestSkillFiles(st, cfg, name)
	if err != nil {
		return "", err
	}
	if err := writeSkillFiles(dst, files); err != nil {
		return "", err
	}
	repoSkills := configFileSkills(repoConfigPath(st.Root))
	if contains(repoSkills, name) {
		if err := writeSkillsConfig(repoConfigPath(st.Root), withoutSkill(repoSkills, name)); err != nil {
			return "", err
		}
	}
	return dst, nil
}

// colWidth returns the longest name's length — the padding width for a
// column so entries with names longer than a fixed gutter stay aligned.
func colWidth(names ...string) int {
	w := 0
	for _, n := range names {
		if len(n) > w {
			w = len(n)
		}
	}
	return w
}

// skillStatusLine describes one resolved skill across the target dirs.
// scope is "repo" or "global" for skills: entries — which level enabled them.
func skillStatusLine(st Store, cfg Config, targets []string, name, scope string, width int) (string, bool) {
	src := resolveSkill(st, cfg, name)
	label := src.kind
	if src.kind == "repo" && contains(cfg.Skills, name) {
		label = "repo (shadowed)"
	}
	if src.kind == "library" && scope != "" {
		label = "library(" + scope + ")"
	}
	if src.kind == "" {
		label = "orphaned"
	}
	ok := src.kind != ""
	var srcFiles map[string][]byte
	if src.kind != "" {
		srcFiles, _ = skillFiles(src.dir)
	}
	// One status: "current" when every target matches the source — else only
	// the non-current targets are named (target:status). Orphans name where
	// the leftover install still sits.
	var status string
	if src.kind == "" {
		ok = false
		var present []string
		for _, t := range targets {
			if _, err := os.Lstat(filepath.Join(st.Root, t, name)); err == nil {
				present = append(present, t)
			}
		}
		status = "orphaned"
		if len(present) > 0 {
			status += " in " + strings.Join(present, ", ")
		}
	} else {
		var bad []string
		for _, t := range targets {
			dst := filepath.Join(st.Root, t, name)
			if s := skillInstallStatus(st.Root, dst, src, srcFiles); s != "current" {
				ok = false
				bad = append(bad, t+":"+s)
			}
		}
		status = "current"
		if len(bad) > 0 {
			status = strings.Join(bad, "  ")
		}
	}
	line := fmt.Sprintf("%-*s %-16s %s", width, name, label, status)
	if src.kind != "" {
		if desc := skillDesc(src.dir); desc != "" {
			line += "  " + desc
		}
	}
	return line, ok
}

// checkSkillInstalls audits the resolved skill set's installs for
// spec check: a stale or missing install means the harness reads
// outdated (or no) skill guidance. Returns the problem count;
// equal-content copies awaiting link upgrade only warn.
func checkSkillInstalls(st Store, out io.Writer) int {
	cfg := loadConfig(st.Root)
	targets := skillTargets(cfg)
	problems := 0
	for _, name := range skillNames(st, cfg) {
		src := resolveSkill(st, cfg, name)
		if src.kind == "" {
			problems++
			fmt.Fprintf(out, "FAIL  skill %s in skills: has no resolvable source\n", name)
			continue
		}
		srcFiles, _ := skillFiles(src.dir)
		if src.dir != "" {
			for _, p := range lintSkillDir(name, src.dir) {
				fmt.Fprintf(out, "WARN  %s: %s\n", src.dir, p)
			}
		}
		for _, t := range targets {
			dst := filepath.Join(st.Root, t, name)
			switch status := skillInstallStatus(st.Root, dst, src, srcFiles); status {
			case "current":
			case "copied":
				fmt.Fprintf(out, "WARN  %s installed as copy — 'spec skill sync' upgrades it to a link\n", dst)
			case "stale":
				problems++
				fix := "'spec skill sync --force' to overwrite"
				if src.kind == "built-in" {
					fix = "'spec skill sync' repairs"
				}
				fmt.Fprintf(out, "FAIL  %s stale (differs from source; %s)\n", dst, fix)
			default:
				problems++
				fmt.Fprintf(out, "FAIL  %s %s — run 'spec skill sync'\n", dst, status)
			}
		}
	}
	return problems
}

// syncSkills materializes the resolved skill set into each target dir.
// force overwrites installed copies that differ from their source; without
// it they are reported stale and left alone. Returns the number of
// non-current paths.
func syncSkills(st Store, cfg Config, targets []string, force, quiet bool, out io.Writer) (int, error) {
	// generated skill dirs are install output — ensure .gitignore covers
	// them (and secrets facts) on every repo sync, not only at spec init;
	// repo-free library commands skip this (no .gitignore context).
	// Same for the agent-pointer files harnesses read at session start.
	if isDir(filepath.Join(st.Root, configDir)) {
		registerRepo(st.Root)
		if err := ensureGitignore(st.Root, targets); err != nil {
			return 0, err
		}
		if err := ensureAgentPointers(st.Root); err != nil {
			return 0, err
		}
		ensureSpecHooks(st.Root, out)
	}
	bad := 0
	resolved := map[string]bool{}
	for _, name := range skillNames(st, cfg) {
		src := resolveSkill(st, cfg, name)
		resolved[name] = true
		if src.kind == "" {
			bad++
			fmt.Fprintf(out, "%-8s %s (in skills: but no library entry)\n", "orphaned", name)
			continue
		}
		srcFiles, err := skillFiles(src.dir)
		if err != nil {
			return bad, err
		}
		for _, t := range targets {
			dst := filepath.Join(st.Root, t, name)
			status := skillInstallStatus(st.Root, dst, src, srcFiles)
			switch {
			case status == "current":
			// a stale copy might be a user fork — but built-in installs
			// are generated output (embedded source, no dir to fork),
			// so staleness just means an older binary wrote it
			case status == "stale" && !force && src.kind != "built-in":
				bad++
				fmt.Fprintf(out, "%-8s %s (differs from source; sync --force to overwrite)\n", "stale", dst)
				continue
			default:
				what, err := installSkill(st.Root, dst, src, srcFiles)
				if err != nil {
					return bad, err
				}
				status = what
			}
			if !quiet {
				fmt.Fprintf(out, "%-8s %s\n", status, dst)
			}
		}
	}
	// generated dirs for names no longer resolved: links we created and
	// real dirs that still match the library are removed; anything else is
	// foreign and only reported
	for _, t := range targets {
		tdir := filepath.Join(st.Root, t)
		entries, _ := os.ReadDir(tdir)
		for _, e := range entries {
			if !(e.IsDir() || e.Type()&os.ModeSymlink != 0) || resolved[e.Name()] {
				continue
			}
			dst := filepath.Join(tdir, e.Name())
			removable := false
			if e.Type()&os.ModeSymlink != 0 {
				removable = generatedLink(st, dst)
			} else {
				installed, _ := skillFiles(dst)
				lib, _ := skillFiles(librarySkillDir(e.Name()))
				removable = lib != nil && skillFilesEqual(installed, lib)
			}
			if !removable {
				fmt.Fprintf(out, "%-8s %s\n", "foreign", dst)
				continue
			}
			if err := os.RemoveAll(dst); err != nil {
				return bad, err
			}
			fmt.Fprintf(out, "%-8s %s\n", "removed", dst)
		}
	}
	return bad, nil
}

func cmdSkill(out, errOut io.Writer, args []string) error {
	var sub, operand, operand2 string
	var global, force, library, local, noEdit, quiet bool
	var positional []string
	for _, a := range args {
		switch a {
		case "--global":
			global = true
		case "--force":
			force = true
		case "--library":
			library = true
		case "--local":
			local = true
		case "--no-edit":
			noEdit = true
		case "--quiet":
			quiet = true
		default:
			positional = append(positional, a)
		}
	}
	if len(positional) > 0 {
		sub = positional[0]
	}
	if len(positional) > 1 {
		operand = positional[1]
	}
	if len(positional) > 2 {
		if sub != "import" || len(positional) > 3 {
			return fmt.Errorf("usage: spec skill [check|sync|update|view|which|diff|new|edit|refine|use|unuse|adopt|push|import|rm <name>] [--global|--force|--library|--local]")
		}
		operand2 = positional[2]
		if !personaNamePattern.MatchString(operand2) {
			return fmt.Errorf("invalid skill name %q", operand2)
		}
	}

	if global {
		return cmdSkillGlobal(out, errOut, sub, operand, force, quiet)
	}

	st, err := mustStore()
	if err != nil {
		// library-level commands never touch repo state — run them
		// outside a spec-managed repo
		if sub == "" {
			return cmdSkillLibraryList(out)
		}
		repoFree := library && (sub == "import" || sub == "new") ||
			(sub == "edit" || sub == "refine") && !local ||
			sub == "view"
		if !repoFree {
			return err
		}
		st = Store{Root: "."}
	}
	cfg := loadConfig(st.Root)
	targets := skillTargets(cfg)

	switch sub {
	case "", "check":
		bad := 0
		repoSet := stringSet(configFileSkills(repoConfigPath(st.Root)))
		globalSet := stringSet(configFileSkills(globalConfigPath()))
		names := skillNames(st, cfg)
		avail := availableSkills(st, cfg)
		w := colWidth(append(append([]string{}, names...), avail...)...)
		fmt.Fprintln(out, "enabled")
		for _, name := range names {
			scope := ""
			switch {
			case repoSet[name]:
				scope = "repo"
			case globalSet[name]:
				scope = "global"
			}
			line, ok := skillStatusLine(st, cfg, targets, name, scope, w)
			fmt.Fprintln(out, "  "+line)
			if !ok {
				bad++
			}
		}
		if len(avail) > 0 {
			fmt.Fprintln(out, "\nlibrary (not enabled — 'spec skill use <name>' to enable)")
			for _, n := range avail {
				fmt.Fprintf(out, "  %-*s %s\n", w, n, skillDesc(librarySkillDir(n)))
			}
		}
		fmt.Fprintln(out, "\ninstall state vs source — current: matches · copied: matching copy, upgradeable to a link ·")
		fmt.Fprintln(out, "relink: link to the wrong source · stale: differs · missing: absent — 'spec skill sync' repairs")
		// foreign dirs: installed by other means or left behind — sync
		// removes generated leftovers, the rest is just reported
		resolvedSet := stringSet(names)
		for _, t := range targets {
			entries, _ := os.ReadDir(filepath.Join(st.Root, t))
			for _, e := range entries {
				if !(e.IsDir() || e.Type()&os.ModeSymlink != 0) || resolvedSet[e.Name()] {
					continue
				}
				fmt.Fprintf(errOut, "note: foreign skill dir %s — 'spec skill sync' removes generated leftovers\n", filepath.Join(st.Root, t, e.Name()))
			}
		}
		linted := map[string]bool{}
		for _, name := range names {
			if src := resolveSkill(st, cfg, name); src.dir != "" && !linted[src.dir] {
				linted[src.dir] = true
				for _, p := range lintSkillDir(name, src.dir) {
					fmt.Fprintf(errOut, "note: skill %s: %s\n", src.dir, p)
				}
			}
		}
		for _, n := range skillDirNames(skillLibraryDir()) {
			if d := librarySkillDir(n); !linted[d] {
				for _, p := range lintSkillDir(n, d) {
					fmt.Fprintf(errOut, "note: skill %s: %s\n", d, p)
				}
			}
		}
		if sub == "" {
			return nil
		}
		if bad > 0 {
			return fmt.Errorf("%d skill(s) not current — run 'spec skill sync'", bad)
		}
		return nil
	case "sync", "update":
		bad, err := syncSkills(st, cfg, targets, force || sub == "update", quiet, out)
		if err != nil {
			return err
		}
		if bad > 0 {
			return fmt.Errorf("%d skill path(s) not synced", bad)
		}
		return nil
	}

	if operand == "" {
		if sub == "import" && !library {
			return cmdSkillImportAll(st, cfg, targets, out, errOut)
		}
		return fmt.Errorf("usage: spec skill %s <name>", sub)
	}
	if sub != "import" && !personaNamePattern.MatchString(operand) {
		return fmt.Errorf("usage: spec skill %s <name>", sub)
	}
	name := operand

	switch sub {
	case "which":
		return cmdSkillWhich(st, cfg, name, out)
	case "diff":
		return cmdSkillDiff(st, cfg, targets, name, library, out)
	case "view":
		src := readableSkill(st, name)
		if src.kind == "" {
			return fmt.Errorf("no skill %q", name)
		}
		data, err := os.ReadFile(filepath.Join(src.dir, "SKILL.md"))
		if src.kind == "built-in" {
			data = []byte(specSkill)
		} else if err != nil {
			return fmt.Errorf("skill %q has no SKILL.md", name)
		}
		if _, err := out.Write(data); err != nil {
			return err
		}
		fmt.Fprintf(errOut, "(from %s skill)\n", src.kind)
		return nil
	case "new":
		return cmdSkillNew(st, cfg, name, library, noEdit, out, errOut)
	case "edit":
		return cmdSkillEdit(st, cfg, name, library, local, out, errOut)
	case "refine":
		return cmdSkillRefine(st, cfg, name, local, out, errOut)
	case "use":
		return cmdSkillUse(st, cfg, targets, name, out, errOut)
	case "unuse":
		return cmdSkillUnuse(st, cfg, targets, name, force, out, errOut)
	case "adopt":
		return cmdSkillAdopt(st, cfg, targets, name, out, errOut)
	case "rm":
		return cmdSkillRm(st, cfg, targets, name, library, force, out, errOut)
	case "push":
		return cmdSkillPush(st, name, force, out, errOut)
	case "import":
		return cmdSkillImport(st, cfg, targets, name, operand2, library, out, errOut)
	}
	return fmt.Errorf("usage: spec skill [check|sync|update|view|which|diff|new|edit|refine|use|unuse|adopt|push|import|rm <name>] [--global|--force|--library|--local]")
}

// cmdSkillWhich prints a name's install-time resolution: the source
// kind and directory, "orphaned" for a skills: entry with no source,
// "not enabled" for a readable skill outside the resolved set.
func cmdSkillWhich(st Store, cfg Config, name string, out io.Writer) error {
	src := resolveSkill(st, cfg, name)
	if src.kind != "" {
		dir := src.dir
		if dir == "" {
			dir = "(embedded)"
		}
		fmt.Fprintf(out, "%s: %s %s\n", name, src.kind, dir)
		reportSkillUsers(st, name, out)
		return nil
	}
	if contains(cfg.Skills, name) {
		fmt.Fprintf(out, "%s: orphaned (in skills: but no library entry)\n", name)
		reportSkillUsers(st, name, out)
		return nil
	}
	if r := readableSkill(st, name); r.kind != "" {
		fmt.Fprintf(out, "%s: %s (not enabled) %s\n", name, r.kind, r.dir)
		reportSkillUsers(st, name, out)
		return nil
	}
	return fmt.Errorf("no skill %q", name)
}

// reportSkillUsers appends the "enabled in:" line to `which` output:
// this repo's skills:, the global skills:, and registered repos.
func reportSkillUsers(st Store, name string, out io.Writer) {
	var users []string
	cur, _ := filepath.Abs(st.Root)
	if contains(configFileSkills(repoConfigPath(st.Root)), name) {
		users = append(users, "this repo")
	}
	if contains(configFileSkills(globalConfigPath()), name) {
		users = append(users, "global skills:")
	}
	for _, r := range reposUsingSkill(name) {
		if r != cur {
			users = append(users, r)
		}
	}
	if len(users) > 0 {
		fmt.Fprintf(out, "  enabled in: %s\n", strings.Join(users, ", "))
	}
}

// cmdSkillLibraryList is the repo-free view of the shared library:
// built-in, globally enabled, and available entries.
func cmdSkillLibraryList(out io.Writer) error {
	enabled := stringSet(configFileSkills(globalConfigPath()))
	names := skillDirNames(librarySkillDir(""))
	w := colWidth(append([]string{"spec"}, names...)...)
	fmt.Fprintf(out, "%-*s %-16s %s\n", w, "spec", "built-in", skillDesc(""))
	for _, n := range names {
		label := "available"
		if enabled[n] {
			label = "library(global)"
		}
		fmt.Fprintf(out, "%-*s %-16s %s\n", w, n, label, skillDesc(librarySkillDir(n)))
	}
	fmt.Fprintln(out, "\nenable: 'spec skill use <name>' in a repo, 'spec skill use <name> --global' everywhere")
	return nil
}

// availableSkills lists library skills not enabled or present in the repo.
func availableSkills(st Store, cfg Config) []string {
	known := map[string]bool{}
	for _, n := range skillNames(st, cfg) {
		known[n] = true
	}
	var out []string
	for _, n := range skillDirNames(skillLibraryDir()) {
		if !known[n] {
			out = append(out, n)
		}
	}
	return out
}

const skillScaffold = `---
name: %s
description: 
---

# %s
`

func cmdSkillNew(st Store, cfg Config, name string, library, noEdit bool, out, errOut io.Writer) error {
	dir := repoSkillDir(st.Root, name)
	if library {
		dir = librarySkillDir(name)
	}
	if isDir(dir) {
		return fmt.Errorf("skill %q already exists at %s", name, dir)
	}
	path := filepath.Join(dir, "SKILL.md")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	if err := writeFileAtomic(path, []byte(fmt.Sprintf(skillScaffold, name, name)), 0o644); err != nil {
		return err
	}
	if noEdit || !stdinIsTerminal() {
		fmt.Fprintf(out, "Created %s\n", path)
	} else {
		fmt.Fprintf(out, "Opening %s with %s\n", path, cfg.editor())
		cmd := exec.Command(cfg.editor(), path)
		cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
		if err := cmd.Run(); err != nil {
			return err
		}
	}
	if library {
		return nil
	}
	_, err := syncSkills(st, cfg, skillTargets(cfg), false, true, errOut)
	return err
}

func cmdSkillEdit(st Store, cfg Config, name string, library, local bool, out, errOut io.Writer) error {
	var dir string
	switch {
	case library:
		dir = librarySkillDir(name)
		if !isDir(dir) {
			return fmt.Errorf("no library skill %q", name)
		}
	case local:
		var err error
		if dir, err = ensureRepoSkill(st, cfg, name); err != nil {
			return err
		}
	default:
		src := readableSkill(st, name)
		if src.kind == "" {
			return fmt.Errorf("no skill %q", name)
		}
		if src.kind == "built-in" {
			return fmt.Errorf("skill %q is built in — use spec skill edit %s --local to fork it", name, name)
		}
		dir = src.dir
	}
	path := filepath.Join(dir, "SKILL.md")
	fmt.Fprintf(out, "Opening %s with %s\n", path, cfg.editor())
	cmd := exec.Command(cfg.editor(), path)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		return err
	}
	if filepath.Dir(dir) != filepath.Join(st.Root, configDir, "skills") {
		return nil // library file — nothing installed to refresh
	}
	_, err := syncSkills(st, cfg, skillTargets(cfg), false, true, errOut)
	return err
}

func cmdSkillRefine(st Store, cfg Config, name string, local bool, out, errOut io.Writer) error {
	ag, err := resolveAgent(cfg)
	if err != nil {
		return err
	}
	if ag == nil {
		return fmt.Errorf("skill refine requires an LLM agent (none configured; set agent/agent_cmd in .spec/config.yml)")
	}
	src := readableSkill(st, name)
	if local {
		dir, err := ensureRepoSkill(st, cfg, name)
		if err != nil {
			return err
		}
		src = skillSource{dir, "repo"}
	} else if src.kind == "" {
		return fmt.Errorf("no skill %q", name)
	}
	files, err := skillFiles(src.dir)
	if err != nil {
		return err
	}
	current := string(files["SKILL.md"])
	var prompt string
	if src.kind == "repo" {
		prompt = fmt.Sprintf(`You are improving an agent skill file (SKILL.md) named %q for this repository. The file tells the agent how to perform a task or follow a convention here.

Improve the skill below, or draft one when it is empty. Keep it focused on this repository's conventions.

Your reply is written verbatim to the file, so output ONLY the file contents: a SKILL.md document starting with a YAML frontmatter block (--- ... ---) containing name: and description: fields, then a "# " heading. No commentary, no narration, no surrounding code fences.

Current skill:
%s

%s`, name, current, repoContext(st, 50))
	} else {
		// repo-blind: a shared skill must not absorb repo specifics
		prompt = fmt.Sprintf(`You are improving a reusable agent skill file (SKILL.md) named %q, shared across repositories. The file tells the agent how to perform a task or follow a convention.

Improve the skill below, or draft one when it is empty. It must stay repository-agnostic — no project names, paths, or repo-specific conventions.

Your reply is written verbatim to the file, so output ONLY the file contents: a SKILL.md document starting with a YAML frontmatter block (--- ... ---) containing name: and description: fields, then a "# " heading. No commentary, no narration, no surrounding code fences.

Current skill:
%s`, name, current)
	}
	fmt.Fprintf(out, "Refining skill %q via %s…\n", name, ag.name)
	output, err := ag.run(prompt, st.Root, cfg.AgentTimeout)
	if err != nil {
		return err
	}
	body := skillDocFromAgentOutput(output)
	if meta, _ := parseFrontmatter(body); metaGet(meta, "name", "") == "" || metaGet(meta, "description", "") == "" {
		dumpAgentOutput(errOut, output)
		return fmt.Errorf("agent returned no valid SKILL.md — skill left unchanged")
	}
	var path string
	if src.kind == "repo" {
		path = filepath.Join(src.dir, "SKILL.md")
	} else {
		path = filepath.Join(librarySkillDir(name), "SKILL.md")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	if err := writeFileAtomic(path, []byte(body+"\n"), 0o644); err != nil {
		return err
	}
	fmt.Fprintln(out, "Refined", path, "via", ag.name)
	if src.kind == "repo" {
		_, err := syncSkills(st, cfg, skillTargets(cfg), false, true, errOut)
		return err
	}
	return nil
}

func cmdSkillUse(st Store, cfg Config, targets []string, name string, out, errOut io.Writer) error {
	if name == "spec" {
		return fmt.Errorf("the spec skill is built in and always enabled")
	}
	repoSkills := configFileSkills(repoConfigPath(st.Root))
	if contains(repoSkills, name) {
		fmt.Fprintf(out, "Skill %q already enabled\n", name)
		return nil
	}
	if contains(configFileSkills(globalConfigPath()), name) {
		fmt.Fprintf(out, "Skill %q already enabled via global skills:\n", name)
		return nil
	}
	if !isDir(librarySkillDir(name)) {
		return fmt.Errorf("no library skill %q (see: spec skill)", name)
	}
	repoSkills = append(repoSkills, name)
	if err := writeSkillsConfig(repoConfigPath(st.Root), repoSkills); err != nil {
		return err
	}
	fmt.Fprintf(out, "Using library skill %q\n", name)
	if isDir(repoSkillDir(st.Root, name)) {
		fmt.Fprintln(errOut, "note: .spec/skills/"+name+" exists and takes precedence")
	}
	cfg.Skills = unionStrings(cfg.Skills, []string{name})
	_, err := syncSkills(st, cfg, targets, false, true, errOut)
	return err
}

func cmdSkillUnuse(st Store, cfg Config, targets []string, name string, force bool, out, errOut io.Writer) error {
	if name == "spec" {
		return fmt.Errorf("the spec skill is built in and always enabled")
	}
	repoSkills := configFileSkills(repoConfigPath(st.Root))
	if !contains(repoSkills, name) {
		if contains(configFileSkills(globalConfigPath()), name) {
			return fmt.Errorf("skill %q is enabled via global skills: — use spec skill unuse %s --global", name, name)
		}
		return fmt.Errorf("skill %q is not in skills:", name)
	}
	if contains(configFileSkills(globalConfigPath()), name) {
		// still enabled globally — the repo entry is redundant; drop it
		if err := writeSkillsConfig(repoConfigPath(st.Root), withoutSkill(repoSkills, name)); err != nil {
			return err
		}
		fmt.Fprintf(out, "Skill %q removed from repo skills: — still enabled via global skills:\n", name)
		return nil
	}
	// installed copies may only be removed when they still match the
	// source; a differing copy is a fork the user must keep or discard
	src := resolveSkill(st, cfg, name)
	srcFiles, _ := skillFiles(src.dir)
	for _, t := range targets {
		dst := filepath.Join(st.Root, t, name)
		li, err := os.Lstat(dst)
		if err != nil {
			continue
		}
		if li.Mode()&os.ModeSymlink != 0 {
			continue // links are removed by unlinking — nothing to protect
		}
		installed, err := skillFiles(dst)
		if err != nil {
			return err
		}
		if installed == nil {
			continue
		}
		if !force && (srcFiles == nil || !skillFilesEqual(installed, srcFiles)) {
			return fmt.Errorf("installed copy at %s differs — use spec skill adopt %s to keep it, or --force to discard", dst, name)
		}
	}
	for _, t := range targets {
		if err := os.RemoveAll(filepath.Join(st.Root, t, name)); err != nil {
			return err
		}
	}
	if err := writeSkillsConfig(repoConfigPath(st.Root), withoutSkill(repoSkills, name)); err != nil {
		return err
	}
	fmt.Fprintf(out, "Skill %q no longer managed\n", name)
	return nil
}

// cmdSkillRm removes a repo-owned skill and its generated installs.
// Installed copies are only removed while they still match the source —
// a differing copy is a fork and needs --force. --library removes the
// library entry instead (enabled repos then report it orphaned).
func cmdSkillRm(st Store, cfg Config, targets []string, name string, library, force bool, out, errOut io.Writer) error {
	if library {
		lib := librarySkillDir(name)
		if !isDir(lib) {
			return fmt.Errorf("no library skill %q", name)
		}
		if contains(configFileSkills(repoConfigPath(st.Root)), name) {
			fmt.Fprintf(errOut, "note: %s is enabled here — this repo will report it orphaned\n", name)
		}
		if contains(configFileSkills(globalConfigPath()), name) {
			fmt.Fprintf(errOut, "note: %s is in global skills: — every repo will report it orphaned\n", name)
		}
		cur, _ := filepath.Abs(st.Root)
		var others []string
		for _, r := range reposUsingSkill(name) {
			if r != cur {
				others = append(others, r)
			}
		}
		if len(others) > 0 {
			fmt.Fprintf(errOut, "note: %s is enabled in registered repo(s): %s — they will report it orphaned\n",
				name, strings.Join(others, ", "))
		}
		if err := os.RemoveAll(lib); err != nil {
			return err
		}
		fmt.Fprintf(out, "Removed library skill %q\n", name)
		return nil
	}
	dir := repoSkillDir(st.Root, name)
	if !isDir(dir) {
		if contains(cfg.Skills, name) {
			return fmt.Errorf("skill %q is managed — use spec skill unuse %s", name, name)
		}
		return fmt.Errorf("no repo skill %q (see: spec skill)", name)
	}
	srcFiles, _ := skillFiles(dir)
	for _, t := range targets {
		dst := filepath.Join(st.Root, t, name)
		li, err := os.Lstat(dst)
		if err != nil || li.Mode()&os.ModeSymlink != 0 {
			continue // absent, or a generated link — removal only unlinks it
		}
		installed, _ := skillFiles(dst)
		if installed != nil && !force && !skillFilesEqual(installed, srcFiles) {
			return fmt.Errorf("installed copy at %s differs — --force to remove it anyway", dst)
		}
	}
	for _, t := range targets {
		if err := os.RemoveAll(filepath.Join(st.Root, t, name)); err != nil {
			return err
		}
	}
	if err := os.RemoveAll(dir); err != nil {
		return err
	}
	if name == "spec" {
		fmt.Fprintln(errOut, "note: spec reverts to the built-in skill")
	}
	fmt.Fprintf(out, "Removed skill %q\n", name)
	return nil
}

func cmdSkillAdopt(st Store, cfg Config, targets []string, name string, out, errOut io.Writer) error {
	if isDir(repoSkillDir(st.Root, name)) {
		return fmt.Errorf(".spec/skills/%s already exists — it is already repo-owned", name)
	}
	files, err := bestSkillFiles(st, cfg, name)
	if err != nil {
		return fmt.Errorf("skill %q is not managed (nothing to adopt)", name)
	}
	if !contains(cfg.Skills, name) && name != "spec" && !isDir(librarySkillDir(name)) {
		return fmt.Errorf("skill %q is not managed (nothing to adopt)", name)
	}
	if err := writeSkillFiles(repoSkillDir(st.Root, name), files); err != nil {
		return err
	}
	repoSkills := configFileSkills(repoConfigPath(st.Root))
	if contains(repoSkills, name) {
		if err := writeSkillsConfig(repoConfigPath(st.Root), withoutSkill(repoSkills, name)); err != nil {
			return err
		}
	}
	if contains(configFileSkills(globalConfigPath()), name) {
		fmt.Fprintln(errOut, "note: still in global skills: — the repo copy shadows the library one")
	}
	fmt.Fprintf(out, "Adopted skill %q → %s\n", name, repoSkillDir(st.Root, name))
	_, err = syncSkills(st, cfg, targets, false, true, errOut)
	return err
}

func cmdSkillPush(st Store, name string, force bool, out, errOut io.Writer) error {
	repoDir := repoSkillDir(st.Root, name)
	if !isDir(repoDir) {
		return fmt.Errorf("no repo skill %q (push works on .spec/skills/<name>; adopt a managed skill first)", name)
	}
	repoFiles, err := skillFiles(repoDir)
	if err != nil {
		return err
	}
	libDir := librarySkillDir(name)
	libFiles, err := skillFiles(libDir)
	if err != nil {
		return err
	}
	var pushed bool
	switch {
	case libFiles == nil:
		if err := writeSkillFiles(libDir, repoFiles); err != nil {
			return err
		}
		pushed = true
	case skillFilesEqual(libFiles, repoFiles):
		fmt.Fprintf(out, "Skill %q already in sync with the library\n", name)
	case !force:
		return fmt.Errorf("library skill %q differs — --force to overwrite the shared copy", name)
	default:
		if err := writeSkillFiles(libDir, repoFiles); err != nil {
			return err
		}
		pushed = true
	}
	if pushed {
		fmt.Fprintf(out, "Pushed skill %q → %s\n", name, libDir)
		if n := len(reposUsingSkill(name)); n > 0 {
			fmt.Fprintf(errOut, "note: enabled in %d registered repo(s) — linked installs see the change now, copies need a sync there\n", n)
		}
	}
	return nil
}

// skillImportSources are the directories searched when `import` gets a bare
// skill name rather than a path.
func skillImportSources(st Store, name string) []string {
	home, _ := os.UserHomeDir()
	return []string{
		filepath.Join(st.Root, ".devin", "skills", name),
		filepath.Join(st.Root, ".claude", "skills", name),
		filepath.Join(home, ".claude", "skills", name),
		filepath.Join(home, ".config", "devin", "skills", name),
	}
}

// isGitURL reports whether source looks like a git remote rather than
// a local path or bare skill name.
func isGitURL(source string) bool {
	return strings.Contains(source, "://") ||
		strings.HasPrefix(source, "git@") ||
		strings.HasSuffix(source, ".git")
}

func cmdSkillImport(st Store, cfg Config, targets []string, source, sel string, library bool, out, errOut io.Writer) error {
	var srcDir, skillName string
	var extraInRepo []string // other repo dirs holding the same skill
	src := source
	var cloned bool
	if !isDir(src) && isGitURL(src) {
		tmp, err := os.MkdirTemp("", "spec-skill-import-*")
		if err != nil {
			return err
		}
		defer os.RemoveAll(tmp)
		dst := filepath.Join(tmp, "repo")
		fmt.Fprintf(out, "Cloning %s…\n", src)
		cmd := exec.Command("git", "clone", "--depth", "1", "-q", src, dst)
		cmd.Stdout, cmd.Stderr = errOut, errOut
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("git clone %s: %w", src, err)
		}
		src, cloned = dst, true
	}
	if isDir(src) {
		switch repoSkillsDir := filepath.Join(src, configDir, "skills"); {
		case isDir(repoSkillsDir):
			// a spec-managed repo root — pick one of its repo skills
			names := skillDirNames(repoSkillsDir)
			switch {
			case sel != "":
				if !contains(names, sel) {
					return fmt.Errorf("no skill %q in %s (has: %s)", sel, repoSkillsDir, strings.Join(names, ", "))
				}
				skillName = sel
			case len(names) == 1:
				skillName = names[0]
			case len(names) == 0:
				return fmt.Errorf("%s has no skills in .spec/skills", src)
			default:
				return fmt.Errorf("%s has several skills (%s) — pass a name", src, strings.Join(names, ", "))
			}
			srcDir = filepath.Join(repoSkillsDir, skillName)
		case cloned:
			return fmt.Errorf("%s has no .spec/skills to import", source)
		default: // a skill dir
			srcDir = src
			if sel != "" {
				skillName = sel
			} else {
				skillName = filepath.Base(filepath.Clean(src))
			}
		}
	} else {
		if sel != "" {
			skillName = sel
		} else {
			skillName = source
		}
		for _, cand := range skillImportSources(st, source) {
			if !isDir(cand) {
				continue
			}
			if srcDir == "" {
				srcDir = cand
			} else if withinRoot(st.Root, cand) {
				extraInRepo = append(extraInRepo, cand)
			}
		}
		if srcDir == "" {
			return fmt.Errorf("no skill dir named %q in the usual locations (pass a path or git URL)", source)
		}
	}
	if !personaNamePattern.MatchString(skillName) {
		return fmt.Errorf("invalid skill name %q", skillName)
	}
	dst := repoSkillDir(st.Root, skillName)
	if library {
		dst = librarySkillDir(skillName)
	}
	if isDir(dst) {
		return fmt.Errorf("skill %q already exists at %s", skillName, dst)
	}
	files, err := skillFiles(srcDir)
	if err != nil {
		return err
	}
	if len(files) == 0 {
		return fmt.Errorf("%s contains no files", srcDir)
	}
	if err := writeSkillFiles(dst, files); err != nil {
		return err
	}
	fmt.Fprintf(out, "Imported %s → %s\n", srcDir, dst)
	if library {
		return nil
	}
	// a source inside the repo is a migration: remove it (git rm when
	// tracked) and let sync regenerate the installed copy from .spec/skills
	for _, d := range append([]string{srcDir}, extraInRepo...) {
		if !withinRoot(st.Root, d) {
			continue // ~/ copies stay put
		}
		if d != srcDir {
			if other, _ := skillFiles(d); !skillFilesEqual(files, other) {
				fmt.Fprintf(errOut, "note: %s differs — left in place\n", d)
				continue
			}
		}
		if err := removeSkillSource(st.Root, d); err != nil {
			return err
		}
		fmt.Fprintf(out, "Removed source %s\n", d)
	}
	_, err = syncSkills(st, cfg, targets, false, true, errOut)
	return err
}

// cmdSkillImportAll imports every foreign skill dir in the repo's target
// dirs into .spec/skills/ — the migration path for hand-authored agent
// skills that predate the .spec/skills layer.
func cmdSkillImportAll(st Store, cfg Config, targets []string, out, errOut io.Writer) error {
	resolved := stringSet(skillNames(st, cfg))
	byName := map[string][]string{}
	for _, t := range unionStrings(targets, []string{".devin/skills", ".claude/skills"}) {
		for _, name := range skillDirNames(filepath.Join(st.Root, t)) {
			if !resolved[name] {
				byName[name] = append(byName[name], filepath.Join(st.Root, t, name))
			}
		}
	}
	if len(byName) == 0 {
		fmt.Fprintln(out, "No foreign skills to import.")
		return nil
	}
	var names []string
	for n := range byName {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, name := range names {
		dirs := byName[name]
		files, err := skillFiles(dirs[0])
		if err != nil {
			return err
		}
		differ := false
		for _, other := range dirs[1:] {
			if of, _ := skillFiles(other); !skillFilesEqual(files, of) {
				fmt.Fprintf(errOut, "skipped %s: %s and %s differ — import one explicitly\n", name, dirs[0], other)
				differ = true
			}
		}
		if differ {
			continue
		}
		if err := writeSkillFiles(repoSkillDir(st.Root, name), files); err != nil {
			return err
		}
		fmt.Fprintf(out, "Imported %s → %s\n", dirs[0], repoSkillDir(st.Root, name))
		for _, d := range dirs {
			if err := removeSkillSource(st.Root, d); err != nil {
				return err
			}
		}
	}
	_, err := syncSkills(st, cfg, targets, false, true, errOut)
	return err
}

// withinRoot reports whether dir is inside the repo.
func withinRoot(root, dir string) bool {
	absRoot, _ := filepath.Abs(root)
	absDir, _ := filepath.Abs(dir)
	rel, err := filepath.Rel(absRoot, absDir)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// removeSkillSource deletes an imported skill's old location: git rm when
// it's tracked, plain removal otherwise.
func removeSkillSource(root, dir string) error {
	absRoot, _ := filepath.Abs(root)
	absDir, _ := filepath.Abs(dir)
	rel, err := filepath.Rel(absRoot, absDir)
	if err != nil {
		rel = dir
	}
	if exec.Command("git", "-C", root, "rm", "-rqf", "--", rel).Run() == nil {
		return nil
	}
	return os.RemoveAll(dir)
}

// cmdSkillGlobal applies check/sync to the user-level agent dirs, resolving
// the globally enabled set (global skills: plus the built-in spec).
// use/unuse --global edit the global skills: list itself.
func cmdSkillGlobal(out, errOut io.Writer, sub, name string, force, quiet bool) error {
	switch sub {
	case "", "check", "sync", "update", "use", "unuse":
	default:
		return fmt.Errorf("usage: spec skill [check|sync|use|unuse <name>] --global")
	}
	if (sub == "use" || sub == "unuse") && (name == "" || !personaNamePattern.MatchString(name)) {
		return fmt.Errorf("usage: spec skill %s <name> --global", sub)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	globalPath := globalConfigPath()
	skills := configFileSkills(globalPath)
	raw := parseConfigFile(globalPath)
	agent := strings.Trim(raw["agent"], `"'`)
	var targets []string
	if list := parseList(raw["skill_global_targets"]); len(list) > 0 {
		// explicit override — the user-level counterpart of skill_targets:
		for _, d := range list {
			if strings.HasPrefix(d, "~/") {
				d = filepath.Join(home, d[2:])
			} else if !filepath.IsAbs(d) {
				d = filepath.Join(home, d)
			}
			targets = append(targets, d)
		}
	} else if d, ok := agentGlobalSkillDirs[agent]; ok {
		targets = []string{filepath.Join(home, d)}
	} else {
		for _, d := range agentGlobalSkillDirs {
			targets = append(targets, filepath.Join(home, d))
		}
	}

	if sub == "use" {
		if name == "spec" {
			return fmt.Errorf("the spec skill is built in and always enabled")
		}
		if contains(skills, name) {
			fmt.Fprintf(out, "Skill %q already enabled globally\n", name)
			return nil
		}
		if !isDir(librarySkillDir(name)) {
			return fmt.Errorf("no library skill %q (see: spec skill)", name)
		}
		if err := writeSkillsConfig(globalPath, append(skills, name)); err != nil {
			return err
		}
		fmt.Fprintf(out, "Using library skill %q globally\n", name)
		sub = "sync"
		skills = append(skills, name)
	}
	if sub == "unuse" {
		if name == "spec" {
			return fmt.Errorf("the spec skill is built in and always enabled")
		}
		if !contains(skills, name) {
			return fmt.Errorf("skill %q is not in global skills:", name)
		}
		libFiles, _ := skillFiles(librarySkillDir(name))
		libAbs, _ := filepath.Abs(librarySkillDir(name))
		for _, t := range targets {
			dst := filepath.Join(t, name)
			li, err := os.Lstat(dst)
			if err != nil {
				continue
			}
			removable := false
			if li.Mode()&os.ModeSymlink != 0 {
				tg, _ := os.Readlink(dst)
				if !filepath.IsAbs(tg) {
					tg = filepath.Join(filepath.Dir(dst), tg)
				}
				removable = filepath.Clean(tg) == filepath.Clean(libAbs)
			} else {
				installed, _ := skillFiles(dst)
				removable = libFiles != nil && skillFilesEqual(installed, libFiles)
			}
			if !removable && !force {
				return fmt.Errorf("installed copy at %s differs — --force to discard it", dst)
			}
			if err := os.RemoveAll(dst); err != nil {
				return err
			}
		}
		if err := writeSkillsConfig(globalPath, withoutSkill(skills, name)); err != nil {
			return err
		}
		fmt.Fprintf(out, "Skill %q no longer managed globally\n", name)
		return nil
	}
	names := append([]string{"spec"}, skills...)
	bad := 0
	for _, name := range names {
		src := skillSource{"", "built-in"}
		if isDir(librarySkillDir(name)) {
			src = skillSource{librarySkillDir(name), "library"}
		}
		srcFiles, err := skillFiles(src.dir)
		if err != nil {
			return err
		}
		for _, t := range targets {
			dst := filepath.Join(t, name)
			status := skillInstallStatus("", dst, src, srcFiles)
			if status != "current" {
				bad++
				if sub == "sync" || sub == "update" {
					if status == "stale" && !force && sub != "update" && src.kind != "built-in" {
						fmt.Fprintf(out, "%-8s %s (differs from source; sync --force to overwrite)\n", "stale", dst)
						continue
					}
					what, err := installSkill("", dst, src, srcFiles)
					if err != nil {
						return err
					}
					status = what
				}
			}
			if !quiet {
				fmt.Fprintf(out, "%-8s %s\n", status, dst)
			}
		}
	}
	if bad > 0 && sub != "sync" && sub != "update" {
		return fmt.Errorf("%d skill file(s) not current — run 'spec skill sync --global'", bad)
	}
	return nil
}
