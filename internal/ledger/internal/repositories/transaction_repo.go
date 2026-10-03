package repositories

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/irfanali17899/fintech/internal/ledger/internal/domain"
	"github.com/irfanali17899/fintech/pkg/database"
	"github.com/irfanali17899/fintech/pkg/errdefs"
)

type TransactionRepo struct {
	db database.DBTX
}

func NewTransactionRepo(db database.DBTX) *TransactionRepo {
	return &TransactionRepo{db: db}
}

func (r *TransactionRepo) WithTx(tx *sql.Tx) *TransactionRepo { return &TransactionRepo{db: tx} }

const transactionColumns = `id, kind, status, parent_id, rail, rail_ref, expires_at, created_at, posted_at`

func scanTransaction(row interface{ Scan(...any) error }, t *domain.Transaction) error {
	return row.Scan(&t.ID, &t.Kind, &t.Status, &t.ParentID, &t.Rail, &t.RailRef, &t.ExpiresAt, &t.CreatedAt, &t.PostedAt)
}

func (r *TransactionRepo) Create(ctx context.Context, t domain.Transaction) (*domain.Transaction, error) {
	if t.ID == uuid.Nil {
		t.ID = uuid.Must(uuid.NewV7())
	}
	err := r.db.QueryRowContext(ctx,
		`INSERT INTO transactions (id, kind, status, parent_id, rail, rail_ref, expires_at, posted_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, CASE WHEN $3::transaction_status = 'posted' THEN now() END)
		 RETURNING created_at, posted_at`,
		t.ID, t.Kind, t.Status, t.ParentID, t.Rail, t.RailRef, t.ExpiresAt,
	).Scan(&t.CreatedAt, &t.PostedAt)
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
	q := `SELECT ` + transactionColumns + ` FROM transactions WHERE id = $1`
	if lock {
		q += ` FOR UPDATE`
	}
	var t domain.Transaction
	err := scanTransaction(r.db.QueryRowContext(ctx, q, id), &t)
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

// ExpiredHolds returns ids of pending transactions whose expiry has passed, at
// most limit, skipping rows another worker already holds. Only meaningful
// inside a transaction; the lock lasts until that transaction ends.
func (r *TransactionRepo) ExpiredHolds(ctx context.Context, now time.Time, limit int) ([]uuid.UUID, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id FROM transactions
		WHERE status = 'pending' AND expires_at <= $1
		ORDER BY expires_at
		LIMIT $2
		FOR UPDATE SKIP LOCKED`, now, limit)
	if err != nil {
		return nil, errdefs.Internalf(err, "expired holds")
	}
	defer rows.Close()

	var ids []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, errdefs.Internalf(err, "scan expired hold")
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, errdefs.Internalf(err, "expired holds rows")
	}
	return ids, nil
}