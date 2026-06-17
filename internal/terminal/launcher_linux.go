//go:build linux

package terminal

import (
	"fmt"
	"os"
	"os/exec"
)

func hasWindow() bool {
	return os.Getenv("DISPLAY") != "" || os.Getenv("WAYLAND_DISPLAY") != ""
}

func windowLaunch(spec Spec) error {
	line := ShellLine(spec)
	type emulator struct {
		bin  string
		args []string
	}
	var candidates []emulator
	if t := os.Getenv("TERMINAL"); t != "" {
		candidates = append(candidates, emulator{t, []string{"-e", "sh", "-c", line}})
	}
	candidates = append(candidates,
		emulator{"x-terminal-emulator", []string{"-e", "sh", "-c", line}},
		emulator{"gnome-terminal", []string{"--", "sh", "-c", line}},
		emulator{"konsole", []string{"-e", "sh", "-c", line}},
		emulator{"alacritty", []string{"-e", "sh", "-c", line}},
		emulator{"kitty", []string{"sh", "-c", line}},
		emulator{"xterm", []string{"-e", "sh", "-c", line}},
	)
	for _, e := range candidates {
		if _, err := exec.LookPath(e.bin); err != nil {
			continue
		}
		c := exec.Command(e.bin, e.args...)
		c.Env = environ(spec)
		return c.Start() // detached: don't block on the new window
	}
	return fmt.Errorf("no supported terminal emulator found (set $TERMINAL or launch.strategy: inplace)")
}
