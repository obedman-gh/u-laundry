package security

import (
	"sync"
	"time"
)

type bucket struct {
	count    int
	until    time.Time
	notified bool
}
type Limit struct {
	Key    string
	Count  int
	Window time.Duration
}

// Limiter has bounded storage; live buckets are never evicted to bypass limits.
// It is per process. Deploy behind one instance or replace with a shared limiter.
type Limiter struct {
	mu       sync.Mutex
	buckets  map[string]bucket
	Capacity int
}

func (l *Limiter) Take(now time.Time, limits ...Limit) (bool, time.Duration, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.buckets == nil {
		l.buckets = map[string]bucket{}
	}
	capacity := l.Capacity
	if capacity == 0 {
		capacity = 4096
	}
	for key, b := range l.buckets {
		if !now.Before(b.until) {
			delete(l.buckets, key)
		}
	}
	for _, limit := range limits {
		b := l.buckets[limit.Key]
		if b.count >= limit.Count {
			notify := !b.notified
			b.notified = true
			l.buckets[limit.Key] = b
			return false, b.until.Sub(now), notify
		}
	}
	needed := 0
	for _, limit := range limits {
		if _, ok := l.buckets[limit.Key]; !ok {
			needed++
		}
	}
	if len(l.buckets)+needed > capacity {
		return false, time.Minute, false
	}
	for _, limit := range limits {
		b, exists := l.buckets[limit.Key]
		if !exists {
			b.until = now.Add(limit.Window)
		}
		b.count++
		l.buckets[limit.Key] = b
	}
	return true, 0, false
}
