package domain

import (
	"time"

	"github.com/google/uuid"
)

type Balance struct {
	AccountID uuid.UUID
	Currency  string
	Amount    int64
	AsOf      time.Time
}
