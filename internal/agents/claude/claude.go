// Package claude registers the Claude Code agent.
//
// It is a thin plugin: importing this package (for side effects) makes
// "claude" available in the agent registry. The launch command and arguments
// can be overridden per-user via config.Agents["claude"].
package claude

import "github.com/GustavoMinelli/sprintmate/internal/agents"

func init() {
	agents.Register("claude", func() agents.Agent {
		// `claude "<file>"` starts a session with the context as the opening prompt.
		return agents.NewBuiltin("claude", "claude", []string{"{context_file}"})
	})
}
