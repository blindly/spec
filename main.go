package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
)

var version = "dev" // overridden by -ldflags "-X main.version=x.y.z"

var commands = map[string]func(args []string, out, errOut io.Writer) error{
	"init":      func(a []string, o, e io.Writer) error { return cmdInit(o, e) },
	"bootstrap": func(a []string, o, e io.Writer) error { return cmdBootstrap(o, e) },
	"analyze":   func(a []string, o, e io.Writer) error { return cmdAnalyze(o, e) },
	"validate":  func(a []string, o, e io.Writer) error { return cmdValidate(o, e) },
	"trace":     func(a []string, o, e io.Writer) error { return cmdTrace(o, e) },
	"config": func(a []string, o, e io.Writer) error {
		return cmdConfig(o, e, a)
	},
	"status":  func(a []string, o, e io.Writer) error { return cmdStatus(o, e) },
	"ps":      func(a []string, o, e io.Writer) error { return cmdPs(o, e) },
	"commit":  func(a []string, o, e io.Writer) error { return cmdCommit(o, e, a) },
	"release": func(a []string, o, e io.Writer) error { return cmdRelease(o, e, a) },
	"ls":      func(a []string, o, e io.Writer) error { return cmdList(o, e, a) },
	"list":    func(a []string, o, e io.Writer) error { return cmdList(o, e, a) },
	"inbox":   func(a []string, o, e io.Writer) error { return cmdInbox(o, e, a) },
	"role":    cmdRole,
	"persona": cmdPersona,
	"fact":    func(a []string, o, e io.Writer) error { return cmdFact(o, e, a) },
	"remind":  func(a []string, o, e io.Writer) error { return cmdRemind(o, e, a) },
	"check":   func(a []string, o, e io.Writer) error { return cmdCheck(o, e) },
	"verify":  func(a []string, o, e io.Writer) error { return cmdVerify(o, e) },
	"update": func(a []string, o, e io.Writer) error {
		tag := ""
		if len(a) > 0 {
			tag = a[0]
		}
		return cmdUpdate(o, e, tag)
	},
	"idea": func(a []string, o, e io.Writer) error {
		if len(a) < 1 {
			return fmt.Errorf("usage: spec idea <title>")
		}
		return cmdNew(o, e, "idea", strings.Join(a, " "))
	},
	"discard":   func(a []string, o, e io.Writer) error { return cmdDiscard(o, a) },
	"archive":   func(a []string, o, e io.Writer) error { return cmdArchive(o, a) },
	"unarchive": func(a []string, o, e io.Writer) error { return cmdUnarchive(o, a) },
	"promote":   func(a []string, o, e io.Writer) error { return cmdPromote(o, a) },
	"accept":    func(a []string, o, e io.Writer) error { return cmdAccept(o, a) },
	"drift":     func(a []string, o, e io.Writer) error { return cmdDrift(os.Stdin, o, e, a) },
	"covers":    func(a []string, o, e io.Writer) error { return cmdCovers(o, a) },
	"verified":  func(a []string, o, e io.Writer) error { return cmdVerified(o, a) },
	"skill":     func(a []string, o, e io.Writer) error { return cmdSkill(o, e, a) },
	"new": func(a []string, o, e io.Writer) error {
		if len(a) < 2 {
			return fmt.Errorf("usage: spec new <project|feature|adr|idea> <title>")
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
		var opts applyOpts
		for _, arg := range a {
			switch arg {
			case "--print", "--dry-run":
				opts.Print = true
			case "--run": // kept for compatibility; running is the default
			case "--force":
				opts.Force = true
			case "--yes", "-y":
				opts.Yes = true
			case "--no-verify":
				opts.NoVerify = true
			case "--no-review":
				opts.NoReview = true
			default:
				if strings.HasPrefix(arg, "--") {
					return fmt.Errorf("unknown flag %s", arg)
				}
				ids = append(ids, arg)
			}
		}
		return cmdApply(os.Stdin, o, e, ids, opts)
	},
	"view": cmdView,
	"edit": func(a []string, o, e io.Writer) error {
		if len(a) < 1 {
			return fmt.Errorf("usage: spec edit <id>")
		}
		return cmdEdit(o, e, a[0])
	},
	"refine": func(a []string, o, e io.Writer) error {
		var ids []string
		force := false
		for _, arg := range a {
			switch {
			case arg == "--force":
				force = true
			case strings.HasPrefix(arg, "--"):
				return fmt.Errorf("unknown flag %s", arg)
			default:
				ids = append(ids, arg)
			}
		}
		if len(ids) != 1 {
			return fmt.Errorf("usage: spec refine <id> [--force]")
		}
		return refine(os.Stdin, o, e, ids[0], force)
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
	for _, a := range args[1:] {
		if a == "--help" || a == "-h" {
			if doc, ok := commandDocByName[args[0]]; ok {
				fmt.Fprintln(out, doc)
				return 0
			}
			break
		}
	}
	switch args[0] {
	case "version", "--version":
		fmt.Fprintln(out, "spec "+version)
		return 0
	case "completion":
		if err := cmdCompletion(out, errOut, args[1:]); err != nil {
			fmt.Fprintln(errOut, "error:", err)
			return 1
		}
		return 0
	case "help", "--help", "-h":
		if len(args) > 1 {
			doc, ok := commandDocByName[args[1]]
			if !ok {
				fmt.Fprintf(errOut, "spec: unknown command %q\n\n", args[1])
				fmt.Fprint(errOut, usage)
				return 1
			}
			fmt.Fprintln(out, doc)
			return 0
		}
		fmt.Fprint(out, usage)
		return 0
	case "__complete":
		cmdCompleteKinds(out, args[1:])
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
