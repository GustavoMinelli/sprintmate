// Package config loads, validates and persists the SprintMate configuration.
//
// The config lives in the user config dir (e.g. ~/.config/sprintmate/config.yaml
// on Linux, ~/Library/Application Support on macOS, %AppData% on Windows) and is
// normally written by the TUI wizard, though it remains hand-editable.
package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// EnvToken is the environment variable that overrides jira.token when set.
const EnvToken = "SPRINTMATE_JIRA_TOKEN"

// Config is the root configuration document.
type Config struct {
	Jira    Jira             `yaml:"jira"`
	Agent   AgentDefaults    `yaml:"agent"`
	Agents  map[string]Agent `yaml:"agents"`
	Launch  Launch           `yaml:"launch"`
	Git     Git              `yaml:"git"`
	Keys    Keys             `yaml:"keys"`
	Workdir string           `yaml:"workdir"`

	// tokenFromEnv records that Jira.Token was sourced from the environment, so
	// Save can keep it out of the YAML file. Unexported => never marshalled.
	tokenFromEnv bool
}

// SetToken sets the Jira token explicitly (e.g. typed in the wizard) and marks
// it as file-persistable.
func (c *Config) SetToken(token string) {
	c.Jira.Token = token
	c.tokenFromEnv = false
}

// TokenFromEnv reports whether the active token came from the environment.
func (c *Config) TokenFromEnv() bool { return c.tokenFromEnv }

// Jira holds the connection and the configurable issue source (board, columns,
// sprint). When JQL is set it overrides board/columns/sprint entirely.
type Jira struct {
	Host     string   `yaml:"host"`
	Email    string   `yaml:"email"`
	Token    string   `yaml:"token"`
	Board    string   `yaml:"board"`
	Sprint   string   `yaml:"sprint"`
	Columns  []string `yaml:"columns"`
	Assignee string   `yaml:"assignee"`
	JQL      string   `yaml:"jql"`
	Fields   Fields   `yaml:"fields"`
}

// Fields lets the user override auto-discovered custom field IDs.
type Fields struct {
	Sprint      string `yaml:"sprint,omitempty"`
	StoryPoints string `yaml:"story_points,omitempty"`
}

// AgentDefaults selects which registered agent is used by default.
type AgentDefaults struct {
	Default string `yaml:"default"`
}

// Agent is a per-agent launch override. Args support the placeholders
// {context_file}, {key}, {branch} and {dir}.
type Agent struct {
	Command string   `yaml:"command"`
	Args    []string `yaml:"args"`
}

// Launch picks the terminal launch strategy: auto | tmux | window | inplace.
type Launch struct {
	Strategy string `yaml:"strategy"`
}

// Git controls branch automation.
type Git struct {
	CreateBranch  bool   `yaml:"create_branch"`
	BranchPattern string `yaml:"branch_pattern"`
}

// Keys holds the configurable keybindings for the dashboard.
type Keys struct {
	Up          []string `yaml:"up"`
	Down        []string `yaml:"down"`
	Launch      []string `yaml:"launch"`
	SwitchAgent []string `yaml:"switch_agent"`
	Refresh     []string `yaml:"refresh"`
	OpenBrowser []string `yaml:"open_browser"`
	Search      []string `yaml:"search"`
	Settings    []string `yaml:"settings"`
	Quit        []string `yaml:"quit"`
}

// Sprint selection modes.
const (
	SprintActive = "active"
	SprintFuture = "future"
	SprintAll    = "all"
)

// Launch strategies.
const (
	StrategyAuto    = "auto"
	StrategyTmux    = "tmux"
	StrategyWindow  = "window"
	StrategyInplace = "inplace"
)

// Path returns the absolute path to the config file.
func Path() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("resolving user config dir: %w", err)
	}
	return filepath.Join(dir, "sprintmate", "config.yaml"), nil
}

// Exists reports whether a config file is already present.
func Exists() bool {
	p, err := Path()
	if err != nil {
		return false
	}
	_, err = os.Stat(p)
	return err == nil
}

// Default returns a Config populated with sensible defaults.
func Default() *Config {
	return &Config{
		Jira: Jira{
			Sprint:   SprintActive,
			Assignee: "currentUser",
		},
		Agent: AgentDefaults{Default: "claude"},
		Agents: map[string]Agent{
			"claude": {Command: "claude", Args: []string{"{context_file}"}},
			"codex":  {Command: "codex", Args: []string{}},
		},
		Launch: Launch{Strategy: StrategyAuto},
		Git:    Git{CreateBranch: true, BranchPattern: "{key}-{slug}"},
		Keys:   DefaultKeys(),
	}
}

// DefaultKeys returns the default keybindings.
func DefaultKeys() Keys {
	return Keys{
		Up:          []string{"up", "k"},
		Down:        []string{"down", "j"},
		Launch:      []string{"enter"},
		SwitchAgent: []string{"tab"},
		Refresh:     []string{"r"},
		OpenBrowser: []string{"o"},
		Search:      []string{"/"},
		Settings:    []string{"s"},
		Quit:        []string{"q", "ctrl+c"},
	}
}

