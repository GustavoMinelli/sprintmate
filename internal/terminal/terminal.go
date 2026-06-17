// Package terminal launches an agent command using a configurable, layered
// strategy: a tmux window when inside tmux, otherwise a new OS terminal
// window, falling back to an in-place handoff that always works.
package terminal

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
)

// Spec describes the process to launch.
type Spec struct {
	Bin  string
	Args []string
	Dir  string
	Env  []string // extra KEY=VALUE entries appended to the inherited environment
}

// Strategies (mirrors config values).
const (
	Auto    = "auto"
	Tmux    = "tmux"
	Window  = "window"
	Inplace = "inplace"
)

// Launch runs spec according to strategy. For tmux/window it returns once the
// agent has been started in its own window; for inplace it returns after the
// agent process exits.
func Launch(spec Spec, strategy string) error {
	if strings.TrimSpace(spec.Bin) == "" {
		return fmt.Errorf("no agent command to launch")
	}
	switch strategy {
	case Tmux:
		return tmuxLaunch(spec)
	case Window:
		return windowLaunch(spec)
	case Inplace:
		return inplaceLaunch(spec)
	case Auto, "":
		return autoLaunch(spec)
	default:
		return fmt.Errorf("unknown launch strategy %q", strategy)
	}
}

// Resolve reports which concrete strategy Auto would pick, for display.
func Resolve(strategy string) string {
	if strategy != Auto && strategy != "" {
		return strategy
	}
	switch {
	case InTmux():
		return Tmux
	case hasWindow():
		return Window
	default:
		return Inplace
	}
}

func autoLaunch(spec Spec) error {
	if InTmux() {
		if err := tmuxLaunch(spec); err == nil {
			return nil
		}
	}
	if hasWindow() {
		if err := windowLaunch(spec); err == nil {
			return nil
		}
	}
	return inplaceLaunch(spec)
}

// InTmux reports whether SprintMate is running inside a tmux session.
func InTmux() bool { return os.Getenv("TMUX") != "" }

func tmuxLaunch(spec Spec) error {
	if _, err := exec.LookPath("tmux"); err != nil {
		return fmt.Errorf("tmux not found")
	}
	args := []string{"new-window"}
	if spec.Dir != "" {
		args = append(args, "-c", spec.Dir)
	}
	args = append(args, shellJoin(append([]string{spec.Bin}, spec.Args...)))
	c := exec.Command("tmux", args...)
	c.Env = environ(spec)
	return c.Run()
}

func inplaceLaunch(spec Spec) error {
	c := exec.Command(spec.Bin, spec.Args...)
	c.Dir = spec.Dir
	c.Env = environ(spec)
	c.Stdin = os.Stdin
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	return c.Run()
}

// OpenURL opens a URL in the user's default browser.
func OpenURL(url string) error {
	var c *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		c = exec.Command("open", url)
	case "windows":
		c = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		c = exec.Command("xdg-open", url)
	}
	return c.Start()
}

// environ returns the environment for the child: inherited plus any extras.
func environ(spec Spec) []string {
	if len(spec.Env) == 0 {
		return nil // nil => inherit current process environment
	}
	return append(os.Environ(), spec.Env...)
}

// ShellLine builds a `cd <dir> && exec <cmd>` shell line used by GUI launchers
// that run a shell.
func ShellLine(spec Spec) string {
	cmd := shellJoin(append([]string{spec.Bin}, spec.Args...))
	if spec.Dir == "" {
		return cmd
	}
	return "cd " + shellQuote(spec.Dir) + " && exec " + cmd
}

func shellJoin(parts []string) string {
	out := make([]string, len(parts))
	for i, p := range parts {
		out[i] = shellQuote(p)
	}
	return strings.Join(out, " ")
}

// shellQuote wraps s in single quotes, escaping embedded single quotes.
func shellQuote(s string) string {
	if s == "" {
		return "''"
	}
	if !strings.ContainsAny(s, " \t\n'\"\\$`&|;<>(){}[]*?#~!") {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
