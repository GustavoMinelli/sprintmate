package queue

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/GustavoMinelli/sprintmate/internal/terminal"
)

// captureDir is the per-worktree folder holding the captured plan and log.
const captureDir = ".sprintmate"

// PlanPath returns where a worktree's captured plan is written.
func PlanPath(worktree string) string {
	return filepath.Join(worktree, captureDir, "plan.md")
}

// LogPath returns where a worktree's agent session output is streamed.
func LogPath(worktree string) string {
	return filepath.Join(worktree, captureDir, "session.log")
}

// RunPhase executes spec in the job's worktree, appending all output to
// job.LogPath. For the plan phase it additionally captures stdout and writes it
// to job.PlanPath — that captured text is the plan the user reviews, which is
// why the plan-only agent invocation must print its plan to stdout. It blocks
// until the process exits or ctx is cancelled.
func RunPhase(ctx context.Context, job *Job, phase Phase, spec terminal.Spec) error {
	if err := os.MkdirAll(filepath.Dir(job.LogPath), 0o755); err != nil {
		return fmt.Errorf("preparing job dir: %w", err)
	}
	logFile, err := os.OpenFile(job.LogPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return fmt.Errorf("opening session log: %w", err)
	}
	defer func() { _ = logFile.Close() }()

	cmd := exec.CommandContext(ctx, spec.Bin, spec.Args...)
	cmd.Dir = spec.Dir
	if len(spec.Env) > 0 {
		cmd.Env = append(os.Environ(), spec.Env...)
	}

	// Feed the issue context to the agent on stdin (the prompt). Closing the file
	// at the end delivers EOF so codex exec doesn't hang on a non-TTY pipe.
	if spec.StdinFile != "" {
		f, err := os.Open(spec.StdinFile)
		if err != nil {
			return fmt.Errorf("opening context for stdin: %w", err)
		}
		defer func() { _ = f.Close() }()
		cmd.Stdin = f
	}

	var planBuf bytes.Buffer
	if phase == PlanPhase {
		cmd.Stdout = io.MultiWriter(logFile, &planBuf)
	} else {
		cmd.Stdout = logFile
	}
	cmd.Stderr = logFile

	runErr := cmd.Run()

	if phase == PlanPhase {
		if werr := os.WriteFile(job.PlanPath, planBuf.Bytes(), 0o600); werr != nil && runErr == nil {
			return fmt.Errorf("writing plan: %w", werr)
		}
	}
	if runErr != nil {
		return fmt.Errorf("agent %q: %w", spec.Bin, runErr)
	}
	return nil
}
