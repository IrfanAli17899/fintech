package application

import (
	"context"

	"github.com/google/uuid"
	"github.com/irfanali17899/fintech/internal/ledger/internal/domain"
)

func (s *Service) GetTransaction(ctx context.Context, id uuid.UUID) (*domain.Transaction, error) {
	return s.loadTransaction(ctx, s.transactionRepo, s.entryRepo, id)
}

// Children returns the transactions that capture or reverse the given one.
func (s *Service) Children(ctx context.Context, parentID uuid.UUID) ([]domain.Transaction, error) {
	return s.transactionRepo.ListByParent(ctx, parentID)
}
