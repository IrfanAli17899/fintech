package domain

import (
	"time"

	"github.com/google/uuid"
)

type AttemptState string

const (
	AttemptInitiated AttemptState = "initiated"
	AttemptSettled   AttemptState = "settled"
	AttemptBounced   AttemptState = "bounced"
	AttemptReversed  AttemptState = "reversed"
)

// Attempt is one movement of money through an external rail, tracked from
// initiation to its final state. TransactionID is a correlation id only.
type Attempt struct {
	Ref           string
	Rail          string
	TransactionID uuid.UUID
	Amount        int64
	Currency      string
	State         AttemptState
	SettleAfter   time.Time
	Reason        *string
	CreatedAt     time.Time
	UpdatedAt     time.Time
}
