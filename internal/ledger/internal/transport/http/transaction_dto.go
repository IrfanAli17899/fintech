package transporthttp

import (
	"time"

	"github.com/google/uuid"
	"github.com/irfanali17899/fintech/internal/ledger/internal/application"
	"github.com/irfanali17899/fintech/internal/ledger/internal/domain"
)

// ---- requests ----

type entryRequest struct {
	AccountID uuid.UUID `json:"account_id"`
	Amount    int64     `json:"amount"`
	Direction string    `json:"direction"`
}

type createTransactionRequest struct {
	Kind           string         `json:"kind"`
	Status         string         `json:"status,omitempty"`
	Rail           string         `json:"rail,omitempty"`
	ExpiresAt      *time.Time     `json:"expires_at,omitempty"`
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
		Status:         domain.TransactionStatus(r.Status),
		Rail:           r.Rail,
		ExpiresAt:      r.ExpiresAt,
		IdempotencyKey: r.IdempotencyKey,
		Entries:        entries,
	}
}

type reverseTransactionRequest struct {
	Kind           string `json:"kind"`
	Amount         int64  `json:"amount,omitempty"` // 0 or omitted = the rest
	IdempotencyKey string `json:"idempotency_key"`
}

func (r reverseTransactionRequest) toCommand(originalID uuid.UUID) application.ReverseCommand {
	return application.ReverseCommand{
		Kind:           r.Kind,
		OriginalID:     originalID,
		Amount:         r.Amount,
		IdempotencyKey: r.IdempotencyKey,
	}
}

type captureTransactionRequest struct {
	Amount         int64  `json:"amount,omitempty"` // 0 or omitted = everything still held
	IdempotencyKey string `json:"idempotency_key"`
}

func (r captureTransactionRequest) toCommand(holdID uuid.UUID) application.CaptureCommand {
	return application.CaptureCommand{HoldID: holdID, Amount: r.Amount, IdempotencyKey: r.IdempotencyKey}
}

type voidTransactionRequest struct {
	Reason         string `json:"reason,omitempty"`
	IdempotencyKey string `json:"idempotency_key"`
}

func (r voidTransactionRequest) toCommand(holdID uuid.UUID) application.VoidCommand {
	return application.VoidCommand{HoldID: holdID, Reason: r.Reason, IdempotencyKey: r.IdempotencyKey}
}

// ---- responses ----

type entryResponse struct {
	ID        uuid.UUID `json:"id"`
	AccountID uuid.UUID `json:"account_id"`
	Amount    int64     `json:"amount"`
	Direction string    `json:"direction"`
	CreatedAt time.Time `json:"created_at"`
}

type transactionResponse struct {
	ID           uuid.UUID       `json:"id"`
	Kind         string          `json:"kind"`
	Relation     *string         `json:"relation,omitempty"`
	ParentID     *uuid.UUID      `json:"parent_id,omitempty"`
	Status       string          `json:"status"`
	StatusReason *string         `json:"status_reason,omitempty"`
	Rail         string          `json:"rail"`
	RailRef      string          `json:"rail_ref,omitempty"`
	ExpiresAt    *time.Time      `json:"expires_at,omitempty"`
	CreatedAt    time.Time       `json:"created_at"`
	PostedAt     *time.Time      `json:"posted_at,omitempty"`
	Entries      []entryResponse `json:"entries"`
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
		ID:           t.ID,
		Kind:         string(t.Kind),
		Relation:     relationStr(t.Relation),
		ParentID:     t.ParentID,
		Status:       string(t.Status),
		StatusReason: t.StatusReason,
		Rail:         t.Rail,
		RailRef:      t.RailRef,
		ExpiresAt:    t.ExpiresAt,
		CreatedAt:    t.CreatedAt,
		PostedAt:     t.PostedAt,
		Entries:      entries,
	}
}

func relationStr(r *domain.TransactionRelation) *string {
	if r == nil {
		return nil
	}
	s := string(*r)
	return &s
}
