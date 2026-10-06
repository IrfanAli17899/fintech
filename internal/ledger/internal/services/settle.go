package services

import (
	"context"
	"log"
	"time"
)

// SettleJob drains due attempts; wrap it in a scheduler.Loop.
func (s *LedgerService) SettleJob(batchSize int) func(context.Context) {
	return func(ctx context.Context) {
		for {
			st, bo, err := s.SettleDue(ctx, time.Now(), batchSize)
			if err != nil {
				log.Printf("settle due: %v", err)
				return
			}
			if st+bo < batchSize {
				return
			}
		}
	}
}
