package application

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/irfanali17899/fintech/internal/ledger/internal/domain"
	"github.com/irfanali17899/fintech/internal/ledger/ports"
)

const (
	// reconcileGrace skips anything younger: a post whose rails tx committed
	// but whose ledger tx has not yet, or an outcome inside its lease.
	reconcileGrace = 10 * time.Minute
	reconcilePage  = 500
	// stuckAfter failures, an outcome is reported as stuck; it is still retried.
	stuckAfter = 10
)

// Reconcile compares every rail attempt and outcome with the ledger, then
// checks the trial balance and the balance cache. It never joins across
// schemas: rails pages its records out through the port and the comparison
// happens here. It reports; it never changes either side. Rows younger than
// reconcileGrace as of `at` are skipped.
func (s *Service) Reconcile(ctx context.Context, at time.Time) (*domain.ReconciliationReport, error) {
	rep := &domain.ReconciliationReport{At: at}
	cutoff := at.Add(-reconcileGrace)
	refundsAtRail := map[string]bool{} // ref + "/" + key

	for after := ""; ; {
		page, err := s.rails.Attempts(ctx, after, reconcilePage)
		if err != nil {
			return nil, err
		}
		if len(page) == 0 {
			break
		}
		states, err := s.transactionRepo.States(ctx, idsIn(page))
		if err != nil {
			return nil, err
		}
		for _, a := range page {
			rep.AttemptsChecked++
			checkAttempt(rep, a, states, cutoff, refundsAtRail)
		}
		if len(page) < reconcilePage {
			break
		}
		after = page[len(page)-1].Ref
	}

	pending, err := s.transactionRepo.PendingRailRefunds(ctx, cutoff)
	if err != nil {
		return nil, err
	}
	for _, c := range pending {
		if !refundsAtRail[c.RailRef+"/"+c.ID.String()] {
			rep.Add(domain.Finding{
				Kind: domain.FindingDivergence, Reason: domain.ReasonRefundNeverSent,
				Rail: c.Rail, Ref: c.RailRef, TransactionID: c.ID,
				Detail: "pending refund child has no refund at the rail; the reservation is held until it is retried",
			})
		}
	}

	tb, err := s.entryRepo.TrialBalance(ctx)
	if err != nil {
		return nil, err
	}
	if tb.Debits != tb.Credits || tb.PendingDebits != tb.PendingCredits {
		rep.Add(domain.Finding{Kind: domain.FindingTrialBalance, Detail: fmt.Sprintf(
			"posted debits %d credits %d; pending debits %d credits %d",
			tb.Debits, tb.Credits, tb.PendingDebits, tb.PendingCredits)})
	}

	drift, err := s.balanceRepo.Verify(ctx)
	if err != nil {
		return nil, err
	}
	for _, m := range drift {
		rep.Add(domain.Finding{Kind: domain.FindingBalanceDrift, TransactionID: uuid.Nil,
			Detail: fmt.Sprintf("account %s cached %+v live %+v", m.AccountID, m.Cached, m.Live)})
	}
	return rep, nil
}

// idsIn collects every ledger id a page refers to: the attempts' transactions
// and the refund children named by refund outcome keys.
func idsIn(page []ports.AttemptRecord) []uuid.UUID {
	ids := make([]uuid.UUID, 0, len(page))
	for _, a := range page {
		ids = append(ids, a.TransactionID)
		for _, o := range a.Outcomes {
			if o.Kind == ports.OutcomeRefund {
				if id, err := uuid.Parse(o.Key); err == nil {
					ids = append(ids, id)
				}
			}
		}
	}
	return ids
}

