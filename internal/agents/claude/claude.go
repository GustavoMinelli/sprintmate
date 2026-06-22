// Package claude registers the Claude Code agent.
//
// It is a thin plugin: importing this package (for side effects) makes
// "claude" available in the agent registry. The launch command and arguments
// can be overridden per-user via config.Agents["claude"].
package claude

import "github.com/GustavoMinelli/sprintmate/internal/agents"

func init() {
	agents.Register("claude", func() agents.Agent {
		return agents.NewBuiltinHeadless("claude", "claude",
			// interactive: open a session with the context file as the opening
			// prompt, booting into plan mode so the agent plans before editing.
			[]string{"--permission-mode", "plan", "{context_file}"},
			// headless plan: print mode reading the prompt from stdin (the context
			// is piped in). Plan mode is read-only; SprintMate captures stdout as
			// the plan to review.
			[]string{"-p", "--permission-mode", "plan"},
			// headless execute: print mode auto-accepting edits inside the worktree
			// so the approved plan can be implemented unattended.
			[]string{"-p", "--permission-mode", "acceptEdits"},
		)
	})
}
