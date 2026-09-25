package sync

import (
	"context"
	"testing"
	"time"

	"github.com/brocahontaz/apekeeper/backend/internal/domain"
)

func TestServiceStartManualCreatesRunningRunBeforeQueueing(t *testing.T) {
	blocked := make(chan struct{})
	e, s, g := testEngine(t, &fakeClient{ilvl: 600, profileWait: blocked})
	service := Service{Engine: e, Guild: g}
	run, err := service.StartManual(context.Background())
	if err != nil || run.ID == 0 || run.Status != domain.RunRunning {
		t.Fatalf("run=%+v err=%v", run, err)
	}
	history, err := s.SyncRuns.History(context.Background(), g.ID, 1)
	if err != nil || len(history) != 1 || history[0].ID != run.ID || history[0].Status != domain.RunRunning {
		t.Fatalf("history=%+v err=%v", history, err)
	}
	if _, err := service.StartManual(context.Background()); err != ErrAlreadyRunning {
		t.Fatalf("concurrent manual sync error=%v", err)
	}
	close(blocked)
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		history, err = s.SyncRuns.History(context.Background(), g.ID, 1)
		if err == nil && len(history) == 1 && history[0].Status == domain.RunSuccess {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("run did not complete: history=%+v err=%v", history, err)
}
