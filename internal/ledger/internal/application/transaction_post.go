package application

import (
	"context"
	"database/sql"

	"github.com/irfanali17899/fintech/internal/ledger/internal/domain"
)

// Post records a balanced transaction atomically. The bool is true when the
// idempotency key had already been used and the original transaction is returned.
func (s *Service) Post(ctx context.Context, t domain.Transaction) (*domain.Transaction, bool, error) {
	if t.Status == "" {
		t.Status = domain.TransactionStatusPosted
	}
	if err := t.Validate(); err != nil {
		return nil, false, err
	}

	var (
		created  *domain.Transaction
		replayed bool
	)
	err := s.txRunner.InTx(ctx, func(tx *sql.Tx) error {
		r := s.repos(tx)

		rp, err := claimKey(ctx, r.keys, t.IdempotencyKey, t.Hash(), domain.ResourceTransaction)
		if err != nil {
			return err
		}
		if rp != nil {
			created, err = s.loadTransaction(ctx, r.txns, r.entries, rp.ResourceID)
			replayed = true
			return err
		}

		created, err = s.post(ctx, r, t)
		if err != nil {
			return err
		}
		return r.keys.SetResource(ctx, t.IdempotencyKey, created.ID)
	})
	if err != nil {
		return nil, false, err
	}
	return created, replayed, nil
}
