// Package api is the ledger's inbound contract: what other modules and the
// ledger's own transports may call. It is designed as if remote: every
// mutating call takes a required idempotency key, errors are *errdefs.Error
// carrying a code from package codes, and types are plain values.
package api

import (
	"context"

	"github.com/google/uuid"
)

// Ledger is what products use. It has no administrative operations; those are
// on LedgerAdmin, which the module never hands to another module.
type Ledger interface {
	// CreateAccount opens an account. An owner has at most one account per
	// type and currency; a second attempt returns codes.AccountExists.
	CreateAccount(ctx context.Context, cmd CreateAccount) (Account, error)
	GetAccount(ctx context.Context, id uuid.UUID) (Account, error)
	ListAccounts(ctx context.Context, q AccountQuery) ([]Account, error)
	Balance(ctx context.Context, accountID uuid.UUID) (Balance, error)
	// History pages an account's entries newest first.
	History(ctx context.Context, accountID uuid.UUID, p Page) (HistoryPage, error)

	// Post records a balanced transaction. With a deferred rail (card,
	// swift) it comes back pending and the rail settles or bounces it later.
	Post(ctx context.Context, cmd PostTransaction) (Result, error)
	// Reverse refunds all or part of a posted transaction. On a rail-backed
	// transaction the refund comes back pending until the rail confirms it.
	Reverse(ctx context.Context, cmd Reverse) (Result, error)
	// Capture posts all or part of an internal hold. A hold a rail owns
	// returns codes.RailManagedHold: the rail settles it.
	Capture(ctx context.Context, cmd Capture) (Result, error)
	// Void releases a hold. On a rail-backed hold it asks the rail to cancel
	// and the hold comes back still pending until the rail confirms; a rail
	// that cannot cancel returns codes.RailCannotCancel, and one that has
	// already settled returns codes.HoldAlreadySettling.
	Void(ctx context.Context, cmd Void) (Result, error)

	GetTransaction(ctx context.Context, id uuid.UUID) (Transaction, error)
}

// LedgerAdmin is for operators: rebuilding and checking derived state.
type LedgerAdmin interface {
	RebuildBalances(ctx context.Context) error
	VerifyBalances(ctx context.Context) ([]BalanceMismatch, error)
	// Reconcile compares every rail attempt with the ledger and reports
	// disagreements. It never changes either side.
	Reconcile(ctx context.Context) (ReconciliationReport, error)
}
