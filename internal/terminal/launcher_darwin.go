//go:build darwin

package terminal

import (
	"fmt"
	"os"
	"os/exec"
)

// hasWindow reports whether a GUI Terminal can be opened. Over SSH there is no
// desktop session, so osascript would fail; reporting false here keeps Resolve's
// prediction in step with autoLaunch's in-place fallback.
func hasWindow() bool {
	if os.Getenv("SSH_CONNECTION") != "" || os.Getenv("SSH_TTY") != "" {
		return false
	}
	_, err := exec.LookPath("osascript")
	return err == nil
}

func windowLaunch(spec Spec) error {
	line := ShellLine(spec)
	var script string
	if os.Getenv("TERM_PROGRAM") == "iTerm.app" {
		script = fmt.Sprintf(`tell application "iTerm"
  activate
  set w to (create window with default profile)
  tell current session of w to write text %q
end tell`, line)
	} else {
		script = fmt.Sprintf(`tell application "Terminal"
  activate
  do script %q
end tell`, line)
	}
	return exec.Command("osascript", "-e", script).Run()
}
