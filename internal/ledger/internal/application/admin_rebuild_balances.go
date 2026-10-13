package application

import (
	"context"
	"database/sql"
)

func (s *Service) RebuildBalances(ctx context.Context) error {
	return s.txRunner.InTx(ctx, func(tx *sql.Tx) error {
		return s.balanceRepo.WithTx(tx).RebuildAll(ctx)
	})
}
