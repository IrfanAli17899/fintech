package services

import (
	"context"

	"github.com/google/uuid"
	"github.com/irfanali17899/fintech/internal/domain"
	"github.com/irfanali17899/fintech/internal/repositories"
)

type AccountService struct {
	accRepo *repositories.AccountRepo
}

func NewAccountService(accRepo *repositories.AccountRepo) *AccountService {
	return &AccountService{accRepo}
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
