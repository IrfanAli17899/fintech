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

type TransactionRepo struct {
	db database.DBTX
}

func NewTransactionRepo(db *sql.DB) *TransactionRepo {
	return &TransactionRepo{db}
}

func (r *TransactionRepo) WithTx(tx *sql.Tx) *TransactionRepo { return &TransactionRepo{db: tx} }

func (r *TransactionRepo) Create(ctx context.Context, t domain.Transaction) (*domain.Transaction, error) {
	err := r.db.QueryRowContext(ctx,
		`INSERT INTO transactions (kind, idempotency_key, reverses_id) 
		VALUES ($1,$2,$3) RETURNING id, status, created_at, posted_at`,
		t.Kind, t.IdempotencyKey, t.ReversesID,
	).Scan(&t.ID, &t.Status, &t.CreatedAt, &t.PostedAt)

	if database.IsUniqueViolation(err) {
		return nil, domain.ErrTransactionExists(t.IdempotencyKey)
	}

	if err != nil {
		return nil, errdefs.Internalf(err, "create transaction")
	}

	return &t, nil
}

func (r *TransactionRepo) GetByID(ctx context.Context, id uuid.UUID) (*domain.Transaction, error) {
	var t domain.Transaction
	err := r.db.QueryRowContext(ctx,
		`SELECT id, kind, status, idempotency_key, reverses_id, created_at, posted_at from transactions where id=$1`,
		id).Scan(&t.ID, &t.Kind, &t.Status, &t.IdempotencyKey, &t.ReversesID, &t.CreatedAt, &t.PostedAt)

	if errors.Is(err, sql.ErrNoRows) {
		return nil, domain.ErrTransactionNotFound(id)
	}

	if err != nil {
		return nil, errdefs.Internalf(err, "get transaction")
	}
	return &t, nil
}
