package application

import (
	"context"
	"database/sql"

	"github.com/irfanali17899/fintech/internal/ledger/internal/domain"
	"github.com/irfanali17899/fintech/internal/ledger/internal/persistence"
	"github.com/irfanali17899/fintech/pkg/database"
)

type AdminService struct {
	txRunner    database.TxRunner
	balanceRepo *persistence.BalanceRepo
}

func NewAdminService(txRunner database.TxRunner, balanceRepo *persistence.BalanceRepo) *AdminService {
	return &AdminService{txRunner, balanceRepo}
}

func (s *AdminService) RebuildBalances(ctx context.Context) error {
	return s.txRunner.InTx(ctx, func(tx *sql.Tx) error {
		return s.balanceRepo.WithTx(tx).RebuildAll(ctx)
	})
}

func (s *AdminService) VerifyBalances(ctx context.Context) ([]domain.BalanceMismatch, error) {
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