// Load reads, validates and normalizes the config from disk. If the file does
// not exist it returns an error wrapping os.ErrNotExist so callers can launch
// the setup wizard instead.
func Load() (*Config, error) {
	p, err := Path()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(p)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("config not found at %s: %w", p, os.ErrNotExist)
		}
		return nil, fmt.Errorf("reading config: %w", err)
	}
	cfg := Default()
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parsing config %s: %w", p, err)
	}
	cfg.applyEnv()
	cfg.applyDefaults()
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return cfg, nil
}

// LoadRaw parses the config without validating it, applying env overrides and
// defaults. It returns nil if no file exists. Used to pre-fill the setup wizard
// from an incomplete config.
func LoadRaw() *Config {
	p, err := Path()
	if err != nil {
		return nil
	}
	data, err := os.ReadFile(p)
	if err != nil {
		return nil
	}
	cfg := Default()
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil
	}
	cfg.applyEnv()
	cfg.applyDefaults()
	return cfg
}

// Save writes the config to disk atomically, creating parent dirs as needed.
func Save(cfg *Config) error {
	p, err := Path()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return fmt.Errorf("creating config dir: %w", err)
	}
	// Don't persist an environment-provided token to disk.
	toWrite := cfg
	if cfg.tokenFromEnv {
		clone := *cfg
		clone.Jira.Token = ""
		toWrite = &clone
	}
	body, err := yaml.Marshal(toWrite)
	if err != nil {
		return fmt.Errorf("encoding config: %w", err)
	}
	out := append([]byte(fileHeader), body...)
	tmp := p + ".tmp"
	if err := os.WriteFile(tmp, out, 0o600); err != nil {
		return fmt.Errorf("writing config: %w", err)
	}
	if err := os.Rename(tmp, p); err != nil {
		return fmt.Errorf("replacing config: %w", err)
	}
	return nil
}

const fileHeader = "# SprintMate configuration. Generated by the setup wizard; safe to edit by hand.\n" +
	"# Tip: set " + EnvToken + " in your environment to keep the token out of this file.\n\n"

// applyEnv overlays environment-provided secrets and records their provenance
// so Save can avoid persisting them.
func (c *Config) applyEnv() {
	if v := os.Getenv(EnvToken); v != "" {
		c.Jira.Token = v
		c.tokenFromEnv = true
	}
}

// applyDefaults fills in any zero values left after unmarshalling.
func (c *Config) applyDefaults() {
	d := Default()
	if c.Jira.Sprint == "" {
		c.Jira.Sprint = d.Jira.Sprint
	}
	if c.Jira.Assignee == "" {
		c.Jira.Assignee = d.Jira.Assignee
	}
	if c.Agent.Default == "" {
		c.Agent.Default = d.Agent.Default
	}
	if len(c.Agents) == 0 {
		c.Agents = d.Agents
	}
	if c.Launch.Strategy == "" {
		c.Launch.Strategy = d.Launch.Strategy
	}
	if c.Git.BranchPattern == "" {
		c.Git.BranchPattern = d.Git.BranchPattern
	}
	c.Keys = mergeKeys(c.Keys, d.Keys)
}

// mergeKeys keeps user-provided bindings and falls back to defaults per action.
func mergeKeys(k, d Keys) Keys {
	pick := func(a, b []string) []string {
		if len(a) > 0 {
			return a
		}
		return b
	}
	return Keys{
		Up:          pick(k.Up, d.Up),
		Down:        pick(k.Down, d.Down),
		Launch:      pick(k.Launch, d.Launch),
		SwitchAgent: pick(k.SwitchAgent, d.SwitchAgent),
		Refresh:     pick(k.Refresh, d.Refresh),
		OpenBrowser: pick(k.OpenBrowser, d.OpenBrowser),
		Search:      pick(k.Search, d.Search),
		Settings:    pick(k.Settings, d.Settings),
		Quit:        pick(k.Quit, d.Quit),
	}
}

// Validate checks the minimum required fields to run.
func (c *Config) Validate() error {
	var missing []string
	if strings.TrimSpace(c.Jira.Host) == "" {
		missing = append(missing, "jira.host")
	}
	if strings.TrimSpace(c.Jira.Email) == "" {
		missing = append(missing, "jira.email")
	}
	if strings.TrimSpace(c.Jira.Token) == "" {
		missing = append(missing, "jira.token (or "+EnvToken+")")
	}
	if c.Jira.JQL == "" && strings.TrimSpace(c.Jira.Board) == "" {
		missing = append(missing, "jira.board or jira.jql")
	}
	if strings.TrimSpace(c.Workdir) == "" {
		missing = append(missing, "workdir")
	}
	if len(missing) > 0 {
		return fmt.Errorf("incomplete config: missing %s", strings.Join(missing, ", "))
	}
	return nil
}

// WorkdirPath resolves the configured working directory, expanding a leading ~.
// Every issue launches here. It returns ("", false) when none is configured.
func (c *Config) WorkdirPath() (string, bool) {
	if strings.TrimSpace(c.Workdir) == "" {
		return "", false
	}
	return ExpandPath(c.Workdir), true
}

// ExpandPath expands a leading ~ to the user's home directory.
func ExpandPath(p string) string {
	if p == "~" || strings.HasPrefix(p, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, strings.TrimPrefix(p, "~"))
		}
	}
	return p
}
