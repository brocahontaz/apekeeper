package sched

import (
	"context"
	"fmt"
	"log/slog"
	"time"
)

type Scheduler struct {
	Schedule   string
	TriggerNow chan struct{}
	Log        *slog.Logger
}

func New(schedule string, log *slog.Logger) *Scheduler {
	return &Scheduler{Schedule: schedule, TriggerNow: make(chan struct{}, 1), Log: log}
}
func (s *Scheduler) Trigger() {
	select {
	case s.TriggerNow <- struct{}{}:
	default:
	}
}
func (s *Scheduler) Run(ctx context.Context, fn func(context.Context)) {
	for {
		next, err := s.next(time.Now())
		if err != nil {
			s.Log.Error("invalid sync schedule", "error", err)
			return
		}
		timer := time.NewTimer(time.Until(next))
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-s.TriggerNow:
			timer.Stop()
			s.Log.Info("manual sync trigger")
			fn(ctx)
		case <-timer.C:
			s.Log.Info("scheduled sync trigger")
			fn(ctx)
		}
	}
}
func (s *Scheduler) next(now time.Time) (time.Time, error) {
	t, e := time.Parse("15:04", s.Schedule)
	if e != nil {
		return time.Time{}, fmt.Errorf("SYNC_SCHEDULE: %w", e)
	}
	u := now.UTC()
	n := time.Date(u.Year(), u.Month(), u.Day(), t.Hour(), t.Minute(), 0, 0, time.UTC)
	if !n.After(u) {
		n = n.AddDate(0, 0, 1)
	}
	return n, nil
}
