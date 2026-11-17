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
		// orphaned or unknown: fall back to an installed copy
		for _, t := range skillTargets(cfg) {
			if files, _ := skillFiles(filepath.Join(st.Root, t, name)); files != nil {
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
		installed, err := skillFiles(filepath.Join(st.Root, t, name))
		if err != nil {
			return nil, err
		}
		if installed != nil && !skillFilesEqual(installed, srcFiles) {
			return installed, nil
		}
	}
	return srcFiles, nil
}

// writeSkillsConfig replaces the skills: list in .spec/config.yml; an empty
// list drops the key.
func writeSkillsConfig(root string, skills []string) error {
	v := ""
	if len(skills) > 0 {
		v = "[" + strings.Join(skills, ", ") + "]"
	}
	return setConfigKey(root, "skills", v)
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
	if contains(cfg.Skills, name) {
		if err := writeSkillsConfig(st.Root, withoutSkill(cfg.Skills, name)); err != nil {
			return "", err
		}
	}
	return dst, nil
}

// skillStatusLine describes one resolved skill across the target dirs.
func skillStatusLine(st Store, cfg Config, targets []string, name string) (string, bool) {
	src := resolveSkill(st, cfg, name)
	label := src.kind
	if src.kind == "repo" && contains(cfg.Skills, name) {
		label = "repo (shadowed)"
	}
	if src.kind == "" {
		label = "orphaned"
	}
	ok := src.kind != ""
	var parts []string
	for _, t := range targets {
		installed, err := skillFiles(filepath.Join(st.Root, t, name))
		if err != nil {
			parts = append(parts, t+":error")
			ok = false
			continue
		}
		if installed == nil {
			parts = append(parts, t+":missing")
			if src.kind != "" {
				ok = false
			}
			continue
		}
		srcFiles, err := skillFiles(src.dir)
		if err != nil || !skillFilesEqual(installed, srcFiles) {
			parts = append(parts, t+":stale")
			ok = false
			continue
		}
		parts = append(parts, t+":current")
	}
	return fmt.Sprintf("%-16s %-16s %s", name, label, strings.Join(parts, "  ")), ok
}

// syncSkills materializes the resolved skill set into each target dir.
// force overwrites installed copies that differ from their source; without
// it they are reported stale and left alone. Returns the number of
// non-current paths.
func syncSkills(st Store, cfg Config, targets []string, force bool, out io.Writer) (int, error) {
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
			installed, err := skillFiles(dst)
			if err != nil {
				return bad, err
			}
			switch {
			case installed == nil:
				if err := writeSkillFiles(dst, srcFiles); err != nil {
					return bad, err
				}
				fmt.Fprintf(out, "%-8s %s\n", "installed", dst)
			case skillFilesEqual(installed, srcFiles):
				fmt.Fprintf(out, "%-8s %s\n", "current", dst)
			case force:
				if err := writeSkillFiles(dst, srcFiles); err != nil {
					return bad, err
				}
				fmt.Fprintf(out, "%-8s %s\n", "updated", dst)
			default:
				bad++
				fmt.Fprintf(out, "%-8s %s (differs from source; sync --force to overwrite)\n", "stale", dst)
			}
		}
	}
	// generated dirs for names no longer resolved: remove when they still
	// match the library (unmodified leftovers), report anything else
	for _, t := range targets {
		for _, name := range skillDirNames(filepath.Join(st.Root, t)) {
			if resolved[name] {
				continue
			}
			dst := filepath.Join(st.Root, t, name)
			installed, _ := skillFiles(dst)
			lib, _ := skillFiles(librarySkillDir(name))
			if lib != nil && skillFilesEqual(installed, lib) {
				if err := os.RemoveAll(dst); err != nil {
					return bad, err
				}
				fmt.Fprintf(out, "%-8s %s\n", "removed", dst)
			} else {
				fmt.Fprintf(out, "%-8s %s\n", "foreign", dst)
			}
		}
	}
	return bad, nil
}

