package api

import (
	"context"
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

// Rail is the port the ledger uses to move money in or out of the outside world.
type Rail interface {
	Name() string
	Properties() RailProperties
	Initiate(ctx context.Context, req RailInitiateRequest) (RailInitiateResult, error)
	Settle(ctx context.Context, ref string) error
	Reverse(ctx context.Context, ref string, amount int64) error
}

// RailRegistry resolves a rail by name.
type RailRegistry interface {
	Get(name string) (Rail, bool)
}
