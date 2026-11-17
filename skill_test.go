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
	// installed copy is reported, never deleted
	if !isDir(filepath.Join(st.Root, ".devin", "skills", "review")) {
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
