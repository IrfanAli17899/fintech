package adapter

import (
	"github.com/irfanali17899/fintech/internal/ledger/api"
	"github.com/irfanali17899/fintech/internal/ledger/internal/domain"
)

func account(a *domain.Account) api.Account {
	if a == nil {
		return api.Account{}
	}
	return api.Account{
		ID: a.ID, OwnerType: string(a.OwnerType), OwnerID: a.OwnerID,
		Currency: a.Currency, Type: string(a.Type), CreatedAt: a.CreatedAt,
	}
}

func result(t *domain.Transaction, replayed bool, err error) (api.Result, error) {
	return api.Result{Transaction: transaction(t), Replayed: replayed}, err
}

func transaction(t *domain.Transaction) api.Transaction {
	if t == nil {
		return api.Transaction{}
	}
	out := api.Transaction{
		ID: t.ID, Kind: string(t.Kind), ParentID: t.ParentID, Status: string(t.Status),
		Rail: t.Rail, RailRef: t.RailRef, ExpiresAt: t.ExpiresAt,
		CreatedAt: t.CreatedAt, PostedAt: t.PostedAt,
		Entries: make([]api.Entry, len(t.Entries)),
		Summary: api.Summary{
			Total: t.Summary.Total, Captured: t.Summary.Captured, Reversed: t.Summary.Reversed,
			Remaining: t.Summary.Remaining, Settlement: t.Summary.Settlement,
		},
	}
	if t.Relation != nil {
		out.Relation = string(*t.Relation)
	}
	if t.StatusReason != nil {
		out.StatusReason = *t.StatusReason
	}
	for i, e := range t.Entries {
		out.Entries[i] = api.Entry{
			ID: e.ID, AccountID: e.AccountID, Amount: e.Amount,
			Direction: string(e.Direction), CreatedAt: e.CreatedAt,
		}
	}
	return out
}
