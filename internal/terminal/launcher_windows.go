//go:build windows

package terminal

import "os/exec"

func hasWindow() bool { return true }

func windowLaunch(spec Spec) error {
	// Prefer Windows Terminal when available.
	if _, err := exec.LookPath("wt.exe"); err == nil {
		args := []string{}
		if spec.Dir != "" {
			args = append(args, "-d", spec.Dir)
		}
		args = append(args, spec.Bin)
		args = append(args, spec.Args...)
		c := exec.Command("wt.exe", args...)
		c.Env = environ(spec)
		return c.Start()
	}
	// Fallback: open a new console window via cmd start.
	args := append([]string{"/c", "start", "", spec.Bin}, spec.Args...)
	c := exec.Command("cmd", args...)
	c.Dir = spec.Dir
	c.Env = environ(spec)
	return c.Start()
}
