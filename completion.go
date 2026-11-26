package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// cmdComp describes how to complete one command's words. Everything is
// derived from this table so the three shell scripts stay consistent.
type cmdComp struct {
	subs     []string            // subcommand words offered at the first position
	flags    []string            // flags offered on "-" words
	args     []string            // completion kinds for positionals (last kind repeats)
	subArgs  map[string][]string // positional kinds after a subcommand
	subFlags map[string][]string // flags that only apply to a subcommand
	flagArgs map[string]string   // flags taking a value → the value's kind
}

// Completion kinds: "" and "file" fall back to the shell's default file
// completion; the rest map to candidates in completeCandidates.
var completionTable = map[string]cmdComp{
	"help":       {args: []string{"command"}},
	"completion": {args: []string{"shell"}, flags: []string{"--install"}},
	"ls":         {flags: []string{"--all", "-a", "--paths"}},
	"list":       {flags: []string{"--all", "-a", "--paths"}},
	"inbox":      {flags: []string{"--all", "-a", "--paths"}},
	"new":        {args: []string{"spec-type"}},
	"promote":    {flags: []string{"--force"}, args: []string{"spec-id", "spec-type"}},
	"discard":    {args: []string{"spec-id"}},
	"archive":    {flags: []string{"--force"}, args: []string{"spec-id"}},
	"unarchive":  {args: []string{"spec-id"}},
	"edit":       {args: []string{"spec-id"}},
	"refine":     {flags: []string{"--force"}, args: []string{"spec-id"}},
	"apply": {flags: []string{"--print", "--dry-run", "--run", "--force", "--yes", "-y", "--no-verify", "--no-review"},
		args: []string{"spec-id"}},
	"drift":    {flags: []string{"--review", "--yes"}, args: []string{"spec-id"}},
	"covers":   {flags: []string{"--infer"}, args: []string{"spec-id"}},
	"verified": {args: []string{"spec-id"}},
	"import":   {args: []string{"file"}},
	"export":   {args: []string{"file"}},
	"roadmap":  {args: []string{"file"}},
	"graph":    {flags: []string{"--mermaid"}},
	"web":      {flags: []string{"--port", "--host"}},
	"view": {flags: []string{"--cmd"}, flagArgs: map[string]string{"--cmd": "shell-cmd"},
		args: []string{"spec-id"}},
	"fact": {
		subs:     []string{"list", "get", "set", "unset", "discover"},
		subArgs:  map[string][]string{"get": {"fact-key"}, "set": {"fact-key"}, "unset": {"fact-key"}},
		subFlags: map[string][]string{"set": {"--secret"}, "discover": {"--yes"}},
	},
	"skill": {
		subs:  []string{"check", "sync", "update", "view", "which", "diff", "new", "edit", "refine", "use", "unuse", "adopt", "push", "import", "rm"},
		flags: []string{"--global", "--force", "--library", "--local", "--no-edit"},
		subArgs: map[string][]string{
			"use": {"skill"}, "unuse": {"skill"}, "adopt": {"skill"}, "push": {"skill"},
			"rm": {"skill"}, "edit": {"skill"}, "view": {"skill"}, "refine": {"skill"},
			"which": {"skill"}, "diff": {"skill"},
			"import": {"file"},
		},
	},
	"persona": {
		subs:    []string{"new", "edit", "refine", "use", "view"},
		subArgs: map[string][]string{"edit": {"persona"}, "refine": {"persona"}, "view": {"persona"}, "use": {"persona-or-none"}},
	},
	"role": {subs: []string{"edit", "refine", "adopt"}},
	"config": {
		subs:     []string{"init", "edit", "keys"},
		subArgs:  map[string][]string{"init": {"config-scope"}, "edit": {"config-scope"}},
		subFlags: map[string][]string{"init": {"--force"}},
	},
	"remind": {
		subs:     []string{"list", "add", "done", "remove"},
		subArgs:  map[string][]string{"done": {"reminder"}, "remove": {"reminder"}},
		flagArgs: map[string]string{"--every": "reminder-every", "--spec": "spec-id"},
	},
	"draft": {subs: []string{"feature"}},
	// no completions beyond the command name — listed so the table
	// provably covers every command
	"init": {}, "bootstrap": {}, "analyze": {}, "validate": {}, "trace": {},
	"status": {}, "ps": {}, "check": {}, "verify": {}, "precheck": {}, "push": {}, "idea": {}, "search": {},
	"accept":  {args: []string{"spec-id"}},
	"commit":  {flags: []string{"-m", "--exempt", "--agent", "--no-agent", "--no-check", "--verify"}},
	"release": {flags: []string{"--major"}},
	"update":  {},
}

