package application

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/irfanali17899/fintech/internal/ledger/internal/domain"
	"github.com/irfanali17899/fintech/internal/ledger/internal/persistence"
	"github.com/irfanali17899/fintech/internal/ledger/ports"
)

type VoidCommand struct {
	HoldID         uuid.UUID
	IdempotencyKey string
	Reason         string
	System         bool
}

// Void releases whatever a hold still reserves and marks it voided. Captures
// already posted against it are untouched.
//
// A rail-backed hold is not voided here: the key is committed, then the rail
// is asked to cancel, and the outcome worker voids once it confirms. The hold
// comes back still pending. A replay of a still-pending rail void asks the rail
// again.
func (s *Service) Void(ctx context.Context, cmd VoidCommand) (*domain.Transaction, bool, error) {
	var (
		voided   *domain.Transaction
		replayed bool
	)
	err := s.uow.Do(ctx, func(r persistence.Stores) error {
		var err error
		voided, replayed, err = s.voidLocked(ctx, r, cmd)
		return err
	})
	if err != nil {
		return nil, false, err
	}
	if !cmd.System && voided.RailBacked() && voided.Status == domain.TransactionStatusPending {
		if err := s.cancelAtRail(ctx, voided); err != nil {
			return voided, replayed, err
		}
	}
	return voided, replayed, nil
}

// voidLocked is Void's work inside the caller's transaction: claim the key,
// lock the hold, release what it still reserves. For a rail-backed hold it only
// checks the rail can cancel and links the key; nothing changes until the rail
// decides.
func (s *Service) voidLocked(ctx context.Context, r persistence.Stores, cmd VoidCommand) (*domain.Transaction, bool, error) {
	hash := domain.HashOf("void", cmd.HoldID.String(), cmd.Reason)
	rp, err := claimKey(ctx, r.Keys, cmd.IdempotencyKey, hash, domain.ResourceTransaction)
	if err != nil {
		return nil, false, err
	}
	if rp != nil {
		t, err := s.loadTransaction(ctx, r.Transactions, r.Entries, rp.ResourceID)
		return t, true, err
	}

	h, err := s.lockPending(ctx, r, cmd.HoldID)
	if err != nil {
		return nil, false, err
	}

	if h.RailBacked() && !cmd.System {
		// checked before the key is linked, so a refusal never spends it.
		// A pending refund child is a rail.Reverse in flight, which no rail
		// can withdraw; it settles or is refused.
		if h.Relation != nil && *h.Relation == domain.RelationReverses {
			return nil, false, domain.ErrRailCannotCancel(h.ID, h.Rail)
		}
		if rail, ok := s.rails.Get(h.Rail); !ok || !rail.Properties().CanCancel {
			return nil, false, domain.ErrRailCannotCancel(h.ID, h.Rail)
		}
		return h, false, r.Keys.SetResource(ctx, cmd.IdempotencyKey, h.ID)
	}

	capturedBy, err := r.Transactions.ChildAmountsByAccount(ctx, h.ID, domain.RelationCaptures)
	if err != nil {
		return nil, false, err
	}
	captured := h.Consumed(capturedBy)
	if remaining := h.Total() - captured; remaining > 0 {
		rest := domain.Transaction{Entries: h.Scaled(remaining, capturedBy)}
		if err := s.releaseReservation(ctx, r, pendingSums(rest)); err != nil {
			return nil, false, err
		}
	}

	var reason *string
	if cmd.Reason != "" {
		reason = &cmd.Reason
	}
	if err := r.Transactions.MarkVoided(ctx, h.ID, reason); err != nil {
		return nil, false, err
	}
	h.Status = domain.TransactionStatusVoided
	h.StatusReason = reason
	return h, false, r.Keys.SetResource(ctx, cmd.IdempotencyKey, h.ID)
}

// cancelAtRail asks the rail to withdraw a pending transfer, outside any lock.
// Cancel is idempotent at the rail, so a replayed void can safely ask again.
func (s *Service) cancelAtRail(ctx context.Context, h *domain.Transaction) error {
	rail, ok := s.rails.Get(h.Rail)
	if !ok {
		return domain.ErrUnknownRail(h.Rail)
	}
	callCtx, cancel := context.WithTimeout(ctx, railCallBudget)
	defer cancel()
	switch err := rail.Cancel(callCtx, h.RailRef); {
	case err == nil:
		return nil
	case errors.Is(err, ports.ErrRailAlreadyDecided):
		return domain.ErrHoldAlreadySettling(h.ID, h.Rail)
	default:
		return domain.ErrRailFailed(h.Rail, "cancel", err)
	}
}

// Bounce voids a deferred transaction whose rail rejected it before settlement.
func (s *Service) Bounce(ctx context.Context, id uuid.UUID, reason string) (*domain.Transaction, bool, error) {
	return s.Void(ctx, VoidCommand{
		HoldID:         id,
		IdempotencyKey: "bounce:" + id.String(),
		Reason:         "bounced: " + reason,
		System:         true,
	})
}

func (s *Service) systemVoid(ctx context.Context, id uuid.UUID, key, reason string) (*domain.Transaction, bool, error) {
	return s.Void(ctx, VoidCommand{HoldID: id, IdempotencyKey: key, Reason: reason, System: true})
}
