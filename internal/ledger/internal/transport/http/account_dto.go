package transporthttp

import (
	"time"

	"github.com/google/uuid"
	"github.com/irfanali17899/fintech/internal/ledger/internal/application"
	"github.com/irfanali17899/fintech/internal/ledger/internal/domain"
	"github.com/irfanali17899/fintech/pkg/errdefs"
)

type listAccountsQuery struct {
	OwnerType string `query:"owner_type"`
	OwnerID   string `query:"owner_id"`
}

type historyQuery struct {
	Limit  int   `query:"limit"`
	Before int64 `query:"before"`
}

type accountEntryResponse struct {
	Seq           int64      `json:"seq"`
	Amount        int64      `json:"amount"`
	Direction     string     `json:"direction"`
	CreatedAt     time.Time  `json:"created_at"`
	PostedAt      *time.Time `json:"posted_at,omitempty"`
	TransactionID uuid.UUID  `json:"transaction_id"`
	Kind          string     `json:"kind"`
	Status        string     `json:"status"`
	Rail          string     `json:"rail"`
	ParentID      *uuid.UUID `json:"parent_id,omitempty"`
}

type historyResponse struct {
	Entries    []accountEntryResponse `json:"entries"`
	NextBefore int64                  `json:"next_before"`
}

func (q listAccountsQuery) Validate() error {
	if q.OwnerType == "" || q.OwnerID == "" {
		return errdefs.Validationf("MISSING_QUERY_PARAM", "owner_type and owner_id are required")
	}
	return nil
}

type createAccountRequest struct {
	OwnerType string `json:"owner_type"`
	OwnerID   string `json:"owner_id"`
	Currency  string `json:"currency"`
	Type      string `json:"type"`
}

func (r createAccountRequest) toDomain() domain.Account {
	return domain.Account{
		OwnerType: domain.OwnerType(r.OwnerType),
		OwnerID:   r.OwnerID,
		Currency:  r.Currency,
		Type:      domain.AccountType(r.Type),
	}
}

type accountResponse struct {
	ID        uuid.UUID `json:"id"`
	OwnerType string    `json:"owner_type"`
	OwnerID   string    `json:"owner_id"`
	Currency  string    `json:"currency"`
	Type      string    `json:"type"`
	CreatedAt time.Time `json:"created_at"`
}

type balanceResponse struct {
	AccountID  uuid.UUID `json:"account_id"`
	Currency   string    `json:"currency"`
	Posted     int64     `json:"posted"`
	PendingIn  int64     `json:"pending_in"`
	PendingOut int64     `json:"pending_out"`
	Available  int64     `json:"available"`
	AsOf       time.Time `json:"as_of"`
}

func toAccountResponse(a *domain.Account) *accountResponse {
	if a == nil {
		return nil
	}
	return &accountResponse{
		ID:        a.ID,
		OwnerType: string(a.OwnerType),
		OwnerID:   a.OwnerID,
		Currency:  a.Currency,
		Type:      string(a.Type),
		CreatedAt: a.CreatedAt,
	}
}

func toAccountListResponse(accs []domain.Account) []accountResponse {
	out := make([]accountResponse, len(accs))
	for i := range accs {
		out[i] = *toAccountResponse(&accs[i])
	}
	return out
}

func toBalanceResponse(b *domain.Balance) *balanceResponse {
	if b == nil {
		return nil
	}
	return &balanceResponse{
		AccountID:  b.AccountID,
		Currency:   b.Currency,
		Posted:     b.Posted,
		PendingIn:  b.PendingIn,
		PendingOut: b.PendingOut,
		Available:  b.Available,
		AsOf:       b.AsOf,
	}
}

func toHistoryResponse(h *application.History) *historyResponse {
	if h == nil {
		return nil
	}
	out := make([]accountEntryResponse, len(h.Entries))
	for i, e := range h.Entries {
		out[i] = accountEntryResponse{
			Seq: e.Seq, Amount: e.Amount, Direction: string(e.Direction), CreatedAt: e.CreatedAt, PostedAt: e.PostedAt,
			TransactionID: e.TransactionID, Kind: string(e.Kind), Status: string(e.Status),
			Rail: e.Rail, ParentID: e.ParentID,
		}
	}
	return &historyResponse{Entries: out, NextBefore: h.NextBefore}
}
