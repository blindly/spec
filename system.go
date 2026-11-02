package main

import (
	"os"
	"path/filepath"
)

// inSystem reports whether s describes the current system (FEAT-011): an
// implemented feature/project, an accepted ADR (a standing decision — ADRs
// are never "implemented"), or a still-valid imported doc. These live on
// the /system page; the board holds work in progress.
func inSystem(cfg Config, s Spec) bool {
	if s.Archived {
		return false
	}
	switch s.Type() {
	case "feature", "project":
		return s.Status() == "implemented"
	case "adr":
		return s.Status() == "accepted"
	case "imported":
		return !cfg.isDone(s.Status())
	}
	return false
}

// lastChanged maps spec file paths to the date (YYYY-MM-DD) they last
// changed: the newest git commit touching each file, from the history
// already loaded in g. Files git doesn't know about (or no git at all) fall
// back to the file's modification time.
func lastChanged(st Store, g *gitRepo, specs []Spec) map[string]string {
	dates := map[string]string{}
	if g != nil {
		for _, c := range g.commits { // newest first: the first date seen wins
			for _, f := range c.Files {
				p := filepath.Join(st.Root, filepath.FromSlash(f))
				if _, seen := dates[p]; !seen {
					dates[p] = c.Date
				}
			}
		}
	}
	for _, s := range specs {
		if _, ok := dates[s.Path]; ok {
			continue
		}
		if fi, err := os.Stat(s.Path); err == nil {
			dates[s.Path] = fi.ModTime().Format("2006-01-02")
		}
	}
	return dates
}
