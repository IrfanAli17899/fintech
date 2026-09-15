package handlers

import (
	"time"

	"github.com/google/uuid"
	"github.com/irfanali17899/fintech/internal/domain"
	"github.com/irfanali17899/fintech/pkg/errdefs"
)

type listAccountsQuery struct {
	OwnerType string `query:"owner_type"`
	OwnerID   string `query:"owner_id"`
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
	AccountID uuid.UUID `json:"account_id"`
	Currency  string    `json:"currency"`
	Amount    int64     `json:"amount"`
	AsOf      time.Time `json:"as_of"`
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
		AccountID: b.AccountID,
		Currency:  b.Currency,
		Amount:    b.Amount,
		AsOf:      b.AsOf,
	}
}
