package services

import (
	"context"
	"database/sql"

	"github.com/google/uuid"
	"github.com/irfanali17899/fintech/internal/ledger/internal/domain"
	"github.com/irfanali17899/fintech/internal/ledger/internal/repositories"
	"github.com/irfanali17899/fintech/pkg/database"
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
	return &LedgerService{
		txRunner:        txRunner,
		accRepo:         accRepo,
		transactionRepo: transactionRepo,
		entryRepo:       entryRepo,
		balanceRepo:     balanceRepo,
		idemRepo:        idemRepo,
	}
}

// ---- public API ----

// Post records a balanced transaction atomically. The bool is true when the
// idempotency key had already been used and the original transaction is returned.
func (s *LedgerService) Post(ctx context.Context, t domain.Transaction) (*domain.Transaction, bool, error) {
	if err := t.Validate(); err != nil {
		return nil, false, err
	}

	var (
		created  *domain.Transaction
		replayed bool
	)

	err := s.txRunner.InTx(ctx, func(tx *sql.Tx) error {
		r := s.repos(tx)

		rp, err := claimKey(ctx, r.keys, t.IdempotencyKey, t.Hash(), domain.ResourceTransaction)
		if err != nil {
			return err
		}
		if rp != nil {
			created, err = s.loadTransaction(ctx, r.txns, r.entries, rp.ResourceID)
			replayed = true
			return err
		}

		accMap, err := s.resolveAccounts(ctx, r, t)
		if err != nil {
			return err
		}
		if err := s.checkFunds(ctx, r, t, accMap); err != nil {
			return err
		}
		created, err = s.write(ctx, r, t)
		if err != nil {
			return err
		}
		return r.keys.SetResource(ctx, t.IdempotencyKey, created.ID)
	})
	if err != nil {
		return nil, false, err
	}
	return created, replayed, nil
}

func (s *LedgerService) GetTransaction(ctx context.Context, id uuid.UUID) (*domain.Transaction, error) {
	return s.loadTransaction(ctx, s.transactionRepo, s.entryRepo, id)
}

// ---- posting steps ----

// txRepos is the set of repositories bound to one transaction.
type txRepos struct {
	accounts *repositories.AccountRepo
	txns     *repositories.TransactionRepo
	entries  *repositories.EntryRepo
	balances *repositories.BalanceRepo
	keys     *repositories.IdempotencyRepo
}

func (s *LedgerService) repos(tx *sql.Tx) txRepos {
	return txRepos{
		accounts: s.accRepo.WithTx(tx),
		txns:     s.transactionRepo.WithTx(tx),
		entries:  s.entryRepo.WithTx(tx),
		balances: s.balanceRepo.WithTx(tx),
		keys:     s.idemRepo.WithTx(tx),
	}
}

// resolveAccounts loads every account the transaction touches and enforces
// existence and a single currency.
func (s *LedgerService) resolveAccounts(ctx context.Context, r txRepos, t domain.Transaction) (map[uuid.UUID]domain.Account, error) {
	ids := t.AccountIDs()
	accMap, err := r.accounts.GetByIDs(ctx, ids)
	if err != nil {
		return nil, err
	}
	var currency string
	for _, id := range ids {
		acc, ok := accMap[id]
		if !ok {
			return nil, domain.ErrAccountNotFound(id)
		}
		if currency == "" {
			currency = acc.Currency
		}
		if acc.Currency != currency {
			return nil, domain.ErrCurrencyMismatch(acc.ID, currency, acc.Currency)
		}
	}
	return accMap, nil
}

// checkFunds locks the balance rows of every touched account (ids are sorted,
// so concurrent posts acquire locks in the same order and cannot deadlock) and
// rejects any liability debit that would overdraw.
func (s *LedgerService) checkFunds(ctx context.Context, r txRepos, t domain.Transaction, accMap map[uuid.UUID]domain.Account) error {
	locked, err := r.balances.LockAndGet(ctx, t.AccountIDs())
	if err != nil {
		return err
	}
	for id, sums := range t.SumsByAccount() {
		acc := accMap[id]
		if sums.Debits == 0 || acc.Type != domain.AccountLiability {
			continue
		}
		available := acc.Type.NormalBalance(locked[id].Debits, locked[id].Credits)
		if available < sums.Debits {
			return domain.ErrInsufficientFunds(id, available, sums.Debits)
		}
	}
	return nil
}

// write persists the transaction, its entries, and the balance deltas.
func (s *LedgerService) write(ctx context.Context, r txRepos, t domain.Transaction) (*domain.Transaction, error) {
	created, err := r.txns.Create(ctx, t)
	if err != nil {
		return nil, err
	}
	for i := range t.Entries {
		t.Entries[i].TransactionID = created.ID
	}
	created.Entries, err = r.entries.CreateMany(ctx, t.Entries)
	if err != nil {
		return nil, err
	}
	for id, sums := range t.SumsByAccount() {
		if err := r.balances.Apply(ctx, id, sums); err != nil {
			return nil, err
		}
	}
	return created, nil
}

// loadTransaction works with either pool-bound or tx-bound repositories.
func (s *LedgerService) loadTransaction(ctx context.Context, txns *repositories.TransactionRepo, ents *repositories.EntryRepo, id uuid.UUID) (*domain.Transaction, error) {
	t, err := txns.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	t.Entries, err = ents.ListByTransaction(ctx, id)
	return t, err
}
