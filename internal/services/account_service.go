package services

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/irfanali17899/fintech/internal/domain"
	"github.com/irfanali17899/fintech/internal/repositories"
)

type AccountService struct {
	accRepo   *repositories.AccountRepo
	entryRepo *repositories.EntryRepo
}

func NewAccountService(accRepo *repositories.AccountRepo, entryRepo *repositories.EntryRepo) *AccountService {
	return &AccountService{accRepo, entryRepo}
}

func (s *AccountService) CreateAccount(ctx context.Context, a domain.Account) (*domain.Account, error) {
	if err := a.Validate(); err != nil {
		return nil, err
	}
	return s.accRepo.Create(ctx, a)
}

func (s *AccountService) GetAccount(ctx context.Context, id uuid.UUID) (*domain.Account, error) {
	return s.accRepo.GetByID(ctx, id)
}

func (s *AccountService) GetAccountBalance(ctx context.Context, id uuid.UUID) (*domain.Balance, error) {
	acc, err := s.accRepo.GetByID(ctx, id)

	if err != nil {
		return nil, err
	}

	debits, credits, err := s.entryRepo.SumByAccount(ctx, id)

	if err != nil {
		return nil, err
	}

	balance := acc.Type.NormalBalance(debits, credits)
	return &domain.Balance{
		AccountID: id,
		Currency:  acc.Currency,
		Amount:    balance,
		AsOf:      time.Now(),
	}, nil

}

func (s *AccountService) ListByOwner(ctx context.Context, ownerType domain.OwnerType, ownerID string) ([]domain.Account, error) {
	if !ownerType.Valid() {
		return nil, domain.ErrInvalidOwnerType(ownerType)
	}
	return s.accRepo.ListByOwner(ctx, ownerType, ownerID)
}
