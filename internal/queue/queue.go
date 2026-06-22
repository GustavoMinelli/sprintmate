// Package queue is the engine behind SprintMate's autonomous runs: a pure state
// machine that schedules headless agent jobs (plan, then — once approved —
// execute) under a concurrency limit, plus a RunPhase helper that actually
// drives an agent process and captures its output.
//
// The Engine performs NO I/O and is not safe for concurrent use: the TUI mutates
// it only from the Bubble Tea update loop, while the (goroutine) tea.Cmds run
// RunPhase and report back via messages. Keeping all state transitions on one
// goroutine is what lets the engine stay lock-free.
package queue

// State is a job's position in the plan → approve → execute lifecycle.
type State int

const (
	Queued    State = iota // waiting for a free slot to plan
	Planning               // plan phase running
	PlanReady              // plan captured, waiting for approval (the gate)
	Executing              // execute phase running
	Done                   // execute finished successfully
	Failed                 // a phase errored
)

func (s State) String() string {
	switch s {
	case Queued:
		return "queued"
	case Planning:
		return "planning"
	case PlanReady:
		return "plan ready"
	case Executing:
		return "executing"
	case Done:
		return "done"
	case Failed:
		return "failed"
	default:
		return "?"
	}
}

// Phase is which agent pass a runnable job should start next.
type Phase int

const (
	PlanPhase Phase = iota
	ExecPhase
)

// Job is one autonomous run, isolated in its own worktree directory.
type Job struct {
	ID       int
	Key      string // issue key
	Title    string
	Agent    string
	Dir      string // worktree directory the agent runs in
	Branch   string
	PlanPath string // where the captured plan is written
	LogPath  string // where stdout/stderr is streamed
	State    State
	Err      error

	approved bool // gate: set by Approve (or AutoApprove) to allow execution
}

// Engine schedules jobs under a concurrency limit and an approval gate.
type Engine struct {
	Concurrency int
	AutoApprove bool // when true, a job executes as soon as its plan is ready

	jobs    []*Job
	running int
	nextID  int
}

// New returns an Engine running at most concurrency jobs at once.
func New(concurrency int, autoApprove bool) *Engine {
	if concurrency < 1 {
		concurrency = 1
	}
	return &Engine{Concurrency: concurrency, AutoApprove: autoApprove}
}

// Add appends a job in the Queued state and returns it (with its ID assigned).
func (e *Engine) Add(j *Job) *Job {
	e.nextID++
	j.ID = e.nextID
	j.State = Queued
	e.jobs = append(e.jobs, j)
	return j
}

// Jobs returns the jobs in insertion order.
func (e *Engine) Jobs() []*Job { return e.jobs }

// Get returns the job with id, or nil.
func (e *Engine) Get(id int) *Job {
	for _, j := range e.jobs {
		if j.ID == id {
			return j
		}
	}
	return nil
}

// Approve opens the gate for a plan-ready job so it can execute.
func (e *Engine) Approve(id int) {
	if j := e.Get(id); j != nil && j.State == PlanReady {
		j.approved = true
	}
}

// Dequeue reserves a concurrency slot and returns the next job whose next phase
// can start now, transitioning it to Planning/Executing. It returns (nil, _)
// when no job can start (slots full, nothing queued, or all plans awaiting
// approval). The caller runs the returned phase and then calls Finish.
func (e *Engine) Dequeue() (*Job, Phase) {
	if e.running >= e.Concurrency {
		return nil, PlanPhase
	}
	for _, j := range e.jobs {
		switch {
		case j.State == Queued:
			j.State = Planning
			e.running++
			return j, PlanPhase
		case j.State == PlanReady && j.approved:
			j.State = Executing
			e.running++
			return j, ExecPhase
		}
	}
	return nil, PlanPhase
}

// Finish releases the slot reserved by Dequeue and advances the job: a failed
// phase moves it to Failed; a completed plan moves it to PlanReady (auto-approved
// when AutoApprove); a completed execution moves it to Done.
func (e *Engine) Finish(id int, err error) {
	j := e.Get(id)
	if j == nil {
		return
	}
	if e.running > 0 {
		e.running--
	}
	if err != nil {
		j.State = Failed
		j.Err = err
		return
	}
	switch j.State {
	case Planning:
		j.State = PlanReady
		if e.AutoApprove {
			j.approved = true
		}
	case Executing:
		j.State = Done
	}
}

// Active counts jobs with a phase currently running (used to warn before quit).
func (e *Engine) Active() int {
	n := 0
	for _, j := range e.jobs {
		if j.State == Planning || j.State == Executing {
			n++
		}
	}
	return n
}

// HasOpen reports whether a non-terminal job already exists for the issue key,
// so the same issue isn't enqueued twice into the same worktree.
func (e *Engine) HasOpen(key string) bool {
	for _, j := range e.jobs {
		if j.Key == key && j.State != Done && j.State != Failed {
			return true
		}
	}
	return false
}

// Pending counts jobs that have not reached a terminal state.
func (e *Engine) Pending() int {
	n := 0
	for _, j := range e.jobs {
		if j.State != Done && j.State != Failed {
			n++
		}
	}
	return n
}

// AwaitingApproval counts plan-ready jobs blocked on the approval gate.
func (e *Engine) AwaitingApproval() int {
	n := 0
	for _, j := range e.jobs {
		if j.State == PlanReady {
			n++
		}
	}
	return n
}