// completionCommandNames is every word `spec <TAB>` offers — the command
// table plus help/version (handled in run, not the commands map).
func completionCommandNames() []string {
	names := make([]string, 0, len(commands)+3)
	for n := range commands {
		names = append(names, n)
	}
	names = append(names, "help", "version", "completion")
	sort.Strings(names)
	return names
}

// completeCandidates resolves a completion kind to its candidates. Kinds
// needing repo state return nil outside a spec tree — scripts silence
// stderr anyway. ""/"file"/"shell-cmd" have no candidates (native shell
// fallback handles them).
func completeCandidates(kind string) []string {
	switch kind {
	case "command":
		return completionCommandNames()
	case "shell":
		return []string{"bash", "zsh", "fish"}
	case "spec-type":
		t := make([]string, 0, len(validTypes))
		for ty := range validTypes {
			t = append(t, ty)
		}
		sort.Strings(t)
		return t
	case "config-scope":
		return []string{"global", "repo"}
	case "reminder-every":
		return []string{"7d", "30d", "weekly", "monthly", "quarterly", "yearly"}
	case "spec-id":
		st, err := mustStore()
		if err != nil {
			return nil
		}
		var ids []string
		for _, s := range st.Specs() {
			ids = append(ids, s.ID())
		}
		sort.Strings(ids)
		return ids
	case "skill":
		if st, err := mustStore(); err == nil {
			cfg := loadConfig(st.Root)
			return append(skillNames(st, cfg), availableSkills(st, cfg)...)
		}
		return append([]string{"spec"}, skillDirNames(librarySkillDir(""))...)
	case "persona":
		entries, _ := os.ReadDir(personaDir())
		var names []string
		for _, e := range entries {
			if !e.IsDir() && strings.HasSuffix(e.Name(), ".md") {
				names = append(names, strings.TrimSuffix(e.Name(), ".md"))
			}
		}
		sort.Strings(names)
		return names
	case "persona-or-none":
		return append(completeCandidates("persona"), "none")
	case "fact-key":
		st, err := mustStore()
		if err != nil {
			return nil
		}
		seen := map[string]bool{}
		var keys []string
		for _, f := range append(loadFacts(st.Root), loadSecretFacts(st.Root)...) {
			if !seen[f.Key] {
				seen[f.Key] = true
				keys = append(keys, f.Key)
			}
		}
		sort.Strings(keys)
		return keys
	case "reminder":
		st, err := mustStore()
		if err != nil {
			return nil
		}
		var ids []string
		for _, r := range loadReminders(st.Root) {
			ids = append(ids, r.ID)
		}
		return ids
	}
	return nil
}

// cmdCompleteKinds backs `spec __complete <kind>` — a script helper that
// prints one candidate per line. Silent outside a repo so completion
// degrades to the static parts.
func cmdCompleteKinds(out io.Writer, args []string) {
	if len(args) != 1 {
		return
	}
	for _, c := range completeCandidates(args[0]) {
		fmt.Fprintln(out, c)
	}
}

// cmdCompletion prints a shell completion script (spec completion <shell>).
func cmdCompletion(out, errOut io.Writer, args []string) error {
	install := false
	var rest []string
	for _, a := range args {
		if a == "--install" {
			install = true
		} else {
			rest = append(rest, a)
		}
	}
	if len(rest) != 1 {
		return fmt.Errorf("usage: spec completion <bash|zsh|fish> [--install]")
	}
	var script string
	switch rest[0] {
	case "bash":
		script = bashCompletion()
	case "zsh":
		script = zshCompletion()
	case "fish":
		script = fishCompletion()
	default:
		return fmt.Errorf("usage: spec completion <bash|zsh|fish> [--install]")
	}
	if !install {
		_, err := out.Write([]byte(script))
		return err
	}
	path, err := completionInstallPath(rest[0])
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	if err := writeFileAtomic(path, []byte(script), 0o644); err != nil {
		return err
	}
	fmt.Fprintf(out, "Wrote %s\n", path)
	if rest[0] == "zsh" {
		fmt.Fprintln(errOut, "note: zsh has no standard user dir — ensure fpath includes ~/.zfunc (fpath+=~/.zfunc before compinit)")
	}
	return nil
}

