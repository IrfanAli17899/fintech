package adapter

import (
	"context"
	"time"

	"github.com/irfanali17899/fintech/ledger/api"
	"github.com/irfanali17899/fintech/ledger/internal/application"
	"github.com/irfanali17899/fintech/ledger/internal/domain"
)

type Admin struct{ svc *application.Service }

var _ api.LedgerAdmin = (*Admin)(nil)

func NewAdmin(svc *application.Service) *Admin { return &Admin{svc: svc} }

func (a *Admin) RebuildBalances(ctx context.Context) error { return a.svc.RebuildBalances(ctx) }

func (a *Admin) VerifyBalances(ctx context.Context) ([]api.BalanceMismatch, error) {
	ms, err := a.svc.VerifyBalances(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]api.BalanceMismatch, len(ms))
	for i, m := range ms {
		out[i] = api.BalanceMismatch{AccountID: m.AccountID, Cached: sums(m.Cached), Live: sums(m.Live)}
	}
	return out, nil
}

func (a *Admin) Reconcile(ctx context.Context) (api.ReconciliationReport, error) {
	rep, err := a.svc.Reconcile(ctx, time.Now())
	if err != nil || rep == nil {
		return api.ReconciliationReport{}, err
	}
	out := api.ReconciliationReport{
		At: rep.At, AttemptsChecked: rep.AttemptsChecked,
		Findings:     make([]api.Finding, len(rep.Findings)),
		TrialBalance: make([]api.TrialBalanceLine, len(rep.TrialBalance)),
	}
	for i, l := range rep.TrialBalance {
		out.TrialBalance[i] = api.TrialBalanceLine{Currency: l.Currency, AccountType: string(l.Type), Sums: sums(l.Sums)}
	}
	for i, f := range rep.Findings {
		out.Findings[i] = api.Finding{
			Kind: string(f.Kind), Reason: f.Reason, Rail: f.Rail, Ref: f.Ref,
			TransactionID: f.TransactionID, Detail: f.Detail,
		}
	}
	return out, nil
}

func sums(s domain.AccountSums) api.Sums {
	return api.Sums{Debits: s.Debits, Credits: s.Credits, PendingDebits: s.PendingDebits, PendingCredits: s.PendingCredits}
}
