package sync

import (
	"context"
	"github.com/brocahontaz/apekeeper/backend/internal/domain"
	"github.com/brocahontaz/apekeeper/backend/internal/store"
	"sync"
)

type Service struct {
	Engine  Engine
	Guild   domain.Guild
	mu      sync.Mutex
	running bool
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
	}
	s.mu.Unlock()
	return r, e
}

func (s *Service) complete(r store.SyncRun) {
	s.mu.Lock()
	s.running = false
	s.last = r
	s.mu.Unlock()
}

var ErrAlreadyRunning = &runningError{}

type runningError struct{}

func (*runningError) Error() string      { return "sync already running" }
func (s *Service) Status() store.SyncRun { s.mu.Lock(); defer s.mu.Unlock(); return s.last }
func (s *Service) History(ctx context.Context, limit int) ([]store.SyncRun, error) {
	return s.Engine.Stores.SyncRuns.History(ctx, s.Guild.ID, limit)
}
