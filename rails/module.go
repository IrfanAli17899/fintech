package rails

import (
	"time"

	"github.com/irfanali17899/fintech/ledger/ports"
	"github.com/irfanali17899/fintech/platform/database"
	"github.com/irfanali17899/fintech/rails/internal/adapters"
	"github.com/irfanali17899/fintech/rails/internal/persistence"
)

// New returns the registry of rails this deployment supports.
func New(db *database.DB) ports.RailRegistry {
	attempts := persistence.NewAttemptRepo(db.DB)
	outcomes := persistence.NewOutcomeRepo(db.DB)
	store := adapters.NewStore(db, attempts, outcomes)

	return adapters.NewRegistry(
		db,
		attempts,
		outcomes,
		adapters.NullRail{},
		adapters.NewRTPRail("raast", "PKR"),
		adapters.NewRTPRail("sarie", "SAR"),
		adapters.NewCardRail(store, 48*time.Hour),
		adapters.NewSwiftRail(store, 72*time.Hour),
	)
}
