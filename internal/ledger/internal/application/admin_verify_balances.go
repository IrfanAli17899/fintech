package application

import (
	"context"

	"github.com/irfanali17899/fintech/internal/ledger/internal/domain"
	"github.com/irfanali17899/fintech/internal/ledger/internal/persistence"
)

func (s *Service) VerifyBalances(ctx context.Context) ([]domain.BalanceMismatch, error) {
	var result []domain.BalanceMismatch

	err := s.uow.Do(ctx, func(r persistence.Stores) error {
		out, err := r.Balances.Verify(ctx)
		if err != nil {
			return err
		}
		result = out
		return nil
	})

	return result, err
}