// completionInstallPath is the conventional per-user file each shell
// auto-loads completion scripts from.
func completionInstallPath(shell string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	switch shell {
	case "bash":
		return filepath.Join(home, ".local", "share", "bash-completion", "completions", "spec"), nil
	case "zsh":
		return filepath.Join(home, ".zfunc", "_spec"), nil
	case "fish":
		return filepath.Join(home, ".config", "fish", "completions", "spec.fish"), nil
	}
	return "", fmt.Errorf("usage: spec completion <bash|zsh|fish> [--install]")
}

// kindAction renders the bash/zsh/fish word for completing one kind.
// shell "words" are static lists; dynamic kinds shell out to
// `spec __complete <kind>`.
func bashKindAction(kind string) string {
	switch kind {
	case "", "file":
		return "" // default file completion
	case "shell-cmd":
		return `$(compgen -c -- "$cur")`
	case "shell":
		return `$(compgen -W "bash zsh fish" -- "$cur")`
	case "spec-type":
		return `$(compgen -W "` + strings.Join(completeCandidates("spec-type"), " ") + `" -- "$cur")`
	case "config-scope":
		return `$(compgen -W "global repo" -- "$cur")`
	case "reminder-every":
		return `$(compgen -W "7d 30d weekly monthly quarterly yearly" -- "$cur")`
	case "command":
		return `$(compgen -W "` + strings.Join(completionCommandNames(), " ") + `" -- "$cur")`
	}
	return `$(compgen -W "$(spec __complete ` + kind + ` 2>/dev/null)" -- "$cur")`
}

func bashCompletion() string {
	var b strings.Builder
	b.WriteString(`# bash completion for spec — generated by: spec completion bash
_spec() {
  local cur prev npos i
  COMPREPLY=()
  cur="${COMP_WORDS[COMP_CWORD]}"
  prev="${COMP_WORDS[COMP_CWORD-1]}"
  if (( COMP_CWORD == 1 )); then
    COMPREPLY=( $(compgen -W "` + strings.Join(completionCommandNames(), " ") + `" -- "$cur") )
    return 0
  fi
  local cmd="${COMP_WORDS[1]}"
`)
	// flag-value completions: prev word is a flag that takes a value
	b.WriteString(`  case "$cmd $prev" in
`)
	for _, c := range sortedCmds() {
		for f, k := range completionTable[c].flagArgs {
			fmt.Fprintf(&b, "    \"%s %s\") COMPREPLY=( %s ); return 0 ;;\n", c, f, bashKindAction(k))
		}
	}
	b.WriteString(`  esac
`)
	// "-" words: flags (command flags + current sub's flags)
	b.WriteString(`  if [[ "$cur" == -* ]]; then
    local f=""
    case "$cmd" in
`)
	for _, c := range sortedCmds() {
		comp := completionTable[c]
		all := append(append([]string{}, comp.flags...), collectSubFlags(comp)...)
		if len(all) > 0 {
			fmt.Fprintf(&b, "      %s) f=%q;;\n", c, strings.Join(all, " "))
		}
	}
	b.WriteString(`    esac
    COMPREPLY=( $(compgen -W "$f" -- "$cur") )
    return 0
  fi
`)
	// positional counting
	b.WriteString(`  local -a pos=()
  for (( i=2; i<COMP_CWORD; i++ )); do
    [[ "${COMP_WORDS[i]}" != -* ]] && pos+=("${COMP_WORDS[i]}")
  done
  npos=${#pos[@]}
  case "$cmd" in
`)
	for _, c := range sortedCmds() {
		comp := completionTable[c]
		if len(comp.subs) == 0 && len(comp.args) == 0 {
			continue
		}
		fmt.Fprintf(&b, "    %s)\n", c)
		if len(comp.subs) > 0 {
			fmt.Fprintf(&b, "      if (( npos == 0 )); then\n        COMPREPLY=( $(compgen -W %q -- \"$cur\") )\n      else\n        case \"${pos[0]}\" in\n", strings.Join(comp.subs, " "))
			// group subs by arg kind
			kindToSubs := map[string][]string{}
			var kindOrder []string
			for _, s := range comp.subs {
				k := ""
				if a, ok := comp.subArgs[s]; ok && len(a) > 0 {
					k = a[0]
				}
				if _, seen := kindToSubs[k]; !seen {
					kindOrder = append(kindOrder, k)
				}
				kindToSubs[k] = append(kindToSubs[k], s)
			}
			for _, k := range kindOrder {
				act := bashKindAction(k)
				fmt.Fprintf(&b, "          %s) COMPREPLY=( %s );;\n", strings.Join(kindToSubs[k], "|"), act)
			}
			b.WriteString("        esac\n      fi ;;\n")
		} else {
			if len(comp.args) == 1 {
				fmt.Fprintf(&b, "      COMPREPLY=( %s ) ;;\n", bashKindAction(comp.args[0]))
			} else {
				b.WriteString("      case \"$npos\" in\n")
				for i, k := range comp.args {
					if i == len(comp.args)-1 {
						fmt.Fprintf(&b, "        *) COMPREPLY=( %s );;\n", bashKindAction(k))
					} else {
						fmt.Fprintf(&b, "        %d) COMPREPLY=( %s );;\n", i, bashKindAction(k))
					}
				}
				b.WriteString("      esac ;;\n")
			}
		}
	}
	b.WriteString(`  esac
  return 0
}
complete -o default -o bashdefault -F _spec spec
`)
	return b.String()
}

