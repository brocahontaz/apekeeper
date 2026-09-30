package sync

import (
	"context"
	"encoding/json"
	"log/slog"
	"sort"
	"sync"
	"time"

	"github.com/brocahontaz/apekeeper/backend/internal/domain"
	"github.com/brocahontaz/apekeeper/backend/internal/notify"
	"github.com/brocahontaz/apekeeper/backend/internal/store"
)

// Notifier receives a summary after each finished run. Implementations must
// be safe for concurrent use and must never block the sync result.
type Notifier interface {
	NotifySyncRun(notify.SyncRunSummary)
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
}

func (s *Service) Start(ctx context.Context, trigger string) (store.SyncRun, error) {
	r, e := s.begin(ctx, trigger)
	if e != nil {
		return r, e
	}
	r, e = s.Engine.RunGuildSyncWithRun(ctx, s.Guild, r)
	s.complete(r)
	return r, e
}

// StartManual creates the run before returning so callers can report its ID,
// then completes it asynchronously.
func (s *Service) StartManual(ctx context.Context) (store.SyncRun, error) {
	r, e := s.begin(ctx, "manual")
	if e != nil {
		return r, e
	}
	go func() {
		r, _ = s.Engine.RunGuildSyncWithRun(context.WithoutCancel(ctx), s.Guild, r)
		s.complete(r)
	}()
	return r, nil
}

func (s *Service) begin(ctx context.Context, trigger string) (store.SyncRun, error) {
	s.mu.Lock()
	if !domain.CanStart(s.running) {
		s.mu.Unlock()
		return store.SyncRun{}, ErrAlreadyRunning
	}
	s.running = true
	r, e := s.Engine.Stores.SyncRuns.Create(ctx, s.Guild.ID, trigger)
	if e != nil {
		s.running = false
	} else {
		s.started = time.Now()
	}
	s.mu.Unlock()
	if e == nil && s.Log != nil {
		s.Log.Info("sync run started", "runId", r.ID, "guild", s.Guild.Slug, "trigger", trigger)
	}
	return r, e
}

func (s *Service) complete(r store.SyncRun) {
	s.mu.Lock()
	s.running = false
	s.last = r
	started := s.started
	s.mu.Unlock()
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
		go s.Notifier.NotifySyncRun(summary)
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

type runningError struct{}

func (*runningError) Error() string      { return "sync already running" }
func (s *Service) Status() store.SyncRun { s.mu.Lock(); defer s.mu.Unlock(); return s.last }
func (s *Service) History(ctx context.Context, limit int) ([]store.SyncRun, error) {
	return s.Engine.Stores.SyncRuns.History(ctx, s.Guild.ID, limit)
}
