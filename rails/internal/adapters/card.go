package adapters

import (
	"context"
	"fmt"
	"time"

	"github.com/irfanali17899/fintech/ledger/ports"
	"github.com/irfanali17899/fintech/rails/internal/domain"
)

// CardRail simulates card acquiring: authorization now, settlement after a
// delay, auth reversal before settlement, refunds after.
type CardRail struct {
	store Store
	delay time.Duration
}

func NewCardRail(s Store, delay time.Duration) *CardRail {
	return &CardRail{store: s, delay: delay}
}

func (c *CardRail) Name() string { return "card" }

func (c *CardRail) Properties() ports.RailProperties {
	return ports.RailProperties{Reversible: true, SettlementDelay: c.delay, CanCancel: true}
}

// Initiate records the authorization and its settle outcome; the worker asks
// us to decide that outcome once it is due.
func (c *CardRail) Initiate(ctx context.Context, req ports.RailInitiateRequest) (ports.RailInitiateResult, error) {
	ref := "CARD-" + req.TransactionID.String()
	err := c.store.initiate(ctx, domain.Attempt{
		Ref: ref, Rail: c.Name(), TransactionID: req.TransactionID,
		Amount: req.Amount, Currency: req.Currency, SettleAfter: time.Now().Add(c.delay),
	})
	if err != nil {
		return ports.RailInitiateResult{}, err
	}
	return ports.RailInitiateResult{Ref: ref, Settlement: ports.SettlementDeferred}, nil
}

// Settle decides the settle outcome: cards always settle in the simulator.
// Idempotent once settled; a cancelled authorization never settles.
func (c *CardRail) Settle(ctx context.Context, ref string) error {
	return c.store.locked(ctx, ref, func(a *domain.Attempt, r txRepos) error {
		switch a.State {
		case domain.AttemptSettled:
			return nil
		case domain.AttemptInitiated:
		default:
			return fmt.Errorf("settle %s (%s): %w", ref, a.State, domain.ErrWrongState)
		}
		if err := r.decide(ctx, ref, domain.OutcomeSettle, "", domain.AttemptSettled, nil); err != nil {
			return err
		}
		return r.transition(ctx, ref, domain.AttemptSettled, nil)
	})
}

// Cancel reverses the authorization and supersedes the settle outcome so it
// never settles. Idempotent once cancelled.
func (c *CardRail) Cancel(ctx context.Context, ref string) error {
	return c.store.locked(ctx, ref, func(a *domain.Attempt, r txRepos) error {
		switch a.State {
		case domain.AttemptCancelled:
			return nil
		case domain.AttemptInitiated:
		default:
			return fmt.Errorf("cancel %s (%s): %w", ref, a.State, ports.ErrRailAlreadyDecided)
		}
		if err := r.outcomes.Create(ctx, domain.Outcome{Ref: ref, Kind: domain.OutcomeCancel, Amount: a.Amount}); err != nil {
			return err
		}
		if err := r.decide(ctx, ref, domain.OutcomeCancel, "", domain.AttemptCancelled, nil); err != nil {
			return err
		}
		// the worker acks the superseded settle outcome without acting on it
		if err := r.decide(ctx, ref, domain.OutcomeSettle, "", domain.AttemptCancelled, nil); err != nil {
			return err
		}
		return r.transition(ctx, ref, domain.AttemptCancelled, nil)
	})
}

// Reverse records a refund against a settled attempt; idempotent on key.
func (c *CardRail) Reverse(ctx context.Context, ref, key string, amount int64) error {
	return c.store.locked(ctx, ref, func(a *domain.Attempt, r txRepos) error {
		if a.State != domain.AttemptSettled {
			return fmt.Errorf("refund %s (%s): %w", ref, a.State, domain.ErrWrongState)
		}
		if err := r.outcomes.Create(ctx, domain.Outcome{Ref: ref, Kind: domain.OutcomeRefund, Key: key, Amount: amount}); err != nil {
			return err
		}
		// false = this key was already decided; a repeat is a no-op
		_, err := r.outcomes.Decide(ctx, ref, domain.OutcomeRefund, key, domain.AttemptSettled, nil)
		return err
	})
}
