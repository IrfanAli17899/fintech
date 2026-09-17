package domain

import (
	"bytes"
	"slices"
	"time"

	"github.com/google/uuid"
)

type TransactionKind string
type TransactionStatus string
type EntryDirection string

const (
	TransactionKindTopup    TransactionKind = "topup"
	TransactionKindTransfer TransactionKind = "transfer"
	TransactionKindPayment  TransactionKind = "payment"
	TransactionKindRefund   TransactionKind = "refund"
	TransactionKindFee      TransactionKind = "fee"

	TransactionStatusPending  TransactionStatus = "pending"
	TransactionStatusPosted   TransactionStatus = "posted"
	TransactionStatusVoided   TransactionStatus = "voided"
	TransactionStatusReversed TransactionStatus = "reversed"

	EntryDirectionDebit  EntryDirection = "debit"
	EntryDirectionCredit EntryDirection = "credit"
)

var validTransactionKinds = map[TransactionKind]bool{
	TransactionKindFee: true, TransactionKindPayment: true, TransactionKindRefund: true,
	TransactionKindTopup: true, TransactionKindTransfer: true,
}

var validTransactionStatus = map[TransactionStatus]bool{
	TransactionStatusPending: true, TransactionStatusPosted: true, TransactionStatusReversed: true,
	TransactionStatusVoided: true,
}

var validEntryDirection = map[EntryDirection]bool{
	EntryDirectionCredit: true, EntryDirectionDebit: true,
}

// MaxAmount caps a single entry at 1e15 minor units (10 trillion PKR).
// Prevents int64 overflow when summing entries: ~9,200 max-size entries
// would be needed to wrap, far beyond any realistic transaction.
// each entry is capped at 10^5 and to overflow int64 you need at least
// ~92K such entries which is not realistic
const MaxEntryAmount int64 = 1_000_000_000_000_000

type Transaction struct {
	ID             uuid.UUID
	IdempotencyKey string
	Kind           TransactionKind
	ReversesID     *uuid.UUID
	Status         TransactionStatus
	CreatedAt      time.Time
	PostedAt       *time.Time
	Entries        []Entry
}

type Entry struct {
	ID            uuid.UUID
	TransactionID uuid.UUID
	AccountID     uuid.UUID
	Amount        int64
	Direction     EntryDirection
	CreatedAt     time.Time
}

func (t Transaction) AccountIDs() []uuid.UUID {
	ids := make([]uuid.UUID, 0, len(t.Entries))
	for _, e := range t.Entries {
		ids = append(ids, e.AccountID)
	}
	slices.SortFunc(ids, func(a, b uuid.UUID) int {
		return bytes.Compare(a[:], b[:])
	})
	return slices.Compact(ids)
}

func (t Transaction) SumsByAccount() map[uuid.UUID]AccountSums {
	sums := make(map[uuid.UUID]AccountSums)
	for _, e := range t.Entries {
		s := sums[e.AccountID]
		if e.Direction == EntryDirectionDebit {
			s.Debits += e.Amount
		} else {
			s.Credits += e.Amount
		}
		sums[e.AccountID] = s
	}
	return sums
}

func (k TransactionKind) Valid() bool {
	return validTransactionKinds[k]
}

func (s TransactionStatus) Valid() bool {
	return validTransactionStatus[s]
}

func (d EntryDirection) Valid() bool {
	return validEntryDirection[d]
}

func (t Transaction) Validate() error {
	switch {
	case !t.Kind.Valid():
		return ErrInvalidTransactionKind(t.Kind)
	case t.IdempotencyKey == "":
		return ErrIdempotencyKeyRequired()
	case len(t.Entries) < 2:
		return ErrTooFewEntries(len(t.Entries))
	}

	var debits, credits int64
	net := make(map[uuid.UUID]int64, len(t.Entries))

	for _, e := range t.Entries {
		if !e.Direction.Valid() {
			return ErrInvalidEntryDirection(e.Direction)
		}
		if e.Amount <= 0 {
			return ErrInvalidAmount(e.Amount)
		}
		if e.Amount > MaxEntryAmount {
			return ErrAmountTooLarge(e.Amount)
		}
		if e.Direction == EntryDirectionDebit {
			debits += e.Amount
			net[e.AccountID] -= e.Amount
		} else {
			credits += e.Amount
			net[e.AccountID] += e.Amount
		}
	}

	// double-entry ledger constraint check, all debits should be equal to all credits.
	if debits != credits {
		return ErrTransactionUnbalanced(debits, credits)
	}

	// check if an account has real money movement instead of just equal credit/debit which causes it
	// to land to same account state, then there is no reason to log it.
	for id, n := range net {
		if n == 0 {
			return ErrAccountNetZero(id)
		}
	}

	return nil
}
