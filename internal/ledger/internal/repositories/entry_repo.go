package repositories

import (
	"context"
	"database/sql"

	"github.com/google/uuid"
	"github.com/irfanali17899/fintech/internal/ledger/internal/domain"
	"github.com/irfanali17899/fintech/pkg/database"
	"github.com/irfanali17899/fintech/pkg/errdefs"
)

type EntryRepo struct {
	db database.DBTX
}

func NewEntryRepo(db database.DBTX) *EntryRepo {
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
		`INSERT INTO ledger.entries (transaction_id, account_id, amount, direction)
		 SELECT * FROM unnest($1::uuid[], $2::uuid[], $3::bigint[], $4::ledger.entry_direction[])
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
			` FROM ledger.entries WHERE transaction_id = $1 ORDER BY created_at, id`, txID)

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

// ListByAccount returns an account's entries newest first, each with its
// transaction's header. beforeSeq == 0 means start from the newest.
func (r *EntryRepo) ListByAccount(ctx context.Context, accountID uuid.UUID, beforeSeq int64, limit int) ([]domain.AccountEntry, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT e.seq, e.amount, e.direction, e.created_at,
		       t.id, t.kind, t.status, t.rail, t.parent_id
		FROM ledger.entries e
		JOIN ledger.transactions t ON t.id = e.transaction_id
		WHERE e.account_id = $1 AND ($2 = 0 OR e.seq < $2)
		ORDER BY e.seq DESC
		LIMIT $3`, accountID, beforeSeq, limit)
	if err != nil {
		return nil, errdefs.Internalf(err, "list account entries")
	}
	defer rows.Close()

	out := make([]domain.AccountEntry, 0, limit)
	for rows.Next() {
		var a domain.AccountEntry
		if err := rows.Scan(&a.Seq, &a.Amount, &a.Direction, &a.CreatedAt, &a.TransactionID, &a.Kind, &a.Status, &a.Rail, &a.ParentID); err != nil {
			return nil, errdefs.Internalf(err, "scan account entry")
		}
		out = append(out, a)
	}
	if err := rows.Err(); err != nil {
		return nil, errdefs.Internalf(err, "list account entries rows")
	}
	return out, nil
}
