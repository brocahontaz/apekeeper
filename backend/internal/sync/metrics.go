package sync

import "sync/atomic"

// Metrics is an in-process, dependency-free sync counter set. It contains no
// guild names, character names, credentials, or upstream payloads.
type Metrics struct {
	Started       atomic.Uint64
	Completed     atomic.Uint64
	Errors        atomic.Uint64
	DurationNanos atomic.Uint64
}

type MetricsSnapshot struct {
	Started, Completed, Errors uint64
	DurationNanos              uint64
}

func (m *Metrics) Snapshot() MetricsSnapshot {
	if m == nil {
		return MetricsSnapshot{}
	}
	return MetricsSnapshot{Started: m.Started.Load(), Completed: m.Completed.Load(), Errors: m.Errors.Load(), DurationNanos: m.DurationNanos.Load()}
}
