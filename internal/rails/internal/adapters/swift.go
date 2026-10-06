package adapters

import (
	"context"
	"fmt"
	"time"

	"github.com/irfanali17899/fintech/internal/ledger/api"
	"github.com/irfanali17899/fintech/internal/rails/internal/domain"
	"github.com/irfanali17899/fintech/internal/rails/internal/repositories"
)

// SwiftRail simulates a correspondent-banking wire: slow, final, and able to
// fail before settlement. Amounts ending in 99 bounce, so tests can trigger it.
type SwiftRail struct {
	attempts *repositories.AttemptRepo
	delay    time.Duration
}

func NewSwiftRail(attempts *repositories.AttemptRepo, delay time.Duration) *SwiftRail {
	return &SwiftRail{attempts: attempts, delay: delay}
}

func (s *SwiftRail) Name() string { return "swift" }

func (s *SwiftRail) Properties() api.RailProperties {
	return api.RailProperties{Instant: false, Reversible: false, SettlementDelay: s.delay, CanBounce: true}
}

func (s *SwiftRail) Initiate(ctx context.Context, req api.RailInitiateRequest) (api.RailInitiateResult, error) {
	ref := "SWIFT-" + req.TransactionID.String()
	err := s.attempts.Create(ctx, domain.Attempt{
		Ref:           ref,
		Rail:          s.Name(),
		TransactionID: req.TransactionID,
		Amount:        req.Amount,
		Currency:      req.Currency,
		SettleAfter:   time.Now().Add(s.delay),
	})
	if err != nil {
		return api.RailInitiateResult{}, err
	}
	return api.RailInitiateResult{Ref: ref, Settlement: api.SettlementDeferred}, nil
}

// Settle finalises the wire, or bounces it when the simulated correspondent
// rejects it. A bounce is reported as ErrBounced so the ledger can void.
func (s *SwiftRail) Settle(ctx context.Context, ref string) error {
	a, err := s.attempts.Get(ctx, ref)
	if err != nil {
		return err
	}
	if a.Amount%100 == 99 {
		reason := "rejected by correspondent bank"
		ok, err := s.attempts.Transition(ctx, ref, domain.AttemptInitiated, domain.AttemptBounced, &reason)
		if err != nil {
			return err
		}
		if !ok {
			return fmt.Errorf("bounce %s: %w", ref, domain.ErrWrongState)
		}
		return &api.RailBounce{Reason: reason}
	}
	ok, err := s.attempts.Transition(ctx, ref, domain.AttemptInitiated, domain.AttemptSettled, nil)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("settle %s: %w", ref, domain.ErrWrongState)
	}
	return nil
}

func (s *SwiftRail) Reverse(context.Context, string, int64) error { return domain.ErrNotReversible }
