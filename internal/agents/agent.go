// Package agents defines the pluggable AI-agent abstraction and a registry.
//
// New agents are added by creating a small subpackage that calls Register in
// its init function (see internal/agents/claude). The core never needs to
// change. Each agent turns an issue + prepared context into a terminal.Spec;
// the terminal package decides how that spec is actually launched.
package agents

import (
	"os/exec"
	"sort"
	"strings"

	"github.com/GustavoMinelli/sprintmate/internal/jira"
	"github.com/GustavoMinelli/sprintmate/internal/terminal"
)

// Params carries everything an agent may need to build its launch command.
type Params struct {
	Issue       jira.Issue
	ContextPath string // path to the generated .issue-context.md
	Branch      string
	Dir         string // working directory for the agent
}

// Config is the per-agent launch configuration (from config.Agents[name]).
type Config struct {
	Command string
	Args    []string
}

// Agent is the small interface every supported agent implements.
type Agent interface {
	Name() string
	IsInstalled() bool
	Spec(p Params, cfg Config) terminal.Spec
}

// --- registry -------------------------------------------------------------

var registry = map[string]func() Agent{}

// Register adds an agent factory under name. Intended to be called from a
// subpackage's init function.
func Register(name string, factory func() Agent) {
	registry[name] = factory
}

// Get returns the agent registered under name.
func Get(name string) (Agent, bool) {
	f, ok := registry[name]
	if !ok {
		return nil, false
	}
	return f(), true
}

// Names returns all registered agent names, sorted.
func Names() []string {
	names := make([]string, 0, len(registry))
	for n := range registry {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// Installed returns the names of registered agents whose command is on PATH.
func Installed() []string {
	var out []string
	for _, n := range Names() {
		if a, ok := Get(n); ok && a.IsInstalled() {
			out = append(out, n)
		}
	}
	return out
}

// --- builtin implementation ----------------------------------------------

// builtin is the default Agent implementation: it runs a configurable command
// with placeholder-expanded arguments. Both Claude Code and Codex use it.
type builtin struct {
	name           string
	defaultCommand string
	defaultArgs    []string
}

// NewBuiltin constructs a command-based agent. Subpackages use this in their
// Register call.
func NewBuiltin(name, defaultCommand string, defaultArgs []string) Agent {
	return builtin{name: name, defaultCommand: defaultCommand, defaultArgs: defaultArgs}
}

func (b builtin) Name() string { return b.name }

func (b builtin) IsInstalled() bool {
	_, err := exec.LookPath(b.defaultCommand)
	return err == nil
}

func (b builtin) Spec(p Params, cfg Config) terminal.Spec {
	command := cfg.Command
	if strings.TrimSpace(command) == "" {
		command = b.defaultCommand
	}
	rawArgs := cfg.Args
	if rawArgs == nil {
		rawArgs = b.defaultArgs
	}
	return terminal.Spec{
		Bin:  command,
		Args: ExpandArgs(rawArgs, p),
		Dir:  p.Dir,
	}
}

// ExpandArgs substitutes placeholders in each arg and drops args that expand to
// empty (e.g. {context_file} when no context was generated).
func ExpandArgs(args []string, p Params) []string {
	repl := strings.NewReplacer(
		"{context_file}", p.ContextPath,
		"{key}", p.Issue.Key,
		"{branch}", p.Branch,
		"{dir}", p.Dir,
	)
	out := make([]string, 0, len(args))
	for _, a := range args {
		if expanded := repl.Replace(a); expanded != "" {
			out = append(out, expanded)
		}
	}
	return out
}
