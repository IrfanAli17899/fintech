package repositories

import (
	"context"
	"database/sql"
	"errors"

	"github.com/google/uuid"
	"github.com/irfanali17899/fintech/internal/domain"
	"github.com/irfanali17899/fintech/pkg/database"
	"github.com/irfanali17899/fintech/pkg/errdefs"
)

type AccountRepo struct {
	db database.DBTX
}

func NewAccountRepo(db *sql.DB) *AccountRepo {
	return &AccountRepo{db}
}

func (r *AccountRepo) WithTx(tx *sql.Tx) *AccountRepo { return &AccountRepo{db: tx} }

func (r *AccountRepo) Create(ctx context.Context, a domain.Account) (*domain.Account, error) {
	err := r.db.QueryRowContext(ctx,
		`INSERT INTO accounts (owner_type, owner_id, currency, type) 
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
		`SELECT id, owner_id, owner_type, currency, type, created_at from accounts where id=$1`,
		id).Scan(&a.ID, &a.OwnerID, &a.OwnerType, &a.Currency, &a.Type, &a.CreatedAt)

	if errors.Is(err, sql.ErrNoRows) {
		return nil, domain.ErrAccountNotFound(id)
	}

	if err != nil {
		return nil, errdefs.Internalf(err, "get account")
	}
	return &a, nil
}

func (r *AccountRepo) SeedAccounts(ctx context.Context) error {
	seeds := []domain.Account{
		{OwnerType: domain.OwnerSystem, OwnerID: "cash_at_bank", Currency: "PKR", Type: domain.AccountAsset},
		{OwnerType: domain.OwnerSystem, OwnerID: "fees", Currency: "PKR", Type: domain.AccountRevenue},
	}

	for _, s := range seeds {
		_, err := r.db.ExecContext(ctx,
			`INSERT INTO accounts (owner_type, owner_id, currency, type)
			 VALUES ($1, $2, $3, $4)
			 ON CONFLICT (owner_type, owner_id, currency, type) DO NOTHING`,
			s.OwnerType, s.OwnerID, s.Currency, s.Type,
		)
		if err != nil {
			return errdefs.Internalf(err, "seed account %s", s.OwnerID)
		}
	}
	return nil
}

func (r *AccountRepo) GetByIDs(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]domain.Account, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, owner_type, owner_id, currency, type, created_at
		 FROM accounts WHERE id = ANY($1)`,
		ids)

	if err != nil {
		return nil, errdefs.Internalf(err, "get accounts")
	}
	defer rows.Close()

	out := make(map[uuid.UUID]domain.Account, len(ids))

	for rows.Next() {
		var a domain.Account
		if err := rows.Scan(&a.ID, &a.OwnerType, &a.OwnerID, &a.Currency, &a.Type, &a.CreatedAt); err != nil {
			return nil, errdefs.Internalf(err, "scan account")
		}
		out[a.ID] = a
	}

	if err := rows.Err(); err != nil {
		return nil, errdefs.Internalf(err, "scan accounts")
	}
	return out, nil
}