func checkAttempt(rep *domain.ReconciliationReport, a ports.AttemptRecord, states map[uuid.UUID]domain.TransactionState, cutoff time.Time, refundsAtRail map[string]bool) {
	add := func(kind domain.FindingKind, reason string, id uuid.UUID, detail string) {
		rep.Add(domain.Finding{Kind: kind, Reason: reason, Rail: a.Rail, Ref: a.Ref, TransactionID: id, Detail: detail})
	}
	diverge := func(reason string, id uuid.UUID, detail string) { add(domain.FindingDivergence, reason, id, detail) }

	t, ok := states[a.TransactionID]
	if !ok {
		if a.CreatedAt.Before(cutoff) {
			add(domain.FindingOrphanAttempt, "", a.TransactionID, fmt.Sprintf("rail holds %d with no ledger transaction", a.Amount))
		}
		return
	}

	for _, o := range a.Outcomes {
		if o.Kind == ports.OutcomeRefund {
			refundsAtRail[a.Ref+"/"+o.Key] = true
		}
		if o.AttemptCount >= stuckAfter {
			add(domain.FindingStuck, "", a.TransactionID, fmt.Sprintf("%s outcome failed %d times: %s", o.Kind, o.AttemptCount, o.LastError))
		}
		if o.Decision != ports.DecisionUndecided && !o.Applied && o.UpdatedAt.Before(cutoff) {
			add(domain.FindingUnapplied, "", a.TransactionID, fmt.Sprintf("%s decided %s at %s, never applied", o.Kind, o.Decision, o.UpdatedAt.Format(time.RFC3339)))
		}

		switch o.Kind {
		case ports.OutcomeSettle:
			switch o.Decision {
			case ports.DecisionUndecided:
				if t.Status == domain.TransactionStatusVoided && t.Captured == 0 {
					diverge(domain.ReasonVoidedButLive, t.ID, "ledger voided the hold but the rail can still settle it")
				}
			case ports.DecisionSettled:
				switch {
				case t.Status != domain.TransactionStatusPending && t.Captured == 0:
					diverge(domain.ReasonSettledNotCredited, t.ID, fmt.Sprintf("rail settled %d, ledger %s with nothing captured", a.Amount, t.Status))
				case t.Status == domain.TransactionStatusPending && o.Applied:
					diverge(domain.ReasonSettledButPending, t.ID, "settlement applied but the hold is still pending")
				}
			case ports.DecisionBounced:
				switch {
				case t.Captured > 0:
					diverge(domain.ReasonBouncedButCredited, t.ID, fmt.Sprintf("rail bounced, ledger captured %d", t.Captured))
				case t.Status == domain.TransactionStatusPending && o.Applied:
					diverge(domain.ReasonBouncedButPending, t.ID, "bounce applied but the hold is still pending")
				}
			}
		case ports.OutcomeCancel:
			if o.Decision != ports.DecisionCancelled {
				continue
			}
			switch {
			case t.Captured > 0:
				diverge(domain.ReasonCancelledButCredited, t.ID, fmt.Sprintf("rail cancelled, ledger captured %d", t.Captured))
			case t.Status == domain.TransactionStatusPending && o.Applied:
				diverge(domain.ReasonCancelledButPending, t.ID, "cancel applied but the hold is still pending")
			}
		case ports.OutcomeRefund:
			childID, err := uuid.Parse(o.Key)
			child, found := states[childID]
			if err != nil || !found {
				diverge(domain.ReasonRefundOrphan, t.ID, fmt.Sprintf("rail refund %q names no ledger refund child", o.Key))
				continue
			}
			if !o.Applied {
				continue
			}
			switch {
			case o.Decision == ports.DecisionSettled && child.Status != domain.TransactionStatusPosted:
				diverge(domain.ReasonRefundNotPosted, childID, fmt.Sprintf("rail refunded %d, refund child is %s", o.Amount, child.Status))
			case o.Decision == ports.DecisionBounced && child.Status != domain.TransactionStatusVoided:
				diverge(domain.ReasonRefundNotVoided, childID, fmt.Sprintf("rail refused the refund, refund child is %s", child.Status))
			}
		}
	}
}
