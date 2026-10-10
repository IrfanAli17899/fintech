package persistence

import (
	"context"
	"database/sql"
	"errors"

	"github.com/google/uuid"
	"github.com/irfanali17899/fintech/internal/ledger/internal/domain"
	"github.com/irfanali17899/fintech/pkg/database"
	"github.com/irfanali17899/fintech/pkg/errdefs"
)

type AccountRepo struct {
	db database.DBTX
}

func NewAccountRepo(db database.DBTX) *AccountRepo {
	return &AccountRepo{db}
}

func (r *AccountRepo) WithTx(tx *sql.Tx) *AccountRepo { return &AccountRepo{db: tx} }

const accountColumns = "id, owner_type, owner_id, currency, type, created_at"

func scanAccounts(rows *sql.Rows) ([]domain.Account, error) {
	var out []domain.Account
	for rows.Next() {
		var a domain.Account
		if err := rows.Scan(&a.ID, &a.OwnerType, &a.OwnerID, &a.Currency, &a.Type, &a.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func (r *AccountRepo) Create(ctx context.Context, a domain.Account) (*domain.Account, error) {
	err := r.db.QueryRowContext(ctx,
		`INSERT INTO ledger.accounts (owner_type, owner_id, currency, type) 
		VALUES ($1,$2,$3,$4) RETURNING id, created_at`,
		a.OwnerType, a.OwnerID, a.Currency, a.Type,
	).Scan(&a.ID, &a.CreatedAt)

	if database.IsUniqueViolation(err) {
		return nil, domain.ErrAccountExists(a)
	}

	if err != nil {
		return nil, errdefs.Internalf(err, "create account")
	}

	return &a, nil
}

func (r *AccountRepo) GetByID(ctx context.Context, id uuid.UUID) (*domain.Account, error) {
	var a domain.Account
	err := r.db.QueryRowContext(ctx,
		`SELECT `+accountColumns+` from ledger.accounts where id=$1`,
		id).Scan(&a.ID, &a.OwnerType, &a.OwnerID, &a.Currency, &a.Type, &a.CreatedAt)

	if errors.Is(err, sql.ErrNoRows) {
		return nil, domain.ErrAccountNotFound(id)
	}

	if err != nil {
		return nil, errdefs.Internalf(err, "get account")
	}
	return &a, nil
}

func (r *AccountRepo) SeedAccounts(ctx context.Context) ([]uuid.UUID, error) {
	seeds := []domain.Account{
		{OwnerType: domain.OwnerSystem, OwnerID: "cash_at_bank", Currency: "PKR", Type: domain.AccountAsset},
		{OwnerType: domain.OwnerSystem, OwnerID: "fees", Currency: "PKR", Type: domain.AccountRevenue},
	}

	ids := make([]uuid.UUID, len(seeds))

	for i, s := range seeds {
		err := r.db.QueryRowContext(ctx,
			`INSERT INTO ledger.accounts (owner_type, owner_id, currency, type)
			 VALUES ($1, $2, $3, $4)
			 ON CONFLICT (owner_type, owner_id, currency, type) DO UPDATE SET owner_id = EXCLUDED.owner_id RETURNING id`,
			s.OwnerType, s.OwnerID, s.Currency, s.Type,
		).Scan(&ids[i])

		if err != nil {
			return nil, errdefs.Internalf(err, "seed account %s", s.OwnerID)
		}
	}
	return ids, nil
}

func (r *AccountRepo) GetByIDs(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]domain.Account, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, owner_type, owner_id, currency, type, created_at
		 FROM ledger.accounts WHERE id = ANY($1)`,
		ids)

	if err != nil {
		return nil, errdefs.Internalf(err, "get accounts")
	}
	defer rows.Close()

	accounts, err := scanAccounts(rows)
	if err != nil {
		return nil, errdefs.Internalf(err, "scan accounts")
	}
	out := make(map[uuid.UUID]domain.Account, len(ids))
	for _, a := range accounts {
		out[a.ID] = a
	}

	return out, nil
}

func (r *AccountRepo) ListByOwner(ctx context.Context, ownerType domain.OwnerType, ownerID string) ([]domain.Account, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT `+accountColumns+`
		 FROM ledger.accounts WHERE owner_type = $1 AND owner_id = $2 ORDER BY created_at`,
		ownerType, ownerID)

	if err != nil {
		return nil, errdefs.Internalf(err, "list accounts by owner")
	}
	defer rows.Close()

	out, err := scanAccounts(rows)
	if err != nil {
		return nil, errdefs.Internalf(err, "scan accounts")
	}
	return out, nil
}
