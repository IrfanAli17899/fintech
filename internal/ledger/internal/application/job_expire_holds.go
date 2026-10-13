package application

import (
	"context"
	"database/sql"
	"log"
	"time"
)

// ExpireHolds voids internal pending transactions past their expiry, at most
// batchSize, one transaction per hold: the row is selected FOR UPDATE SKIP
// LOCKED and voided in the same transaction, so replicas partition the work
// and no transaction ever holds balance locks for more than one hold.
// Rail-backed holds are never expired here; their rail resolves them.
func (s *Service) ExpireHolds(ctx context.Context, now time.Time, batchSize int) (int, error) {
	voided := 0
	for voided < batchSize {
		found := false
		err := s.txRunner.InTx(ctx, func(tx *sql.Tx) error {
			r := s.repos(tx)
			id, ok, err := r.txns.NextExpiredHold(ctx, now)
			if err != nil || !ok {
				return err
			}
			found = true
			_, _, err = s.voidLocked(ctx, r, VoidCommand{HoldID: id, IdempotencyKey: "expire:" + id.String(), Reason: "expired"})
			return err
		})
		if err != nil {
			return voided, err
		}
		if !found {
			return voided, nil
		}
		voided++
	}
	return voided, nil
}

// ExpirerJob drains due holds; wrap it in a scheduler.Loop.
func (s *Service) ExpirerJob(batchSize int) func(context.Context) {
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
