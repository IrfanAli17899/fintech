package application

import (
	"context"

	"github.com/irfanali17899/fintech/ledger/internal/domain"
	"github.com/irfanali17899/fintech/ledger/internal/persistence"
)

func (s *Service) CreateAccount(ctx context.Context, a domain.Account) (*domain.Account, error) {
	if err := a.Validate(); err != nil {
		return nil, err
	}

	var created *domain.Account

	// create account and its zero balance row
	err := s.uow.Do(ctx, func(r persistence.Stores) error {
		accounts, balances := r.Accounts, r.Balances

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
	return s.uow.Do(ctx, func(r persistence.Stores) error {
		accounts, balances := r.Accounts, r.Balances

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
