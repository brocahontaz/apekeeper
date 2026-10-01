package sync

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/brocahontaz/apekeeper/backend/internal/domain"
	"github.com/brocahontaz/apekeeper/backend/internal/notify"
	"github.com/brocahontaz/apekeeper/backend/internal/store"
)

// Notifier receives a summary after each finished run and reports what its
// delivery attempt did. Implementations must be safe for concurrent use and
// must never block or fail the sync result.
type Notifier interface {
	NotifySyncRun(notify.SyncRunSummary) notify.Outcome
}

type Service struct {
	Engine Engine
	Guild  domain.Guild
	// Log is optional; when nil the service stays silent.
	Log *slog.Logger
	// Notifier is optional; when nil no external notifications are sent.
	Notifier Notifier
	mu       sync.Mutex
	running  bool
	started  time.Time
	last     store.SyncRun
	// progress is the live in-memory snapshot reported to the API; updates
	// are never persisted, Finish records the final outcome in the store.
	progress domain.Progress
	cancel   context.CancelFunc
	done     chan struct{}
}

func (s *Service) Start(ctx context.Context, trigger string) (store.SyncRun, error) {
	r, runCtx, e := s.begin(ctx, ctx, trigger)
	if e != nil {
		return r, e
	}
	r, e = s.Engine.RunGuildSyncWithRun(runCtx, s.Guild, r)
	s.complete(r)
	return r, e
}

// StartManual creates the run before returning so callers can report its ID,
// then completes it asynchronously.
func (s *Service) StartManual(ctx context.Context) (store.SyncRun, error) {
	return s.startAsync(ctx, "manual", RunOptions{})
}

// StartDryRun validates and fetches the full roster without changing guild
// data. Its ledger entry is explicitly marked dry-run.
func (s *Service) StartDryRun(ctx context.Context) (store.SyncRun, error) {
	return s.startAsync(ctx, "dry-run", RunOptions{DryRun: true})
}

func (s *Service) StartRetry(ctx context.Context, sourceID int64) (store.SyncRun, error) {
	source, err := s.Engine.Stores.SyncRuns.ByID(ctx, s.Guild.ID, sourceID)
	if err != nil {
		return store.SyncRun{}, ErrRetryUnavailable
	}
	if source.Status == domain.RunRunning || source.Failed == 0 {
		return store.SyncRun{}, ErrRetryUnavailable
	}
	var detail map[string]string
	_ = json.Unmarshal(source.Detail, &detail)
	if len(detail) == 0 {
		return store.SyncRun{}, ErrRetryUnavailable
	}
	names := make(map[string]bool, len(detail))
	for n := range detail {
		names[strings.ToLower(n)] = true
	}
	return s.startAsync(ctx, "retry", RunOptions{Names: names})
}

func (s *Service) startAsync(ctx context.Context, trigger string, options RunOptions) (store.SyncRun, error) {
	// The request context is only for admitting the run. The run itself must
	// outlive the HTTP request and be controlled by Cancel or Shutdown.
	r, runCtx, e := s.begin(ctx, context.Background(), trigger)
	if e != nil {
		return r, e
	}
	s.mu.Lock()
	s.progress.DryRun = options.DryRun
	s.mu.Unlock()
	go func(run store.SyncRun) {
		completed, _ := s.Engine.RunGuildSyncWithOptions(runCtx, s.Guild, run, options)
		s.complete(completed)
	}(r)
	return r, nil
}

// begin admits a run and installs its cancellation lifecycle before any engine
// work starts. createCtx controls only the ledger write, while runParent
// controls the work itself (asynchronous requests deliberately use a detached
// parent so their request cancellation cannot abort an admitted run).
func (s *Service) begin(createCtx, runParent context.Context, trigger string) (store.SyncRun, context.Context, error) {
	s.mu.Lock()
	if !domain.CanStart(s.running) {
		s.mu.Unlock()
		return store.SyncRun{}, nil, ErrAlreadyRunning
	}
	s.running = true
	r, e := s.Engine.Stores.SyncRuns.Create(createCtx, s.Guild.ID, trigger)
	var runCtx context.Context
	if e != nil {
		s.running = false
		// A run that never started leaves nothing live to report.
		s.progress = domain.Progress{}
	} else {
		runCtx, s.cancel = context.WithCancel(runParent)
		s.done = make(chan struct{})
		s.started = time.Now()
		s.progress = domain.Progress{
			Active:    true,
			RunID:     r.ID,
			Status:    domain.RunRunning,
			Phase:     domain.PhaseQueued,
			StartedAt: s.started,
		}
	}
	s.mu.Unlock()
	if e == nil && s.Log != nil {
		s.Log.Info("sync run started", "runId", r.ID, "guild", s.Guild.Slug, "trigger", trigger)
	}
	return r, runCtx, e
}

