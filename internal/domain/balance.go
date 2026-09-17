package domain

import (
	"time"

	"github.com/google/uuid"
)

type AccountSums struct{ Debits, Credits int64 }

type Balance struct {
	AccountID uuid.UUID
	Currency  string
	Amount    int64
	AsOf      time.Time
}

type BalanceMismatch struct {
	AccountID uuid.UUID
	Cached    AccountSums
	Live      AccountSums
}
