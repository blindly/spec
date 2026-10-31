package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
)

var version = "dev" // overridden by -ldflags "-X main.version=x.y.z"

const usage = `spec — offline specification management

Usage:
  spec init                          Initialize repository
  spec new <project|feature|adr> <title>   Create a specification
  spec bootstrap                     Bootstrap specs from existing project
  spec import <file>                 Import existing documentation
  spec analyze                       Analyze repository
  spec search <query>                Search specifications
  spec validate                      Validate specifications
  spec export [file.json]            Export metadata as JSON
  spec roadmap [file.md]             Generate roadmap
  spec graph [--mermaid]             Dependency graph
  spec trace                         Requirements traceability
  spec draft feature <title>         Generate feature draft (LLM agent if configured)
  spec edit <id>                     Open a spec in your editor
  spec refine <id>                   LLM agent improves an existing spec
  spec apply [--print|--force] [id...] Run agent to implement specs; --print outputs the prompt(s)
  spec status                        Summarize repository spec state
  spec fact [list|get|set|unset]     Manage .spec/facts.yml project facts
  spec check                         Audit {fact:key} references vs facts.yml
  spec verify                        Run configured verification commands
  spec config                        Show effective configuration
  spec config init [global|repo] [--force]   Write config template (--force regenerates)
  spec config edit [global|repo]     Open config file in $EDITOR (or editor: key)
  spec web [--port N]                Web interface (default: web_port from config)
  spec update [tag]                  Self-update to a release (default: latest)
  spec version                       Print version

Agent configuration lives in .spec/config.yml:
  agent: none|claude|devin|gemini|codex|aider|opencode|amp|cline|vibe   (default: none)
  agent_cmd: "mycli --flag {prompt}" # {prompt} = prompt arg; absent = prompt via stdin
  agent_timeout: 3600                 # seconds
`

var commands = map[string]func(args []string, out, errOut io.Writer) error{
	"init":      func(a []string, o, e io.Writer) error { return cmdInit(o, e) },
	"bootstrap": func(a []string, o, e io.Writer) error { return cmdBootstrap(o, e) },
	"analyze":   func(a []string, o, e io.Writer) error { return cmdAnalyze(o, e) },
	"validate":  func(a []string, o, e io.Writer) error { return cmdValidate(o, e) },
	"trace":     func(a []string, o, e io.Writer) error { return cmdTrace(o, e) },
	"config": func(a []string, o, e io.Writer) error {
		return cmdConfig(o, e, a)
	},
	"status": func(a []string, o, e io.Writer) error { return cmdStatus(o, e) },
	"fact":   func(a []string, o, e io.Writer) error { return cmdFact(o, e, a) },
	"check":  func(a []string, o, e io.Writer) error { return cmdCheck(o, e) },
	"verify": func(a []string, o, e io.Writer) error { return cmdVerify(o, e) },
	"update": func(a []string, o, e io.Writer) error {
		tag := ""
		if len(a) > 0 {
			tag = a[0]
		}
		return cmdUpdate(o, e, tag)
	},
	"new": func(a []string, o, e io.Writer) error {
		if len(a) < 2 {
			return fmt.Errorf("usage: spec new <project|feature|adr> <title>")
		}
		return cmdNew(o, e, a[0], strings.Join(a[1:], " "))
	},
	"import": func(a []string, o, e io.Writer) error {
		if len(a) < 1 {
			return fmt.Errorf("usage: spec import <file>")
		}
		return cmdImport(o, e, a[0])
	},
	"search": func(a []string, o, e io.Writer) error {
		if len(a) < 1 {
			return fmt.Errorf("usage: spec search <query>")
		}
		return cmdSearch(o, e, strings.Join(a, " "))
	},
	"export": func(a []string, o, e io.Writer) error {
		file := ""
		if len(a) > 0 {
			file = a[0]
		}
		return cmdExport(o, e, file)
	},
	"roadmap": func(a []string, o, e io.Writer) error {
		file := ""
		if len(a) > 0 {
			file = a[0]
		}
		return cmdRoadmap(o, e, file)
	},
	"apply": func(a []string, o, e io.Writer) error {
		// extract flags manually: Go's flag pkg stops at the first
		// positional arg, so `spec apply ID --force` would lose --force
		var ids []string
		var printOnly, force bool
		for _, arg := range a {
			switch arg {
			case "--print", "--dry-run":
				printOnly = true
			case "--run": // kept for compatibility; running is the default
			case "--force":
				force = true
			default:
				if strings.HasPrefix(arg, "--") {
					return fmt.Errorf("unknown flag %s", arg)
				}
				ids = append(ids, arg)
			}
		}
		return cmdApply(o, e, ids, printOnly, force)
	},
	"edit": func(a []string, o, e io.Writer) error {
		if len(a) < 1 {
			return fmt.Errorf("usage: spec edit <id>")
		}
		return cmdEdit(o, e, a[0])
	},
	"refine": func(a []string, o, e io.Writer) error {
		if len(a) < 1 {
			return fmt.Errorf("usage: spec refine <id>")
		}
		return cmdRefine(o, e, a[0])
	},
	"draft": func(a []string, o, e io.Writer) error {
		if len(a) < 2 || a[0] != "feature" {
			return fmt.Errorf("usage: spec draft feature <title>")
		}
		return cmdDraftFeature(o, e, strings.Join(a[1:], " "))
	},
	"graph": func(a []string, o, e io.Writer) error {
		fs := flag.NewFlagSet("graph", flag.ContinueOnError)
		fs.SetOutput(e)
		mermaid := fs.Bool("mermaid", false, "emit mermaid diagram")
		if err := fs.Parse(a); err != nil {
			return err
		}
		return cmdGraph(o, e, *mermaid)
	},
	"web": func(a []string, o, e io.Writer) error {
		fs := flag.NewFlagSet("web", flag.ContinueOnError)
		fs.SetOutput(e)
		port := fs.Int("port", 0, "port to listen on (0 = web_port from config)")
		host := fs.String("host", "", "bind address (e.g. 127.0.0.1, 0.0.0.0; empty = web_host from config)")
		if err := fs.Parse(a); err != nil {
			return err
		}
		return cmdWeb(o, e, *host, *port)
	},
}

// run dispatches a command; returns a process exit code.
func run(args []string, out, errOut io.Writer) int {
	if len(args) == 0 {
		fmt.Fprint(out, usage)
		return 1
	}
	switch args[0] {
	case "version", "--version":
		fmt.Fprintln(out, "spec "+version)
		return 0
	case "help", "--help", "-h":
		fmt.Fprint(out, usage)
		return 0
	}
	fn, ok := commands[args[0]]
	if !ok {
		fmt.Fprintf(errOut, "spec: unknown command %q\n\n", args[0])
		fmt.Fprint(errOut, usage)
		return 1
	}
	if err := fn(args[1:], out, errOut); err != nil {
		fmt.Fprintln(errOut, "error:", err)
		return 1
	}
	return 0
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}
