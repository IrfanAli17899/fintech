package adapters

import (
	"context"
	"fmt"
	"time"

	"github.com/irfanali17899/fintech/internal/ledger/ports"
	"github.com/irfanali17899/fintech/internal/rails/internal/domain"
)

// SwiftRail simulates a correspondent-banking wire: slow, final, and able to
// fail before settlement. Amounts ending in 99 bounce, so tests can trigger it.
type SwiftRail struct {
	store Store
	delay time.Duration
}

func NewSwiftRail(s Store, delay time.Duration) *SwiftRail {
	return &SwiftRail{store: s, delay: delay}
}

func (s *SwiftRail) Name() string { return "swift" }

func (s *SwiftRail) Properties() ports.RailProperties {
	return ports.RailProperties{SettlementDelay: s.delay, CanBounce: true}
}

func (s *SwiftRail) Initiate(ctx context.Context, req ports.RailInitiateRequest) (ports.RailInitiateResult, error) {
	ref := "SWIFT-" + req.TransactionID.String()
	err := s.store.initiate(ctx, domain.Attempt{
		Ref: ref, Rail: s.Name(), TransactionID: req.TransactionID,
		Amount: req.Amount, Currency: req.Currency, SettleAfter: time.Now().Add(s.delay),
	})
	if err != nil {
		return ports.RailInitiateResult{}, err
	}
	return ports.RailInitiateResult{Ref: ref, Settlement: ports.SettlementDeferred}, nil
}

// Settle finalises the wire, or bounces it when the simulated correspondent
// rejects it. A bounce is committed first, then reported as RailBounce.
func (s *SwiftRail) Settle(ctx context.Context, ref string) error {
	var reason *string
	err := s.store.locked(ctx, ref, func(a *domain.Attempt, r txRepos) error {
		if a.State != domain.AttemptInitiated {
			return fmt.Errorf("settle %s (%s): %w", ref, a.State, domain.ErrWrongState)
		}
		to := domain.AttemptSettled
		if a.Amount%100 == 99 {
			why := "rejected by correspondent bank"
			to, reason = domain.AttemptBounced, &why
		}
		if err := r.decide(ctx, ref, domain.OutcomeSettle, "", to, reason); err != nil {
			return err
		}
		return r.transition(ctx, ref, to, reason)
	})
	if err != nil {
		return err
	}
	if reason != nil {
		return &ports.RailBounce{Reason: *reason}
	}
	return nil
}

func (s *SwiftRail) Cancel(context.Context, string) error { return domain.ErrCannotCancel }

func (s *SwiftRail) Reverse(context.Context, string, string, int64) error {
	return domain.ErrNotReversible
}
