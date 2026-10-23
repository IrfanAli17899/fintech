package transporthttp

import (
	"time"

	"github.com/google/uuid"
	"github.com/irfanali17899/fintech/ledger/api"
	"github.com/irfanali17899/fintech/platform/errdefs"
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

func (r createAccountRequest) toCommand() api.CreateAccount {
	return api.CreateAccount{OwnerType: r.OwnerType, OwnerID: r.OwnerID, Currency: r.Currency, Type: r.Type}
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

func toAccountResponse(a api.Account) accountResponse {
	return accountResponse{
		ID: a.ID, OwnerType: a.OwnerType, OwnerID: a.OwnerID,
		Currency: a.Currency, Type: a.Type, CreatedAt: a.CreatedAt,
	}
}

func toAccountListResponse(accs []api.Account) []accountResponse {
	out := make([]accountResponse, len(accs))
	for i, a := range accs {
		out[i] = toAccountResponse(a)
	}
	return out
}

func toBalanceResponse(b api.Balance) balanceResponse {
	return balanceResponse{
		AccountID: b.AccountID, Currency: b.Currency, Posted: b.Posted,
		PendingIn: b.PendingIn, PendingOut: b.PendingOut, Available: b.Available, AsOf: b.AsOf,
	}
}

func toHistoryResponse(h api.HistoryPage) historyResponse {
	out := make([]accountEntryResponse, len(h.Entries))
	for i, e := range h.Entries {
		out[i] = accountEntryResponse{
			Seq: e.Seq, Amount: e.Amount, Direction: e.Direction, CreatedAt: e.CreatedAt, PostedAt: e.PostedAt,
			TransactionID: e.TransactionID, Kind: e.Kind, Status: e.Status, Rail: e.Rail, ParentID: e.ParentID,
		}
	}
	return historyResponse{Entries: out, NextBefore: h.NextBefore}
}
