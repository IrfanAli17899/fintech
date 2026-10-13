// Package port holds what the application layer needs from infrastructure but
// does not own. Repos stay concrete (see ARCHITECTURE.md); only the unit of
// work is abstracted, so use cases never see a database transaction.
package port

import (
	"context"

	"github.com/irfanali17899/fintech/internal/ledger/internal/persistence"
)

// UnitOfWork runs fn atomically, with every store bound to the same
// transaction: fn's writes all commit, or none do.
type UnitOfWork interface {
	Do(ctx context.Context, fn func(persistence.Stores) error) error
}
