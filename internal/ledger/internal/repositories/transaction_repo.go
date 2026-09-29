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
		`INSERT INTO transactions (kind, status, parent_id, posted_at) 
		VALUES ($1, $2, $3, CASE WHEN $2::transaction_status = 'posted' THEN now() END) RETURNING id, status, created_at, posted_at`,
		t.Kind, t.Status, t.ParentID,
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

	query := `SELECT id, kind, status, parent_id, created_at, posted_at from transactions where id=$1`
	if lock {
		query += ` FOR UPDATE`
	}

	err := r.db.QueryRowContext(ctx, query, id).Scan(&t.ID, &t.Kind, &t.Status, &t.ParentID, &t.CreatedAt, &t.PostedAt)

	if errors.Is(err, sql.ErrNoRows) {
		return nil, domain.ErrTransactionNotFound(id)
	}

	if err != nil {
		return nil, errdefs.Internalf(err, "get transaction")
	}
	return &t, nil
}

// ChildAmountsByAccount sums, per account, the posted children of parent that
// have the given kind: refunds against a payment, or captures against a hold.
func (r *TransactionRepo) ChildAmountsByAccount(ctx context.Context, parentID uuid.UUID, kind domain.TransactionKind) (map[uuid.UUID]int64, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT e.account_id, SUM(e.amount)
		FROM transactions t
		JOIN entries e ON e.transaction_id = t.id
		WHERE t.parent_id = $1 AND t.kind = $2 AND t.status = 'posted'
		GROUP BY e.account_id`, parentID, kind)
	if err != nil {
		return nil, errdefs.Internalf(err, "child amounts by account")
	}
	defer rows.Close()

	out := make(map[uuid.UUID]int64)
	for rows.Next() {
		var id uuid.UUID
		var sum int64
		if err := rows.Scan(&id, &sum); err != nil {
			return nil, errdefs.Internalf(err, "scan child amounts")
		}
		out[id] = sum
	}
	if err := rows.Err(); err != nil {
		return nil, errdefs.Internalf(err, "child amounts rows")
	}
	return out, nil
}

func (r *TransactionRepo) MarkVoided(ctx context.Context, id uuid.UUID) error {
	res, err := r.db.ExecContext(ctx,
		`UPDATE transactions SET status = 'voided' WHERE id = $1 AND status = 'pending'`, id)
	if err != nil {
		return errdefs.Internalf(err, "mark voided")
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return errdefs.Internalf(nil, "transaction %s not pending when voiding", id)
	}
	return nil
}