package handlers

import (
	"github.com/google/uuid"
	"github.com/irfanali17899/fintech/internal/domain"
)

type sumsResponse struct {
	Debits  int64 `json:"debits"`
	Credits int64 `json:"credits"`
}

type balanceMismatchResponse struct {
	AccountID uuid.UUID    `json:"account_id"`
	Cached    sumsResponse `json:"cached"`
	Live      sumsResponse `json:"live"`
}

type verifyBalancesResponse struct {
	Healthy    bool                      `json:"healthy"`
	Mismatches []balanceMismatchResponse `json:"mismatches"`
}

func toSumsResponse(s domain.AccountSums) sumsResponse {
	return sumsResponse{Debits: s.Debits, Credits: s.Credits}
}

func toVerifyBalancesResponse(ms []domain.BalanceMismatch) *verifyBalancesResponse {
	if ms == nil {
		return nil
	}
	out := make([]balanceMismatchResponse, len(ms))
	for i, m := range ms {
		out[i] = balanceMismatchResponse{
			AccountID: m.AccountID,
			Cached:    toSumsResponse(m.Cached),
			Live:      toSumsResponse(m.Live),
		}
	}
	return &verifyBalancesResponse{Healthy: len(ms) == 0, Mismatches: out}
}
