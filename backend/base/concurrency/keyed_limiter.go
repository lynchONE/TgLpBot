package concurrency

import (
	"log"
	"sync"
)

// KeyedLimiter runs at most 1 job per key at a time, with a global max parallelism.
// TryRun never blocks the caller; if the key is already running or the global limit is reached,
// it returns false and the job is skipped.
type KeyedLimiter struct {
	mu       sync.Mutex
	inflight map[string]struct{}
	sem      chan struct{}
}

func NewKeyedLimiter(maxParallel int) *KeyedLimiter {
	if maxParallel <= 0 {
		maxParallel = 1
	}
	return &KeyedLimiter{
		inflight: make(map[string]struct{}),
		sem:      make(chan struct{}, maxParallel),
	}
}

func (l *KeyedLimiter) TryRun(key string, fn func()) bool {
	if l == nil || fn == nil || key == "" {
		return false
	}

	l.mu.Lock()
	if _, ok := l.inflight[key]; ok {
		l.mu.Unlock()
		return false
	}

	select {
	case l.sem <- struct{}{}:
		l.inflight[key] = struct{}{}
		l.mu.Unlock()
	default:
		l.mu.Unlock()
		return false
	}

	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("[KeyedLimiter] panic key=%s: %v", key, r)
			}
			l.mu.Lock()
			delete(l.inflight, key)
			l.mu.Unlock()
			<-l.sem
		}()
		fn()
	}()

	return true
}
