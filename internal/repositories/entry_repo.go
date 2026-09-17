package repositories

import (
	"context"
	"database/sql"

	"github.com/google/uuid"
	"github.com/irfanali17899/fintech/internal/domain"
	"github.com/irfanali17899/fintech/pkg/database"
	"github.com/irfanali17899/fintech/pkg/errdefs"
)

type EntryRepo struct {
	db database.DBTX
}

func NewEntryRepo(db *sql.DB) *EntryRepo {
	return &EntryRepo{db}
}

func (r *EntryRepo) WithTx(tx *sql.Tx) *EntryRepo { return &EntryRepo{db: tx} }

const entryColumns = `id, transaction_id, account_id, amount, direction, created_at`

func scanEntries(rows *sql.Rows) ([]domain.Entry, error) {
	var out []domain.Entry
	for rows.Next() {
		var e domain.Entry
		if err := rows.Scan(&e.ID, &e.TransactionID, &e.AccountID, &e.Amount, &e.Direction, &e.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func (r *EntryRepo) CreateMany(ctx context.Context, entries []domain.Entry) ([]domain.Entry, error) {
	if len(entries) == 0 {
		return nil, nil
	}

	n := len(entries)
	txIDs := make([]uuid.UUID, n)
	accIDs := make([]uuid.UUID, n)
	amounts := make([]int64, n)
	dirs := make([]string, n)
	for i, e := range entries {
		txIDs[i] = e.TransactionID
		accIDs[i] = e.AccountID
		amounts[i] = e.Amount
		dirs[i] = string(e.Direction)
	}

	rows, err := r.db.QueryContext(ctx,
		`INSERT INTO entries (transaction_id, account_id, amount, direction)
		 SELECT * FROM unnest($1::uuid[], $2::uuid[], $3::bigint[], $4::entry_direction[])
		 RETURNING `+entryColumns,
		txIDs, accIDs, amounts, dirs,
	)
	if err != nil {
		return nil, errdefs.Internalf(err, "create entries")
	}
	defer rows.Close()

	out, err := scanEntries(rows)
	if err != nil {
		return nil, errdefs.Internalf(err, "scan entries")
	}
	return out, nil
}

func (r *EntryRepo) ListByTransaction(ctx context.Context, txID uuid.UUID) ([]domain.Entry, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT `+entryColumns+
			` FROM entries WHERE transaction_id = $1 ORDER BY created_at, id`, txID)

	if err != nil {
		return nil, errdefs.Internalf(err, "list entries")
	}
	defer rows.Close()

	out, err := scanEntries(rows)
	if err != nil {
		return nil, errdefs.Internalf(err, "scan entries")
	}

	return out, nil
}

func (r *EntryRepo) SumByAccount(ctx context.Context, accId uuid.UUID) (debits, credits int64, err error) {
	err = r.db.QueryRowContext(ctx,
		`SELECT
  		COALESCE(SUM(CASE WHEN e.direction = 'debit'  THEN e.amount ELSE 0 END), 0) AS debits,
  		COALESCE(SUM(CASE WHEN e.direction = 'credit' THEN e.amount ELSE 0 END), 0) AS credits
		FROM entries e
		JOIN transactions t ON t.id = e.transaction_id
		WHERE e.account_id = $1 AND t.status = 'posted'`,
		accId).Scan(&debits, &credits)

	if err != nil {
		return 0, 0, errdefs.Internalf(err, "sum entries for account")
	}

	return debits, credits, nil

}
