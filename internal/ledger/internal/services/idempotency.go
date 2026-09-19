package services

import (
	"context"

	"github.com/google/uuid"
	"github.com/irfanali17899/fintech/internal/ledger/internal/domain"
	"github.com/irfanali17899/fintech/internal/ledger/internal/repositories"
	"github.com/irfanali17899/fintech/pkg/errdefs"
)

// replay describes an idempotency key that has already completed.
type replay struct {
	ResourceID uuid.UUID
}

// claimKey attempts to claim key for this request. It returns (nil, nil) when
// the key is new and the caller should proceed, (*replay, nil) when the same
// request was already completed, and an error when the key was reused with a
// different payload. Must be called on a tx-bound repo; the caller links the
// key to its result with SetResource before commit.
func claimKey(ctx context.Context, keys *repositories.IdempotencyRepo, key, hash string, rt domain.ResourceType) (*replay, error) {
	_, err := keys.Insert(ctx, key, hash, rt)
	if err == nil {
		return nil, nil
	}
	if !errdefs.Is(err, domain.CodeIdempotencyKeyExists) {
		return nil, err
	}

	existing, err := keys.Get(ctx, key)
	if err != nil {
		return nil, err
	}
	if existing.RequestHash != hash {
		return nil, domain.ErrIdempotencyKeyReused(key)
	}
	if existing.ResourceID == nil {
		return nil, errdefs.Internalf(nil, "idempotency key %s committed without resource", key)
	}
	return &replay{ResourceID: *existing.ResourceID}, nil
}
