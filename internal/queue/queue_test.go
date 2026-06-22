package queue

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/GustavoMinelli/sprintmate/internal/terminal"
)

func TestEngineGateBlocksExecutionUntilApproved(t *testing.T) {
	e := New(2, false) // gate on
	a := e.Add(&Job{Key: "A"})
	e.Add(&Job{Key: "B"})

	// Both can start planning (concurrency 2).
	j1, p1 := e.Dequeue()
	j2, p2 := e.Dequeue()
	if j1 == nil || j2 == nil || p1 != PlanPhase || p2 != PlanPhase {
		t.Fatalf("expected two plan phases, got %+v/%+v", j1, j2)
	}
	// Slots are full now.
	if j, _ := e.Dequeue(); j != nil {
		t.Fatalf("expected no slot free, got job %d", j.ID)
	}

	// A's plan finishes → PlanReady, but gated (not approved) so it can't execute.
	e.Finish(a.ID, nil)
	if a.State != PlanReady {
		t.Fatalf("A state = %v, want PlanReady", a.State)
	}
	if j, _ := e.Dequeue(); j != nil && j.ID == a.ID {
		t.Fatal("ungated plan must not execute")
	}

	// Approve A → it executes.
	e.Approve(a.ID)
	j, ph := e.Dequeue()
	if j == nil || j.ID != a.ID || ph != ExecPhase {
		t.Fatalf("approved job should execute, got %+v phase %v", j, ph)
	}
	e.Finish(a.ID, nil)
	if a.State != Done {
		t.Errorf("A state = %v, want Done", a.State)
	}
}

func TestEngineAutoApproveExecutesWithoutGate(t *testing.T) {
	e := New(1, true) // auto-approve
	a := e.Add(&Job{Key: "A"})

	j, ph := e.Dequeue()
	if j == nil || ph != PlanPhase {
		t.Fatal("expected plan phase")
	}
	e.Finish(a.ID, nil) // plan done → auto-approved
	j, ph = e.Dequeue()
	if j == nil || j.ID != a.ID || ph != ExecPhase {
		t.Fatalf("auto-approve should let it execute, got %+v phase %v", j, ph)
	}
}

func TestEngineConcurrencyLimit(t *testing.T) {
	e := New(1, false)
	e.Add(&Job{Key: "A"})
	e.Add(&Job{Key: "B"})
	if j, _ := e.Dequeue(); j == nil {
		t.Fatal("first job should start")
	}
	if j, _ := e.Dequeue(); j != nil {
		t.Fatal("second job should wait for the single slot")
	}
}

func TestEngineFinishFailure(t *testing.T) {
	e := New(1, false)
	a := e.Add(&Job{Key: "A"})
	e.Dequeue()
	e.Finish(a.ID, context.Canceled)
	if a.State != Failed || a.Err == nil {
		t.Errorf("expected Failed with error, got %v/%v", a.State, a.Err)
	}
	if e.Active() != 0 {
		t.Errorf("Active = %d, want 0 after finish", e.Active())
	}
}

func TestEngineActiveAndPending(t *testing.T) {
	e := New(2, true)
	a := e.Add(&Job{Key: "A"})
	e.Add(&Job{Key: "B"})
	e.Dequeue() // A planning
	if e.Active() != 1 || e.Pending() != 2 {
		t.Errorf("Active=%d Pending=%d, want 1/2", e.Active(), e.Pending())
	}
	e.Finish(a.ID, nil) // A plan ready (auto-approved)
	if e.Active() != 0 {
		t.Errorf("Active=%d, want 0", e.Active())
	}
}

func TestRunPhaseCapturesPlan(t *testing.T) {
	dir := t.TempDir()
	job := &Job{
		Dir:      dir,
		PlanPath: PlanPath(dir),
		LogPath:  LogPath(dir),
	}
	// Emulate an agent that prints a plan to stdout.
	spec := terminal.Spec{Bin: "sh", Args: []string{"-c", "echo 'step 1: do the thing'"}, Dir: dir}
	if err := RunPhase(context.Background(), job, PlanPhase, spec); err != nil {
		t.Fatalf("RunPhase: %v", err)
	}
	plan, err := os.ReadFile(job.PlanPath)
	if err != nil {
		t.Fatalf("read plan: %v", err)
	}
	if !strings.Contains(string(plan), "step 1: do the thing") {
		t.Errorf("plan.md missing captured stdout: %q", plan)
	}
	if _, err := os.Stat(filepath.Join(dir, captureDir, "session.log")); err != nil {
		t.Errorf("session.log not written: %v", err)
	}
}

func TestEngineHasOpen(t *testing.T) {
	e := New(2, false)
	a := e.Add(&Job{Key: "A"})
	e.Add(&Job{Key: "B"})
	if !e.HasOpen("A") || !e.HasOpen("B") {
		t.Fatal("queued jobs should be open")
	}
	if e.HasOpen("C") {
		t.Error("unknown key should not be open")
	}
	e.Dequeue()
	e.Finish(a.ID, nil) // A -> PlanReady (still open)
	if !e.HasOpen("A") {
		t.Error("plan-ready job is still open")
	}
	e.Approve(a.ID)
	e.Dequeue()
	e.Finish(a.ID, nil) // A -> Done (terminal)
	if e.HasOpen("A") {
		t.Error("done job should no longer be open")
	}
}

func TestRunPhaseFeedsContextOnStdin(t *testing.T) {
	dir := t.TempDir()
	ctxFile := filepath.Join(dir, "ctx.md")
	if err := os.WriteFile(ctxFile, []byte("PLAN THIS TASK"), 0o600); err != nil {
		t.Fatal(err)
	}
	job := &Job{Dir: dir, PlanPath: PlanPath(dir), LogPath: LogPath(dir)}
	// `cat` echoes stdin to stdout; the captured plan must contain the piped context.
	spec := terminal.Spec{Bin: "cat", Dir: dir, StdinFile: ctxFile}
	if err := RunPhase(context.Background(), job, PlanPhase, spec); err != nil {
		t.Fatalf("RunPhase: %v", err)
	}
	plan, _ := os.ReadFile(job.PlanPath)
	if !strings.Contains(string(plan), "PLAN THIS TASK") {
		t.Errorf("stdin context not delivered: plan=%q", plan)
	}
}

func TestRunPhaseReportsFailure(t *testing.T) {
	dir := t.TempDir()
	job := &Job{Dir: dir, PlanPath: PlanPath(dir), LogPath: LogPath(dir)}
	spec := terminal.Spec{Bin: "sh", Args: []string{"-c", "exit 3"}, Dir: dir}
	if err := RunPhase(context.Background(), job, ExecPhase, spec); err == nil {
		t.Error("expected a non-zero exit to surface as an error")
	}
}
