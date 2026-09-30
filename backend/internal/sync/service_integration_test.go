package sync

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/brocahontaz/apekeeper/backend/internal/domain"
	"github.com/brocahontaz/apekeeper/backend/internal/notify"
	"github.com/brocahontaz/apekeeper/backend/internal/store"
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

// recordingNotifier captures the summary the service hands to a notifier;
// NotifySyncRun runs in its own goroutine, so access is locked.
type recordingNotifier struct {
	mu   sync.Mutex
	got  notify.SyncRunSummary
	hits int
}

func (r *recordingNotifier) NotifySyncRun(s notify.SyncRunSummary) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.got = s
	r.hits++
}

func (r *recordingNotifier) summary() (notify.SyncRunSummary, int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.got, r.hits
}

// Every finished run — manual or scheduled — must fire one notification
// summarizing the outcome; without a notifier nothing fires at all.
func TestServiceNotifiesAfterEveryRunCompletes(t *testing.T) {
	e, _, g := testEngine(t, &fakeClient{ilvl: 600})
	notifier := &recordingNotifier{}
	service := Service{Engine: e, Guild: g, Notifier: notifier}
	run, err := service.Start(context.Background(), "manual")
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		got, hits := notifier.summary()
		if hits > 1 {
			t.Fatalf("notification fired %d times, want one per run", hits)
		}
		if hits == 1 {
			if got.Trigger != "manual" || got.Status != string(domain.RunSuccess) ||
				got.Total != run.Total || got.Updated != run.Updated || got.Failed != 0 ||
				got.Guild != g.Name || got.Duration <= 0 || len(got.Reasons) != 0 {
				t.Fatalf("summary=%+v run=%+v", got, run)
			}
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("no notification arrived; summary=%+v", notifier.got)
}

// runFailureReasons flattens the stored per-character detail into a sorted
// reason list and falls back to the overall error summary.
func TestRunFailureReasons(t *testing.T) {
	run := store.SyncRun{
		ErrorSummary: "2 of 3 characters failed",
		Detail:       []byte(`{"Alpha":"profile unavailable","Beta":"raids unavailable"}`),
	}
	reasons := runFailureReasons(run)
	if len(reasons) != 2 || reasons[0] != "Alpha: profile unavailable" || reasons[1] != "Beta: raids unavailable" {
		t.Fatalf("reasons=%q, want the sorted detail entries", reasons)
	}
	empty := runFailureReasons(store.SyncRun{})
	if len(empty) != 0 {
		t.Fatalf("reasons=%q, want none", empty)
	}
	overall := runFailureReasons(store.SyncRun{ErrorSummary: "roster unavailable"})
	if len(overall) != 1 || overall[0] != "roster unavailable" {
		t.Fatalf("reasons=%q, want the overall summary", overall)
	}
}
