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
}
type AccountEntry struct {
	Seq           int64
	Amount        int64
	Direction     EntryDirection
	CreatedAt     time.Time
	TransactionID uuid.UUID
	Kind          TransactionKind
	Status        TransactionStatus
	Rail          string
	ParentID      *uuid.UUID
}
