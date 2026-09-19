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

const balanceColumns = `account_id, debits, credits, updated_at`

func (r *BalanceRepo) GetBalance(ctx context.Context, accountID uuid.UUID) (*domain.AccountSums, error) {
	var s domain.AccountSums
	err := r.db.QueryRowContext(ctx,
		`SELECT debits, credits from balances where account_id=$1`,
		accountID).Scan(&s.Debits, &s.Credits)

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
		`INSERT INTO balances (account_id, debits, credits)
		VALUES ($1, $2, $3)
		ON CONFLICT (account_id) DO UPDATE
		SET debits = balances.debits  + EXCLUDED.debits,
		credits = balances.credits + EXCLUDED.credits,
		updated_at = now()
		`,
		accountID, s.Debits, s.Credits,
	)

	if err != nil {
		return errdefs.Internalf(err, "apply balance")

	}
	return nil
}

func (r *BalanceRepo) RebuildAll(ctx context.Context) error {
	if _, err := r.db.ExecContext(ctx, `TRUNCATE balances`); err != nil {
		return errdefs.Internalf(err, "truncate balance table for rebuild")
	}

	_, err := r.db.ExecContext(ctx, `
	INSERT INTO balances (account_id, debits, credits)
	SELECT 
		e.account_id,
		COALESCE(SUM(CASE WHEN direction='debit' THEN amount END),0),
		COALESCE(SUM(CASE WHEN direction='credit' THEN amount END),0)
	FROM entries e
	JOIN transactions t ON e.transaction_id = t.id
	WHERE t.status = 'posted'
	GROUP BY e.account_id
	`)

	if err != nil {
		return errdefs.Internalf(err, "rebuild balances")
	}
	return nil
}

func (r *BalanceRepo) Verify(ctx context.Context) ([]domain.BalanceMismatch, error) {
	rows, err := r.db.QueryContext(ctx, `
	WITH live AS (
		SELECT e.account_id,
				COALESCE(SUM(CASE WHEN e.direction = 'debit'  THEN e.amount END), 0) AS debits,
				COALESCE(SUM(CASE WHEN e.direction = 'credit' THEN e.amount END), 0) AS credits
		FROM entries e
		JOIN transactions t ON t.id = e.transaction_id
		WHERE t.status = 'posted'
		GROUP BY e.account_id
	)
	SELECT COALESCE(b.account_id, l.account_id) AS account_id,
		COALESCE(b.debits, 0)                AS cached_debits,
		COALESCE(b.credits, 0)               AS cached_credits,
		COALESCE(l.debits, 0)                AS live_debits,
		COALESCE(l.credits, 0)               AS live_credits
	FROM balances b
	FULL OUTER JOIN live l ON l.account_id = b.account_id
	WHERE COALESCE(b.debits, 0)  <> COALESCE(l.debits, 0)
	OR COALESCE(b.credits, 0) <> COALESCE(l.credits, 0)
	ORDER BY account_id;
	`)

	if err != nil {
		return nil, errdefs.Internalf(err, "verify balances")
	}
	defer rows.Close()

	m := make([]domain.BalanceMismatch, 0)

	for rows.Next() {
		o := domain.BalanceMismatch{}
		err := rows.Scan(&o.AccountID, &o.Cached.Debits, &o.Cached.Credits, &o.Live.Debits, &o.Live.Credits)
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