func cmdSkill(out, errOut io.Writer, args []string) error {
	var sub, operand string
	var global, force, library, local bool
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
		return fmt.Errorf("usage: spec skill [check|sync|update|view|new|edit|refine|use|unuse|adopt|push|import <name>] [--global|--force|--library|--local]")
	}

	if global {
		return cmdSkillGlobal(out, errOut, sub, force)
	}

	st, err := mustStore()
	if err != nil {
		return err
	}
	cfg := loadConfig(st.Root)
	targets := skillTargets(cfg)

	switch sub {
	case "", "check":
		bad := 0
		for _, name := range skillNames(st, cfg) {
			line, ok := skillStatusLine(st, cfg, targets, name)
			fmt.Fprintln(out, line)
			if !ok {
				bad++
			}
		}
		for _, n := range availableSkills(st, cfg) {
			fmt.Fprintf(out, "%-16s %-16s\n", n, "available")
		}
		if sub == "" {
			return nil
		}
		if bad > 0 {
			return fmt.Errorf("%d skill(s) not current — run 'spec skill sync'", bad)
		}
		return nil
	case "sync", "update":
		bad, err := syncSkills(st, cfg, targets, force || sub == "update", out)
		if err != nil {
			return err
		}
		if bad > 0 {
			return fmt.Errorf("%d skill path(s) not synced", bad)
		}
		return nil
	}

	if operand == "" || (sub != "import" && !personaNamePattern.MatchString(operand)) {
		return fmt.Errorf("usage: spec skill %s <name>", sub)
	}
	name := operand

	switch sub {
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
		return cmdSkillNew(st, cfg, name, library, out, errOut)
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
	case "push":
		return cmdSkillPush(st, name, force, out, errOut)
	case "import":
		return cmdSkillImport(st, cfg, targets, name, library, out, errOut)
	}
	return fmt.Errorf("usage: spec skill [check|sync|update|view|new|edit|refine|use|unuse|adopt|push|import <name>] [--global|--force|--library|--local]")
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

func cmdSkillNew(st Store, cfg Config, name string, library bool, out, errOut io.Writer) error {
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
	fmt.Fprintf(out, "Opening %s with %s\n", path, cfg.editor())
	cmd := exec.Command(cfg.editor(), path)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		return err
	}
	if library {
		return nil
	}
	_, err := syncSkills(st, cfg, skillTargets(cfg), false, errOut)
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
	_, err := syncSkills(st, cfg, skillTargets(cfg), false, errOut)
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
	body := strings.TrimSpace(bodyFromAgentOutput(output))
	if !strings.HasPrefix(body, "---") || !strings.Contains(body, "description:") {
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
		_, err := syncSkills(st, cfg, skillTargets(cfg), false, errOut)
		return err
	}
	return nil
}

func cmdSkillUse(st Store, cfg Config, targets []string, name string, out, errOut io.Writer) error {
	if name == "spec" {
		return fmt.Errorf("the spec skill is built in and always enabled")
	}
	if contains(cfg.Skills, name) {
		fmt.Fprintf(out, "Skill %q already enabled\n", name)
		return nil
	}
	if !isDir(librarySkillDir(name)) {
		return fmt.Errorf("no library skill %q (see: spec skill)", name)
	}
	skills := append(cfg.Skills, name)
	if err := writeSkillsConfig(st.Root, skills); err != nil {
		return err
	}
	fmt.Fprintf(out, "Using library skill %q\n", name)
	if isDir(repoSkillDir(st.Root, name)) {
		fmt.Fprintln(errOut, "note: .spec/skills/"+name+" exists and takes precedence")
	}
	cfg.Skills = skills
	_, err := syncSkills(st, cfg, targets, false, errOut)
	return err
}

