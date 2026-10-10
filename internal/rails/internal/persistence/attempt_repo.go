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

type AttemptRepo struct{ db database.DBTX }

func NewAttemptRepo(db database.DBTX) *AttemptRepo    { return &AttemptRepo{db: db} }
func (r *AttemptRepo) WithTx(tx *sql.Tx) *AttemptRepo { return &AttemptRepo{db: tx} }

const attemptColumns = `ref, rail, transaction_id, amount, currency, state, settle_after, reason, created_at, updated_at`

func scanAttempt(row interface{ Scan(...any) error }, a *domain.Attempt) error {
	return row.Scan(&a.Ref, &a.Rail, &a.TransactionID, &a.Amount, &a.Currency, &a.State, &a.SettleAfter, &a.Reason, &a.CreatedAt, &a.UpdatedAt)
}

// Create inserts an attempt; ON CONFLICT DO NOTHING makes Initiate idempotent
// on ref, which rails derive from the transaction id.
func (r *AttemptRepo) Create(ctx context.Context, a domain.Attempt) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO rails.attempts (ref, rail, transaction_id, amount, currency, settle_after)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (ref) DO NOTHING`,
		a.Ref, a.Rail, a.TransactionID, a.Amount, a.Currency, a.SettleAfter)
	if err != nil {
		return errdefs.Internalf(err, "create rail attempt")
	}
	return nil
}

func (r *AttemptRepo) Get(ctx context.Context, ref string) (*domain.Attempt, error) {
	var a domain.Attempt
	err := scanAttempt(r.db.QueryRowContext(ctx,
		`SELECT `+attemptColumns+` FROM rails.attempts WHERE ref = $1`, ref), &a)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, domain.ErrAttemptNotFound
	}
	if err != nil {
		return nil, errdefs.Internalf(err, "get rail attempt")
	}
	return &a, nil
}

// Transition moves an attempt from one state to another. False means it was
// not in the expected state.
func (r *AttemptRepo) Transition(ctx context.Context, ref string, from, to domain.AttemptState, reason *string) (bool, error) {
	res, err := r.db.ExecContext(ctx, `
		UPDATE rails.attempts
		SET state = $3, reason = COALESCE($4, reason), updated_at = now()
		WHERE ref = $1 AND state = $2`,
		ref, from, to, reason)
	if err != nil {
		return false, errdefs.Internalf(err, "transition rail attempt")
	}
	n, _ := res.RowsAffected()
	return n == 1, nil
}

// Due returns initiated attempts whose settle_after has passed, skipping rows
// another worker holds. Tx-bound only.
func (r *AttemptRepo) Due(ctx context.Context, now time.Time, limit int) ([]domain.Attempt, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT `+attemptColumns+` FROM rails.attempts
		WHERE state = 'initiated' AND settle_after <= $1
		ORDER BY settle_after
		LIMIT $2
		FOR UPDATE SKIP LOCKED`, now, limit)
	if err != nil {
		return nil, errdefs.Internalf(err, "due rail attempts")
	}
	defer rows.Close()

	var out []domain.Attempt
	for rows.Next() {
		var a domain.Attempt
		if err := scanAttempt(rows, &a); err != nil {
			return nil, errdefs.Internalf(err, "scan rail attempt")
		}
		out = append(out, a)
	}
	if err := rows.Err(); err != nil {
		return nil, errdefs.Internalf(err, "due rail attempts rows")
	}
	return out, nil
}
