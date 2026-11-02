package main

import (
	"bufio"
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
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
// changed: the newest git commit touching each file under specs/, from a
// single git log call. Files git doesn't know about (or no git at all) fall
// back to the file's modification time.
func lastChanged(st Store, specs []Spec) map[string]string {
	dates := map[string]string{}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", "log", "--format=%x00%cs", "--name-only", "--relative", "--", specDir)
	cmd.Dir = st.Root
	if out, err := cmd.Output(); err == nil {
		date := ""
		sc := bufio.NewScanner(bytes.NewReader(out))
		for sc.Scan() {
			line := sc.Text()
			switch {
			case strings.HasPrefix(line, "\x00"):
				date = line[1:]
			case line != "" && date != "":
				// git log is newest-first: the first date seen for a file wins
				p := filepath.Join(st.Root, filepath.FromSlash(line))
				if _, seen := dates[p]; !seen {
					dates[p] = date
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
