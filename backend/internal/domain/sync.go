package domain

import "time"

type RunStatus string

const (
	RunRunning RunStatus = "running"
	RunSuccess RunStatus = "success"
	RunPartial RunStatus = "partial"
	RunFailed  RunStatus = "failed"
)

// Phase names the stage a sync run is in. Queued covers the window between
// run creation and the engine's first progress report.
type Phase string

const (
	PhaseQueued     Phase = "queued"
	PhaseRoster     Phase = "roster"
	PhaseTierAnchor Phase = "tier-anchor"
	PhaseCharacters Phase = "characters"
	PhaseFinalizing Phase = "finalizing"
)

// CoercePhase maps an unknown phase onto PhaseQueued so engine events or
// future persisted values can never leak an unrecognized stage to the UI.
func CoercePhase(p Phase) Phase {
	switch p {
	case PhaseRoster, PhaseTierAnchor, PhaseCharacters, PhaseFinalizing:
		return p
	}
	return PhaseQueued
}

// Progress is the live snapshot of a sync run reported while it executes.
// It carries no credentials and no upstream detail, only what an officer
// needs to watch the expedition move; the last snapshot stays visible with
// Active=false after a run settles so clients can observe the transition.
type Progress struct {
	Active    bool      `json:"active"`
	RunID     int64     `json:"runId"`
	Status    RunStatus `json:"status"`
	Phase     Phase     `json:"phase"`
	Total     int       `json:"total"`
	Updated   int       `json:"updated"`
	Failed    int       `json:"failed"`
	StartedAt time.Time `json:"startedAt"`
	// DryRun stays false until the dry-run slice lands; the field is part
	// of the wire shape from the start so clients can rely on it.
	DryRun bool `json:"dryRun"`
}

func Transition(from, to RunStatus) bool {
	return from == RunRunning && (to == RunSuccess || to == RunPartial || to == RunFailed)
}
func Outcome(updated, failed int) RunStatus {
	if failed == 0 {
		return RunSuccess
	}
	if updated > 0 {
		return RunPartial
	}
	return RunFailed
}
func CanStart(running bool) bool { return !running }
