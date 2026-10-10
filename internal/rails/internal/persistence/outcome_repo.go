package persistence

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/irfanali17899/fintech/internal/rails/internal/domain"
	"github.com/irfanali17899/fintech/pkg/database"
	"github.com/irfanali17899/fintech/pkg/errdefs"
)

type OutcomeRepo struct{ db database.DBTX }

func NewOutcomeRepo(db database.DBTX) *OutcomeRepo    { return &OutcomeRepo{db: db} }
func (r *OutcomeRepo) WithTx(tx *sql.Tx) *OutcomeRepo { return &OutcomeRepo{db: tx} }

const outcomeColumns = `ref, kind, key, amount, state, reason, applied_at, claimed_until, attempt_count, next_attempt_at, last_error, created_at, updated_at`

func scanOutcome(row interface{ Scan(...any) error }, o *domain.Outcome) error {
	return row.Scan(&o.Ref, &o.Kind, &o.Key, &o.Amount, &o.State, &o.Reason, &o.AppliedAt, &o.ClaimedUntil,
		&o.AttemptCount, &o.NextAttemptAt, &o.LastError, &o.CreatedAt, &o.UpdatedAt)
}

// Create records an outcome request; repeats of the same (ref, kind, key) are no-ops.
func (r *OutcomeRepo) Create(ctx context.Context, o domain.Outcome) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO rails.outcomes (ref, kind, key, amount)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (ref, kind, key) DO NOTHING`, o.Ref, o.Kind, o.Key, o.Amount)
	if err != nil {
		return errdefs.Internalf(err, "create outcome")
	}
	return nil
}

func (r *OutcomeRepo) Get(ctx context.Context, ref string, kind domain.OutcomeKind, key string) (*domain.Outcome, error) {
	var o domain.Outcome
	err := scanOutcome(r.db.QueryRowContext(ctx,
		`SELECT `+outcomeColumns+` FROM rails.outcomes WHERE ref = $1 AND kind = $2 AND key = $3`, ref, kind, key), &o)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, domain.ErrOutcomeNotFound
	}
	if err != nil {
		return nil, errdefs.Internalf(err, "get outcome")
	}
	return &o, nil
}

// Decide moves an initiated outcome to its decided state. False = not initiated.
func (r *OutcomeRepo) Decide(ctx context.Context, ref string, kind domain.OutcomeKind, key string, to domain.AttemptState, reason *string) (bool, error) {
	res, err := r.db.ExecContext(ctx, `
		UPDATE rails.outcomes
		SET state = $4, reason = COALESCE($5, reason), updated_at = now()
		WHERE ref = $1 AND kind = $2 AND key = $3 AND state = 'initiated'`,
		ref, kind, key, to, reason)
	if err != nil {
		return false, errdefs.Internalf(err, "decide outcome")
	}
	n, _ := res.RowsAffected()
	return n == 1, nil
}

// ClaimDue leases outcomes that need work: settle outcomes whose attempt is past
// settle_after and still initiated, or any decided outcome the ledger hasn't
// applied. Rows under a live lease, backing off after a failure, or locked by
// another worker are skipped. Tx-bound only.
func (r *OutcomeRepo) ClaimDue(ctx context.Context, now time.Time, lease time.Duration, limit int) ([]domain.Outcome, error) {
	rows, err := r.db.QueryContext(ctx, `
		WITH due AS (
			SELECT o.ref, o.kind, o.key
			FROM rails.outcomes o
			JOIN rails.attempts a ON a.ref = o.ref
			WHERE (o.claimed_until IS NULL OR o.claimed_until < $1)
			  AND (o.next_attempt_at IS NULL OR o.next_attempt_at <= $1)
			  AND (
			    (o.kind = 'settle' AND o.state = 'initiated' AND a.settle_after <= $1)
			    OR (o.state <> 'initiated' AND o.applied_at IS NULL)
			  )
			ORDER BY o.updated_at
			LIMIT $3
			FOR UPDATE OF o SKIP LOCKED
		)
		UPDATE rails.outcomes o
		SET claimed_until = $2, updated_at = now()
		FROM due
		WHERE o.ref = due.ref AND o.kind = due.kind AND o.key = due.key
		RETURNING `+qualified(outcomeColumns, "o"),
		now, now.Add(lease), limit)
	if err != nil {
		return nil, errdefs.Internalf(err, "claim due outcomes")
	}
	defer rows.Close()

	var out []domain.Outcome
	for rows.Next() {
		var o domain.Outcome
		if err := scanOutcome(rows, &o); err != nil {
			return nil, errdefs.Internalf(err, "scan claimed outcome")
		}
		out = append(out, o)
	}
	if err := rows.Err(); err != nil {
		return nil, errdefs.Internalf(err, "claim due outcomes rows")
	}
	return out, nil
}

// MarkApplied records the ledger's ack and releases the lease. False = already applied.
func (r *OutcomeRepo) MarkApplied(ctx context.Context, ref string, kind domain.OutcomeKind, key string) (bool, error) {
	res, err := r.db.ExecContext(ctx, `
		UPDATE rails.outcomes
		SET applied_at = now(), claimed_until = NULL, updated_at = now()
		WHERE ref = $1 AND kind = $2 AND key = $3 AND applied_at IS NULL`, ref, kind, key)
	if err != nil {
		return false, errdefs.Internalf(err, "mark outcome applied")
	}
	n, _ := res.RowsAffected()
	return n == 1, nil
}

// RecordFailure counts a failed attempt to decide or apply, backs the row off
// exponentially (1, 2, 4 ... minutes, capped at 6 hours) and releases its lease.
func (r *OutcomeRepo) RecordFailure(ctx context.Context, ref string, kind domain.OutcomeKind, key, cause string) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE rails.outcomes
		SET attempt_count   = attempt_count + 1,
		    next_attempt_at = now() + LEAST(power(2, LEAST(attempt_count, 9)) * interval '1 minute', interval '6 hours'),
		    last_error      = $4,
		    claimed_until   = NULL,
		    updated_at      = now()
		WHERE ref = $1 AND kind = $2 AND key = $3`, ref, kind, key, cause)
	if err != nil {
		return errdefs.Internalf(err, "record outcome failure")
	}
	return nil
}

// ByAttempts loads every outcome for the given refs, for reconciliation.
func (r *OutcomeRepo) ByAttempts(ctx context.Context, refs []string) ([]domain.Outcome, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT `+outcomeColumns+` FROM rails.outcomes WHERE ref = ANY($1)`, refs)
	if err != nil {
		return nil, errdefs.Internalf(err, "outcomes by attempts")
	}
	defer rows.Close()
	var out []domain.Outcome
	for rows.Next() {
		var o domain.Outcome
		if err := scanOutcome(rows, &o); err != nil {
			return nil, errdefs.Internalf(err, "scan outcome")
		}
		out = append(out, o)
	}
	if err := rows.Err(); err != nil {
		return nil, errdefs.Internalf(err, "outcomes by attempts rows")
	}
	return out, nil
}
