package application

import (
	"context"
	"database/sql"

	"github.com/irfanali17899/fintech/internal/ledger/internal/domain"
)

func (s *Service) VerifyBalances(ctx context.Context) ([]domain.BalanceMismatch, error) {
	var result []domain.BalanceMismatch

	err := s.txRunner.InTx(ctx, func(tx *sql.Tx) error {
		out, err := s.balanceRepo.WithTx(tx).Verify(ctx)
		if err != nil {
			return err
		}
		result = out
		return nil
	})

	return result, err
}
