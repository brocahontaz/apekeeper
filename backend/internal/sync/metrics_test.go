package sync

import "testing"

func TestMetricsSnapshotIsStableAndSecretFree(t *testing.T) {
	m := &Metrics{}
	m.Started.Add(2)
	m.Completed.Add(1)
	m.Errors.Add(1)
	m.DurationNanos.Add(42)
	got := m.Snapshot()
	if got.Started != 2 || got.Completed != 1 || got.Errors != 1 || got.DurationNanos != 42 {
		t.Fatalf("snapshot=%+v", got)
	}
}
