package handlers

import (
	"time"

	"github.com/google/uuid"
	"github.com/irfanali17899/fintech/internal/ledger/internal/domain"
	"github.com/irfanali17899/fintech/internal/ledger/internal/services"
)

type entryRequest struct {
	AccountID uuid.UUID `json:"account_id"`
	Amount    int64     `json:"amount"`
	Direction string    `json:"direction"`
}

type createTransactionRequest struct {
	Kind           string         `json:"kind"`
	IdempotencyKey string         `json:"idempotency_key"`
	Entries        []entryRequest `json:"entries"`
}

func (r createTransactionRequest) toDomain() domain.Transaction {
	entries := make([]domain.Entry, len(r.Entries))
	for i, e := range r.Entries {
		entries[i] = domain.Entry{
			AccountID: e.AccountID,
			Amount:    e.Amount,
			Direction: domain.EntryDirection(e.Direction),
		}
	}
	return domain.Transaction{
		Kind:           domain.TransactionKind(r.Kind),
		IdempotencyKey: r.IdempotencyKey,
		Entries:        entries,
	}
}

type entryResponse struct {
	ID        uuid.UUID `json:"id"`
	AccountID uuid.UUID `json:"account_id"`
	Amount    int64     `json:"amount"`
	Direction string    `json:"direction"`
	CreatedAt time.Time `json:"created_at"`
}

type transactionResponse struct {
	ID         uuid.UUID       `json:"id"`
	Kind       string          `json:"kind"`
	Status     string          `json:"status"`
	ReversesID *uuid.UUID      `json:"reverses_id,omitempty"`
	CreatedAt  time.Time       `json:"created_at"`
	PostedAt   *time.Time      `json:"posted_at,omitempty"`
	Entries    []entryResponse `json:"entries"`
}


type reverseTransactionRequest struct {
	IdempotencyKey string `json:"idempotency_key"`
	Amount         int64  `json:"amount,omitempty"` // 0 or omitted = full reversal
}

func (r reverseTransactionRequest) toCommand(originalID uuid.UUID) services.ReverseCommand {
	return services.ReverseCommand{
		OriginalID:     originalID,
		Amount:         r.Amount,
		IdempotencyKey: r.IdempotencyKey,
	}
}

func toTransactionResponse(t *domain.Transaction) *transactionResponse {
	if t == nil {
		return nil
	}
	entries := make([]entryResponse, len(t.Entries))
	for i, e := range t.Entries {
		entries[i] = entryResponse{
			ID:        e.ID,
			AccountID: e.AccountID,
			Amount:    e.Amount,
			Direction: string(e.Direction),
			CreatedAt: e.CreatedAt,
		}
	}
	return &transactionResponse{
		ID:         t.ID,
		Kind:       string(t.Kind),
		Status:     string(t.Status),
		ReversesID: t.ReversesID,
		CreatedAt:  t.CreatedAt,
		PostedAt:   t.PostedAt,
		Entries:    entries,
	}
}
