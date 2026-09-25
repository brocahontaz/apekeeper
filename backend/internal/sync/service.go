package sync

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/brocahontaz/apekeeper/backend/internal/domain"
	"github.com/brocahontaz/apekeeper/backend/internal/store"
)

type Service struct {
	Engine Engine
	Guild  domain.Guild
	// Log is optional; when nil the service stays silent.
	Log     *slog.Logger
	mu      sync.Mutex
	running bool
	started time.Time
	last    store.SyncRun
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
}

var ErrAlreadyRunning = &runningError{}

type runningError struct{}

func (*runningError) Error() string      { return "sync already running" }
func (s *Service) Status() store.SyncRun { s.mu.Lock(); defer s.mu.Unlock(); return s.last }
func (s *Service) History(ctx context.Context, limit int) ([]store.SyncRun, error) {
	return s.Engine.Stores.SyncRuns.History(ctx, s.Guild.ID, limit)
}
