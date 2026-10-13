package application

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/irfanali17899/fintech/internal/ledger/internal/domain"
	"github.com/irfanali17899/fintech/internal/ledger/internal/persistence"
	"github.com/irfanali17899/fintech/internal/ledger/ports"
)

// post runs a validated transaction through account resolution, the overdraft
// check, the rail, and persistence. The caller owns the tx, the idempotency
// claim, and any follow-up such as status flips or key linking.
//
// A rail-backed post holds this tx's connection and Initiate opens a second
// one for the rails tx, so the pool must exceed the number of concurrent
// rail-backed posts. The outbox (capstone) removes the second connection.
func (s *Service) post(ctx context.Context, r persistence.Stores, t domain.Transaction) (*domain.Transaction, error) {
	if t.ID == uuid.Nil {
		t.ID = uuid.Must(uuid.NewV7())
	}
	accMap, currency, err := s.resolveAccounts(ctx, r, t)
	if err != nil {
		return nil, err
	}
	// Funds first: the check reads Debits+PendingDebits, which don't depend on
	// the status the rail picks, and a rejected post must never reach a rail.
	if err := s.checkFunds(ctx, r, t, accMap); err != nil {
		return nil, err
	}
	if err := s.initiateRail(ctx, &t, currency); err != nil {
		return nil, err
	}
	if t.Status == domain.TransactionStatusPending && t.ExpiresAt == nil {
		exp := time.Now().Add(domain.DefaultHoldTTL)
		t.ExpiresAt = &exp
	}
	return s.write(ctx, r, t)
}

// resolveAccounts loads every account the transaction touches and enforces
// existence and a single currency.
func (s *Service) resolveAccounts(ctx context.Context, r persistence.Stores, t domain.Transaction) (map[uuid.UUID]domain.Account, string, error) {
	ids := t.AccountIDs()
	accMap, err := r.Accounts.GetByIDs(ctx, ids)
	if err != nil {
		return nil, "", err
	}
	var currency string
	for _, id := range ids {
		acc, ok := accMap[id]
		if !ok {
			return nil, "", domain.ErrAccountNotFound(id)
		}
		if currency == "" {
			currency = acc.Currency
		}
		if acc.Currency != currency {
			return nil, "", domain.ErrCurrencyMismatch(acc.ID, currency, acc.Currency)
		}
	}
	return accMap, currency, nil
}

// initiateRail asks the transaction's rail to begin moving funds and lets the
// rail's answer decide whether this posts now or stays pending.
func (s *Service) initiateRail(ctx context.Context, t *domain.Transaction, currency string) error {
	if t.RailRef != "" {
		return nil // a child of an attempt the rail already knows; nothing to initiate
	}
	if t.Rail == "" {
		t.Rail = domain.RailInternal
	}
	rail, ok := s.rails.Get(t.Rail)
	if !ok {
		return domain.ErrUnknownRail(t.Rail)
	}
	res, err := rail.Initiate(ctx, ports.RailInitiateRequest{
		TransactionID: t.ID, Amount: t.Total(), Currency: currency, Kind: string(t.Kind),
	})
	if err != nil {
		return domain.ErrRailFailed(t.Rail, "initiate", err)
	}
	t.RailRef = res.Ref
	if res.Settlement == ports.SettlementDeferred {
		t.Status = domain.TransactionStatusPending
		if t.ExpiresAt == nil {
			exp := time.Now().Add(rail.Properties().SettlementDelay + domain.SettlementGrace)
			t.ExpiresAt = &exp
		}
	}
	return nil
}

// checkFunds locks the balance rows of every touched account (ids are sorted,
// so concurrent posts acquire locks in the same order and cannot deadlock) and
// rejects any liability debit that would overdraw.
func (s *Service) checkFunds(ctx context.Context, r persistence.Stores, t domain.Transaction, accMap map[uuid.UUID]domain.Account) error {
	locked, err := r.Balances.LockAndGet(ctx, t.AccountIDs())
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
func (s *Service) write(ctx context.Context, r persistence.Stores, t domain.Transaction) (*domain.Transaction, error) {
	created, err := r.Transactions.Create(ctx, t)
	if err != nil {
		return nil, err
	}
	for i := range t.Entries {
		t.Entries[i].TransactionID = created.ID
	}
	created.Entries, err = r.Entries.CreateMany(ctx, t.Entries)
	if err != nil {
		return nil, err
	}
	for id, sums := range t.SumsByAccount() {
		if err := r.Balances.Apply(ctx, id, sums); err != nil {
			return nil, err
		}
	}
	created.Summary = domain.Summarize(created.Status, domain.TransactionSummary{Total: created.Total()})
	return created, nil
}

// loadTransaction works with either pool-bound or tx-bound repos.
func (s *Service) loadTransaction(ctx context.Context, txns *persistence.TransactionRepo, ents *persistence.EntryRepo, id uuid.UUID) (*domain.Transaction, error) {
	t, err := txns.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if t.Entries, err = ents.ListByTransaction(ctx, id); err != nil {
		return nil, err
	}
	raw, err := txns.Summary(ctx, id)
	if err != nil {
		return nil, err
	}
	t.Summary = domain.Summarize(t.Status, raw)
	return t, nil
}

func (s *Service) releaseReservation(ctx context.Context, r persistence.Stores, sums map[uuid.UUID]domain.AccountSums) error {
	ids := make([]uuid.UUID, 0, len(sums))
	for id := range sums {
		ids = append(ids, id)
	}
	domain.SortIDs(ids)
	if _, err := r.Balances.LockAndGet(ctx, ids); err != nil {
		return err
	}
	for id, s := range sums {
		if err := r.Balances.ReleasePending(ctx, id, s); err != nil {
			return err
		}
	}
	return nil
}

// lockPending loads a pending transaction with its row locked, plus entries.
func (s *Service) lockPending(ctx context.Context, r persistence.Stores, id uuid.UUID) (*domain.Transaction, error) {
	h, err := r.Transactions.LockByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if h.Status != domain.TransactionStatusPending {
		return nil, domain.ErrTransactionNotPending(h.ID, h.Status)
	}
	h.Entries, err = r.Entries.ListByTransaction(ctx, h.ID)
	if err != nil {
		return nil, err
	}
	return h, nil
}

// pendingSums returns t's per-account sums as if t were pending, i.e. the
// reservation those entries represent.
func pendingSums(t domain.Transaction) map[uuid.UUID]domain.AccountSums {
	t.Status = domain.TransactionStatusPending
	return t.SumsByAccount()
}

func ptr[T any](v T) *T { return &v }
