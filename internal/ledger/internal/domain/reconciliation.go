package domain

import (
	"time"

	"github.com/google/uuid"
)

// FindingKind groups reconciliation findings by what has to happen next.
type FindingKind string

const (
	// the rail knows a transfer the ledger never committed
	FindingOrphanAttempt FindingKind = "orphan_attempt"
	// the rail and the ledger disagree about the money; Reason says how
	FindingDivergence FindingKind = "divergence"
	// the rail decided but the ledger has not applied it, well past any lease
	FindingUnapplied FindingKind = "unapplied"
	// the worker keeps failing to decide or apply an outcome
	FindingStuck FindingKind = "stuck"
	// posted (or pending) debits and credits across the ledger differ
	FindingTrialBalance FindingKind = "trial_balance"
	// an account's cached balance differs from its entries
	FindingBalanceDrift FindingKind = "balance_drift"
)

// Divergence reasons.
const (
	ReasonVoidedButLive        = "voided_but_live"        // ledger voided, rail may still settle
	ReasonSettledNotCredited   = "settled_not_credited"   // rail settled, ledger captured nothing
	ReasonSettledButPending    = "settled_but_pending"    // settlement applied, hold still pending
	ReasonBouncedButCredited   = "bounced_but_credited"   // rail bounced, ledger captured
	ReasonBouncedButPending    = "bounced_but_pending"    // bounce applied, hold still pending
	ReasonCancelledButCredited = "cancelled_but_credited" // rail cancelled, ledger captured
	ReasonCancelledButPending  = "cancelled_but_pending"  // cancel applied, hold still pending
	ReasonRefundNotPosted      = "refund_not_posted"      // rail refunded, refund child not posted
	ReasonRefundNotVoided      = "refund_not_voided"      // rail refused, refund child not voided
	ReasonRefundOrphan         = "refund_orphan"          // rail refunded a child the ledger lacks
	ReasonRefundNeverSent      = "refund_never_sent"      // pending refund child the rail never heard of
)

// Finding is one thing reconciliation could not make agree.
type Finding struct {
	Kind          FindingKind
	Reason        string // divergence reason; empty for other kinds
	Rail          string
	Ref           string
	TransactionID uuid.UUID
	Detail        string
}

// ReconciliationReport is one reconciliation run.
type ReconciliationReport struct {
	At              time.Time
	AttemptsChecked int
	Findings        []Finding
}

func (r *ReconciliationReport) Add(f Finding) { r.Findings = append(r.Findings, f) }

func (r ReconciliationReport) Clean() bool { return len(r.Findings) == 0 }

// TransactionState is what reconciliation needs to know about a transaction.
type TransactionState struct {
	ID       uuid.UUID
	Status   TransactionStatus
	Captured int64 // posted by capture children
}
