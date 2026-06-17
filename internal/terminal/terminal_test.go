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
