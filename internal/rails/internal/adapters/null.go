package adapters

import (
	"context"

	"github.com/irfanali17899/fintech/internal/ledger/ports"
)

// NullRail is the internal rail: money that never leaves the ledger.
type NullRail struct{}

func (NullRail) Name() string { return "internal" }
func (NullRail) Properties() ports.RailProperties {
	return ports.RailProperties{Instant: true, Reversible: true}
}
func (NullRail) Initiate(_ context.Context, req ports.RailInitiateRequest) (ports.RailInitiateResult, error) {
	return ports.RailInitiateResult{Ref: req.TransactionID.String(), Settlement: ports.SettlementImmediate}, nil
}
func (NullRail) Settle(context.Context, string) error         { return nil }
func (NullRail) Reverse(context.Context, string, int64) error { return nil }
