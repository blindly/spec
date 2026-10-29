package main

import (
	"flag"
	"fmt"
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
  spec refine <id>                   LLM agent improves an existing spec
  spec config                        Show effective configuration
  spec web [--port N]                Web interface (default :8080)
  spec update [tag]                  Self-update to a release (default: latest)
  spec version                       Print version

Agent configuration lives in .spec/config.yml:
  agent: none|claude|gemini|codex|aider|opencode|amp   (default: none)
  agent_cmd: "mycli --flag {prompt}" # {prompt} = prompt arg; absent = prompt via stdin
  agent_timeout: 300                 # seconds
`

func main() {
	if len(os.Args) < 2 {
		fmt.Print(usage)
		os.Exit(1)
	}
	var err error
	switch os.Args[1] {
	case "init":
		err = cmdInit()
	case "new":
		if len(os.Args) < 4 {
			err = fmt.Errorf("usage: spec new <project|feature|adr> <title>")
		} else {
			err = cmdNew(os.Args[2], strings.Join(os.Args[3:], " "))
		}
	case "bootstrap":
		err = cmdBootstrap()
	case "import":
		if len(os.Args) < 3 {
			err = fmt.Errorf("usage: spec import <file>")
		} else {
			err = cmdImport(os.Args[2])
		}
	case "analyze":
		err = cmdAnalyze()
	case "search":
		if len(os.Args) < 3 {
			err = fmt.Errorf("usage: spec search <query>")
		} else {
			err = cmdSearch(strings.Join(os.Args[2:], " "))
		}
	case "validate":
		err = cmdValidate()
	case "export":
		out := ""
		if len(os.Args) > 2 {
			out = os.Args[2]
		}
		err = cmdExport(out)
	case "roadmap":
		out := ""
		if len(os.Args) > 2 {
			out = os.Args[2]
		}
		err = cmdRoadmap(out)
	case "graph":
		fs := flag.NewFlagSet("graph", flag.ContinueOnError)
		mermaid := fs.Bool("mermaid", false, "emit mermaid diagram")
		fs.Parse(os.Args[2:])
		err = cmdGraph(*mermaid)
	case "trace":
		err = cmdTrace()
	case "refine":
		if len(os.Args) < 3 {
			err = fmt.Errorf("usage: spec refine <id>")
		} else {
			err = cmdRefine(os.Args[2])
		}
	case "config":
		err = cmdConfig()
	case "draft":
		if len(os.Args) < 4 || os.Args[2] != "feature" {
			err = fmt.Errorf("usage: spec draft feature <title>")
		} else {
			err = cmdDraftFeature(strings.Join(os.Args[3:], " "))
		}
	case "update":
		tag := ""
		if len(os.Args) > 2 {
			tag = os.Args[2]
		}
		err = cmdUpdate(tag)
	case "version", "--version":
		fmt.Println("spec " + version)
	case "web":
		fs := flag.NewFlagSet("web", flag.ContinueOnError)
		port := fs.Int("port", 0, "port to listen on (0 = web_port from config)")
		fs.Parse(os.Args[2:])
		err = cmdWeb(*port)
	case "help", "--help", "-h":
		fmt.Print(usage)
	default:
		fmt.Print(usage)
		os.Exit(1)
	}
	if err != nil {
		fatal(err)
	}
}