func zshKindAction(kind string) string {
	switch kind {
	case "", "file":
		return "" // _files fallback
	case "shell-cmd":
		return `${(k)commands}`
	case "shell":
		return `bash zsh fish`
	case "spec-type":
		return strings.Join(completeCandidates("spec-type"), " ")
	case "config-scope":
		return `global repo`
	case "reminder-every":
		return `7d 30d weekly monthly quarterly yearly`
	case "command":
		return strings.Join(completionCommandNames(), " ")
	}
	return `${(f)"$(spec __complete ` + kind + ` 2>/dev/null)"}`
}

func zshCompletion() string {
	var b strings.Builder
	b.WriteString(`#compdef spec
# zsh completion for spec — generated by: spec completion zsh
_spec() {
  local -a pos
  local cmd npos i
  if (( CURRENT == 2 )); then
    compadd -- ` + strings.Join(completionCommandNames(), " ") + `
    return
  fi
  cmd="${words[2]}"
`)
	b.WriteString(`  case "$cmd ${words[CURRENT-1]}" in
`)
	for _, c := range sortedCmds() {
		for f, k := range completionTable[c].flagArgs {
			fmt.Fprintf(&b, "    \"%s %s\") compadd -- %s; return ;;\n", c, f, zshKindAction(k))
		}
	}
	b.WriteString(`  esac
  if [[ "${words[CURRENT]}" == -* ]]; then
    case "$cmd" in
`)
	for _, c := range sortedCmds() {
		comp := completionTable[c]
		all := append(append([]string{}, comp.flags...), collectSubFlags(comp)...)
		if len(all) > 0 {
			fmt.Fprintf(&b, "      %s) compadd -- %s ;;\n", c, strings.Join(all, " "))
		}
	}
	b.WriteString(`    esac
    return
  fi
  for (( i=3; i<CURRENT; i++ )); do
    [[ "${words[i]}" != -* ]] && pos+=("${words[i]}")
  done
  npos=${#pos[@]}
  case "$cmd" in
`)
	for _, c := range sortedCmds() {
		comp := completionTable[c]
		if len(comp.subs) == 0 && len(comp.args) == 0 {
			continue
		}
		fmt.Fprintf(&b, "    %s)\n", c)
		if len(comp.subs) > 0 {
			fmt.Fprintf(&b, "      if (( npos == 0 )); then\n        compadd -- %s ; return\n      else\n        case \"${pos[1]}\" in\n", strings.Join(comp.subs, " "))
			kindToSubs := map[string][]string{}
			var kindOrder []string
			for _, s := range comp.subs {
				k := ""
				if a, ok := comp.subArgs[s]; ok && len(a) > 0 {
					k = a[0]
				}
				if _, seen := kindToSubs[k]; !seen {
					kindOrder = append(kindOrder, k)
				}
				kindToSubs[k] = append(kindToSubs[k], s)
			}
			for _, k := range kindOrder {
				fmt.Fprintf(&b, "          %s) compadd -- %s ; return ;;\n", strings.Join(kindToSubs[k], "|"), zshKindAction(k))
			}
			b.WriteString("        esac\n      fi ; return ;;\n")
		} else {
			if len(comp.args) == 1 {
				fmt.Fprintf(&b, "      compadd -- %s ; return ;;\n", zshKindAction(comp.args[0]))
			} else {
				b.WriteString("      case \"$npos\" in\n")
				for i, k := range comp.args {
					if i == len(comp.args)-1 {
						fmt.Fprintf(&b, "        *) compadd -- %s ; return ;;\n", zshKindAction(k))
					} else {
						fmt.Fprintf(&b, "        %d) compadd -- %s ; return ;;\n", i, zshKindAction(k))
					}
				}
				b.WriteString("      esac ;;\n")
			}
		}
	}
	b.WriteString(`  esac
  _files
}
compdef _spec spec
`)
	return b.String()
}

