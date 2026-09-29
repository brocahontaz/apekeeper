package sched

import (
	"context"
	"log/slog"
	"sync/atomic"
	"testing"
	"time"
)

func TestSweeperRunsImmediatelyThenStopsOnCancel(t *testing.T) {
	var calls atomic.Int64
	ctx, cancel := context.WithCancel(context.Background())
	s := NewSweeper(10*time.Millisecond, slog.New(slog.DiscardHandler))
	done := make(chan struct{})
	go func() {
		defer close(done)
		s.Run(ctx, func(context.Context) { calls.Add(1) })
	}()
	// The first call happens immediately at startup; two more prove ticking.
	deadline := time.After(2 * time.Second)
	for calls.Load() < 3 {
		select {
		case <-deadline:
			t.Fatalf("sweeper ran %d times, want at least 3", calls.Load())
		case <-time.After(time.Millisecond):
		}
	}
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("sweeper did not stop after context cancellation")
	}
}

func TestSweeperRejectsNonPositiveInterval(t *testing.T) {
	s := NewSweeper(0, slog.New(slog.DiscardHandler))
	done := make(chan struct{})
	go func() {
		defer close(done)
		s.Run(context.Background(), func(context.Context) {
			t.Error("sweeper ran despite a non-positive interval")
		})
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("sweeper did not return with a non-positive interval")
	}
}
