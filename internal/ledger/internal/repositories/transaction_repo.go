package repositories

import (
	"context"
	"database/sql"
	"errors"

	"github.com/google/uuid"
	"github.com/irfanali17899/fintech/internal/ledger/internal/domain"
	"github.com/irfanali17899/fintech/pkg/database"
	"github.com/irfanali17899/fintech/pkg/errdefs"
)

type TransactionRepo struct {
	db database.DBTX
}

func NewTransactionRepo(db database.DBTX) *TransactionRepo {
	return &TransactionRepo{db}
}

func (r *TransactionRepo) WithTx(tx *sql.Tx) *TransactionRepo { return &TransactionRepo{db: tx} }

func (r *TransactionRepo) Create(ctx context.Context, t domain.Transaction) (*domain.Transaction, error) {
	err := r.db.QueryRowContext(ctx,
		`INSERT INTO transactions (kind, reverses_id) 
		VALUES ($1,$2) RETURNING id, status, created_at, posted_at`,
		t.Kind, t.ReversesID,
	).Scan(&t.ID, &t.Status, &t.CreatedAt, &t.PostedAt)

	if err != nil {
		return nil, errdefs.Internalf(err, "create transaction")
	}

	return &t, nil
}

func (r *TransactionRepo) GetByID(ctx context.Context, id uuid.UUID) (*domain.Transaction, error) {
	return r.get(ctx, id, false)
}

// LockByID loads the transaction with its row locked; tx-bound only.
func (r *TransactionRepo) LockByID(ctx context.Context, id uuid.UUID) (*domain.Transaction, error) {
	return r.get(ctx, id, true)
}

func (r *TransactionRepo) get(ctx context.Context, id uuid.UUID, lock bool) (*domain.Transaction, error) {
	var t domain.Transaction

	query := `SELECT id, kind, status, reverses_id, created_at, posted_at from transactions where id=$1`
	if lock {
		query += ` FOR UPDATE`
	}

	err := r.db.QueryRowContext(ctx, query, id).Scan(&t.ID, &t.Kind, &t.Status, &t.ReversesID, &t.CreatedAt, &t.PostedAt)

	if errors.Is(err, sql.ErrNoRows) {
		return nil, domain.ErrTransactionNotFound(id)
	}

	if err != nil {
		return nil, errdefs.Internalf(err, "get transaction")
	}
	return &t, nil
}

// ReversedByAccount returns, for each account, how much has already been
// reversed against original (taken from the reversals' entries).
func (r *TransactionRepo) ReversedByAccount(ctx context.Context, originalID uuid.UUID) (map[uuid.UUID]int64, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT e.account_id, SUM(e.amount)
		FROM transactions t
		JOIN entries e ON e.transaction_id = t.id
		WHERE t.reverses_id = $1 AND t.status = 'posted'
		GROUP BY e.account_id`, originalID)
	if err != nil {
		return nil, errdefs.Internalf(err, "reversed by account")
	}
	defer rows.Close()

	out := make(map[uuid.UUID]int64)
	for rows.Next() {
		var id uuid.UUID
		var sum int64
		if err := rows.Scan(&id, &sum); err != nil {
			return nil, errdefs.Internalf(err, "scan reversed by account")
		}
		out[id] = sum
	}
	if err := rows.Err(); err != nil {
		return nil, errdefs.Internalf(err, "reversed by account rows")
	}
	return out, nil
}

func (r *TransactionRepo) MarkReversed(ctx context.Context, id uuid.UUID) error {
	res, err := r.db.ExecContext(ctx,
		`UPDATE transactions SET status = 'reversed' WHERE id = $1 AND status = 'posted'`, id)
	if err != nil {
		return errdefs.Internalf(err, "mark reversed")
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return errdefs.Internalf(nil, "transaction %s not posted when marking reversed", id)
	}
	return nil
}
