package rpcpool

import (
	"context"
	"log"
	"sync"
	"time"
)

var (
	bgOnce sync.Once
	bgStop context.CancelFunc
)

// StartDefaultHealthChecker starts a background health checker that probes RPC endpoints
// and updates their availability/status fields in DB. It is safe to call multiple times.
func StartDefaultHealthChecker(interval time.Duration) {
	if interval <= 0 {
		interval = time.Hour
	}
	bgOnce.Do(func() {
		ctx, cancel := context.WithCancel(context.Background())
		bgStop = cancel
		go func() {
			t := time.NewTicker(interval)
			defer t.Stop()

			m := Default()
			// Run once at startup.
			if err := m.CheckAllOnce(ctx); err != nil {
				log.Printf("[rpcpool] health check failed: %v", err)
			}
			for {
				select {
				case <-ctx.Done():
					return
				case <-t.C:
					if err := m.CheckAllOnce(ctx); err != nil {
						log.Printf("[rpcpool] health check failed: %v", err)
					}
				}
			}
		}()
	})
}

func StopDefaultHealthChecker() {
	if bgStop != nil {
		bgStop()
	}
}