func cmdSkillUnuse(st Store, cfg Config, targets []string, name string, force bool, out, errOut io.Writer) error {
	if name == "spec" {
		return fmt.Errorf("the spec skill is built in and always enabled")
	}
	if !contains(cfg.Skills, name) {
		return fmt.Errorf("skill %q is not in skills:", name)
	}
	// installed copies may only be removed when they still match the
	// source; a differing copy is a fork the user must keep or discard
	src := resolveSkill(st, cfg, name)
	srcFiles, _ := skillFiles(src.dir)
	for _, t := range targets {
		installed, err := skillFiles(filepath.Join(st.Root, t, name))
		if err != nil {
			return err
		}
		if installed == nil {
			continue
		}
		if !force && (srcFiles == nil || !skillFilesEqual(installed, srcFiles)) {
			return fmt.Errorf("installed copy at %s differs — use spec skill adopt %s to keep it, or --force to discard", filepath.Join(st.Root, t, name), name)
		}
	}
	for _, t := range targets {
		if err := os.RemoveAll(filepath.Join(st.Root, t, name)); err != nil {
			return err
		}
	}
	if err := writeSkillsConfig(st.Root, withoutSkill(cfg.Skills, name)); err != nil {
		return err
	}
	fmt.Fprintf(out, "Skill %q no longer managed\n", name)
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
	if contains(cfg.Skills, name) {
		if err := writeSkillsConfig(st.Root, withoutSkill(cfg.Skills, name)); err != nil {
			return err
		}
	}
	fmt.Fprintf(out, "Adopted skill %q → %s\n", name, repoSkillDir(st.Root, name))
	_, err = syncSkills(st, cfg, targets, false, errOut)
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
	switch {
	case libFiles == nil:
		if err := writeSkillFiles(libDir, repoFiles); err != nil {
			return err
		}
		fmt.Fprintf(out, "Pushed skill %q → %s\n", name, libDir)
	case skillFilesEqual(libFiles, repoFiles):
		fmt.Fprintf(out, "Skill %q already in sync with the library\n", name)
	case !force:
		return fmt.Errorf("library skill %q differs — --force to overwrite the shared copy", name)
	default:
		if err := writeSkillFiles(libDir, repoFiles); err != nil {
			return err
		}
		fmt.Fprintf(out, "Pushed skill %q → %s\n", name, libDir)
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

func cmdSkillImport(st Store, cfg Config, targets []string, name string, library bool, out, errOut io.Writer) error {
	var srcDir, skillName string
	if isDir(name) { // a path, not a name
		srcDir = name
		skillName = filepath.Base(filepath.Clean(name))
	} else {
		skillName = name
		for _, cand := range skillImportSources(st, name) {
			if isDir(cand) {
				srcDir = cand
				break
			}
		}
		if srcDir == "" {
			return fmt.Errorf("no skill dir named %q in the usual locations (pass a path)", name)
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
	_, err = syncSkills(st, cfg, targets, false, errOut)
	return err
}

// cmdSkillGlobal applies check/sync to the user-level agent dirs, resolving
// the globally enabled set (global skills: plus the built-in spec).
func cmdSkillGlobal(out, errOut io.Writer, sub string, force bool) error {
	if sub != "" && sub != "check" && sub != "sync" && sub != "update" {
		return fmt.Errorf("usage: spec skill [check|sync] --global")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	raw := parseConfigFile(globalConfigPath())
	skills := parseList(raw["skills"])
	agent := strings.Trim(raw["agent"], `"'`)
	var targets []string
	if d, ok := agentGlobalSkillDirs[agent]; ok {
		targets = []string{filepath.Join(home, d)}
	} else {
		for _, d := range agentGlobalSkillDirs {
			targets = append(targets, filepath.Join(home, d))
		}
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
			installed, err := skillFiles(dst)
			if err != nil {
				return err
			}
			var status string
			switch {
			case installed == nil:
				status = "missing"
			case !skillFilesEqual(installed, srcFiles):
				status = "stale"
			default:
				status = "current"
			}
			if status != "current" {
				bad++
				if sub == "sync" || sub == "update" {
					if status == "stale" && !force && sub != "update" {
						fmt.Fprintf(out, "%-8s %s (differs from source; sync --force to overwrite)\n", "stale", dst)
						continue
					}
					if err := writeSkillFiles(dst, srcFiles); err != nil {
						return err
					}
					status = "updated"
				}
			}
			fmt.Fprintf(out, "%-8s %s\n", status, dst)
		}
	}
	if bad > 0 && sub != "sync" && sub != "update" {
		return fmt.Errorf("%d skill file(s) not current — run 'spec skill sync --global'", bad)
	}
	return nil
}
