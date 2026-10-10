package persistence

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

const transactionColumns = `id, kind, relation, status, status_reason, parent_id, rail, rail_ref, expires_at, created_at, posted_at`

func scanTransaction(row interface{ Scan(...any) error }, t *domain.Transaction) error {
	return row.Scan(&t.ID, &t.Kind, &t.Relation, &t.Status, &t.StatusReason, &t.ParentID, &t.Rail, &t.RailRef, &t.ExpiresAt, &t.CreatedAt, &t.PostedAt)
}

func (r *TransactionRepo) Create(ctx context.Context, t domain.Transaction) (*domain.Transaction, error) {
	if t.ID == uuid.Nil {
		t.ID = uuid.Must(uuid.NewV7())
	}
	err := r.db.QueryRowContext(ctx,
		`INSERT INTO ledger.transactions (id, kind, relation, status, parent_id, rail, rail_ref, expires_at, posted_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, CASE WHEN $4::ledger.transaction_status = 'posted' THEN now() END)
		RETURNING created_at, posted_at`,
		t.ID, t.Kind, t.Relation, t.Status, t.ParentID, t.Rail, t.RailRef, t.ExpiresAt,
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
	q := `SELECT ` + transactionColumns + ` FROM ledger.transactions WHERE id = $1`
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

// ChildAmountsByAccount sums, per account, the children of parent with the
// given relation that still claim value: posted captures or reversals, plus
// pending reversals (a rail refund awaiting its outcome still reserves the
// amount). Voided children count for nothing.
func (r *TransactionRepo) ChildAmountsByAccount(ctx context.Context, parentID uuid.UUID, relation domain.TransactionRelation) (map[uuid.UUID]int64, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT e.account_id, SUM(e.amount)
		FROM ledger.transactions t
		JOIN ledger.entries e ON e.transaction_id = t.id
		WHERE t.parent_id = $1 AND t.relation = $2
		  AND t.status IN ('posted', 'pending')
		GROUP BY e.account_id`, parentID, relation)
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

func (r *TransactionRepo) MarkVoided(ctx context.Context, id uuid.UUID, reason *string) error {
	res, err := r.db.ExecContext(ctx,
		`UPDATE ledger.transactions SET status = 'voided', status_reason = $2 WHERE id = $1 AND status = 'pending'`, id, reason)
	if err != nil {
		return errdefs.Internalf(err, "mark voided")
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return errdefs.Internalf(nil, "transaction %s not pending when voiding", id)
	}
	return nil
}

// MarkPosted settles a pending transaction in place and returns posted_at.
func (r *TransactionRepo) MarkPosted(ctx context.Context, id uuid.UUID) (time.Time, error) {
	var at time.Time
	err := r.db.QueryRowContext(ctx,
		`UPDATE ledger.transactions SET status = 'posted', posted_at = now()
		 WHERE id = $1 AND status = 'pending' RETURNING posted_at`, id).Scan(&at)
	if errors.Is(err, sql.ErrNoRows) {
		return at, errdefs.Internalf(nil, "transaction %s not pending when posting", id)
	}
	if err != nil {
		return at, errdefs.Internalf(err, "mark posted")
	}
	return at, nil
}

// NextExpiredHold returns one internal pending transaction past its expiry and
// locks it, skipping rows another worker holds. Rail-backed holds are resolved
// by their rail, never by the clock. Tx-bound only: the caller voids the hold
// in the same transaction, so the lock covers the work.
func (r *TransactionRepo) NextExpiredHold(ctx context.Context, now time.Time) (uuid.UUID, bool, error) {
	var id uuid.UUID
	err := r.db.QueryRowContext(ctx, `
		SELECT id FROM ledger.transactions
		WHERE status = 'pending' AND expires_at <= $1 AND rail IN ('', 'internal')
		ORDER BY expires_at
		LIMIT 1
		FOR UPDATE SKIP LOCKED`, now).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return uuid.Nil, false, nil
	}
	if err != nil {
		return uuid.Nil, false, errdefs.Internalf(err, "next expired hold")
	}
	return id, true, nil
}

// ListByParent returns the direct children of a transaction, oldest first.
func (r *TransactionRepo) ListByParent(ctx context.Context, parentID uuid.UUID) ([]domain.Transaction, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT `+transactionColumns+` FROM ledger.transactions WHERE parent_id = $1 ORDER BY created_at, id`, parentID)
	if err != nil {
		return nil, errdefs.Internalf(err, "list children")
	}
	defer rows.Close()

	var out []domain.Transaction
	for rows.Next() {
		var t domain.Transaction
		if err := scanTransaction(rows, &t); err != nil {
			return nil, errdefs.Internalf(err, "scan child")
		}
		out = append(out, t)
	}
	if err := rows.Err(); err != nil {
		return nil, errdefs.Internalf(err, "list children rows")
	}
	return out, nil
}

func (r *TransactionRepo) Summary(ctx context.Context, id uuid.UUID) (domain.TransactionSummary, error) {
	var s domain.TransactionSummary
	err := r.db.QueryRowContext(ctx,
		`SELECT total, captured, reversed, settlement FROM ledger.transaction_summaries WHERE id = $1`, id,
	).Scan(&s.Total, &s.Captured, &s.Reversed, &s.Settlement)
	if errors.Is(err, sql.ErrNoRows) {
		return s, domain.ErrTransactionNotFound(id)
	}
	if err != nil {
		return s, errdefs.Internalf(err, "transaction summary")
	}
	s.Remaining = s.Total - s.Captured - s.Reversed
	return s, nil
}
