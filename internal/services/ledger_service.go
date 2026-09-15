package services

import (
	"context"
	"database/sql"

	"github.com/google/uuid"
	"github.com/irfanali17899/fintech/internal/domain"
	"github.com/irfanali17899/fintech/internal/repositories"
	"github.com/irfanali17899/fintech/pkg/database"
)

type LedgerService struct {
	txRunner        database.TxRunner
	accRepo         *repositories.AccountRepo
	transactionRepo *repositories.TransactionRepo
	entryRepo       *repositories.EntryRepo
}

func NewLedgerService(txRunner database.TxRunner, accRepo *repositories.AccountRepo, transactionRepo *repositories.TransactionRepo, entryRepo *repositories.EntryRepo) *LedgerService {
	return &LedgerService{txRunner, accRepo, transactionRepo, entryRepo}
}

func (s *LedgerService) Post(ctx context.Context, t domain.Transaction) (*domain.Transaction, error) {
	if err := t.Validate(); err != nil {
		return nil, err
	}

	ids := t.AccountIDs()
	var created *domain.Transaction

	err := s.txRunner.InTx(ctx, func(tx *sql.Tx) error {
		accounts, err := s.accRepo.WithTx(tx).GetByIDs(ctx, ids)
		if err != nil {
			return err
		}

		var currency string
		// check all account exits and currency matches
		for _, id := range ids {
			acc, ok := accounts[id]
			if !ok {
				return domain.ErrAccountNotFound(id)
			}
			if currency == "" {
				currency = acc.Currency
			}

			if acc.Currency != currency {
				return domain.ErrCurrencyMismatch(acc.ID, currency, acc.Currency)
			}
		}
		created, err = s.transactionRepo.WithTx(tx).Create(ctx, t)
		if err != nil {
			return err
		}

		for i := range t.Entries {
			t.Entries[i].TransactionID = created.ID
		}

		entries, err := s.entryRepo.WithTx(tx).CreateMany(ctx, t.Entries)
		if err != nil {
			return err
		}
		created.Entries = entries
		return nil
	})

	if err != nil {
		return nil, err
	}

	return created, nil
}

func (s *LedgerService) GetTransaction(ctx context.Context, id uuid.UUID) (*domain.Transaction, error) {
	txn, err := s.transactionRepo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	entries, err := s.entryRepo.ListByTransaction(ctx, id)
	if err != nil {
		return nil, err
	}
	txn.Entries = entries
	return txn, nil

}
