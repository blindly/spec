package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
)

func cmdView(args []string, out, errOut io.Writer) error {
	fs := flag.NewFlagSet("view", flag.ContinueOnError)
	fs.SetOutput(errOut)
	viewer := fs.String("cmd", "", "external viewer command")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() == 0 {
		return fmt.Errorf("usage: spec view [--cmd CMD] <id>...")
	}
	st, err := mustStore()
	if err != nil {
		return err
	}
	command := loadConfig(st.Root).Viewer
	fs.Visit(func(f *flag.Flag) {
		if f.Name == "cmd" {
			command = *viewer
		}
	})
	for i, id := range fs.Args() {
		s := st.Get(id)
		if s == nil {
			return fmt.Errorf("no spec with id %s", id)
		}
		if command == "" {
			if i > 0 {
				if _, err := fmt.Fprintln(out); err != nil {
					return err
				}
			}
			if _, err := fmt.Fprintf(out, "%s · %s · %s — %s\n---\n%s", s.ID(), s.Type(), s.Status(), s.Title(), s.Body); err != nil {
				return err
			}
			if !strings.HasSuffix(s.Body, "\n") {
				if _, err := fmt.Fprintln(out); err != nil {
					return err
				}
			}
			continue
		}
		argv, err := splitArgs(command)
		if err != nil {
			return fmt.Errorf("viewer %q: %w", command, err)
		}
		if len(argv) == 0 {
			return fmt.Errorf("viewer command is empty")
		}
		hasFile := false
		for j, arg := range argv {
			if strings.Contains(arg, "{file}") {
				hasFile = true
				argv[j] = strings.ReplaceAll(arg, "{file}", s.Path)
			}
		}
		if !hasFile {
			argv = append(argv, s.Path)
		}
		cmd := exec.Command(argv[0], argv[1:]...)
		cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, out, errOut
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("viewer %q for %s: %w", command, id, err)
		}
	}
	return nil
}
