package application

import (
	"context"
	"database/sql"
	"errors"
	"log"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/irfanali17899/fintech/internal/ledger/internal/domain"
	"github.com/irfanali17899/fintech/internal/ledger/internal/persistence"
	"github.com/irfanali17899/fintech/internal/ledger/ports"
	"github.com/irfanali17899/fintech/pkg/database"
	"github.com/irfanali17899/fintech/pkg/errdefs"
)

// System is set only by the settlement path; it lets a capture or void act on
// a hold a rail owns. HTTP callers never set it.
type CaptureCommand struct {
	HoldID         uuid.UUID
	Amount         int64 // 0 = full
	IdempotencyKey string
	System         bool
}
type VoidCommand struct {
	HoldID         uuid.UUID
	IdempotencyKey string
	Reason         string
	System         bool
}

// ReverseCommand asks for a full (Amount == 0) or partial reversal of a posted transaction.
type ReverseCommand struct {
	Kind           string
	OriginalID     uuid.UUID
	Amount         int64
	IdempotencyKey string
}

type LedgerService struct {
	txRunner        database.TxRunner
	accRepo         *persistence.AccountRepo
	transactionRepo *persistence.TransactionRepo
	entryRepo       *persistence.EntryRepo
	balanceRepo     *persistence.BalanceRepo
	idemRepo        *persistence.IdempotencyRepo
	rails           ports.RailRegistry
}

