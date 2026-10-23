package application

import (
	"context"
	"strconv"

	"github.com/google/uuid"
	"github.com/irfanali17899/fintech/ledger/internal/domain"
	"github.com/irfanali17899/fintech/ledger/internal/persistence"
)

// System is set only by the outcome worker; it lets a capture or void act on
// a hold a rail owns. HTTP callers never set it.
type CaptureCommand struct {
	HoldID         uuid.UUID
	Amount         int64 // 0 = full
	Final          bool  // release whatever is left after this capture and close the hold
	IdempotencyKey string
	System         bool
}

func (s *Service) Capture(ctx context.Context, cmd CaptureCommand) (*domain.Transaction, bool, error) {
	var (
		created  *domain.Transaction
		replayed bool
	)
	err := s.uow.Do(ctx, func(r persistence.Stores) error {

		hash := domain.HashOf("capture", cmd.HoldID.String(), strconv.FormatInt(cmd.Amount, 10), strconv.FormatBool(cmd.Final))
		rp, err := claimKey(ctx, r.Keys, cmd.IdempotencyKey, hash, domain.ResourceTransaction)
		if err != nil {
			return err
		}
		if rp != nil {
			created, err = s.loadTransaction(ctx, r.Transactions, r.Entries, rp.ResourceID)
			replayed = true
			return err
		}

		h, err := s.lockPending(ctx, r, cmd.HoldID)
		if err != nil {
			return err
		}
		// only the outcome worker may capture what a rail owns
		if h.RailBacked() && !cmd.System {
			return domain.ErrRailManagedHold(h.ID, h.Rail)
		}
		capturedBy, err := r.Transactions.ChildAmountsByAccount(ctx, h.ID, domain.RelationCaptures)
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
		if cmd.Final || captured+amount == held {
			if err := s.closeHold(ctx, r, h, nil); err != nil {
				return err
			}
		}
		return r.Keys.SetResource(ctx, cmd.IdempotencyKey, created.ID)
	})
	if err != nil {
		return nil, false, err
	}
	return created, replayed, nil
}

// Settle posts everything a deferred transaction still holds, once its rail
// has confirmed the funds. Idempotent on the transaction id.
func (s *Service) Settle(ctx context.Context, id uuid.UUID) (*domain.Transaction, bool, error) {
	return s.Capture(ctx, CaptureCommand{HoldID: id, IdempotencyKey: "settle:" + id.String(), System: true})
}
