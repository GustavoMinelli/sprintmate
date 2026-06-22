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
		return agents.NewBuiltinHeadless("codex", "codex",
			// interactive: launch codex in the project dir, where it finds the
			// generated .issue-context.md.
			nil,
			// headless plan: `codex exec -` reads the prompt from stdin; a read-only
			// sandbox guarantees the plan phase edits nothing, regardless of the
			// preamble. SprintMate captures stdout as the plan.
			[]string{"exec", "--sandbox", "read-only", "-"},
			// headless execute: workspace-write lets codex implement the approved
			// plan inside the isolated worktree.
			[]string{"exec", "--sandbox", "workspace-write", "-"},
		)
	})
}
