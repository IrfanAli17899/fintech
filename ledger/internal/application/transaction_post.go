package application

import (
	"context"

	"github.com/irfanali17899/fintech/ledger/internal/domain"
	"github.com/irfanali17899/fintech/ledger/internal/persistence"
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
	err := s.uow.Do(ctx, func(r persistence.Stores) error {

		rp, err := claimKey(ctx, r.Keys, t.IdempotencyKey, t.Hash(), domain.ResourceTransaction)
		if err != nil {
			return err
		}
		if rp != nil {
			created, err = s.loadTransaction(ctx, r.Transactions, r.Entries, rp.ResourceID)
			replayed = true
			return err
		}

		created, err = s.post(ctx, r, t)
		if err != nil {
			return err
		}
		return r.Keys.SetResource(ctx, t.IdempotencyKey, created.ID)
	})
	if err != nil {
		return nil, false, err
	}
	return created, replayed, nil
}
