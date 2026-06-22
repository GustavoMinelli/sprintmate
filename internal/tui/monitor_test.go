package tui

import (
	"testing"

	"github.com/GustavoMinelli/sprintmate/internal/config"
	"github.com/GustavoMinelli/sprintmate/internal/jira"
	"github.com/GustavoMinelli/sprintmate/internal/queue"
)

// A prepared job is added to the engine and immediately dispatched to Planning
// (a concurrency slot is reserved) without the user approving anything.
func TestMonitorEnqueueDispatchesPlan(t *testing.T) {
	m := newMonitor(config.Default())
	m, _ = m.Update(jobPreparedMsg{job: &queue.Job{Key: "DEMO-1", Agent: "claude", Dir: t.TempDir()}})

	jobs := m.eng.Jobs()
	if len(jobs) != 1 || jobs[0].Key != "DEMO-1" {
		t.Fatalf("job not added: %+v", jobs)
	}
	if jobs[0].State != queue.Planning {
		t.Errorf("state = %v, want Planning (dispatched)", jobs[0].State)
	}
	if m.running != 1 {
		t.Errorf("running = %d, want 1", m.running)
	}
}

// Enqueuing an issue that already has an open job is rejected (worktree reuse
// would otherwise race two agents in the same directory).
func TestMonitorRejectsDuplicateKey(t *testing.T) {
	m := newMonitor(config.Default())
	m, _ = m.Update(jobPreparedMsg{job: &queue.Job{Key: "DEMO-1", Agent: "claude", Dir: t.TempDir()}})

	m, cmd := m.Update(enqueueMsg{issue: jira.Issue{Key: "DEMO-1"}, agent: "claude"})
	if cmd != nil {
		t.Error("duplicate enqueue should not start a prepare cmd")
	}
	if m.notice == "" {
		t.Error("expected a 'already queued' notice")
	}
	if len(m.eng.Jobs()) != 1 {
		t.Errorf("job count = %d, want 1 (no duplicate)", len(m.eng.Jobs()))
	}
}

// A second enqueue of the same key while its worktree is still being prepared
// (async, before the job is added to the engine) is rejected by the in-flight
// reservation — closing the TOCTOU gap that HasOpen alone leaves open.
func TestMonitorRejectsInFlightDuplicate(t *testing.T) {
	m := newMonitor(config.Default())

	m, cmd1 := m.Update(enqueueMsg{issue: jira.Issue{Key: "DEMO-1"}, agent: "claude"})
	if cmd1 == nil || !m.pending["DEMO-1"] {
		t.Fatal("first enqueue should reserve the key and start a prepare cmd")
	}
	// Engine is still empty (prepare is async); HasOpen would not catch this.
	if len(m.eng.Jobs()) != 0 {
		t.Fatal("job should not be in the engine until prepared")
	}

	m, cmd2 := m.Update(enqueueMsg{issue: jira.Issue{Key: "DEMO-1"}, agent: "claude"})
	if cmd2 != nil {
		t.Error("in-flight duplicate enqueue should be rejected (no prepare cmd)")
	}
	if m.notice == "" {
		t.Error("expected a notice for the rejected duplicate")
	}

	// Once prepared, the reservation is released.
	m, _ = m.Update(jobPreparedMsg{key: "DEMO-1", job: &queue.Job{Key: "DEMO-1", Agent: "claude", Dir: t.TempDir()}})
	if m.pending["DEMO-1"] {
		t.Error("reservation should be released after the job is prepared")
	}
}

// A finished plan with the gate on stays at PlanReady until approved.
func TestMonitorGateHoldsPlanReady(t *testing.T) {
	cfg := config.Default()
	cfg.Queue.AutoApprove = false
	m := newMonitor(cfg)
	m, _ = m.Update(jobPreparedMsg{job: &queue.Job{Key: "DEMO-1", Agent: "claude", Dir: t.TempDir()}})
	id := m.eng.Jobs()[0].ID

	m, _ = m.Update(phaseDoneMsg{id: id, phase: queue.PlanPhase, err: nil})
	if got := m.eng.Get(id).State; got != queue.PlanReady {
		t.Fatalf("state = %v, want PlanReady (gated)", got)
	}

	m, _ = m.Update(approveJobMsg{id: id})
	if got := m.eng.Get(id).State; got != queue.Executing {
		t.Errorf("state after approve = %v, want Executing", got)
	}
}

// Navigation messages flip screens through the root model without panicking.
func TestRootRoutesMonitorMessages(t *testing.T) {
	root := newModel(validConfig(), false, "test")
	root.width, root.height = 100, 40

	updated, _ := root.Update(openMonitorMsg{})
	rm := updated.(model)
	if rm.screen != screenMonitor {
		t.Fatalf("screen = %v, want screenMonitor", rm.screen)
	}
	if rm.mon.eng == nil {
		t.Fatal("monitor engine should be created lazily on open")
	}
}

func validConfig() *config.Config {
	c := config.Default()
	c.Jira.Host = "https://x.atlassian.net"
	c.Jira.Email = "e@x.com"
	c.SetToken("t")
	c.Jira.Board = "B"
	c.Workdir = "/tmp"
	return c
}
