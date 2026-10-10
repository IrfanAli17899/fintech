package application_test

import (
	"context"
	"errors"
	"time"

	"github.com/irfanali17899/fintech/internal/ledger/ports"
)

// fakeRail returns a canned Initiate result; Settle is a no-op and Reverse
// returns revErr. With calls set, Cancel and Reverse are counted and the first
// calls.fail of each time out.
type fakeRail struct {
	name   string
	props  ports.RailProperties
	res    ports.RailInitiateResult
	err    error // returned by Initiate
	revErr error // returned by Reverse
	calls  *railCalls
}

type railCalls struct{ fail, cancel, reverse int }

var errRailTimeout = errors.New("rail timeout")

func (f fakeRail) Name() string                     { return f.name }
func (f fakeRail) Properties() ports.RailProperties { return f.props }
func (f fakeRail) Initiate(context.Context, ports.RailInitiateRequest) (ports.RailInitiateResult, error) {
	return f.res, f.err
}
func (fakeRail) Settle(context.Context, string) error { return nil }
func (f fakeRail) Cancel(context.Context, string) error {
	if f.calls != nil {
		f.calls.cancel++
		if f.calls.cancel <= f.calls.fail {
			return errRailTimeout
		}
	}
	return nil
}
func (f fakeRail) Reverse(context.Context, string, string, int64) error {
	if f.calls != nil {
		f.calls.reverse++
		if f.calls.reverse <= f.calls.fail {
			return errRailTimeout
		}
	}
	return f.revErr
}

type fakeRegistry map[string]ports.Rail

func (r fakeRegistry) Get(name string) (ports.Rail, bool) { x, ok := r[name]; return x, ok }
func (fakeRegistry) ClaimDue(context.Context, time.Time, time.Duration, int) ([]ports.Outcome, error) {
	return nil, nil
}
func (fakeRegistry) MarkApplied(context.Context, ports.Outcome) error          { return nil }
func (fakeRegistry) RecordFailure(context.Context, ports.Outcome, error) error { return nil }

var instantInternal = fakeRail{
	name:  "internal",
	props: ports.RailProperties{Instant: true, Reversible: true},
	res:   ports.RailInitiateResult{Settlement: ports.SettlementImmediate},
}
