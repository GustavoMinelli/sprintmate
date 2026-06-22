// Package agents defines the pluggable AI-agent abstraction and a registry.
//
// New agents are added by creating a small subpackage that calls Register in
// its init function (see internal/agents/claude). The core never needs to
// change. Each agent turns an issue + prepared context into a terminal.Spec;
// the terminal package decides how that spec is actually launched.
package agents

import (
	"maps"
	"os/exec"
	"slices"
	"strings"

	"github.com/GustavoMinelli/sprintmate/internal/terminal"
)

// Params carries everything an agent may need to build its launch command. It
// is deliberately free of any issue-source type so the agent layer stays
// source-agnostic.
type Params struct {
	IssueKey    string
	ContextPath string // path to the generated .issue-context.md
	Branch      string
	Dir         string // working directory for the agent
}

// Config is the per-agent launch configuration (from config.Agents[name]).
type Config struct {
	Command string
	Args    []string
}

// Mode selects a non-interactive invocation of an agent for the autonomous
// queue: a plan-only pass (output a plan, edit nothing) or an execution pass
// (implement the approved plan).
type Mode int

const (
	HeadlessPlan Mode = iota
	HeadlessExecute
)

// Agent is the small interface every supported agent implements.
type Agent interface {
	Name() string
	IsInstalled() bool
	// Spec builds the interactive launch command.
	Spec(p Params, cfg Config) terminal.Spec
	// HeadlessSpec builds the non-interactive command for the given mode. The
	// second return is false when the agent has no headless support, so the
	// autonomous queue can skip it gracefully.
	HeadlessSpec(p Params, cfg Config, mode Mode) (terminal.Spec, bool)
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
	return slices.Sorted(maps.Keys(registry))
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
	// headlessArgs holds the per-mode argument lists for non-interactive runs.
	// A missing/empty entry means the agent has no headless support for that mode.
	headlessArgs map[Mode][]string
}

// NewBuiltin constructs an interactive-only command-based agent. Subpackages use
// this in their Register call. The autonomous queue will skip such agents.
func NewBuiltin(name, defaultCommand string, defaultArgs []string) Agent {
	return builtin{name: name, defaultCommand: defaultCommand, defaultArgs: defaultArgs}
}

// NewBuiltinHeadless constructs a command-based agent that also supports the
// autonomous queue: planArgs run a plan-only pass and execArgs run the
// implementation pass (both placeholder-expanded like interactiveArgs).
func NewBuiltinHeadless(name, defaultCommand string, interactiveArgs, planArgs, execArgs []string) Agent {
	return builtin{
		name:           name,
		defaultCommand: defaultCommand,
		defaultArgs:    interactiveArgs,
		headlessArgs:   map[Mode][]string{HeadlessPlan: planArgs, HeadlessExecute: execArgs},
	}
}

func (b builtin) Name() string { return b.name }

func (b builtin) IsInstalled() bool {
	_, err := exec.LookPath(b.defaultCommand)
	return err == nil
}

func (b builtin) command(cfg Config) string {
	if c := strings.TrimSpace(cfg.Command); c != "" {
		return c
	}
	return b.defaultCommand
}

func (b builtin) Spec(p Params, cfg Config) terminal.Spec {
	rawArgs := cfg.Args
	if rawArgs == nil {
		rawArgs = b.defaultArgs
	}
	return terminal.Spec{
		Bin:  b.command(cfg),
		Args: ExpandArgs(rawArgs, p),
		Dir:  p.Dir,
	}
}

func (b builtin) HeadlessSpec(p Params, cfg Config, mode Mode) (terminal.Spec, bool) {
	args, ok := b.headlessArgs[mode]
	if !ok || len(args) == 0 {
		return terminal.Spec{}, false
	}
	// The context is delivered to the agent on stdin (not as a path argument):
	// `claude -p` / `codex exec -` read the prompt from the pipe, and the pipe's
	// EOF prevents codex from hanging on a non-TTY stdin.
	return terminal.Spec{
		Bin:       b.command(cfg),
		Args:      ExpandArgs(args, p),
		Dir:       p.Dir,
		StdinFile: p.ContextPath,
	}, true
}

// ExpandArgs substitutes placeholders in each arg and drops args that expand to
// empty (e.g. {context_file} when no context was generated).
func ExpandArgs(args []string, p Params) []string {
	repl := strings.NewReplacer(
		"{context_file}", p.ContextPath,
		"{key}", p.IssueKey,
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
