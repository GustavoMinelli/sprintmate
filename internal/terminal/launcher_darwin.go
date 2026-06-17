//go:build darwin

package terminal

import (
	"fmt"
	"os"
	"os/exec"
)

// hasWindow assumes a GUI is present on macOS desktops; if osascript can't open
// a window (e.g. over SSH) the auto strategy falls back to in-place.
func hasWindow() bool {
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
