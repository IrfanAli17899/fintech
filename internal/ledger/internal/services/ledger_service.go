package services

import (
	"context"
	"database/sql"
	"strconv"

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
	if t.Status == "" {
		t.Status = domain.TransactionStatusPosted
	}
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

		created, err = s.post(ctx, r, t)
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

// ReverseCommand asks for a full (Amount == 0) or partial reversal of a posted transaction.
type ReverseCommand struct {
	OriginalID     uuid.UUID
	Amount         int64
	IdempotencyKey string
}

// Reverse posts a refund transaction that undoes all or part of the original.
// The original is locked for the duration so concurrent partial reversals
// cannot together exceed its total.
func (s *LedgerService) Reverse(ctx context.Context, cmd ReverseCommand) (*domain.Transaction, bool, error) {
	var (
		created  *domain.Transaction
		replayed bool
	)

	err := s.txRunner.InTx(ctx, func(tx *sql.Tx) error {
		r := s.repos(tx)

		hash := domain.HashOf("reversal", cmd.OriginalID.String(), strconv.FormatInt(cmd.Amount, 10))
		rp, err := claimKey(ctx, r.keys, cmd.IdempotencyKey, hash, domain.ResourceTransaction)
		if err != nil {
			return err
		}
		if rp != nil {
			created, err = s.loadTransaction(ctx, r.txns, r.entries, rp.ResourceID)
			replayed = true
			return err
		}

		original, err := r.txns.LockByID(ctx, cmd.OriginalID)
		if err != nil {
			return err
		}
		if original.Status != domain.TransactionStatusPosted {
			return domain.ErrTransactionNotReversible(original.ID, original.Status)
		}
		original.Entries, err = r.entries.ListByTransaction(ctx, original.ID)
		if err != nil {
			return err
		}

		reversedBy, err := r.txns.ReversedByAccount(ctx, original.ID)
		if err != nil {
			return err
		}
		var reversed int64
		for _, e := range original.Entries {
			if e.Direction == domain.EntryDirectionDebit {
				reversed += reversedBy[e.AccountID]
			}
		}
		total := original.Total()
		amount := cmd.Amount
		if amount == 0 {
			amount = total - reversed
		}
		if amount <= 0 || reversed+amount > total {
			return domain.ErrReversalExceedsOriginal(original.ID, total-reversed, amount)
		}

		t := domain.Transaction{
			Kind:           domain.TransactionKindRefund,
			Status:         domain.TransactionStatusPosted, // ← add this
			IdempotencyKey: cmd.IdempotencyKey,
			ReversesID:     &original.ID,
			Entries:        original.Reverse(amount, reversedBy),
		}
		if err := t.Validate(); err != nil {
			return err
		}

		created, err = s.post(ctx, r, t)
		if err != nil {
			return err
		}

		if reversed+amount == total {
			if err := r.txns.MarkReversed(ctx, original.ID); err != nil {
				return err
			}
		}
		return r.keys.SetResource(ctx, cmd.IdempotencyKey, created.ID)
	})
	if err != nil {
		return nil, false, err
	}
	return created, replayed, nil
}

func (s *LedgerService) GetTransaction(ctx context.Context, id uuid.UUID) (*domain.Transaction, error) {
	return s.loadTransaction(ctx, s.transactionRepo, s.entryRepo, id)
}

// ---- posting pipeline ----

// txRepos is the set of repositories bound to one database transaction.
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

// post runs a validated transaction through account resolution, the overdraft
// check, and persistence. The caller owns the tx, the idempotency claim, and
// any follow-up such as status flips or key linking.
func (s *LedgerService) post(ctx context.Context, r txRepos, t domain.Transaction) (*domain.Transaction, error) {
	accMap, err := s.resolveAccounts(ctx, r, t)
	if err != nil {
		return nil, err
	}
	if err := s.checkFunds(ctx, r, t, accMap); err != nil {
		return nil, err
	}
	return s.write(ctx, r, t)
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
		requested := sums.Debits + sums.PendingDebits
		if requested == 0 || acc.Type != domain.AccountLiability {
			continue
		}
		if available := acc.Type.Balance(locked[id]).Available; available < requested {
			return domain.ErrInsufficientFunds(id, available, requested)
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
