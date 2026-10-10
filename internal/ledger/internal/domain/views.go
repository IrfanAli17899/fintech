package domain

import (
	"time"

	"github.com/google/uuid"
)

// TransactionSummary is derived from a transaction's posted children.
type TransactionSummary struct {
	Total     int64
	Captured  int64 // posted by children that capture this hold
	Reversed  int64 // returned by children that reverse this transaction
	Remaining int64 // Total - Captured - Reversed
	// Settlement is the status, except a voided hold with captures reads
	// settled_via_children (captured = total) or partially_settled.
	Settlement string
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
