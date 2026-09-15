package handlers

import (
	"time"

	"github.com/google/uuid"
	"github.com/irfanali17899/fintech/internal/domain"
)

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
