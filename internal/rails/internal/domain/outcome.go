package domain

import "time"

type OutcomeKind string

const (
	OutcomeSettle OutcomeKind = "settle"
	OutcomeCancel OutcomeKind = "cancel"
	OutcomeRefund OutcomeKind = "refund"
)

// Outcome is one decision a rail makes about an attempt (settlement,
// cancellation, or a refund) and whether the ledger has applied it.
type Outcome struct {
	Ref           string
	Kind          OutcomeKind
	Key           string // idempotency key for refunds; "" otherwise
	Amount        int64
	State         AttemptState // initiated | settled | bounced | cancelled
	Reason        *string
	AppliedAt     *time.Time
	ClaimedUntil  *time.Time
	AttemptCount  int
	NextAttemptAt *time.Time
	LastError     *string
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

func (o Outcome) Decided() bool { return o.State != AttemptInitiated }
