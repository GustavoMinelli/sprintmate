package config

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestDefaultIsValidWithCredentials(t *testing.T) {
	c := Default()
	c.Jira.Host = "https://empresa.atlassian.net"
	c.Jira.Email = "voce@empresa.com"
	c.Jira.Token = "tok"
	c.Jira.Board = "Board"
	c.Workdir = "/tmp/work"
	if err := c.Validate(); err != nil {
		t.Fatalf("expected valid config, got %v", err)
	}
}

func TestValidateReportsMissing(t *testing.T) {
	c := Default()
	err := c.Validate()
	if err == nil {
		t.Fatal("expected error for empty config")
	}
	for _, want := range []string{"jira.host", "jira.email", "jira.token", "jira.board or jira.jql", "workdir"} {
		if !contains(err.Error(), want) {
			t.Errorf("error %q should mention %q", err, want)
		}
	}
}

func TestApplyEnvOverridesToken(t *testing.T) {
	t.Setenv(EnvToken, "from-env")
	c := Default()
	c.Jira.Token = "from-file"
	c.applyEnv()
	if c.Jira.Token != "from-env" {
		t.Fatalf("env token should win, got %q", c.Jira.Token)
	}
}

func TestMergeKeysKeepsUserAndFillsDefaults(t *testing.T) {
	c := &Config{Keys: Keys{Up: []string{"w"}}}
	c.applyDefaults()
	if len(c.Keys.Up) != 1 || c.Keys.Up[0] != "w" {
		t.Errorf("user binding should be kept, got %v", c.Keys.Up)
	}
	if len(c.Keys.Quit) == 0 {
		t.Error("missing binding should fall back to default")
	}
}

func TestExpandPath(t *testing.T) {
	home, _ := os.UserHomeDir()
	if got := ExpandPath("~/projects/demo"); got != filepath.Join(home, "projects/demo") {
		t.Errorf("ExpandPath(~/projects/demo) = %q", got)
	}
	if got := ExpandPath("/abs/path"); got != "/abs/path" {
		t.Errorf("absolute path should be untouched, got %q", got)
	}
}

func TestWorkdirPath(t *testing.T) {
	c := Default()
	if _, ok := c.WorkdirPath(); ok {
		t.Error("empty workdir should report not found")
	}
	home, _ := os.UserHomeDir()
	c.Workdir = "~/projects/demo"
	if p, ok := c.WorkdirPath(); !ok || p != filepath.Join(home, "projects/demo") {
		t.Errorf("WorkdirPath = %q, %v", p, ok)
	}
}

func TestSaveLoadRoundTrip(t *testing.T) {
	// Redirect the user config dir to a temp location for both platforms.
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("HOME", t.TempDir())
	if _, err := Path(); err != nil {
		t.Skipf("user config dir unavailable: %v", err)
	}

	in := Default()
	in.Jira.Host = "https://empresa.atlassian.net"
	in.Jira.Email = "voce@empresa.com"
	in.Jira.Token = "secret"
	in.Jira.Board = "Sprint Board"
	in.Jira.Columns = []string{"To Do", "In Progress"}
	in.Workdir = "~/projects/demo"

	if err := Save(in); err != nil {
		t.Fatalf("Save: %v", err)
	}
	out, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if out.Jira.Host != in.Jira.Host || out.Jira.Board != in.Jira.Board {
		t.Errorf("round-trip mismatch: %+v vs %+v", out.Jira, in.Jira)
	}
	if len(out.Jira.Columns) != 2 {
		t.Errorf("columns not preserved: %v", out.Jira.Columns)
	}
	if out.Workdir != in.Workdir {
		t.Errorf("workdir not preserved: %q", out.Workdir)
	}
}

func TestEnvTokenNotPersisted(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("HOME", t.TempDir())
	t.Setenv(EnvToken, "env-secret")

	c := Default()
	c.Jira.Host = "https://empresa.atlassian.net"
	c.Jira.Email = "voce@empresa.com"
	c.Jira.Board = "Board"
	c.Workdir = "/tmp/work"
	c.applyEnv()
	if c.Jira.Token != "env-secret" || !c.TokenFromEnv() {
		t.Fatalf("applyEnv should set token + provenance: token=%q fromEnv=%v", c.Jira.Token, c.TokenFromEnv())
	}

	if err := Save(c); err != nil {
		t.Fatalf("Save: %v", err)
	}
	p, _ := Path()
	data, _ := os.ReadFile(p)
	if strings.Contains(string(data), "env-secret") {
		t.Errorf("env token must not be written to disk:\n%s", data)
	}

	// With the env var still set, Load resolves the token from the environment.
	loaded, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded.Jira.Token != "env-secret" {
		t.Errorf("env token should be re-applied on Load, got %q", loaded.Jira.Token)
	}

	// A typed token clears env provenance and IS persisted.
	c.SetToken("typed-token")
	if c.TokenFromEnv() {
		t.Error("SetToken should clear env provenance")
	}
}

func TestLoadMissingReturnsNotExist(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("HOME", t.TempDir())
	if _, err := Load(); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected os.ErrNotExist, got %v", err)
	}
}

// ensure the example struct marshals without panicking
func TestMarshalable(t *testing.T) {
	if _, err := yaml.Marshal(Default()); err != nil {
		t.Fatal(err)
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (len(sub) == 0 || indexOf(s, sub) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
