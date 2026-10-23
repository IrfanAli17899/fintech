package domain

import (
	"time"

	"github.com/google/uuid"
)

// TransactionSummary is derived from a transaction's posted children. Storage
// supplies Total, Captured and Reversed; Summarize derives the rest.
type TransactionSummary struct {
	Total     int64
	Captured  int64 // posted by children that capture this hold
	Reversed  int64 // returned by children that reverse this transaction
	Remaining int64 // Total - Captured - Reversed
	// Settlement is the status, except a voided hold with captures reads
	// settled_via_children (captured = total) or partially_settled.
	Settlement string
}

const (
	SettlementSettledViaChildren = "settled_via_children"
	SettlementPartiallySettled   = "partially_settled"
)

// Summarize derives Remaining and Settlement from the raw sums. A hold settles
// by capture children and is then voided, so "voided" alone cannot tell a
// settled hold from a cancelled one; Settlement says which.
func Summarize(status TransactionStatus, raw TransactionSummary) TransactionSummary {
	s := raw
	s.Remaining = s.Total - s.Captured - s.Reversed
	switch {
	case status == TransactionStatusVoided && s.Total > 0 && s.Captured >= s.Total:
		s.Settlement = SettlementSettledViaChildren
	case status == TransactionStatusVoided && s.Captured > 0:
		s.Settlement = SettlementPartiallySettled
	default:
		s.Settlement = string(status)
	}
	return s
}

type AccountEntry struct {
	Seq           int64
	Amount        int64
	Direction     EntryDirection
	CreatedAt     time.Time  // when the entry was written (authorization, for a hold)
	PostedAt      *time.Time // when its transaction posted; nil while pending or voided
	TransactionID uuid.UUID
	Kind          TransactionKind
	Status        TransactionStatus
	Rail          string
	ParentID      *uuid.UUID
}
