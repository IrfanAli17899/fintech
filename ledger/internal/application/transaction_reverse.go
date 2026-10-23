package application

import (
	"context"
	"strconv"

	"github.com/google/uuid"
	"github.com/irfanali17899/fintech/ledger/internal/domain"
	"github.com/irfanali17899/fintech/ledger/internal/persistence"
	"github.com/irfanali17899/fintech/platform/errdefs"
)

// ReverseCommand asks for a full (Amount == 0) or partial reversal of a posted transaction.
type ReverseCommand struct {
	Kind           string
	OriginalID     uuid.UUID
	Amount         int64
	IdempotencyKey string
}

// Reverse posts a refund transaction that undoes all or part of the original.
// The original is locked for the duration so concurrent partial reversals
// cannot together exceed its total.
//
// Internal reversals post immediately. Rail-backed ones post as pending
// (reserving the money), commit, and then ask the rail to refund; the outcome
// worker captures or voids the pending child when the rail decides. A replay
// of a still-pending refund asks the rail again, so a retry after a rail
// timeout actually retries.
func (s *Service) Reverse(ctx context.Context, cmd ReverseCommand) (*domain.Transaction, bool, error) {
	var (
		created  *domain.Transaction
		replayed bool
	)

	err := s.uow.Do(ctx, func(r persistence.Stores) error {

		hash := domain.HashOf("reversal", cmd.OriginalID.String(), cmd.Kind, strconv.FormatInt(cmd.Amount, 10))
		rp, err := claimKey(ctx, r.Keys, cmd.IdempotencyKey, hash, domain.ResourceTransaction)
		if err != nil {
			return err
		}
		if rp != nil {
			created, err = s.loadTransaction(ctx, r.Transactions, r.Entries, rp.ResourceID)
			replayed = true
			return err
		}

		original, err := r.Transactions.LockByID(ctx, cmd.OriginalID)
		if err != nil {
			return err
		}
		if original.Status != domain.TransactionStatusPosted {
			return domain.ErrTransactionNotReversible(original.ID, original.Status)
		}
		if original.RailBacked() {
			if rail, ok := s.rails.Get(original.Rail); !ok || !rail.Properties().Reversible {
				return domain.ErrRailNotReversible(original.ID, original.Rail)
			}
		}
		original.Entries, err = r.Entries.ListByTransaction(ctx, original.ID)
		if err != nil {
			return err
		}

		reversedBy, err := r.Transactions.ChildAmountsByAccount(ctx, original.ID, domain.RelationReverses)
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

		t := domain.Transaction{
			Kind:           domain.TransactionKind(cmd.Kind),
			Relation:       ptr(domain.RelationReverses),
			Status:         domain.TransactionStatusPosted,
			IdempotencyKey: cmd.IdempotencyKey,
			ParentID:       &original.ID,
			Entries:        original.Reverse(amount, reversedBy),
		}
		if original.RailBacked() {
			// reserve the money now; the worker posts it when the rail confirms
			t.Status = domain.TransactionStatusPending
			t.Rail, t.RailRef = original.Rail, original.RailRef
		}
		if err := t.Validate(); err != nil {
			return err
		}

		created, err = s.post(ctx, r, t)
		if err != nil {
			return err
		}
		return r.Keys.SetResource(ctx, cmd.IdempotencyKey, created.ID)
	})
	if err != nil {
		return nil, false, err
	}

	if created.RailBacked() && created.Status == domain.TransactionStatusPending {
		if err := s.refundAtRail(ctx, created); err != nil {
			return created, replayed, err
		}
	}
	return created, replayed, nil
}

// refundAtRail asks the rail to refund a pending refund child, outside any
// lock. The child's id is the refund's idempotency key, so a repeat is a no-op
// at the rail and the outcome worker knows which child to settle or void. If
// the call fails the reservation stays until a retry or the reconciler.
func (s *Service) refundAtRail(ctx context.Context, child *domain.Transaction) error {
	rail, ok := s.rails.Get(child.Rail)
	if !ok {
		return domain.ErrUnknownRail(child.Rail)
	}
	callCtx, cancel := context.WithTimeout(ctx, railCallBudget)
	defer cancel()
	if err := rail.Reverse(callCtx, child.RailRef, child.ID.String(), child.Total()); err != nil {
		return domain.ErrRailFailed(child.Rail, "reverse", err)
	}
	return nil
}

// postPending settles a pending refund child in place: its reservation is
// released and its entries count as posted, on the same row. Only refund
// children use it. They never have children of their own, so unlike a hold
// settled through captures, no sum has to skip a posted parent. Idempotent on
// key, so an outcome re-applied after a crash before its ack is a replay.
func (s *Service) postPending(ctx context.Context, id uuid.UUID, key string) (*domain.Transaction, bool, error) {
	var (
		posted   *domain.Transaction
		replayed bool
	)
	err := s.uow.Do(ctx, func(r persistence.Stores) error {

		rp, err := claimKey(ctx, r.Keys, key, domain.HashOf("post-pending", id.String()), domain.ResourceTransaction)
		if err != nil {
			return err
		}
		if rp != nil {
			posted, err = s.loadTransaction(ctx, r.Transactions, r.Entries, rp.ResourceID)
			replayed = true
			return err
		}

		h, err := s.lockPending(ctx, r, id)
		if err != nil {
			return err
		}
		if h.Relation == nil || *h.Relation != domain.RelationReverses {
			return errdefs.Internalf(nil, "post pending: %s is not a refund child", h.ID)
		}

		// releaseReservation locks the balance rows; they stay locked for Apply
		if err := s.releaseReservation(ctx, r, pendingSums(*h)); err != nil {
			return err
		}
		at, err := r.Transactions.MarkPosted(ctx, h.ID)
		if err != nil {
			return err
		}
		h.Status, h.PostedAt = domain.TransactionStatusPosted, &at
		for acc, sums := range h.SumsByAccount() {
			if err := r.Balances.Apply(ctx, acc, sums); err != nil {
				return err
			}
		}
		posted = h
		return r.Keys.SetResource(ctx, key, h.ID)
	})
	if err != nil {
		return nil, false, err
	}
	return posted, replayed, nil
}
