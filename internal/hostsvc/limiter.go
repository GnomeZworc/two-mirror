package hostsvc

import (
	"sync"
	"time"
)

type limiter struct {
	mu     sync.Mutex
	rate   float64
	tokens float64
	last   time.Time
	now    func() time.Time
}

func newLimiter(rate int, now func() time.Time) *limiter {
	return &limiter{rate: float64(rate), tokens: float64(rate), last: now(), now: now}
}

func (l *limiter) allow() bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	t := l.now()
	l.tokens += t.Sub(l.last).Seconds() * l.rate
	if l.tokens > l.rate {
		l.tokens = l.rate
	}
	l.last = t
	if l.tokens < 1 {
		return false
	}
	l.tokens--
	return true
}
