// Package codex registers the Codex CLI agent.
//
// Importing this package (for side effects) makes "codex" available in the
// agent registry. By default it launches `codex` in the project directory,
// where the generated .issue-context.md is available; users can pass extra
// arguments via config.Agents["codex"].
package codex

import "github.com/GustavoMinelli/sprintmate/internal/agents"

func init() {
	agents.Register("codex", func() agents.Agent {
		return agents.NewBuiltin("codex", "codex", nil)
	})
}
