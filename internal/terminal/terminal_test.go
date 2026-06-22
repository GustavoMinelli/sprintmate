package terminal

import (
	"runtime"
	"strings"
	"testing"
)

func TestShellQuote(t *testing.T) {
	cases := map[string]string{
		"simple":          "simple",
		"with space":      "'with space'",
		"it's":            `'it'\''s'`,
		"":                "''",
		"~/projects/demo": "'~/projects/demo'",
		"a[b]":            "'a[b]'",
		"x!y":             "'x!y'",
	}
	for in, want := range cases {
		if got := shellQuote(in); got != want {
			t.Errorf("shellQuote(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestShellLine(t *testing.T) {
	got := ShellLine(Spec{Bin: "claude", Args: []string{".issue-context.md"}, Dir: "/home/u/p"})
	if !strings.HasPrefix(got, "cd /home/u/p && exec claude") {
		t.Errorf("ShellLine = %q", got)
	}
	got = ShellLine(Spec{Bin: "claude"})
	if got != "claude" {
		t.Errorf("ShellLine without dir = %q", got)
	}
}

func TestTmuxArgs(t *testing.T) {
	got := tmuxArgs(Spec{Bin: "claude", Args: []string{".issue-context.md"}, Dir: "/home/u/p"})
	if len(got) < 3 || got[0] != "new-window" || got[1] != "-c" || got[2] != "/home/u/p" {
		t.Fatalf("tmuxArgs = %v, want new-window -c <dir> ...", got)
	}
	if cmd := got[len(got)-1]; !strings.HasPrefix(cmd, "claude") {
		t.Errorf("tmux command = %q, want it to start with claude", cmd)
	}
	// Without a dir, there is no -c flag.
	noDir := tmuxArgs(Spec{Bin: "claude"})
	for _, a := range noDir {
		if a == "-c" {
			t.Errorf("unexpected -c without a dir: %v", noDir)
		}
	}
}

func TestShellLineFoldsEnv(t *testing.T) {
	got := ShellLine(Spec{Bin: "claude", Env: []string{"K=V"}})
	if !strings.Contains(got, "env K=V claude") {
		t.Errorf("ShellLine with env = %q, want it to prefix env K=V", got)
	}
}

func TestLaunchEmptyBin(t *testing.T) {
	if err := Launch(Spec{}, Inplace); err == nil {
		t.Error("expected error for empty bin")
	}
}

func TestLaunchUnknownStrategy(t *testing.T) {
	if err := Launch(Spec{Bin: "true"}, "bogus"); err == nil {
		t.Error("expected error for unknown strategy")
	}
}

func TestLaunchInplace(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("posix-only helper command")
	}
	if err := Launch(Spec{Bin: "true"}, Inplace); err != nil {
		t.Errorf("inplace launch of true: %v", err)
	}
}

func TestResolve(t *testing.T) {
	if got := Resolve(Inplace); got != Inplace {
		t.Errorf("Resolve(inplace) = %q", got)
	}
	if got := Resolve(Auto); got != Tmux && got != Window && got != Inplace {
		t.Errorf("Resolve(auto) returned unexpected %q", got)
	}
}
