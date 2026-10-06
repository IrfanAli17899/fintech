package adapters

import (
	"context"

	"github.com/irfanali17899/fintech/internal/ledger/api"
)

// NullRail is the internal rail: money that never leaves the ledger.
type NullRail struct{}

func (NullRail) Name() string { return "internal" }
func (NullRail) Properties() api.RailProperties {
	return api.RailProperties{Instant: true, Reversible: true}
}
func (NullRail) Initiate(_ context.Context, req api.RailInitiateRequest) (api.RailInitiateResult, error) {
	return api.RailInitiateResult{Ref: req.TransactionID.String(), Settlement: api.SettlementImmediate}, nil
}
func (NullRail) Settle(context.Context, string) error         { return nil }
func (NullRail) Reverse(context.Context, string, int64) error { return nil }