func NewLedgerService(
	txRunner database.TxRunner,
	accRepo *persistence.AccountRepo,
	transactionRepo *persistence.TransactionRepo,
	entryRepo *persistence.EntryRepo,
	balanceRepo *persistence.BalanceRepo,
	idemRepo *persistence.IdempotencyRepo,
	rails ports.RailRegistry,
) *LedgerService {
	return &LedgerService{
		txRunner:        txRunner,
		accRepo:         accRepo,
		transactionRepo: transactionRepo,
		entryRepo:       entryRepo,
		balanceRepo:     balanceRepo,
		idemRepo:        idemRepo,
		rails:           rails,
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

		hash := domain.HashOf("reversal", cmd.OriginalID.String(), cmd.Kind, strconv.FormatInt(cmd.Amount, 10))
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
		rail, ok := s.rails.Get(original.Rail)
		if ok && !rail.Properties().Reversible {
			return domain.ErrRailNotReversible(original.ID, original.Rail)
		}
		original.Entries, err = r.entries.ListByTransaction(ctx, original.ID)
		if err != nil {
			return err
		}

		reversedBy, err := r.txns.ChildAmountsByAccount(ctx, original.ID, domain.RelationReverses)
		if err != nil {
			return err
		}

		reversed := original.Consumed(reversedBy)
		total := original.Total()
		amount := cmd.Amount
		if amount == 0 {
			amount = total - reversed
		}
		if amount <= 0 || reversed+amount > total {
			return domain.ErrReversalExceedsOriginal(original.ID, total-reversed, amount)
		}
		if original.RailBacked() && amount != total {
			return domain.ErrRailPartialRefund(original.ID, original.Rail)
		}

		t := domain.Transaction{
			Kind:           domain.TransactionKind(cmd.Kind),
			Relation:       ptr(domain.RelationReverses),
			Status:         domain.TransactionStatusPosted,
			IdempotencyKey: cmd.IdempotencyKey,
			ParentID:       &original.ID,
			Entries:        original.Reverse(amount, reversedBy),
		}
		if original.RailBacked() {
			t.Rail, t.RailRef = original.Rail, original.RailRef
		}
		if err := t.Validate(); err != nil {
			return err
		}

		created, err = s.post(ctx, r, t)
		if err != nil {
			return err
		}
		// After post, so a refund the ledger rejects (e.g. insufficient funds)
		// never reaches the rail. A commit failure after this still diverges;
		// story 21 closes that.
		if original.RailBacked() {
			if err := rail.Reverse(ctx, original.RailRef, amount); err != nil {
				return domain.ErrRailFailed(original.Rail, "reverse", err)
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

func (s *LedgerService) Capture(ctx context.Context, cmd CaptureCommand) (*domain.Transaction, bool, error) {
	var (
		created  *domain.Transaction
		replayed bool
	)
	err := s.txRunner.InTx(ctx, func(tx *sql.Tx) error {
		r := s.repos(tx)

		hash := domain.HashOf("capture", cmd.HoldID.String(), strconv.FormatInt(cmd.Amount, 10))
		rp, err := claimKey(ctx, r.keys, cmd.IdempotencyKey, hash, domain.ResourceTransaction)
		if err != nil {
			return err
		}
		if rp != nil {
			created, err = s.loadTransaction(ctx, r.txns, r.entries, rp.ResourceID)
			replayed = true
			return err
		}

		h, err := s.lockPending(ctx, r, cmd.HoldID, cmd.System)
		if err != nil {
			return err
		}
		capturedBy, err := r.txns.ChildAmountsByAccount(ctx, h.ID, domain.RelationCaptures)
		if err != nil {
			return err
		}

		captured := h.Consumed(capturedBy)
		held := h.Total()
		remaining := held - captured
		amount := cmd.Amount
		if amount == 0 {
			amount = remaining
		}
		if amount <= 0 || amount > remaining {
			return domain.ErrCaptureExceedsHold(h.ID, remaining, amount)
		}

		child := domain.Transaction{
			Kind:           h.Kind,
			Relation:       ptr(domain.RelationCaptures),
			Status:         domain.TransactionStatusPosted,
			IdempotencyKey: cmd.IdempotencyKey,
			ParentID:       &h.ID,
			Rail:           h.Rail,
			RailRef:        h.RailRef,
			Entries:        h.Scaled(amount, capturedBy),
		}
		if err := child.Validate(); err != nil {
			return err
		}

		// release exactly this capture's share of the reservation before posting,
		// so the funds check sees the right available
		if err := s.releaseReservation(ctx, r, pendingSums(child)); err != nil {
			return err
		}
		created, err = s.post(ctx, r, child)
		if err != nil {
			return err
		}
		if captured+amount == held {
			if err := r.txns.MarkVoided(ctx, h.ID, nil); err != nil {
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

// Void releases whatever a hold still reserves and marks it voided. Captures
// already posted against it are untouched.
func (s *LedgerService) Void(ctx context.Context, cmd VoidCommand) (*domain.Transaction, bool, error) {
	var (
		voided   *domain.Transaction
		replayed bool
	)
	err := s.txRunner.InTx(ctx, func(tx *sql.Tx) error {
		r := s.repos(tx)

		hash := domain.HashOf("void", cmd.HoldID.String(), cmd.Reason)
		rp, err := claimKey(ctx, r.keys, cmd.IdempotencyKey, hash, domain.ResourceTransaction)
		if err != nil {
			return err
		}
		if rp != nil {
			voided, err = s.loadTransaction(ctx, r.txns, r.entries, rp.ResourceID)
			replayed = true
			return err
		}

		h, err := s.lockPending(ctx, r, cmd.HoldID, cmd.System)
		if err != nil {
			return err
		}
		capturedBy, err := r.txns.ChildAmountsByAccount(ctx, h.ID, domain.RelationCaptures)
		if err != nil {
			return err
		}

		captured := h.Consumed(capturedBy)
		if remaining := h.Total() - captured; remaining > 0 {
			rest := domain.Transaction{Entries: h.Scaled(remaining, capturedBy)}
			if err := s.releaseReservation(ctx, r, pendingSums(rest)); err != nil {
				return err
			}
		}

		var reason *string
		if cmd.Reason != "" {
			reason = &cmd.Reason
		}
		if err := r.txns.MarkVoided(ctx, h.ID, reason); err != nil {
			return err
		}
		h.Status = domain.TransactionStatusVoided
		h.StatusReason = reason
		voided = h
		return r.keys.SetResource(ctx, cmd.IdempotencyKey, h.ID)
	})
	if err != nil {
		return nil, false, err
	}
	return voided, replayed, nil
}

// SettleDue asks each rail with due attempts for an outcome and applies it:
// settled → the pending transaction posts; bounced → it voids with the reason.
// Rails that answer with any other error are left for the next run.
func (s *LedgerService) SettleDue(ctx context.Context, now time.Time, batchSize int) (settled, bounced, diverged int, err error) {
	due, err := s.rails.Due(ctx, now, batchSize)
	if err != nil {
		return 0, 0, 0, err
	}

	for _, d := range due {
		rail, ok := s.rails.Get(d.Rail)
		if !ok {
			log.Printf("settle: unknown rail %q for %s", d.Rail, d.Ref)
			continue
		}

		var bounce *ports.RailBounce
		switch err := rail.Settle(ctx, d.Ref); {
		case err == nil:
		case errors.As(err, &bounce):
		default:
			log.Printf("settle %s via %s: %v", d.Ref, d.Rail, err)
			continue
		}

		if bounce != nil {
			_, _, err = s.Bounce(ctx, d.TransactionID, bounce.Reason)
		} else {
			_, _, err = s.Settle(ctx, d.TransactionID)
		}
		switch {
		case err == nil:
			if bounce != nil {
				bounced++
			} else {
				settled++
			}
		case errdefs.Is(err, domain.CodeTransactionNotPending):
			// rail-backed holds can't be voided or expired outside this path,
			// so the rail and the ledger now disagree
			diverged++
			log.Printf("DIVERGENCE: settle %s via %s: rail decided but ledger transaction %s is not pending", d.Ref, d.Rail, d.TransactionID)
		case errdefs.Is(err, domain.CodeTransactionNotFound):
			// the rail knows a transfer the ledger never committed: an orphan.
			// Reconciliation (21) reports these; the worker moves on.
			diverged++
			log.Printf("DIVERGENCE: settle %s: orphan attempt, no ledger transaction %s", d.Ref, d.TransactionID)
		default:
			return settled, bounced, diverged, err
		}
	}
	return settled, bounced, diverged, nil
}

// ExpireHolds voids every pending transaction past its expiry, in batches of
// batchSize, each void in its own transaction. Safe to run concurrently on
// several replicas: ExpiredHolds uses SKIP LOCKED to partition the work.
func (s *LedgerService) ExpireHolds(ctx context.Context, now time.Time, batchSize int) (int, error) {
	var ids []uuid.UUID
	err := s.txRunner.InTx(ctx, func(tx *sql.Tx) error {
		var err error
		ids, err = s.repos(tx).txns.ExpiredHolds(ctx, now, batchSize)
		return err
	})
	if err != nil {
		return 0, err
	}

	voided := 0
	for _, id := range ids {
		_, _, err := s.Void(ctx, VoidCommand{HoldID: id, IdempotencyKey: "expire:" + id.String(), Reason: "expired"})
		switch {
		case err == nil:
			voided++
		case errdefs.Is(err, domain.CodeTransactionNotPending):
			// captured or voided by someone else between select and void; fine
		default:
			return voided, err
		}
	}
	return voided, nil
}

// Settle posts everything a deferred transaction still holds, once its rail
// has confirmed the funds. Idempotent on the transaction id.
func (s *LedgerService) Settle(ctx context.Context, id uuid.UUID) (*domain.Transaction, bool, error) {
	return s.Capture(ctx, CaptureCommand{HoldID: id, IdempotencyKey: "settle:" + id.String(), System: true})
}

// Bounce voids a deferred transaction whose rail rejected it before settlement.
func (s *LedgerService) Bounce(ctx context.Context, id uuid.UUID, reason string) (*domain.Transaction, bool, error) {
	return s.Void(ctx, VoidCommand{
		HoldID:         id,
		IdempotencyKey: "bounce:" + id.String(),
		Reason:         "bounced: " + reason,
		System:         true,
	})
}

// Children returns the transactions that capture or reverse the given one.
func (s *LedgerService) Children(ctx context.Context, parentID uuid.UUID) ([]domain.Transaction, error) {
	return s.transactionRepo.ListByParent(ctx, parentID)
}

// ---- posting pipeline ----

// txRepos is the set of repos bound to one database transaction.
type txRepos struct {
	accounts *persistence.AccountRepo
	txns     *persistence.TransactionRepo
	entries  *persistence.EntryRepo
	balances *persistence.BalanceRepo
	keys     *persistence.IdempotencyRepo
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
func (s *LedgerService) resolveAccounts(ctx context.Context, r txRepos, t domain.Transaction) (map[uuid.UUID]domain.Account, string, error) {
	ids := t.AccountIDs()
	accMap, err := r.accounts.GetByIDs(ctx, ids)
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
func (s *LedgerService) initiateRail(ctx context.Context, t *domain.Transaction, currency string) error {
	if t.RailRef != "" {
		return nil // settling an attempt the rail already knows; nothing to initiate
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
	created.Summary = domain.TransactionSummary{Total: created.Total(), Remaining: created.Total()}
	return created, nil
}

// loadTransaction works with either pool-bound or tx-bound persistence.
func (s *LedgerService) loadTransaction(ctx context.Context, txns *persistence.TransactionRepo, ents *persistence.EntryRepo, id uuid.UUID) (*domain.Transaction, error) {
	t, err := txns.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if t.Entries, err = ents.ListByTransaction(ctx, id); err != nil {
		return nil, err
	}
	if t.Summary, err = txns.Summary(ctx, id); err != nil {
		return nil, err
	}
	return t, nil
}

func (s *LedgerService) releaseReservation(ctx context.Context, r txRepos, sums map[uuid.UUID]domain.AccountSums) error {
	ids := make([]uuid.UUID, 0, len(sums))
	for id := range sums {
		ids = append(ids, id)
	}
	domain.SortIDs(ids)
	if _, err := r.balances.LockAndGet(ctx, ids); err != nil {
		return err
	}
	for id, s := range sums {
		if err := r.balances.ReleasePending(ctx, id, s); err != nil {
			return err
		}
	}
	return nil
}

func (s *LedgerService) lockPending(ctx context.Context, r txRepos, id uuid.UUID, system bool) (*domain.Transaction, error) {
	h, err := r.txns.LockByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if h.Status != domain.TransactionStatusPending {
		return nil, domain.ErrTransactionNotPending(h.ID, h.Status)
	}
	if h.RailBacked() && !system {
		return nil, domain.ErrRailManagedHold(h.ID, h.Rail)
	}
	h.Entries, err = r.entries.ListByTransaction(ctx, h.ID)
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
