package main

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestViewDefault(t *testing.T) {
	st := setupRepo(t)
	t.Setenv("PAGER", "missing-pager")
	t.Setenv("EDITOR", "missing-editor")
	first := featureWithStatus(t, st, "first", "accepted")
	second := featureWithStatus(t, st, "second", "deprecated")
	if err := cmdArchive(io.Discard, []string{second}); err != nil {
		t.Fatal(err)
	}
	idea := newIdea(t, st, "notes", nil)
	var want strings.Builder
	for i, id := range []string{first, second, idea.ID()} {
		s := st.Get(id)
		if i > 0 {
			want.WriteString("\n")
		}
		fmt.Fprintf(&want, "%s · %s · %s — %s\n---\n%s", s.ID(), s.Type(), s.Status(), s.Title(), s.Body)
		if !strings.HasSuffix(s.Body, "\n") {
			want.WriteString("\n")
		}
	}
	var out, errs bytes.Buffer
	if code := run([]string{"view", first, second, idea.ID()}, &out, &errs); code != 0 || out.String() != want.String() {
		t.Fatalf("code=%d output=%q errors=%s want=%q", code, out.String(), errs.String(), want.String())
	}
	out.Reset()
	errs.Reset()
	if code := run([]string{"view", first, "NO-SUCH-ID"}, &out, &errs); code != 1 || !strings.Contains(errs.String(), "NO-SUCH-ID") || !strings.Contains(out.String(), "first") {
		t.Fatalf("code=%d output=%s errors=%s", code, out.String(), errs.String())
	}
}

func TestViewViewer(t *testing.T) {
	for _, tc := range []struct {
		name, config, command string
	}{
		{"flag appended", "", "cat"},
		{"flag placeholder", "", "cat '{file}'"},
		{"config", "cat", ""},
		{"config quoted", "cat '{file}'", ""},
		{"precedence", "missing-config-viewer", "cat"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			st := setupRepo(t)
			first := featureWithStatus(t, st, "first", "accepted")
			second := featureWithStatus(t, st, "second", "accepted")
			// Spaces in paths must remain one argument after substitution.
			s := st.Get(first)
			path := filepath.Join(filepath.Dir(s.Path), "with spaces.md")
			if err := os.Rename(s.Path, path); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(st.Root, configDir, "config.yml"), []byte("viewer: \""+tc.config+"\"\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			args := []string{"view"}
			if tc.command != "" {
				args = append(args, "--cmd", tc.command)
			}
			args = append(args, first, second)
			var want string
			for _, id := range []string{first, second} {
				data, err := os.ReadFile(st.Get(id).Path)
				if err != nil {
					t.Fatal(err)
				}
				want += string(data)
			}
			var out, errs bytes.Buffer
			if code := run(args, &out, &errs); code != 0 || out.String() != want {
				t.Fatalf("code=%d output=%q errors=%s want=%q", code, out.String(), errs.String(), want)
			}
		})
	}
}

func TestViewErrors(t *testing.T) {
	st := setupRepo(t)
	id := featureWithStatus(t, st, "test", "accepted")
	for _, args := range [][]string{
		{"view"},
		{"view", "--cmd"},
		{"view", "--unknown", id},
		{"view", "--cmd", "spec-nonexistent-viewer-binary", id},
		{"view", "--cmd", "cat 'unterminated", id},
		{"view", "--cmd", "   ", id},
	} {
		var out, errs bytes.Buffer
		if code := run(args, &out, &errs); code == 0 || errs.Len() == 0 || out.Len() != 0 {
			t.Fatalf("%v: code=%d output=%s errors=%s", args, code, out.String(), errs.String())
		}
		if strings.Contains(strings.Join(args, " "), "spec-nonexistent") && !strings.Contains(errs.String(), "spec-nonexistent-viewer-binary") {
			t.Fatalf("missing binary not named: %s", errs.String())
		}
	}
}

func TestViewDocumentation(t *testing.T) {
	if !strings.Contains(usage, "spec view [--cmd CMD] <id>...") {
		t.Fatal("view missing from usage")
	}
	for _, template := range []string{repoConfigTemplate, globalConfigTemplate} {
		if !strings.Contains(template, "# viewer:") || !strings.Contains(template, "{file}") {
			t.Fatal("viewer missing from config template")
		}
	}
}
