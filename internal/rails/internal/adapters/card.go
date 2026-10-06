package adapters

import (
	"context"
	"fmt"
	"time"

	"github.com/irfanali17899/fintech/internal/ledger/api"
	"github.com/irfanali17899/fintech/internal/rails/internal/domain"
	"github.com/irfanali17899/fintech/internal/rails/internal/repositories"
)

// CardRail simulates card acquiring: authorization now, settlement after a
// delay, chargebacks possible after settlement.
type CardRail struct {
	attempts *repositories.AttemptRepo
	delay    time.Duration
}

func NewCardRail(attempts *repositories.AttemptRepo, delay time.Duration) *CardRail {
	return &CardRail{attempts: attempts, delay: delay}
}

func (c *CardRail) Name() string { return "card" }

func (c *CardRail) Properties() api.RailProperties {
	return api.RailProperties{Instant: false, Reversible: true, SettlementDelay: c.delay, CanBounce: false}
}

func (c *CardRail) Initiate(ctx context.Context, req api.RailInitiateRequest) (api.RailInitiateResult, error) {
	ref := "CARD-" + req.TransactionID.String()
	err := c.attempts.Create(ctx, domain.Attempt{
		Ref:           ref,
		Rail:          c.Name(),
		TransactionID: req.TransactionID,
		Amount:        req.Amount,
		Currency:      req.Currency,
		SettleAfter:   time.Now().Add(c.delay),
	})
	if err != nil {
		return api.RailInitiateResult{}, err
	}
	return api.RailInitiateResult{Ref: ref, Settlement: api.SettlementDeferred}, nil
}

func (c *CardRail) Settle(ctx context.Context, ref string) error {
	ok, err := c.attempts.Transition(ctx, ref, domain.AttemptInitiated, domain.AttemptSettled, nil)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("settle %s: %w", ref, domain.ErrWrongState)
	}
	return nil
}

// Reverse records a chargeback against a settled attempt.
func (c *CardRail) Reverse(ctx context.Context, ref string, amount int64) error {
	reason := fmt.Sprintf("chargeback %d", amount)
	ok, err := c.attempts.Transition(ctx, ref, domain.AttemptSettled, domain.AttemptReversed, &reason)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("reverse %s: %w", ref, domain.ErrWrongState)
	}
	return nil
}
