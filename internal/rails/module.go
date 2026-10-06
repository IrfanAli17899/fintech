package rails

import (
	"time"

	"github.com/irfanali17899/fintech/internal/ledger/api"
	"github.com/irfanali17899/fintech/internal/rails/internal/adapters"
	"github.com/irfanali17899/fintech/internal/rails/internal/repositories"
	"github.com/irfanali17899/fintech/pkg/database"
)

// New returns the registry of available rails.
func New(db *database.DB) api.RailRegistry {
	attempts := repositories.NewAttemptRepo(db.DB)

	return adapters.NewRegistry(
		db,
		attempts,
		adapters.NullRail{},
		adapters.NewRTPRail("raast", "PKR"),
		adapters.NewRTPRail("sarie", "SAR"),
		adapters.NewCardRail(attempts, 48*time.Hour),
		adapters.NewSwiftRail(attempts, 72*time.Hour),
	)
}
