package services

import (
	"context"
	"database/sql"

	"github.com/google/uuid"
	"github.com/irfanali17899/fintech/internal/ledger/internal/domain"
	"github.com/irfanali17899/fintech/internal/ledger/internal/repositories"
	"github.com/irfanali17899/fintech/pkg/database"
	"github.com/irfanali17899/fintech/pkg/errdefs"
)

type LedgerService struct {
	txRunner        database.TxRunner
	accRepo         *repositories.AccountRepo
	transactionRepo *repositories.TransactionRepo
	entryRepo       *repositories.EntryRepo
	balanceRepo     *repositories.BalanceRepo
	idemRepo        *repositories.IdempotencyRepo
}

func NewLedgerService(
	txRunner database.TxRunner,
	accRepo *repositories.AccountRepo,
	transactionRepo *repositories.TransactionRepo,
	entryRepo *repositories.EntryRepo,
	balanceRepo *repositories.BalanceRepo,
	idemRepo *repositories.IdempotencyRepo,
) *LedgerService {
	return &LedgerService{txRunner, accRepo, transactionRepo, entryRepo, balanceRepo, idemRepo}
}

func (s *LedgerService) Post(ctx context.Context, t domain.Transaction) (created *domain.Transaction, replayed bool, err error) {
	if err = t.Validate(); err != nil {
		return nil, false, err
	}

	ids := t.AccountIDs()
	hash := t.Hash()

	err = s.txRunner.InTx(ctx, func(tx *sql.Tx) error {
		accounts := s.accRepo.WithTx(tx)
		txns := s.transactionRepo.WithTx(tx)
		entries := s.entryRepo.WithTx(tx)
		balances := s.balanceRepo.WithTx(tx)
		keys := s.idemRepo.WithTx(tx)

		_, err := keys.Insert(ctx, t.IdempotencyKey, hash, domain.ResourceTransaction)
		if errdefs.Is(err, domain.CodeIdempotencyKeyExists) {
			existing, err := keys.Get(ctx, t.IdempotencyKey)
			if err != nil {
				return err
			}
			// same key sent with different payload, its a client bug.
			if existing.RequestHash != hash {
				return domain.ErrIdempotencyKeyReused(t.IdempotencyKey)
			}
			if existing.ResourceID == nil {
				return errdefs.Internalf(nil, "idempotency key %s committed without resource", t.IdempotencyKey)
			}
			created, err = s.loadTransaction(ctx, txns, entries, *existing.ResourceID)
			replayed = true
			return err
		}

		if err != nil {
			return err
		}

		accMap, err := accounts.GetByIDs(ctx, ids)
		if err != nil {
			return err
		}

		var currency string
		// check all account exits and currency matches
		for _, id := range ids {
			acc, ok := accMap[id]
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
		created, err = txns.Create(ctx, t)
		if err != nil {
			return err
		}

		for i := range t.Entries {
			t.Entries[i].TransactionID = created.ID
		}

		ents, err := entries.CreateMany(ctx, t.Entries)
		if err != nil {
			return err
		}
		created.Entries = ents

		for id, sums := range t.SumsByAccount() {
			if err := balances.Apply(ctx, id, sums); err != nil {
				return err
			}
		}
		return keys.SetResource(ctx, t.IdempotencyKey, created.ID)
	})

	if err != nil {
		return nil, false, err
	}

	return created, replayed, nil
}

// reusable as tx and non tx version
func (s *LedgerService) loadTransaction(ctx context.Context, txns *repositories.TransactionRepo, ents *repositories.EntryRepo, id uuid.UUID) (*domain.Transaction, error) {
	t, err := txns.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	t.Entries, err = ents.ListByTransaction(ctx, id)
	if err != nil {
		return nil, err
	}
	return t, nil
}

func (s *LedgerService) GetTransaction(ctx context.Context, id uuid.UUID) (*domain.Transaction, error) {
	return s.loadTransaction(ctx, s.transactionRepo, s.entryRepo, id)
}
