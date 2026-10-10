package application

import (
	"context"
	"log"
	"time"
)

// ExpirerJob drains due holds; wrap it in a scheduler.Loop.
func (s *LedgerService) ExpirerJob(batchSize int) func(context.Context) {
	return func(ctx context.Context) {
		for {
			n, err := s.ExpireHolds(ctx, time.Now(), batchSize)
			if err != nil {
				log.Printf("expire holds: %v", err)
				return
			}
			if n < batchSize {
				return
			}
		}
	}
}
