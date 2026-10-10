package application

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/google/uuid"
	"github.com/irfanali17899/fintech/internal/ledger/internal/domain"
	"github.com/irfanali17899/fintech/internal/ledger/ports"
	"github.com/irfanali17899/fintech/pkg/errdefs"
)

const (
	// railCallBudget bounds every call the ledger makes to a rail.
	railCallBudget = 10 * time.Second
	// leaseSlack covers the ledger-side apply and ack after each rail call.
	leaseSlack = 30 * time.Second
)

// OutcomeStats reports one worker run. Claimed is how many rows were leased;
// the rest count what happened to them.
type OutcomeStats struct {
	Claimed                                 int
	Settled, Bounced, Cancelled, Superseded int
	Refunded, Refused, Failed, Diverged     int
}

// Applied is how many claimed outcomes were acted on and acked.
func (s OutcomeStats) Applied() int {
	return s.Settled + s.Bounced + s.Cancelled + s.Superseded + s.Refunded + s.Refused + s.Diverged
}

// ApplyOutcomes drives every rail outcome through two idempotent phases: the
// rail decides (settle → outcome row), then the ledger applies the decision
// and acks it. A crash between phases is finished by the next run. The lease
// covers the whole batch, so no other worker can claim a row while this run
// may still reach it.
func (s *LedgerService) ApplyOutcomes(ctx context.Context, now time.Time, batchSize int) (OutcomeStats, error) {
	lease := time.Duration(batchSize)*railCallBudget + leaseSlack
	due, err := s.rails.ClaimDue(ctx, now, lease, batchSize)
	st := OutcomeStats{Claimed: len(due)}
	if err != nil {
		return st, err
	}

	for _, o := range due {
		if o.Decision == ports.DecisionUndecided {
			d, reason, err := s.askRail(ctx, o)
			if err != nil {
				st.Failed++
				log.Printf("outcomes: %s %s via %s: %v", o.Kind, o.Ref, o.Rail, err)
				if err := s.rails.RecordFailure(ctx, o, err); err != nil {
					return st, err
				}
				continue
			}
			o.Decision, o.Reason = d, reason
		}

		// a row that keeps failing to apply backs off like one that fails to
		// decide, instead of holding the rest of the batch until the lease ends
		if err := s.applyOutcome(ctx, o, &st); err != nil {
			st.Failed++
			log.Printf("outcomes: apply %s %s via %s: %v", o.Kind, o.Ref, o.Rail, err)
			if err := s.rails.RecordFailure(ctx, o, err); err != nil {
				return st, err
			}
			continue
		}
		if err := s.rails.MarkApplied(ctx, o); err != nil {
			return st, err
		}
	}
	return st, nil
}

// askRail is phase one: only settle outcomes arrive undecided; cancels and
// refunds are decided synchronously by the call that creates them.
func (s *LedgerService) askRail(ctx context.Context, o ports.Outcome) (ports.Decision, string, error) {
	if o.Kind != ports.OutcomeSettle {
		return "", "", fmt.Errorf("undecided %s outcome; only settle outcomes are decided by the worker", o.Kind)
	}
	rail, ok := s.rails.Get(o.Rail)
	if !ok {
		return "", "", fmt.Errorf("unknown rail %q", o.Rail)
	}
	callCtx, cancel := context.WithTimeout(ctx, railCallBudget)
	defer cancel()

	var bounce *ports.RailBounce
	switch err := rail.Settle(callCtx, o.Ref); {
	case err == nil:
		return ports.DecisionSettled, "", nil
	case errors.As(err, &bounce):
		return ports.DecisionBounced, bounce.Reason, nil
	default:
		return "", "", err
	}
}

// applyOutcome is phase two: make the ledger agree with the rail's decision.
func (s *LedgerService) applyOutcome(ctx context.Context, o ports.Outcome, st *OutcomeStats) error {
	var err error
	switch {
	case o.Kind == ports.OutcomeSettle && o.Decision == ports.DecisionSettled:
		_, _, err = s.Settle(ctx, o.TransactionID)
		st.Settled++
	case o.Kind == ports.OutcomeSettle && o.Decision == ports.DecisionBounced:
		_, _, err = s.Bounce(ctx, o.TransactionID, o.Reason)
		st.Bounced++
	case o.Kind == ports.OutcomeSettle && o.Decision == ports.DecisionCancelled:
		// superseded by the cancel outcome, which does the void; ack only
		st.Superseded++
	case o.Kind == ports.OutcomeCancel && o.Decision == ports.DecisionCancelled:
		_, _, err = s.systemVoid(ctx, o.TransactionID, "cancel:"+o.Ref, "cancelled at rail")
		st.Cancelled++
	case o.Kind == ports.OutcomeRefund:
		// the refund's key is the pending refund child's id
		childID, perr := uuid.Parse(o.Key)
		if perr != nil {
			log.Printf("DIVERGENCE: refund outcome %s has non-uuid key %q", o.Ref, o.Key)
			st.Diverged++
			return nil
		}
		switch o.Decision {
		case ports.DecisionSettled:
			// not refund-settle: 21a settled refunds by Capture under that key,
			// and reusing it with a different request would never succeed
			_, _, err = s.postPending(ctx, childID, "refund-post:"+o.Key)
			st.Refunded++
		case ports.DecisionBounced:
			_, _, err = s.systemVoid(ctx, childID, "refund-refuse:"+o.Key, "refund refused: "+o.Reason)
			st.Refused++
		default:
			log.Printf("DIVERGENCE: refund outcome %s/%s decided %s", o.Ref, o.Key, o.Decision)
			st.Diverged++
		}
	default:
		log.Printf("DIVERGENCE: outcome %s/%s/%s has unexpected decision %s", o.Rail, o.Kind, o.Ref, o.Decision)
		st.Diverged++
	}

	switch {
	case err == nil:
		return nil
	case errdefs.Is(err, domain.CodeTransactionNotPending):
		log.Printf("DIVERGENCE: outcome %s/%s/%s decided %s but ledger transaction %s is not pending: %v",
			o.Rail, o.Kind, o.Ref, o.Decision, o.TransactionID, err)
		st.Diverged++
		return nil
	case errdefs.Is(err, domain.CodeTransactionNotFound):
		// the rail knows a transfer the ledger never committed: an orphan.
		// The reconciler (21c) reports these as orphan_attempt.
		log.Printf("DIVERGENCE: orphan attempt %s/%s, no ledger transaction %s", o.Rail, o.Ref, o.TransactionID)
		st.Diverged++
		return nil
	default:
		return err
	}
}

func (s *LedgerService) systemVoid(ctx context.Context, id uuid.UUID, key, reason string) (*domain.Transaction, bool, error) {
	return s.Void(ctx, VoidCommand{HoldID: id, IdempotencyKey: key, Reason: reason, System: true})
}

// OutcomesJob drains due outcomes in leased batches; wrap it in a scheduler.Loop.
func (s *LedgerService) OutcomesJob(batchSize int) func(context.Context) {
	return func(ctx context.Context) {
		for {
			st, err := s.ApplyOutcomes(ctx, time.Now(), batchSize)
			if err != nil {
				log.Printf("apply outcomes: %v", err)
				return
			}
			if st.Claimed < batchSize {
				return
			}
		}
	}
}