func fishCompletion() string {
	var b strings.Builder
	b.WriteString("# fish completion for spec — generated by: spec completion fish\n")
	b.WriteString("complete -c spec -n '__fish_use_subcommand' -f -a '" +
		strings.Join(completionCommandNames(), " ") + "'\n")
	fishSeen := func(cmd string) string {
		return "__fish_seen_subcommand_from " + cmd
	}
	for _, c := range sortedCmds() {
		comp := completionTable[c]
		cond := fishSeen(c)
		if len(comp.subs) > 0 {
			fmt.Fprintf(&b, "complete -c spec -n '%s; and not __fish_seen_subcommand_from %s' -f -a '%s'\n",
				cond, strings.Join(comp.subs, " "), strings.Join(comp.subs, " "))
		}
		// flags
		var longs, shorts []string
		for _, f := range comp.flags {
			if strings.HasPrefix(f, "--") {
				longs = append(longs, strings.TrimPrefix(f, "--"))
			} else {
				shorts = append(shorts, strings.TrimPrefix(f, "-"))
			}
		}
		for _, l := range longs {
			fmt.Fprintf(&b, "complete -c spec -n '%s' -l %s\n", cond, l)
		}
		for _, s := range shorts {
			fmt.Fprintf(&b, "complete -c spec -n '%s' -s %s\n", cond, s)
		}
		// sub-scoped flags
		for sub, fs := range comp.subFlags {
			for _, f := range fs {
				fmt.Fprintf(&b, "complete -c spec -n '%s; and %s' -l %s\n",
					cond, fishSeen(sub), strings.TrimPrefix(f, "--"))
			}
		}
		// flag args
		for f, k := range comp.flagArgs {
			fmt.Fprintf(&b, "complete -c spec -n '%s' -l %s -rf -a '%s'\n",
				cond, strings.TrimPrefix(f, "--"), fishKindWords(k))
		}
		// args for no-sub commands (first kind only — repeats anyway)
		if len(comp.subs) == 0 && len(comp.args) > 0 && comp.args[0] != "" && comp.args[0] != "file" {
			fmt.Fprintf(&b, "complete -c spec -n '%s' -f -a '%s'\n", cond, fishKindWords(comp.args[0]))
		}
		// args after subs: one line per (sub-set, kind)
		if len(comp.subs) > 0 {
			kindToSubs := map[string][]string{}
			var kindOrder []string
			for _, s := range comp.subs {
				a, ok := comp.subArgs[s]
				k := ""
				if ok && len(a) > 0 {
					k = a[0]
				}
				if k == "" || k == "file" {
					continue
				}
				if _, seen := kindToSubs[k]; !seen {
					kindOrder = append(kindOrder, k)
				}
				kindToSubs[k] = append(kindToSubs[k], s)
			}
			for _, k := range kindOrder {
				fmt.Fprintf(&b, "complete -c spec -n '%s; and %s' -f -a '%s'\n",
					cond, fishSeen(strings.Join(kindToSubs[k], " ")), fishKindWords(k))
			}
		}
	}
	return b.String()
}

// fishKindWords renders a kind for fish's -a: static words or a command
// substitution producing dynamic candidates.
func fishKindWords(kind string) string {
	switch kind {
	case "shell":
		return "bash zsh fish"
	case "spec-type":
		return strings.Join(completeCandidates("spec-type"), " ")
	case "config-scope":
		return "global repo"
	case "reminder-every":
		return "7d 30d weekly monthly quarterly yearly"
	case "command":
		return strings.Join(completionCommandNames(), " ")
	case "shell-cmd":
		return "(__fish_complete_command)"
	}
	return "(spec __complete " + kind + " 2>/dev/null)"
}

func collectSubFlags(c cmdComp) []string {
	var out []string
	for _, fs := range c.subFlags {
		out = append(out, fs...)
	}
	return out
}

func sortedCmds() []string {
	var out []string
	for c := range completionTable {
		out = append(out, c)
	}
	sort.Strings(out)
	return out
}
