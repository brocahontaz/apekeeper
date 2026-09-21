package blizzard

import (
	"context"
	"math/rand"
	"net/http"
	"strconv"
	"sync"
	"time"
)

const (
	DefaultRequestsPerSecond = 100
	DefaultBurst             = 50
	MaxAttempts              = 5
)

type Limiter struct {
	tokens chan struct{}
	stop   chan struct{}
	once   sync.Once
}

func NewLimiter(rate, burst int, interval time.Duration) *Limiter {
	if rate <= 0 {
		rate = DefaultRequestsPerSecond
	}
	if burst <= 0 {
		burst = DefaultBurst
	}
	l := &Limiter{make(chan struct{}, burst), make(chan struct{}), sync.Once{}}
	for i := 0; i < burst; i++ {
		l.tokens <- struct{}{}
	}
	go func() {
		t := time.NewTicker(interval / time.Duration(rate))
		defer t.Stop()
		for {
			select {
			case <-t.C:
				select {
				case l.tokens <- struct{}{}:
				default:
				}
			case <-l.stop:
				return
			}
		}
	}()
	return l
}
func (l *Limiter) Wait(ctx context.Context) error {
	select {
	case <-l.tokens:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
func (l *Limiter) Close() { l.once.Do(func() { close(l.stop) }) }
func RetryAfter(r *http.Response) time.Duration {
	if v := r.Header.Get("Retry-After"); v != "" {
		if n, e := strconv.Atoi(v); e == nil {
			return time.Duration(n) * time.Second
		}
		if t, e := http.ParseTime(v); e == nil {
			return time.Until(t)
		}
	}
	return 0
}
func Backoff(attempt int) time.Duration {
	return time.Duration(1<<attempt)*50*time.Millisecond + time.Duration(rand.Intn(25))*time.Millisecond
}
