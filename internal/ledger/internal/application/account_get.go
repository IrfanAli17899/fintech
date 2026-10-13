package application

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/irfanali17899/fintech/internal/ledger/internal/domain"
)

func (s *Service) GetAccount(ctx context.Context, id uuid.UUID) (*domain.Account, error) {
	return s.stores.Accounts.GetByID(ctx, id)
}

func (s *Service) GetAccountBalance(ctx context.Context, id uuid.UUID) (*domain.Balance, error) {
	acc, err := s.stores.Accounts.GetByID(ctx, id)

	if err != nil {
		return nil, err
	}

	sums, err := s.stores.Balances.GetBalance(ctx, id)

	if err != nil {
		return nil, err
	}

	b := acc.Type.Balance(*sums)
	b.AccountID, b.Currency, b.AsOf = id, acc.Currency, time.Now()

	return &b, nil

}

func (s *Service) ListByOwner(ctx context.Context, ownerType domain.OwnerType, ownerID string) ([]domain.Account, error) {
	if !ownerType.Valid() {
		return nil, domain.ErrInvalidOwnerType(ownerType)
	}
	return s.stores.Accounts.ListByOwner(ctx, ownerType, ownerID)
}
