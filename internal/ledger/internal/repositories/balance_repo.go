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

type BalanceRepo struct {
	db database.DBTX
}

func NewBalanceRepo(db database.DBTX) *BalanceRepo {
	return &BalanceRepo{db}
}

func (r *BalanceRepo) WithTx(tx *sql.Tx) *BalanceRepo { return &BalanceRepo{db: tx} }

const sumColumns = `debits, credits, pending_debits, pending_credits`
const balanceColumns = `account_id, ` + sumColumns + `, updated_at`

func (r *BalanceRepo) GetBalance(ctx context.Context, accountID uuid.UUID) (*domain.AccountSums, error) {
	var s domain.AccountSums
	err := r.db.QueryRowContext(ctx,
		`SELECT `+sumColumns+` FROM balances WHERE account_id=$1`,
		accountID).Scan(&s.Debits, &s.Credits, &s.PendingDebits, &s.PendingCredits)

	if errors.Is(err, sql.ErrNoRows) {
		return &domain.AccountSums{}, nil
	}

	if err != nil {
		return nil, errdefs.Internalf(err, "get account balance")
	}
	return &s, nil
}

func (r *BalanceRepo) Apply(ctx context.Context, accountID uuid.UUID, s domain.AccountSums) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO balances (account_id, debits, credits, pending_debits, pending_credits)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (account_id) DO UPDATE
		SET debits = balances.debits  + EXCLUDED.debits,
		credits = balances.credits + EXCLUDED.credits,
		pending_debits = balances.pending_debits  + EXCLUDED.pending_debits,
		pending_credits = balances.pending_credits + EXCLUDED.pending_credits,
		updated_at = now()
		`,
		accountID, s.Debits, s.Credits, s.PendingDebits, s.PendingCredits,
	)

	if err != nil {
		return errdefs.Internalf(err, "apply balance")

	}
	return nil
}

// reversed is a view on a posted transaction, never a change to what it did. Every place that sums money must treat it as posted
// its a product status for posted refunds without subquery

const liveBalancesCTE = `
	SELECT
		e.account_id,
		COALESCE(SUM(CASE WHEN t.status = 'posted' AND e.direction = 'debit'  THEN e.amount END), 0) AS debits,
		COALESCE(SUM(CASE WHEN t.status = 'posted' AND e.direction = 'credit' THEN e.amount END), 0) AS credits,
		COALESCE(SUM(CASE
			WHEN t.status = 'pending' AND e.direction = 'debit' THEN e.amount
			WHEN t.status = 'posted' AND p.status = 'pending' AND e.direction = 'debit' THEN -e.amount
		END), 0) AS pending_debits,
		COALESCE(SUM(CASE
			WHEN t.status = 'pending' AND e.direction = 'credit' THEN e.amount
			WHEN t.status = 'posted' AND p.status = 'pending' AND e.direction = 'credit' THEN -e.amount
		END), 0) AS pending_credits
	FROM entries e
	JOIN transactions t ON t.id = e.transaction_id
	LEFT JOIN transactions p ON p.id = t.parent_id
	WHERE t.status IN ('posted', 'pending')
	GROUP BY e.account_id
`

func (r *BalanceRepo) RebuildAll(ctx context.Context) error {
	if _, err := r.db.ExecContext(ctx, `TRUNCATE balances`); err != nil {
		return errdefs.Internalf(err, "truncate balances")
	}

	if _, err := r.db.ExecContext(ctx,
		`INSERT INTO balances (account_id, `+sumColumns+`) `+liveBalancesCTE); err != nil {
		return errdefs.Internalf(err, "rebuild balances")
	}
	return nil
}

func (r *BalanceRepo) Verify(ctx context.Context) ([]domain.BalanceMismatch, error) {
	rows, err := r.db.QueryContext(ctx, `
	WITH live AS (`+liveBalancesCTE+`)
	SELECT COALESCE(b.account_id, l.account_id) AS account_id,
		COALESCE(b.debits, 0),          COALESCE(b.credits, 0),
		COALESCE(b.pending_debits, 0),  COALESCE(b.pending_credits, 0),
		COALESCE(l.debits, 0),          COALESCE(l.credits, 0),
		COALESCE(l.pending_debits, 0),  COALESCE(l.pending_credits, 0)
	FROM balances b
	FULL OUTER JOIN live l ON l.account_id = b.account_id
	WHERE COALESCE(b.debits, 0)          <> COALESCE(l.debits, 0)
	OR COALESCE(b.credits, 0)         <> COALESCE(l.credits, 0)
	OR COALESCE(b.pending_debits, 0)  <> COALESCE(l.pending_debits, 0)
	OR COALESCE(b.pending_credits, 0) <> COALESCE(l.pending_credits, 0)
	ORDER BY account_id
	`)

	if err != nil {
		return nil, errdefs.Internalf(err, "verify balances")
	}
	defer rows.Close()

	m := make([]domain.BalanceMismatch, 0)

	for rows.Next() {
		o := domain.BalanceMismatch{}
		err := rows.Scan(&o.AccountID,
			&o.Cached.Debits, &o.Cached.Credits, &o.Cached.PendingDebits, &o.Cached.PendingCredits,
			&o.Live.Debits, &o.Live.Credits, &o.Live.PendingDebits, &o.Live.PendingCredits)
		if err != nil {
			return nil, errdefs.Internalf(err, "verify balances scan")
		}
		m = append(m, o)
	}

	if err := rows.Err(); err != nil {
		return nil, errdefs.Internalf(err, "verify balances rows")
	}

	return m, nil
}

// LockAndGet locks the balance rows for ids and returns their sums.
// ids must be sorted so concurrent callers acquire locks in the same order.
// Only meaningful inside a transaction.
func (r *BalanceRepo) LockAndGet(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]domain.AccountSums, error) {
	rows, err := r.db.QueryContext(ctx, `
	SELECT account_id, `+sumColumns+` FROM balances WHERE account_id = ANY($1)
	ORDER BY account_id
	FOR UPDATE
	`, ids)

	if err != nil {
		return nil, errdefs.Internalf(err, "lock balances")
	}
	defer rows.Close()

	out := make(map[uuid.UUID]domain.AccountSums, len(ids))

	for rows.Next() {
		var id uuid.UUID
		var s domain.AccountSums
		if err := rows.Scan(&id, &s.Debits, &s.Credits, &s.PendingDebits, &s.PendingCredits); err != nil {
			return nil, errdefs.Internalf(err, "scan locked balance")
		}
		out[id] = s
	}

	if err := rows.Err(); err != nil {
		return nil, errdefs.Internalf(err, "lock balances rows")
	}
	return out, nil
}

func (r *BalanceRepo) ReleasePending(ctx context.Context, accountID uuid.UUID, s domain.AccountSums) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE balances SET
			pending_debits  = pending_debits  - $2,
			pending_credits = pending_credits - $3,
			updated_at      = now()
		WHERE account_id = $1`, accountID, s.PendingDebits, s.PendingCredits)
	if err != nil {
		return errdefs.Internalf(err, "release pending")
	}
	return nil
}
