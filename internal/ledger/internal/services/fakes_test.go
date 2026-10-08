package services_test

import (
	"context"
	"time"

	"github.com/irfanali17899/fintech/internal/ledger/api"
)

// fakeRail returns a canned Initiate result; Settle is a no-op and Reverse
// returns revErr.
type fakeRail struct {
	name   string
	props  api.RailProperties
	res    api.RailInitiateResult
	err    error
	revErr error
}

func (f fakeRail) Name() string                   { return f.name }
func (f fakeRail) Properties() api.RailProperties { return f.props }
func (f fakeRail) Initiate(context.Context, api.RailInitiateRequest) (api.RailInitiateResult, error) {
	return f.res, f.err
}
func (fakeRail) Settle(context.Context, string) error         { return nil }
func (f fakeRail) Reverse(context.Context, string, int64) error { return f.revErr }

// fakeRegistry is a map that satisfies api.RailRegistry.
type fakeRegistry map[string]api.Rail

func (r fakeRegistry) Get(name string) (api.Rail, bool) {
	x, ok := r[name]
	return x, ok
}

func (r fakeRegistry) Due(context.Context, time.Time, int) ([]api.DueAttempt, error) {
	return nil, nil
}

// instantInternal mirrors the real internal rail so tests using a fake
// registry still have a default rail.
var instantInternal = fakeRail{
	name:  "internal",
	props: api.RailProperties{Instant: true, Reversible: true},
	res:   api.RailInitiateResult{Settlement: api.SettlementImmediate},
}
