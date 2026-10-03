package rails

import (
	"github.com/irfanali17899/fintech/internal/ledger/api"
	"github.com/irfanali17899/fintech/internal/rails/internal"
)

// New returns the registry of available rails.
func New() api.RailRegistry {
	return internal.NewRegistry(internal.NullRail{})
}
