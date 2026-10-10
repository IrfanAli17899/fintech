package adapters

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/irfanali17899/fintech/internal/ledger/ports"
)

var ErrNotReversible = errors.New("rtp rail: transfers are final and cannot be reversed")

// RTPRail simulates an instant, irreversible bank rail such as Raast (PKR) or SARIE (SAR).
type RTPRail struct {
	name     string
	currency string
}

func NewRTPRail(name, currency string) *RTPRail {
	return &RTPRail{name: name, currency: currency}
}

func (r *RTPRail) Name() string { return r.name }

func (r *RTPRail) Properties() ports.RailProperties {
	return ports.RailProperties{Instant: true, Reversible: false}
}

func (r *RTPRail) Initiate(_ context.Context, req ports.RailInitiateRequest) (ports.RailInitiateResult, error) {
	if req.Currency != r.currency {
		return ports.RailInitiateResult{}, fmt.Errorf("%s rail only carries %s, got %s", r.name, r.currency, req.Currency)
	}
	return ports.RailInitiateResult{
		Ref:        strings.ToUpper(r.name) + "-" + req.TransactionID.String(),
		Settlement: ports.SettlementImmediate,
	}, nil
}

func (r *RTPRail) Settle(context.Context, string) error { return nil }

func (r *RTPRail) Reverse(context.Context, string, int64) error { return ErrNotReversible }
