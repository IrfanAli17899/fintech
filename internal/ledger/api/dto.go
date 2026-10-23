package api

import (
	"time"

	"github.com/google/uuid"
)

const (
	StatusPending = "pending"
	StatusPosted  = "posted"
	StatusVoided  = "voided"

	DirectionDebit  = "debit"
	DirectionCredit = "credit"
)

type Account struct {
	ID        uuid.UUID
	OwnerType string
	OwnerID   string
	Currency  string
	Type      string
	CreatedAt time.Time
}

type CreateAccount struct {
	OwnerType string
	OwnerID   string
	Currency  string
	Type      string
}

type AccountQuery struct {
	OwnerType string
	OwnerID   string
}

// Balance is in minor units, signed by the account's normal balance.
// Available is what a debit may draw on now: posted minus outgoing holds.
type Balance struct {
	AccountID  uuid.UUID
	Currency   string
	Posted     int64
	PendingIn  int64
	PendingOut int64
	Available  int64
	AsOf       time.Time
}

type EntryInput struct {
	AccountID uuid.UUID
	Amount    int64 // minor units, positive
	Direction string
}

type PostTransaction struct {
	IdempotencyKey string
	Kind           string // the product's own word: topup, pharmacy_sale ...
	Status         string // "" or StatusPosted posts now; StatusPending places a hold
	Rail           string // "" = internal
	ExpiresAt      *time.Time
	Entries        []EntryInput
}

type Reverse struct {
	IdempotencyKey string
	TransactionID  uuid.UUID
	Kind           string
	Amount         int64 // 0 = whatever is left to reverse
}

type Capture struct {
	IdempotencyKey string
	HoldID         uuid.UUID
	Amount         int64 // 0 = everything still held
	// Final releases whatever is still held after this capture and closes the
	// hold. Without it the rest stays reserved for later captures, a Void, or
	// expiry.
	Final bool
}

type Void struct {
	IdempotencyKey string
	HoldID         uuid.UUID
	Reason         string
}

// Result is a write's outcome. Replayed is true when the idempotency key had
// already been used for the same request and the original result is returned.
type Result struct {
	Transaction Transaction
	Replayed    bool
}

type Transaction struct {
	ID           uuid.UUID
	Kind         string
	Relation     string     // "", "captures" or "reverses"
	ParentID     *uuid.UUID // set when Relation is
	Status       string
	StatusReason string
	Rail         string
	RailRef      string
	ExpiresAt    *time.Time
	CreatedAt    time.Time
	PostedAt     *time.Time
	Entries      []Entry
	Summary      Summary
}

type Entry struct {
	ID        uuid.UUID
	AccountID uuid.UUID
	Amount    int64
	Direction string
	CreatedAt time.Time
}

// Summary is derived from a transaction's children. Settlement is the status,
// except a voided hold whose captures cover it reads "settled_via_children"
// (or "partially_settled" when they cover part of it).
type Summary struct {
	Total      int64
	Captured   int64
	Reversed   int64
	Remaining  int64
	Settlement string
}

type Page struct {
	Before int64 // an entry seq; 0 = newest
	Limit  int   // 0 = default
}

type HistoryPage struct {
	Entries    []AccountEntry
	NextBefore int64 // 0 when there is no further page
}

// AccountEntry is one line of a statement. CreatedAt is when the entry was
// written (authorization, for a hold); PostedAt is when it settled.
type AccountEntry struct {
	Seq           int64
	Amount        int64
	Direction     string
	CreatedAt     time.Time
	PostedAt      *time.Time
	TransactionID uuid.UUID
	Kind          string
	Status        string
	Rail          string
	ParentID      *uuid.UUID
}

// ---- admin ----

type Sums struct {
	Debits         int64
	Credits        int64
	PendingDebits  int64
	PendingCredits int64
}

type BalanceMismatch struct {
	AccountID uuid.UUID
	Cached    Sums
	Live      Sums
}

type ReconciliationReport struct {
	At              time.Time
	AttemptsChecked int
	Findings        []Finding
	TrialBalance    []TrialBalanceLine
}

// TrialBalanceLine sums every entry for one currency and account type. Within
// a currency, debits equal credits; a currency that does not is a finding.
type TrialBalanceLine struct {
	Currency    string
	AccountType string
	Sums        Sums
}

type Finding struct {
	Kind          string // orphan_attempt, divergence, unapplied, stuck, trial_balance, balance_drift
	Reason        string // what disagrees, for a divergence
	Rail          string
	Ref           string
	TransactionID uuid.UUID
	Detail        string
}
