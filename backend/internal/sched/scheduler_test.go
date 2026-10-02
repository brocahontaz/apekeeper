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

func TestSchedulersKeepGuildRunsIsolatedWhenTriggeredConcurrently(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	first, second := New("23:59", slog.New(slog.DiscardHandler)), New("23:59", slog.New(slog.DiscardHandler))
	runs := make(chan string, 2)
	done := make(chan struct{}, 2)
	go func() { first.Run(ctx, func(context.Context) { runs <- "one"; done <- struct{}{} }) }()
	go func() { second.Run(ctx, func(context.Context) { runs <- "two"; done <- struct{}{} }) }()
	first.Trigger()
	second.Trigger()
	for range 2 {
		select {
		case <-done:
		case <-time.After(time.Second):
			t.Fatal("triggered scheduler did not run")
		}
	}
	close(done)
	seen := map[string]bool{}
	for range 2 {
		seen[<-runs] = true
	}
	if !seen["one"] || !seen["two"] {
		t.Fatalf("scheduler runs were not isolated: %v", seen)
	}
}
