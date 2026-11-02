package main

import (
	"fmt"
	"io"
	"path/filepath"
	"strings"
)

func relPath(st Store, path string) string {
	if rel, err := filepath.Rel(st.Root, path); err == nil {
		return rel
	}
	return path
}

// archivable reports whether s may be archived without --force: finished
// specs (done_statuses) and discarded ideas.
func archivable(cfg Config, s Spec) bool {
	if s.IsIdea() {
		return s.Status() == ideaDiscarded
	}
	return cfg.isDone(s.Status())
}

// archiveArgs splits ids from a --force flag.
func archiveArgs(args []string, usage string) (ids []string, force bool, err error) {
	for _, a := range args {
		switch {
		case a == "--force":
			force = true
		case strings.HasPrefix(a, "--"):
			return nil, false, fmt.Errorf("unknown flag %s", a)
		default:
			ids = append(ids, a)
		}
	}
	if len(ids) == 0 {
		return nil, false, fmt.Errorf("usage: %s", usage)
	}
	return ids, force, nil
}

func cmdArchive(out io.Writer, args []string) error {
	ids, force, err := archiveArgs(args, "spec archive <id>... [--force]")
	if err != nil {
		return err
	}
	st, err := mustStore()
	if err != nil {
		return err
	}
	cfg := loadConfig(st.Root)
	// check every id before moving anything
	var targets []Spec
	for _, id := range ids {
		s := st.Get(id)
		if s == nil {
			return fmt.Errorf("no spec with id %s", id)
		}
		if s.Archived {
			return fmt.Errorf("%s is already archived", id)
		}
		if !archivable(cfg, *s) && !force {
			want := "a finished status (" + strings.Join(cfg.DoneStatuses, ", ") + ")"
			if s.IsIdea() {
				want = "status discarded"
			}
			return fmt.Errorf("%s is %s — only specs with %s can be archived (use --force to override)", id, s.Status(), want)
		}
		targets = append(targets, *s)
	}
	for _, s := range targets {
		dst, err := st.Archive(s)
		if err != nil {
			return err
		}
		fmt.Fprintf(out, "Archived %s → %s\n", s.ID(), relPath(st, dst))
	}
	return nil
}

func cmdUnarchive(out io.Writer, args []string) error {
	ids, _, err := archiveArgs(args, "spec unarchive <id>...")
	if err != nil {
		return err
	}
	st, err := mustStore()
	if err != nil {
		return err
	}
	for _, id := range ids {
		s := st.Get(id)
		if s == nil {
			return fmt.Errorf("no spec with id %s", id)
		}
		dst, err := st.Unarchive(*s)
		if err != nil {
			return err
		}
		fmt.Fprintf(out, "Unarchived %s → %s\n", id, relPath(st, dst))
	}
	return nil
}
