package domain

type RunStatus string

const (
	RunRunning RunStatus = "running"
	RunSuccess RunStatus = "success"
	RunPartial RunStatus = "partial"
	RunFailed  RunStatus = "failed"
)

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
