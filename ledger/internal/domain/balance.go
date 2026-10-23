package domain

import (
	"time"

	"github.com/google/uuid"
)

type AccountSums struct{ Debits, Credits, PendingDebits, PendingCredits int64 }

type Balance struct {
	AccountID  uuid.UUID
	Currency   string
	Posted     int64 // normal balance of posted entries
	PendingIn  int64
	PendingOut int64
	Available  int64 // what a debit may draw on right now
	AsOf       time.Time
}

type BalanceMismatch struct {
	AccountID uuid.UUID
	Cached    AccountSums
	Live      AccountSums
}
