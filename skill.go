package main

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

func skillStatus(path string) (string, error) {
	data, err := os.ReadFile(path)
	switch {
	case os.IsNotExist(err):
		return "missing", nil
	case err != nil:
		return "", err
	case !bytes.Equal(data, []byte(specSkill)):
		return "stale", nil
	}
	return "current", nil
}

func cmdSkill(out, errOut io.Writer, args []string) error {
	sub := "check"
	global := false
	for _, a := range args {
		switch a {
		case "--global":
			global = true
		case "check", "update":
			sub = a
		default:
			return fmt.Errorf("unknown argument %q (want check|update|--global)", a)
		}
	}
	var paths []string
	if global {
		home, err := os.UserHomeDir()
		if err != nil {
			return err
		}
		paths = []string{
			filepath.Join(home, ".config", "devin", "skills", "spec", "SKILL.md"),
			filepath.Join(home, ".claude", "skills", "spec", "SKILL.md"),
		}
	} else {
		st, err := mustStore()
		if err != nil {
			return err
		}
		paths = []string{
			filepath.Join(st.Root, ".devin", "skills", "spec", "SKILL.md"),
			filepath.Join(st.Root, ".claude", "skills", "spec", "SKILL.md"),
		}
	}
	outdated := 0
	for _, p := range paths {
		status, err := skillStatus(p)
		if err != nil {
			return err
		}
		if status != "current" {
			outdated++
			if sub == "update" {
				if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
					return err
				}
				if err := os.WriteFile(p, []byte(specSkill), 0o644); err != nil {
					return err
				}
				status = "updated"
			}
		}
		fmt.Fprintf(out, "%-8s %s\n", status, p)
	}
	if outdated > 0 && sub == "check" {
		return fmt.Errorf("%d skill file(s) not current — run 'spec skill update'", outdated)
	}
	return nil
}
