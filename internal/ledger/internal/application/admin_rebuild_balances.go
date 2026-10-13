package application

import (
	"context"

	"github.com/irfanali17899/fintech/internal/ledger/internal/persistence"
)

func (s *Service) RebuildBalances(ctx context.Context) error {
	return s.uow.Do(ctx, func(r persistence.Stores) error {
		return r.Balances.RebuildAll(ctx)
	})
}
