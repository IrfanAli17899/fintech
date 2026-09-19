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

type IdempotencyRepo struct {
	db database.DBTX
}

func NewIdempotencyRepo(db *sql.DB) *IdempotencyRepo {
	return &IdempotencyRepo{db}
}

func (r *IdempotencyRepo) WithTx(tx *sql.Tx) *IdempotencyRepo { return &IdempotencyRepo{db: tx} }

func (r *IdempotencyRepo) Insert(ctx context.Context, key, hash string, resourceType domain.ResourceType) (*domain.IdempotencyKey, error) {
	k := domain.IdempotencyKey{
		Key:          key,
		RequestHash:  hash,
		ResourceType: resourceType,
	}

	// to avoid sql collision in tx which will fail the whole tx for loser/subsequent request and idempotency
	// replay will not be able to proceed
	err := r.db.QueryRowContext(ctx,
		`
		INSERT INTO idempotency_keys (key, request_hash, resource_type)
		VALUES ($1, $2, $3)
		ON CONFLICT (key) DO NOTHING
		RETURNING created_at
		`,
		key, hash, resourceType,
	).Scan(&k.CreatedAt)

	// on conflict will returning will not return created_at and
	// thus empty results on go level can be checked as key already exists as conflict path appeared
	if errors.Is(err, sql.ErrNoRows) {
		return nil, domain.ErrIdempotencyKeyExists(key)
	}

	// if database.IsUniqueViolation(err) {
	// 	return nil, domain.ErrIdempotencyKeyExists(key)
	// }
	if err != nil {
		return nil, errdefs.Internalf(err, "insert idempotency key")
	}

	return &k, nil
}

func (r *IdempotencyRepo) Get(ctx context.Context, key string) (*domain.IdempotencyKey, error) {
	k := domain.IdempotencyKey{}

	err := r.db.QueryRowContext(ctx,
		`SELECT key, request_hash, resource_type, resource_id, created_at FROM idempotency_keys
		WHERE key=$1
		`,
		key,
	).Scan(&k.Key, &k.RequestHash, &k.ResourceType, &k.ResourceID, &k.CreatedAt)

	if errors.Is(err, sql.ErrNoRows) {
		return nil, domain.ErrIdempotencyKeyNotFound(key)
	}
	if err != nil {
		return nil, errdefs.Internalf(err, "get idempotency key")
	}

	return &k, nil
}

func (r *IdempotencyRepo) SetResource(ctx context.Context, key string, resourceID uuid.UUID) error {
	res, err := r.db.ExecContext(ctx,
		`UPDATE idempotency_keys SET resource_id = $2 WHERE key = $1`, key, resourceID)
	if err != nil {
		return errdefs.Internalf(err, "set idempotency resource")
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return errdefs.Internalf(nil, "idempotency key %s missing on set resource", key)
	}
	return nil
}
