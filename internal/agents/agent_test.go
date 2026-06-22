package agents

import (
	"testing"
)

func TestRegistry(t *testing.T) {
	Register("test-agent", func() Agent { return NewBuiltin("test-agent", "sh", nil) })
	a, ok := Get("test-agent")
	if !ok || a.Name() != "test-agent" {
		t.Fatalf("Get(test-agent) = %v, %v", a, ok)
	}
	found := false
	for _, n := range Names() {
		if n == "test-agent" {
			found = true
		}
	}
	if !found {
		t.Error("Names() should include registered agent")
	}
	if _, ok := Get("nope"); ok {
		t.Error("unregistered agent should not be found")
	}
}

func TestIsInstalled(t *testing.T) {
	if !NewBuiltin("x", "sh", nil).IsInstalled() {
		t.Error("sh should be installed")
	}
	if NewBuiltin("x", "definitely-not-a-real-binary-zzz", nil).IsInstalled() {
		t.Error("bogus binary should not be installed")
	}
}

func TestExpandArgs(t *testing.T) {
	p := Params{
		IssueKey:    "DEMO-123",
		ContextPath: "/p/.issue-context.md",
		Branch:      "demo-123-x",
		Dir:         "/p",
	}
	got := ExpandArgs([]string{"{context_file}", "--key", "{key}", "{branch}", "{dir}"}, p)
	want := []string{"/p/.issue-context.md", "--key", "DEMO-123", "demo-123-x", "/p"}
	if len(got) != len(want) {
		t.Fatalf("ExpandArgs len = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("arg[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestExpandArgsDropsEmpty(t *testing.T) {
	// {context_file} with no path should be dropped, not passed as "".
	got := ExpandArgs([]string{"{context_file}"}, Params{IssueKey: "X"})
	if len(got) != 0 {
		t.Errorf("expected empty args, got %v", got)
	}
}

func TestHeadlessSpec(t *testing.T) {
	a := NewBuiltinHeadless("x", "x", []string{"{context_file}"}, []string{"-p"}, []string{"-p", "--write"})
	p := Params{ContextPath: "/p/ctx.md", Dir: "/p"}

	plan, ok := a.HeadlessSpec(p, Config{}, HeadlessPlan)
	if !ok || plan.Bin != "x" || plan.Dir != "/p" {
		t.Fatalf("plan headless spec = %+v ok=%v", plan, ok)
	}
	// The context is delivered on stdin, not as a path argument.
	if plan.StdinFile != "/p/ctx.md" {
		t.Errorf("StdinFile = %q, want /p/ctx.md", plan.StdinFile)
	}
	if len(plan.Args) != 1 || plan.Args[0] != "-p" {
		t.Errorf("plan args = %v", plan.Args)
	}
	exec, ok := a.HeadlessSpec(p, Config{}, HeadlessExecute)
	if !ok || len(exec.Args) != 2 || exec.Args[1] != "--write" {
		t.Errorf("exec headless spec = %+v ok=%v", exec, ok)
	}

	// An interactive-only agent reports no headless support.
	if _, ok := NewBuiltin("y", "y", nil).HeadlessSpec(p, Config{}, HeadlessPlan); ok {
		t.Error("NewBuiltin should not support headless")
	}
}

func TestSpecConfigOverride(t *testing.T) {
	a := NewBuiltin("claude", "claude", []string{"{context_file}"})
	p := Params{ContextPath: "/p/ctx.md", Dir: "/p"}

	// default command + args
	s := a.Spec(p, Config{})
	if s.Bin != "claude" || len(s.Args) != 1 || s.Args[0] != "/p/ctx.md" || s.Dir != "/p" {
		t.Errorf("default spec = %+v", s)
	}

	// overridden command + args
	s = a.Spec(p, Config{Command: "claude-beta", Args: []string{"--ctx", "{context_file}"}})
	if s.Bin != "claude-beta" || len(s.Args) != 2 || s.Args[1] != "/p/ctx.md" {
		t.Errorf("override spec = %+v", s)
	}
}
