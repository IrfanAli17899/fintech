package transporthttp

import (
	"time"

	"github.com/google/uuid"
	"github.com/irfanali17899/fintech/ledger/api"
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

func (r createTransactionRequest) toCommand() api.PostTransaction {
	entries := make([]api.EntryInput, len(r.Entries))
	for i, e := range r.Entries {
		entries[i] = api.EntryInput{AccountID: e.AccountID, Amount: e.Amount, Direction: e.Direction}
	}
	return api.PostTransaction{
		IdempotencyKey: r.IdempotencyKey,
		Kind:           r.Kind,
		Status:         r.Status,
		Rail:           r.Rail,
		ExpiresAt:      r.ExpiresAt,
		Entries:        entries,
	}
}

type reverseTransactionRequest struct {
	Kind           string `json:"kind"`
	Amount         int64  `json:"amount,omitempty"` // 0 or omitted = the rest
	IdempotencyKey string `json:"idempotency_key"`
}

func (r reverseTransactionRequest) toCommand(originalID uuid.UUID) api.Reverse {
	return api.Reverse{IdempotencyKey: r.IdempotencyKey, TransactionID: originalID, Kind: r.Kind, Amount: r.Amount}
}

type captureTransactionRequest struct {
	Amount         int64  `json:"amount,omitempty"` // 0 or omitted = everything still held
	Final          bool   `json:"final,omitempty"`  // release the rest and close the hold
	IdempotencyKey string `json:"idempotency_key"`
}

func (r captureTransactionRequest) toCommand(holdID uuid.UUID) api.Capture {
	return api.Capture{IdempotencyKey: r.IdempotencyKey, HoldID: holdID, Amount: r.Amount, Final: r.Final}
}

type voidTransactionRequest struct {
	Reason         string `json:"reason,omitempty"`
	IdempotencyKey string `json:"idempotency_key"`
}

func (r voidTransactionRequest) toCommand(holdID uuid.UUID) api.Void {
	return api.Void{IdempotencyKey: r.IdempotencyKey, HoldID: holdID, Reason: r.Reason}
}

// ---- responses ----

type entryResponse struct {
	ID        uuid.UUID `json:"id"`
	AccountID uuid.UUID `json:"account_id"`
	Amount    int64     `json:"amount"`
	Direction string    `json:"direction"`
	CreatedAt time.Time `json:"created_at"`
}

type summaryResponse struct {
	Total      int64  `json:"total"`
	Captured   int64  `json:"captured"`
	Reversed   int64  `json:"reversed"`
	Remaining  int64  `json:"remaining"`
	Settlement string `json:"settlement"`
}

type transactionResponse struct {
	ID           uuid.UUID       `json:"id"`
	Kind         string          `json:"kind"`
	Relation     string          `json:"relation,omitempty"`
	ParentID     *uuid.UUID      `json:"parent_id,omitempty"`
	Status       string          `json:"status"`
	StatusReason string          `json:"status_reason,omitempty"`
	Rail         string          `json:"rail"`
	RailRef      string          `json:"rail_ref,omitempty"`
	ExpiresAt    *time.Time      `json:"expires_at,omitempty"`
	CreatedAt    time.Time       `json:"created_at"`
	PostedAt     *time.Time      `json:"posted_at,omitempty"`
	Entries      []entryResponse `json:"entries"`
	Summary      summaryResponse `json:"summary"`
}

func toTransactionResponse(t api.Transaction) transactionResponse {
	entries := make([]entryResponse, len(t.Entries))
	for i, e := range t.Entries {
		entries[i] = entryResponse{ID: e.ID, AccountID: e.AccountID, Amount: e.Amount, Direction: e.Direction, CreatedAt: e.CreatedAt}
	}
	return transactionResponse{
		ID: t.ID, Kind: t.Kind, Relation: t.Relation, ParentID: t.ParentID,
		Status: t.Status, StatusReason: t.StatusReason, Rail: t.Rail, RailRef: t.RailRef,
		ExpiresAt: t.ExpiresAt, CreatedAt: t.CreatedAt, PostedAt: t.PostedAt, Entries: entries,
		Summary: summaryResponse{
			Total: t.Summary.Total, Captured: t.Summary.Captured, Reversed: t.Summary.Reversed,
			Remaining: t.Summary.Remaining, Settlement: t.Summary.Settlement,
		},
	}
}