// ReportProgress folds an engine progress event into the live snapshot. The
// engine fires it from its counter critical section, so this update is
// guarded here and stays free of any blocking work.
func (s *Service) ReportProgress(phase domain.Phase, updated, failed, total int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.running {
		return
	}
	s.progress.Phase = domain.CoercePhase(phase)
	s.progress.Updated = updated
	s.progress.Failed = failed
	s.progress.Total = total
}

// Progress reports the live run snapshot; the last snapshot stays visible
// with Active=false after completion so clients can observe the terminal
// transition. A service that has never run answers with the zero snapshot.
func (s *Service) Progress() domain.Progress {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.progress
}

func (s *Service) complete(r store.SyncRun) {
	s.mu.Lock()
	s.running = false
	s.cancel = nil
	done := s.done
	s.done = nil
	s.last = r
	started := s.started
	// The terminal snapshot keeps the run's final counts visible; the next
	// begin overwrites it.
	s.progress.Active = false
	s.progress.Status = r.Status
	s.progress.Phase = domain.PhaseFinalizing
	s.progress.Updated = r.Updated
	s.progress.Failed = r.Failed
	s.progress.Total = r.Total
	s.mu.Unlock()
	if done != nil {
		close(done)
	}
	// Single completion log covering both the synchronous and the manual
	// asynchronous paths.
	if s.Log != nil {
		s.Log.Info("sync run completed",
			"runId", r.ID,
			"status", string(r.Status),
			"total", r.Total,
			"updated", r.Updated,
			"failed", r.Failed,
			"duration", time.Since(started).String(),
		)
	}
	// The notification fans out after the run is settled and must neither
	// delay nor affect its outcome; delivery errors stay inside the notifier.
	if s.Notifier != nil {
		summary := notify.SyncRunSummary{
			Guild:    s.Guild.Name,
			Trigger:  r.Trigger,
			Status:   string(r.Status),
			Total:    r.Total,
			Updated:  r.Updated,
			Failed:   r.Failed,
			Duration: time.Since(started),
			Reasons:  runFailureReasons(r),
		}
		go s.recordNotification(summary, r.ID)
	}
}

// recordNotification sends one notification and logs plus persists its
// outcome, best-effort on both counts: neither the delivery nor the
// bookkeeping may change the run's stored status or result, and no logged
// value ever carries the webhook URL.
func (s *Service) recordNotification(summary notify.SyncRunSummary, runID int64) {
	o := s.Notifier.NotifySyncRun(summary)
	if s.Log != nil {
		switch o.Status {
		case notify.OutcomeFailed:
			s.Log.Warn("sync notification failed", "runId", runID, "detail", o.Detail)
		default:
			s.Log.Info("sync notification outcome", "runId", runID, "status", o.Status)
		}
	}
	// A detached context with its own deadline: the caller's request may be
	// gone by the time the fan-out lands, but the outcome still deserves a
	// bounded write attempt.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if e := s.Engine.Stores.SyncRuns.SetNotifyStatus(ctx, runID, o.Status); e != nil && s.Log != nil {
		s.Log.Warn("sync notification status persist failed", "runId", runID, "error", e)
	}
}

// runFailureReasons flattens the per-character detail map and, when there is
// no detail, the overall error summary into one deterministic reason list.
func runFailureReasons(r store.SyncRun) []string {
	details := map[string]string{}
	if len(r.Detail) > 0 {
		_ = json.Unmarshal(r.Detail, &details)
	}
	reasons := make([]string, 0, len(details))
	for name, err := range details {
		reasons = append(reasons, name+": "+err)
	}
	sort.Strings(reasons)
	if r.ErrorSummary != "" && len(reasons) == 0 {
		reasons = append(reasons, r.ErrorSummary)
	}
	return reasons
}

var ErrAlreadyRunning = &runningError{}
var ErrRetryUnavailable = errors.New("retry unavailable: completed run has no recorded character failures")
var ErrNotRunning = errors.New("no sync is running")

type runningError struct{}

func (*runningError) Error() string { return "sync already running" }
func (s *Service) Cancel() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.running || s.cancel == nil {
		return ErrNotRunning
	}
	s.cancel()
	return nil
}

// Shutdown cancels a manual operation and waits for its engine workers to
// finish, so process shutdown does not leave detached sync goroutines behind.
func (s *Service) Shutdown(ctx context.Context) error {
	s.mu.Lock()
	done := s.done
	if s.cancel != nil {
		s.cancel()
	}
	s.mu.Unlock()
	if done == nil {
		return nil
	}
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
func (s *Service) Status() store.SyncRun { s.mu.Lock(); defer s.mu.Unlock(); return s.last }
func (s *Service) History(ctx context.Context, limit int) ([]store.SyncRun, error) {
	return s.Engine.Stores.SyncRuns.History(ctx, s.Guild.ID, limit)
}
