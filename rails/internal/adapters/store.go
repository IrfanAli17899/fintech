package adapters

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/irfanali17899/fintech/platform/database"
	"github.com/irfanali17899/fintech/rails/internal/domain"
	"github.com/irfanali17899/fintech/rails/internal/persistence"
)

// Store makes every rail write atomic: an attempt and its outcomes change
// together or not at all, under the attempt's row lock.
type Store struct {
	tx       database.TxRunner
	attempts *persistence.AttemptRepo
	outcomes *persistence.OutcomeRepo
}

func NewStore(tx database.TxRunner, attempts *persistence.AttemptRepo, outcomes *persistence.OutcomeRepo) Store {
	return Store{tx: tx, attempts: attempts, outcomes: outcomes}
}

type txRepos struct {
	attempts *persistence.AttemptRepo
	outcomes *persistence.OutcomeRepo
}

// initiate records an attempt and its undecided settle outcome. Both inserts
// are ON CONFLICT DO NOTHING, so a retried Initiate is a no-op.
func (s Store) initiate(ctx context.Context, a domain.Attempt) error {
	return s.tx.InTx(ctx, func(tx *sql.Tx) error {
		r := s.bind(tx)
		if err := r.attempts.Create(ctx, a); err != nil {
			return err
		}
		return r.outcomes.Create(ctx, domain.Outcome{Ref: a.Ref, Kind: domain.OutcomeSettle, Amount: a.Amount})
	})
}

// locked runs fn with the attempt's row locked, inside one transaction.
func (s Store) locked(ctx context.Context, ref string, fn func(a *domain.Attempt, r txRepos) error) error {
	return s.tx.InTx(ctx, func(tx *sql.Tx) error {
		r := s.bind(tx)
		a, err := r.attempts.Lock(ctx, ref)
		if err != nil {
			return err
		}
		return fn(a, r)
	})
}

func (s Store) bind(tx *sql.Tx) txRepos {
	return txRepos{attempts: s.attempts.WithTx(tx), outcomes: s.outcomes.WithTx(tx)}
}

// decide moves an initiated outcome to its decision. False from the repo means
// it was not initiated, which the caller's state check should have ruled out.
func (r txRepos) decide(ctx context.Context, ref string, kind domain.OutcomeKind, key string, to domain.AttemptState, reason *string) error {
	ok, err := r.outcomes.Decide(ctx, ref, kind, key, to, reason)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("decide %s %s %q: %w", ref, kind, key, domain.ErrWrongState)
	}
	return nil
}

// transition moves the attempt out of initiated, under the lock taken by locked.
func (r txRepos) transition(ctx context.Context, ref string, to domain.AttemptState, reason *string) error {
	ok, err := r.attempts.Transition(ctx, ref, domain.AttemptInitiated, to, reason)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("transition %s to %s: %w", ref, to, domain.ErrWrongState)
	}
	return nil
}
