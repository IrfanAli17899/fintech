package transporthttp

import (
	"time"

	"github.com/google/uuid"
	"github.com/irfanali17899/fintech/internal/ledger/internal/domain"
)

type sumsResponse struct {
	Debits         int64 `json:"debits"`
	Credits        int64 `json:"credits"`
	PendingDebits  int64 `json:"pending_debits"`
	PendingCredits int64 `json:"pending_credits"`
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

type findingResponse struct {
	Kind          string    `json:"kind"`
	Reason        string    `json:"reason,omitempty"`
	Rail          string    `json:"rail,omitempty"`
	Ref           string    `json:"ref,omitempty"`
	TransactionID uuid.UUID `json:"transaction_id"`
	Detail        string    `json:"detail"`
}

type reconciliationResponse struct {
	At              time.Time         `json:"at"`
	Clean           bool              `json:"clean"`
	AttemptsChecked int               `json:"attempts_checked"`
	Counts          map[string]int    `json:"counts"`
	Findings        []findingResponse `json:"findings"`
}

func toReconciliationResponse(r *domain.ReconciliationReport) *reconciliationResponse {
	if r == nil {
		return nil
	}
	out := &reconciliationResponse{
		At: r.At, Clean: r.Clean(), AttemptsChecked: r.AttemptsChecked,
		Counts: map[string]int{}, Findings: make([]findingResponse, len(r.Findings)),
	}
	for i, f := range r.Findings {
		key := string(f.Kind)
		if f.Reason != "" {
			key += ":" + f.Reason
		}
		out.Counts[key]++
		out.Findings[i] = findingResponse{
			Kind: string(f.Kind), Reason: f.Reason, Rail: f.Rail, Ref: f.Ref,
			TransactionID: f.TransactionID, Detail: f.Detail,
		}
	}
	return out
}
