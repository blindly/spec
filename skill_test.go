package main

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeLibrarySkill creates a library skill at ~/.config/spec/skills/<name>/.
func writeLibrarySkill(t *testing.T, name string, files map[string]string) {
	t.Helper()
	dir := librarySkillDir(name)
	for rel, content := range files {
		p := filepath.Join(dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func writeRepoSkill(t *testing.T, st Store, name string, files map[string]string) {
	t.Helper()
	dir := repoSkillDir(st.Root, name)
	for rel, content := range files {
		p := filepath.Join(dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// materializeInstall replaces a linked install at dst with a real copy of
// its contents, so a test can diverge it from the source.
func materializeInstall(t *testing.T, dst string) {
	t.Helper()
	real, err := filepath.EvalSymlinks(dst)
	if err != nil {
		t.Fatalf("%s is not a link: %v", dst, err)
	}
	files, err := skillFiles(real)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(dst); err != nil {
		t.Fatal(err)
	}
	if err := writeSkillFiles(dst, files); err != nil {
		t.Fatal(err)
	}
}

func simpleSkill(name, body string) map[string]string {
	return map[string]string{"SKILL.md": "---\nname: " + name + "\ndescription: test\n---\n\n" + body}
}

func TestSkillUseUnuse(t *testing.T) {
	st := setupRepo(t)
	writeLibrarySkill(t, "review", simpleSkill("review", "Review carefully."))

	var out bytes.Buffer
	if err := cmdSkill(&out, io.Discard, []string{"use", "review"}); err != nil {
		t.Fatal(err)
	}
	cfg := loadConfig(st.Root)
	if !contains(cfg.Skills, "review") {
		t.Fatal("skills: not written to config")
	}
	for _, target := range []string{".devin/skills", ".claude/skills"} {
		data, err := os.ReadFile(filepath.Join(st.Root, target, "review", "SKILL.md"))
		if err != nil || !strings.Contains(string(data), "Review carefully") {
			t.Fatalf("not installed at %s: %v", target, err)
		}
	}

	out.Reset()
	if err := cmdSkill(&out, io.Discard, []string{"unuse", "review"}); err != nil {
		t.Fatal(err)
	}
	cfg = loadConfig(st.Root)
	if contains(cfg.Skills, "review") {
		t.Fatal("skills: entry not removed")
	}
	for _, target := range []string{".devin/skills", ".claude/skills"} {
		if isDir(filepath.Join(st.Root, target, "review")) {
			t.Fatalf("installed copy left at %s", target)
		}
	}
}

func TestSkillUnuseProtectsModified(t *testing.T) {
	st := setupRepo(t)
	writeLibrarySkill(t, "review", simpleSkill("review", "Review carefully."))
	if err := cmdSkill(io.Discard, io.Discard, []string{"use", "review"}); err != nil {
		t.Fatal(err)
	}
	materializeInstall(t, filepath.Join(st.Root, ".devin", "skills", "review"))
	installed := filepath.Join(st.Root, ".devin", "skills", "review", "SKILL.md")
	os.WriteFile(installed, []byte("local tweaks"), 0o644)

	if err := cmdSkill(io.Discard, io.Discard, []string{"unuse", "review"}); err == nil {
		t.Fatal("unuse should refuse to delete a modified copy")
	}
	if err := cmdSkill(io.Discard, io.Discard, []string{"unuse", "review", "--force"}); err != nil {
		t.Fatalf("unuse --force: %v", err)
	}
}

func TestSkillLocalShadowsLibrary(t *testing.T) {
	st := setupRepo(t)
	writeLibrarySkill(t, "review", simpleSkill("review", "shared"))
	writeRepoSkill(t, st, "review", simpleSkill("review", "repo variant"))

	src := resolveSkill(st, loadConfig(st.Root), "review")
	if src.kind != "repo" {
		t.Fatalf("repo copy should shadow, got %q", src.kind)
	}
	var out, errs bytes.Buffer
	if err := cmdSkill(&out, &errs, []string{"use", "review"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(errs.String(), "takes precedence") {
		t.Fatalf("want shadow warning on stderr, got %q", errs.String())
	}
	out.Reset()
	if err := cmdSkill(&out, io.Discard, nil); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "shadowed") {
		t.Fatalf("want shadowed state, got:\n%s", out.String())
	}
	// installed copies track the repo copy, not the library
	data, _ := os.ReadFile(filepath.Join(st.Root, ".devin", "skills", "review", "SKILL.md"))
	if !strings.Contains(string(data), "repo variant") {
		t.Fatal("installed copy should come from .spec/skills, not library")
	}
}


func TestSkillAdopt(t *testing.T) {
	st := setupRepo(t)
	writeLibrarySkill(t, "review", simpleSkill("review", "shared"))
	if err := cmdSkill(io.Discard, io.Discard, []string{"use", "review"}); err != nil {
		t.Fatal(err)
	}
	if err := cmdSkill(io.Discard, io.Discard, []string{"adopt", "review"}); err != nil {
		t.Fatal(err)
	}
	cfg := loadConfig(st.Root)
	if contains(cfg.Skills, "review") {
		t.Fatal("skills: entry should be dropped by adopt")
	}
	data, err := os.ReadFile(repoSkillDir(st.Root, "review") + "/SKILL.md")
	if err != nil || !strings.Contains(string(data), "shared") {
		t.Fatalf("adopt did not copy content: %v", err)
	}
	src := resolveSkill(st, cfg, "review")
	if src.kind != "repo" {
		t.Fatalf("adopted skill should resolve repo, got %q", src.kind)
	}
}

func TestSkillAdoptPreservesEdits(t *testing.T) {
	st := setupRepo(t)
	writeLibrarySkill(t, "review", simpleSkill("review", "shared"))
	if err := cmdSkill(io.Discard, io.Discard, []string{"use", "review"}); err != nil {
		t.Fatal(err)
	}
	installed := filepath.Join(st.Root, ".devin", "skills", "review", "SKILL.md")
	os.WriteFile(installed, []byte("my edits"), 0o644)
	if err := cmdSkill(io.Discard, io.Discard, []string{"adopt", "review"}); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(repoSkillDir(st.Root, "review") + "/SKILL.md")
	if string(data) != "my edits" {
		t.Fatalf("adopt should preserve installed edits, got %q", data)
	}
}

func TestSkillPush(t *testing.T) {
	st := setupRepo(t)
	writeRepoSkill(t, st, "review", simpleSkill("review", "v1"))

	var out bytes.Buffer
	if err := cmdSkill(&out, io.Discard, []string{"push", "review"}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(librarySkillDir("review") + "/SKILL.md")
	if err != nil || !strings.Contains(string(data), "v1") {
		t.Fatal("push did not create library entry")
	}

	// identical: no-op
	out.Reset()
	if err := cmdSkill(&out, io.Discard, []string{"push", "review"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "in sync") {
		t.Fatalf("want in-sync, got %q", out.String())
	}

	// diverged library copy needs --force
	writeLibrarySkill(t, "review", simpleSkill("review", "other"))
	if err := cmdSkill(io.Discard, io.Discard, []string{"push", "review"}); err == nil {
		t.Fatal("push should refuse to overwrite a differing library copy")
	}
	if err := cmdSkill(io.Discard, io.Discard, []string{"push", "review", "--force"}); err != nil {
		t.Fatalf("push --force: %v", err)
	}
	data, _ = os.ReadFile(librarySkillDir("review") + "/SKILL.md")
	if !strings.Contains(string(data), "v1") {
		t.Fatal("push --force did not overwrite")
	}
}

func TestSkillPushManagedRefused(t *testing.T) {
	setupRepo(t)
	writeLibrarySkill(t, "review", simpleSkill("review", "shared"))
	if err := cmdSkill(io.Discard, io.Discard, []string{"use", "review"}); err != nil {
		t.Fatal(err)
	}
	if err := cmdSkill(io.Discard, io.Discard, []string{"push", "review"}); err == nil {
		t.Fatal("push of a managed skill should fail (adopt first)")
	}
}

func TestSkillSyncClobberProtection(t *testing.T) {
	st := setupRepo(t)
	writeLibrarySkill(t, "review", simpleSkill("review", "v1"))
	if err := cmdSkill(io.Discard, io.Discard, []string{"use", "review"}); err != nil {
		t.Fatal(err)
	}
	materializeInstall(t, filepath.Join(st.Root, ".devin", "skills", "review"))
	installed := filepath.Join(st.Root, ".devin", "skills", "review", "SKILL.md")
	os.WriteFile(installed, []byte("hand edit"), 0o644)

	var out bytes.Buffer
	err := cmdSkill(&out, io.Discard, []string{"sync"})
	if err == nil {
		t.Fatal("sync should refuse to clobber a modified copy")
	}
	if data, _ := os.ReadFile(installed); string(data) != "hand edit" {
		t.Fatal("sync overwrote a modified copy")
	}
	out.Reset()
	if err := cmdSkill(&out, io.Discard, []string{"sync", "--force"}); err != nil {
		t.Fatal(err)
	}
	if data, _ := os.ReadFile(installed); !strings.Contains(string(data), "v1") {
		t.Fatal("sync --force did not restore source content")
	}
}

func TestSkillOrphaned(t *testing.T) {
	st := setupRepo(t)
	writeLibrarySkill(t, "review", simpleSkill("review", "shared"))
	if err := cmdSkill(io.Discard, io.Discard, []string{"use", "review"}); err != nil {
		t.Fatal(err)
	}
	os.RemoveAll(librarySkillDir("review"))

	var out bytes.Buffer
	err := cmdSkill(&out, io.Discard, []string{"check"})
	if err == nil {
		t.Fatal("check should fail on an orphaned skill")
	}
	if !strings.Contains(out.String(), "orphaned") {
		t.Fatalf("want orphaned, got:\n%s", out.String())
	}
	// installed entry (now a dangling link) is reported, never deleted
	if _, err := os.Lstat(filepath.Join(st.Root, ".devin", "skills", "review")); err != nil {
		t.Fatal("orphaned installed copy was deleted")
	}
}

func TestSkillImport(t *testing.T) {
	st := setupRepo(t)
	ext := filepath.Join(st.Root, "ext", "thing")
	os.MkdirAll(ext, 0o755)
	os.WriteFile(filepath.Join(ext, "SKILL.md"), []byte("---\nname: thing\ndescription: x\n---\n\nbody"), 0o644)

	if err := cmdSkill(io.Discard, io.Discard, []string{"import", ext}); err != nil {
		t.Fatal(err)
	}
	if data, _ := os.ReadFile(repoSkillDir(st.Root, "thing") + "/SKILL.md"); !strings.Contains(string(data), "body") {
		t.Fatal("import did not copy the skill")
	}
	// imported skill is local and gets installed
	if !isDir(filepath.Join(st.Root, ".devin", "skills", "thing")) {
		t.Fatal("imported skill not installed to targets")
	}
}

func TestSkillTargets(t *testing.T) {
	st := setupRepo(t)
	writeLibrarySkill(t, "review", simpleSkill("review", "shared"))
	if err := setConfigKey(st.Root, "agent", "claude"); err != nil {
		t.Fatal(err)
	}
	if err := cmdSkill(io.Discard, io.Discard, []string{"use", "review"}); err != nil {
		t.Fatal(err)
	}
	if !isDir(filepath.Join(st.Root, ".claude", "skills", "review")) {
		t.Fatal("agent: claude should install to .claude/skills")
	}
	if isDir(filepath.Join(st.Root, ".devin", "skills", "review")) {
		t.Fatal("agent: claude should not install to .devin/skills")
	}
}

func TestSkillAvailableListed(t *testing.T) {
	setupRepo(t)
	writeLibrarySkill(t, "unused", simpleSkill("unused", "x"))
	var out bytes.Buffer
	if err := cmdSkill(&out, io.Discard, nil); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "unused") || !strings.Contains(out.String(), "available") {
		t.Fatalf("available skill not listed:\n%s", out.String())
	}
}

func TestSkillBuiltinAdopt(t *testing.T) {
	st := setupRepo(t)
	if err := cmdSkill(io.Discard, io.Discard, []string{"adopt", "spec"}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(repoSkillDir(st.Root, "spec") + "/SKILL.md")
	if err != nil || string(data) != specSkill {
		t.Fatal("adopt spec did not fork the built-in skill")
	}
	src := resolveSkill(st, loadConfig(st.Root), "spec")
	if src.kind != "repo" {
		t.Fatalf("adopted spec skill should resolve repo, got %q", src.kind)
	}
}

func writeGlobalConfig(t *testing.T, contents string) {
	t.Helper()
	path := globalConfigPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestSkillGlobalRepoUnion(t *testing.T) {
	st := setupRepo(t)
	writeLibrarySkill(t, "alpha", simpleSkill("alpha", "A"))
	writeLibrarySkill(t, "beta", simpleSkill("beta", "B"))
	writeGlobalConfig(t, "skills: [alpha]\n")

	if err := cmdSkill(io.Discard, io.Discard, []string{"use", "beta"}); err != nil {
		t.Fatal(err)
	}
	cfg := loadConfig(st.Root)
	if !contains(cfg.Skills, "alpha") || !contains(cfg.Skills, "beta") {
		t.Fatalf("skills: should union global+repo, got %v", cfg.Skills)
	}
	repoSkills := configFileSkills(repoConfigPath(st.Root))
	if len(repoSkills) != 1 || repoSkills[0] != "beta" {
		t.Fatalf("repo skills: should hold only repo additions, got %v", repoSkills)
	}
	// the global-baseline skill still installs on repo sync
	if !isDir(filepath.Join(st.Root, ".devin", "skills", "alpha")) {
		t.Fatal("global-baseline skill alpha was not installed by repo sync")
	}
}

func TestSkillListProvenance(t *testing.T) {
	setupRepo(t)
	writeLibrarySkill(t, "alpha", simpleSkill("alpha", "A"))
	writeLibrarySkill(t, "beta", simpleSkill("beta", "B"))
	writeGlobalConfig(t, "skills: [alpha]\n")
	if err := cmdSkill(io.Discard, io.Discard, []string{"use", "beta"}); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := cmdSkill(&out, io.Discard, nil); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	if !strings.Contains(got, "alpha") || !strings.Contains(got, "library(global)") {
		t.Fatalf("alpha should be library(global):\n%s", got)
	}
	if !strings.Contains(got, "beta") || !strings.Contains(got, "library(repo)") {
		t.Fatalf("beta should be library(repo):\n%s", got)
	}
}

func TestSkillUseAlreadyGlobal(t *testing.T) {
	setupRepo(t)
	writeLibrarySkill(t, "alpha", simpleSkill("alpha", "A"))
	writeGlobalConfig(t, "skills: [alpha]\n")
	var out bytes.Buffer
	if err := cmdSkill(&out, io.Discard, []string{"use", "alpha"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "global") {
		t.Fatalf("should report already enabled globally, got: %s", out.String())
	}
}

func TestSkillUnuseGlobalRefuses(t *testing.T) {
	setupRepo(t)
	writeGlobalConfig(t, "skills: [alpha]\n")
	err := cmdSkill(io.Discard, io.Discard, []string{"unuse", "alpha"})
	if err == nil || !strings.Contains(err.Error(), "--global") {
		t.Fatalf("unuse of global skill should point at --global, got %v", err)
	}
}

func TestSkillUseUnuseGlobal(t *testing.T) {
	setupRepo(t)
	t.Setenv("HOME", t.TempDir()) // --global targets live under UserHomeDir
	writeLibrarySkill(t, "alpha", simpleSkill("alpha", "A"))
	writeGlobalConfig(t, "agent: claude\n")

	var out bytes.Buffer
	if err := cmdSkill(&out, io.Discard, []string{"use", "alpha", "--global"}); err != nil {
		t.Fatal(err)
	}
	if !contains(configFileSkills(globalConfigPath()), "alpha") {
		t.Fatal("use --global did not write global skills:")
	}
	home, _ := os.UserHomeDir()
	if !isDir(filepath.Join(home, ".claude", "skills", "alpha")) {
		t.Fatal("use --global did not install to the user-level claude dir")
	}
	if err := cmdSkill(io.Discard, io.Discard, []string{"unuse", "alpha", "--global"}); err != nil {
		t.Fatal(err)
	}
	if contains(configFileSkills(globalConfigPath()), "alpha") {
		t.Fatal("unuse --global did not drop the global skills: entry")
	}
	if isDir(filepath.Join(home, ".claude", "skills", "alpha")) {
		t.Fatal("unuse --global did not remove the installed copy")
	}
}

func TestInitGitignoresSkillTargets(t *testing.T) {
	st := setupRepo(t)
	data, err := os.ReadFile(filepath.Join(st.Root, ".gitignore"))
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range []string{"/.devin/skills/", "/.claude/skills/"} {
		if !strings.Contains(string(data), e) {
			t.Fatalf(".gitignore missing %s:\n%s", e, data)
		}
	}
	// idempotent — init again must not duplicate the entries
	if err := cmdInit(io.Discard, io.Discard); err != nil {
		t.Fatal(err)
	}
	again, _ := os.ReadFile(filepath.Join(st.Root, ".gitignore"))
	if strings.Count(string(again), "/.devin/skills/") != 1 {
		t.Fatalf("init duplicated gitignore entries:\n%s", again)
	}
}

func TestSkillSyncGitignoresTargets(t *testing.T) {
	st := setupRepo(t)
	// a repo initialized before .gitignore seeding existed
	os.WriteFile(filepath.Join(st.Root, ".gitignore"), []byte("/spec\n"), 0o644)
	if err := cmdSkill(io.Discard, io.Discard, []string{"sync"}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(st.Root, ".gitignore"))
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range []string{"/.devin/skills/", "/.claude/skills/"} {
		if !strings.Contains(string(data), e) {
			t.Fatalf("sync did not gitignore %s:\n%s", e, data)
		}
	}
	if !strings.Contains(string(data), "/spec\n") {
		t.Fatalf("sync clobbered existing .gitignore entries:\n%s", data)
	}
}

func TestSkillImportMovesRepoSource(t *testing.T) {
	st := setupRepo(t)
	dir := filepath.Join(st.Root, ".devin", "skills", "runbook")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("runbook body"), 0o644)

	var out bytes.Buffer
	if err := cmdSkill(&out, io.Discard, []string{"import", "runbook"}); err != nil {
		t.Fatal(err)
	}
	if data, err := os.ReadFile(repoSkillDir(st.Root, "runbook") + "/SKILL.md"); err != nil || string(data) != "runbook body" {
		t.Fatal("source not copied to .spec/skills")
	}
	// sync regenerated the installed copy from .spec/skills
	if data, err := os.ReadFile(filepath.Join(dir, "SKILL.md")); err != nil || string(data) != "runbook body" {
		t.Fatal("installed copy not regenerated by sync")
	}
	if !strings.Contains(out.String(), "Removed source") {
		t.Fatalf("in-repo source should be moved, not left:\n%s", out.String())
	}
}

func TestSkillImportKeepsHomeSource(t *testing.T) {
	st := setupRepo(t)
	home := t.TempDir()
	t.Setenv("HOME", home)
	dir := filepath.Join(home, ".claude", "skills", "toolkit")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("toolkit body"), 0o644)

	if err := cmdSkill(io.Discard, io.Discard, []string{"import", "toolkit"}); err != nil {
		t.Fatal(err)
	}
	if !isDir(repoSkillDir(st.Root, "toolkit")) {
		t.Fatal("not imported to .spec/skills")
	}
	if !isDir(dir) {
		t.Fatal("home source must not be removed")
	}
}

func TestSkillImportAll(t *testing.T) {
	st := setupRepo(t)
	for _, p := range []string{
		filepath.Join(st.Root, ".devin", "skills", "one", "SKILL.md"),
		filepath.Join(st.Root, ".claude", "skills", "one", "SKILL.md"),
		filepath.Join(st.Root, ".claude", "skills", "two", "SKILL.md"),
	} {
		os.MkdirAll(filepath.Dir(p), 0o755)
		os.WriteFile(p, []byte("body of "+filepath.Base(filepath.Dir(p))), 0o644)
	}
	// a managed name's generated copy is not foreign — must not be imported
	writeLibrarySkill(t, "managed", simpleSkill("managed", "m"))
	if err := cmdSkill(io.Discard, io.Discard, []string{"use", "managed"}); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	if err := cmdSkill(&out, io.Discard, []string{"import"}); err != nil {
		t.Fatal(err)
	}
	for _, n := range []string{"one", "two"} {
		if !isDir(repoSkillDir(st.Root, n)) {
			t.Fatalf("%s not imported to .spec/skills", n)
		}
	}
	if isDir(repoSkillDir(st.Root, "managed")) {
		t.Fatal("generated copy of a managed skill was wrongly imported")
	}
	if !strings.Contains(out.String(), "one") || !strings.Contains(out.String(), "two") {
		t.Fatalf("expected both imports reported:\n%s", out.String())
	}
}

func TestSkillImportAllDivergingSkipped(t *testing.T) {
	st := setupRepo(t)
	d := filepath.Join(st.Root, ".devin", "skills", "fork")
	c := filepath.Join(st.Root, ".claude", "skills", "fork")
	os.MkdirAll(d, 0o755)
	os.MkdirAll(c, 0o755)
	os.WriteFile(filepath.Join(d, "SKILL.md"), []byte("devin version"), 0o644)
	os.WriteFile(filepath.Join(c, "SKILL.md"), []byte("claude version"), 0o644)

	var errOut bytes.Buffer
	if err := cmdSkill(io.Discard, &errOut, []string{"import"}); err != nil {
		t.Fatal(err)
	}
	if isDir(repoSkillDir(st.Root, "fork")) {
		t.Fatal("diverging copies must not be auto-imported")
	}
	if !strings.Contains(errOut.String(), "differ") {
		t.Fatalf("divergence should be reported:\n%s", errOut.String())
	}
}

func TestSkillImportLibraryOutsideRepo(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, ".xdg"))
	t.Setenv("HOME", dir)
	old, _ := os.Getwd()
	os.Chdir(dir)
	t.Cleanup(func() { os.Chdir(old) })
	// no .spec here — mustStore would fail; --library must still work
	src := filepath.Join(dir, ".claude", "skills", "loose")
	os.MkdirAll(src, 0o755)
	os.WriteFile(filepath.Join(src, "SKILL.md"), []byte("loose body"), 0o644)

	var out bytes.Buffer
	if err := cmdSkill(&out, io.Discard, []string{"import", "loose", "--library"}); err != nil {
		t.Fatal(err)
	}
	if !isDir(librarySkillDir("loose")) {
		t.Fatal("not imported to the library")
	}
	if !isDir(src) {
		t.Fatal("home source must not be removed")
	}
}

func TestSkillRm(t *testing.T) {
	st := setupRepo(t)
	writeRepoSkill(t, st, "dead", simpleSkill("dead", "gone"))
	if err := cmdSkill(io.Discard, io.Discard, []string{"sync"}); err != nil {
		t.Fatal(err)
	}
	if err := cmdSkill(io.Discard, io.Discard, []string{"rm", "dead"}); err != nil {
		t.Fatal(err)
	}
	if isDir(repoSkillDir(st.Root, "dead")) {
		t.Fatal(".spec/skills/dead still exists")
	}
	for _, target := range []string{".devin/skills", ".claude/skills"} {
		if isDir(filepath.Join(st.Root, target, "dead")) {
			t.Fatalf("installed copy left at %s", target)
		}
	}
}

func TestSkillLinkedInstalls(t *testing.T) {
	st := setupRepo(t)

	// repo skill → relative symlink into .spec/skills
	writeRepoSkill(t, st, "local", simpleSkill("local", "local v1"))
	// library skills → absolute symlinks into the library
	writeLibrarySkill(t, "shared", simpleSkill("shared", "shared v1"))
	writeLibrarySkill(t, "temp", simpleSkill("temp", "temp"))
	for _, n := range []string{"shared", "temp"} {
		if err := cmdSkill(io.Discard, io.Discard, []string{"use", n}); err != nil {
			t.Fatal(err)
		}
	}
	if err := cmdSkill(io.Discard, io.Discard, []string{"sync"}); err != nil {
		t.Fatal(err)
	}

	repoLink := filepath.Join(st.Root, ".devin", "skills", "local")
	tg, err := os.Readlink(repoLink)
	if err != nil {
		t.Fatalf("repo skill not linked: %v", err)
	}
	if filepath.IsAbs(tg) {
		t.Fatalf("repo skill link should be relative, got %s", tg)
	}
	libLink := filepath.Join(st.Root, ".devin", "skills", "shared")
	tg, err = os.Readlink(libLink)
	if err != nil {
		t.Fatalf("library skill not linked: %v", err)
	}
	if !filepath.IsAbs(tg) {
		t.Fatalf("library skill link should be absolute, got %s", tg)
	}
	if tg != librarySkillDir("shared") {
		t.Fatalf("library link → %s, want %s", tg, librarySkillDir("shared"))
	}

	// built-in spec skill stays a real copied dir
	specDst := filepath.Join(st.Root, ".devin", "skills", "spec")
	if li, err := os.Lstat(specDst); err != nil || li.Mode()&os.ModeSymlink != 0 {
		t.Fatal("built-in spec should be copied, not linked")
	}

	// edits to sources are live through the links — no re-sync needed
	os.WriteFile(filepath.Join(repoSkillDir(st.Root, "local"), "SKILL.md"), []byte("local v2"), 0o644)
	os.WriteFile(filepath.Join(librarySkillDir("shared"), "SKILL.md"), []byte("shared v2"), 0o644)
	if data, _ := os.ReadFile(filepath.Join(repoLink, "SKILL.md")); string(data) != "local v2" {
		t.Fatal("repo link not live")
	}
	if data, _ := os.ReadFile(filepath.Join(libLink, "SKILL.md")); string(data) != "shared v2" {
		t.Fatal("library link not live")
	}
	var out bytes.Buffer
	if err := cmdSkill(&out, io.Discard, []string{"check"}); err != nil {
		t.Fatalf("check should pass through current links: %v\n%s", err, out.String())
	}

	// adopt re-points the install from library link to relative repo link
	if err := cmdSkill(io.Discard, io.Discard, []string{"adopt", "shared"}); err != nil {
		t.Fatal(err)
	}
	tg, err = os.Readlink(libLink)
	if err != nil {
		t.Fatalf("adopted skill not linked: %v", err)
	}
	if filepath.IsAbs(tg) {
		t.Fatalf("adopted skill link should be relative, got %s", tg)
	}

	// unuse removes only the link — the library source stays
	tempLink := filepath.Join(st.Root, ".devin", "skills", "temp")
	if err := cmdSkill(io.Discard, io.Discard, []string{"unuse", "temp"}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(tempLink); !os.IsNotExist(err) {
		t.Fatal("unuse left the link in place")
	}
	if !isDir(librarySkillDir("temp")) {
		t.Fatal("unuse deleted the library skill")
	}

	// rm removes links and the source — and only those
	if err := cmdSkill(io.Discard, io.Discard, []string{"rm", "local"}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(repoLink); !os.IsNotExist(err) {
		t.Fatal("rm left the link in place")
	}
	if isDir(repoSkillDir(st.Root, "local")) {
		t.Fatal("rm did not remove the repo skill")
	}
}

func TestSkillSyncUpgradesLegacyCopies(t *testing.T) {
	st := setupRepo(t)
	writeLibrarySkill(t, "shared", simpleSkill("shared", "v1"))
	if err := cmdSkill(io.Discard, io.Discard, []string{"use", "shared"}); err != nil {
		t.Fatal(err)
	}
	dst := filepath.Join(st.Root, ".devin", "skills", "shared")

	// an equal-content copy upgrades to a link without --force
	materializeInstall(t, dst)
	var out bytes.Buffer
	if err := cmdSkill(&out, io.Discard, []string{"sync"}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Readlink(dst); err != nil {
		t.Fatalf("equal copy was not upgraded to a link: %v", err)
	}

	// check reports a still-copied install as "copied", not current
	materializeInstall(t, dst)
	out.Reset()
	err := cmdSkill(&out, io.Discard, []string{"check"})
	if err == nil {
		t.Fatal("check should flag a copied install")
	}
	if !strings.Contains(out.String(), "copied") {
		t.Fatalf("want copied status, got:\n%s", out.String())
	}
}

func TestSkillRmDifferingNeedsForce(t *testing.T) {
	st := setupRepo(t)
	writeRepoSkill(t, st, "dead", simpleSkill("dead", "v1"))
	if err := cmdSkill(io.Discard, io.Discard, []string{"sync"}); err != nil {
		t.Fatal(err)
	}
	materializeInstall(t, filepath.Join(st.Root, ".devin", "skills", "dead"))
	installed := filepath.Join(st.Root, ".devin", "skills", "dead", "SKILL.md")
	os.WriteFile(installed, []byte("hand edit"), 0o644)
	if err := cmdSkill(io.Discard, io.Discard, []string{"rm", "dead"}); err == nil || !strings.Contains(err.Error(), "--force") {
		t.Fatalf("differing copy should need --force, got %v", err)
	}
	if err := cmdSkill(io.Discard, io.Discard, []string{"rm", "dead", "--force"}); err != nil {
		t.Fatal(err)
	}
	if isDir(repoSkillDir(st.Root, "dead")) {
		t.Fatal("rm --force did not remove the repo skill")
	}
}

func TestSkillRmManagedRefuses(t *testing.T) {
	setupRepo(t)
	writeLibrarySkill(t, "lib", simpleSkill("lib", "x"))
	if err := cmdSkill(io.Discard, io.Discard, []string{"use", "lib"}); err != nil {
		t.Fatal(err)
	}
	err := cmdSkill(io.Discard, io.Discard, []string{"rm", "lib"})
	if err == nil || !strings.Contains(err.Error(), "unuse") {
		t.Fatalf("rm on a managed skill should point at unuse, got %v", err)
	}
}

func TestSkillRmLibrary(t *testing.T) {
	setupRepo(t)
	writeLibrarySkill(t, "lib", simpleSkill("lib", "x"))
	if err := cmdSkill(io.Discard, io.Discard, []string{"rm", "lib", "--library"}); err != nil {
		t.Fatal(err)
	}
	if isDir(librarySkillDir("lib")) {
		t.Fatal("library entry still exists")
	}
}

func TestSkillRefineKeepsFrontmatter(t *testing.T) {
	st := setupRepo(t)
	writeRepoSkill(t, st, "tidy", simpleSkill("tidy", "Old body."))
	fakeRefineAgent(t, st, "---\nname: tidy\ndescription: tidies things\n---\n\n# Tidy\n\nNew body.\n")
	var out, errs bytes.Buffer
	if err := cmdSkill(&out, &errs, []string{"refine", "tidy"}); err != nil {
		t.Fatalf("%v\n%s", err, errs.String())
	}
	want := "---\nname: tidy\ndescription: tidies things\n---\n\n# Tidy\n\nNew body.\n"
	data, err := os.ReadFile(filepath.Join(repoSkillDir(st.Root, "tidy"), "SKILL.md"))
	if err != nil || string(data) != want {
		t.Fatalf("refined skill (%v):\n%s", err, data)
	}
	// garbage output leaves the skill alone
	fakeRefineAgent(t, st, "I couldn't improve it.\n")
	if err := cmdSkill(&out, &errs, []string{"refine", "tidy"}); err == nil {
		t.Fatal("garbage output accepted")
	}
	data, _ = os.ReadFile(filepath.Join(repoSkillDir(st.Root, "tidy"), "SKILL.md"))
	if string(data) != want {
		t.Fatal("skill lost on bad output")
	}
}
