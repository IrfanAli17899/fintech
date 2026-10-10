// Package ports holds the ledger's outbound contracts: what it needs from the
// outside world. Implementers (the rails module) import this package; the
// ledger's own internals use it; consumers of the ledger never need it.
package ports

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
)

// Settlement is what a rail reports about funds after Initiate.
type Settlement string

const (
	SettlementImmediate Settlement = "immediate" // funds are final now
	SettlementDeferred  Settlement = "deferred"  // funds land after SettlementDelay, or bounce
)

// RailProperties declare how a rail behaves so the ledger can decide
// lifecycle without knowing which rail it is.
type RailProperties struct {
	Instant         bool
	Reversible      bool
	SettlementDelay time.Duration
	CanBounce       bool
	CanCancel       bool // a pending transfer can be withdrawn before settlement
}

type RailInitiateRequest struct {
	TransactionID uuid.UUID
	Amount        int64
	Currency      string
	Kind          string
}

type RailInitiateResult struct {
	Ref        string     // provider reference, stored on the transaction
	Settlement Settlement // immediate → posted, deferred → pending
}

// RailBounce is returned by Settle when the rail rejected the transfer.
type RailBounce struct{ Reason string }

func (b *RailBounce) Error() string { return "rail bounced: " + b.Reason }

// ErrRailAlreadyDecided is returned (wrapped) by Cancel when the rail has
// already settled or bounced the transfer, so it can no longer be withdrawn.
var ErrRailAlreadyDecided = errors.New("rail has already decided this transfer")

// Rail is the port the ledger uses to move money in or out of the outside world.
// Every method must be idempotent: Initiate on TransactionID, Settle and Cancel
// on ref, Reverse on (ref, key). The rail records its decision in an outcome
// row; the ledger learns of it through RailRegistry.ClaimDue and acks with
// MarkApplied. A rail never changes the ledger directly.
type Rail interface {
	Name() string
	Properties() RailProperties
	Initiate(ctx context.Context, req RailInitiateRequest) (RailInitiateResult, error)
	// Settle finalises a deferred transfer. Returns *RailBounce if the rail refused it.
	Settle(ctx context.Context, ref string) error
	// Cancel withdraws a deferred transfer before settlement. Wraps
	// ErrRailAlreadyDecided when the transfer has settled or bounced.
	Cancel(ctx context.Context, ref string) error
	// Reverse sends amount back after settlement. Idempotent on key; several
	// partial reversals may exist for one ref.
	Reverse(ctx context.Context, ref, key string, amount int64) error
}

// OutcomeKind is what a rail decided about: the transfer itself, a
// cancellation, or one refund.
type OutcomeKind string

const (
	OutcomeSettle OutcomeKind = "settle"
	OutcomeCancel OutcomeKind = "cancel"
	OutcomeRefund OutcomeKind = "refund"
)

// Decision is where an outcome stands. Exactly one holds, so it is one value
// rather than a set of flags that could contradict each other.
type Decision string

const (
	DecisionUndecided Decision = "undecided" // the ledger should ask the rail to decide
	DecisionSettled   Decision = "settled"   // the action went through
	DecisionBounced   Decision = "bounced"   // the rail refused it
	DecisionCancelled Decision = "cancelled" // withdrawn; on a settle outcome, superseded by a cancel
)

// Outcome is a rail decision the ledger must act on.
type Outcome struct {
	Rail          string
	Ref           string
	Kind          OutcomeKind
	Key           string
	TransactionID uuid.UUID
	Amount        int64
	Decision      Decision
	Reason        string
}

// AttemptRecord is one attempt and every outcome recorded for it, as the rail
// sees them; reconciliation compares it with the ledger.
type AttemptRecord struct {
	Rail          string
	Ref           string
	TransactionID uuid.UUID
	Amount        int64
	CreatedAt     time.Time
	Outcomes      []OutcomeRecord
}

type OutcomeRecord struct {
	Kind         OutcomeKind
	Key          string
	Amount       int64
	Decision     Decision
	Applied      bool
	AttemptCount int
	LastError    string
	UpdatedAt    time.Time
}

// RailRegistry resolves rails by name and hands the ledger the outcomes that
// need work.
type RailRegistry interface {
	Get(name string) (Rail, bool)
	// ClaimDue leases outcomes ready for work, at most limit, for the given
	// lease duration. Partitioned across concurrent callers while the lease
	// holds; the caller must finish or give up before it expires.
	ClaimDue(ctx context.Context, now time.Time, lease time.Duration, limit int) ([]Outcome, error)
	// MarkApplied acks that the ledger acted on an outcome and releases its lease.
	MarkApplied(ctx context.Context, o Outcome) error
	// RecordFailure counts a failed attempt to decide or apply an outcome,
	// backs it off and releases its lease so the backoff decides the retry.
	RecordFailure(ctx context.Context, o Outcome, cause error) error
	// Attempts pages through every attempt with its outcomes, ordered by ref,
	// starting after afterRef ("" for the first page). For reconciliation.
	Attempts(ctx context.Context, afterRef string, limit int) ([]AttemptRecord, error)
}
