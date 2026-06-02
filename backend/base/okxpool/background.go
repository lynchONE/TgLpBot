package okxpool

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

func StartDefaultHealthChecker(interval time.Duration) {
	if interval <= 0 {
		interval = 30 * time.Minute
	}
	bgOnce.Do(func() {
		ctx, cancel := context.WithCancel(context.Background())
		bgStop = cancel
		go func() {
			t := time.NewTicker(interval)
			defer t.Stop()

			m := Default()
			if err := m.CheckAllOnce(ctx); err != nil {
				log.Printf("[okxpool] health check failed: %v", err)
			}
			for {
				select {
				case <-ctx.Done():
					return
				case <-t.C:
					if err := m.CheckAllOnce(ctx); err != nil {
						log.Printf("[okxpool] health check failed: %v", err)
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
