package application

import (
	"context"
	"database/sql"

	"github.com/irfanali17899/fintech/internal/ledger/internal/domain"
)

func (s *Service) CreateAccount(ctx context.Context, a domain.Account) (*domain.Account, error) {
	if err := a.Validate(); err != nil {
		return nil, err
	}

	var created *domain.Account

	// create account and its zero balance row
	err := s.txRunner.InTx(ctx, func(tx *sql.Tx) error {
		accounts := s.accRepo.WithTx(tx)
		balances := s.balanceRepo.WithTx(tx)

		acc, err := accounts.Create(ctx, a)
		if err != nil {
			return err
		}
		created = acc
		return balances.Apply(ctx, created.ID, domain.AccountSums{})
	})

	if err != nil {
		return nil, err
	}

	return created, nil
}

func (s *Service) SeedSystem(ctx context.Context) error {
	return s.txRunner.InTx(ctx, func(tx *sql.Tx) error {
		accounts := s.accRepo.WithTx(tx)
		balances := s.balanceRepo.WithTx(tx)

		ids, err := accounts.SeedAccounts(ctx)
		if err != nil {
			return err
		}
		for _, id := range ids {
			if err := balances.Apply(ctx, id, domain.AccountSums{}); err != nil {
				return err
			}
		}
		return nil
	})
}
