// Package adapter implements the ledger's api over its application layer. It
// is the only package that sees both sides, so it is the only place domain
// types and api types are mapped.
package adapter

import (
	"context"

	"github.com/google/uuid"
	"github.com/irfanali17899/fintech/internal/ledger/api"
	"github.com/irfanali17899/fintech/internal/ledger/internal/application"
	"github.com/irfanali17899/fintech/internal/ledger/internal/domain"
)

type Ledger struct{ svc *application.Service }

var _ api.Ledger = (*Ledger)(nil)

func NewLedger(svc *application.Service) *Ledger { return &Ledger{svc: svc} }

func (l *Ledger) CreateAccount(ctx context.Context, cmd api.CreateAccount) (api.Account, error) {
	a, err := l.svc.CreateAccount(ctx, domain.Account{
		OwnerType: domain.OwnerType(cmd.OwnerType),
		OwnerID:   cmd.OwnerID,
		Currency:  cmd.Currency,
		Type:      domain.AccountType(cmd.Type),
	})
	return account(a), err
}

func (l *Ledger) GetAccount(ctx context.Context, id uuid.UUID) (api.Account, error) {
	a, err := l.svc.GetAccount(ctx, id)
	return account(a), err
}

func (l *Ledger) ListAccounts(ctx context.Context, q api.AccountQuery) ([]api.Account, error) {
	accs, err := l.svc.ListByOwner(ctx, domain.OwnerType(q.OwnerType), q.OwnerID)
	if err != nil {
		return nil, err
	}
	out := make([]api.Account, len(accs))
	for i := range accs {
		out[i] = account(&accs[i])
	}
	return out, nil
}

func (l *Ledger) Balance(ctx context.Context, accountID uuid.UUID) (api.Balance, error) {
	b, err := l.svc.GetAccountBalance(ctx, accountID)
	if err != nil || b == nil {
		return api.Balance{}, err
	}
	return api.Balance{
		AccountID: b.AccountID, Currency: b.Currency, Posted: b.Posted,
		PendingIn: b.PendingIn, PendingOut: b.PendingOut, Available: b.Available, AsOf: b.AsOf,
	}, nil
}

func (l *Ledger) History(ctx context.Context, accountID uuid.UUID, p api.Page) (api.HistoryPage, error) {
	h, err := l.svc.History(ctx, accountID, p.Before, p.Limit)
	if err != nil || h == nil {
		return api.HistoryPage{}, err
	}
	out := api.HistoryPage{Entries: make([]api.AccountEntry, len(h.Entries)), NextBefore: h.NextBefore}
	for i, e := range h.Entries {
		out.Entries[i] = api.AccountEntry{
			Seq: e.Seq, Amount: e.Amount, Direction: string(e.Direction),
			CreatedAt: e.CreatedAt, PostedAt: e.PostedAt, TransactionID: e.TransactionID,
			Kind: string(e.Kind), Status: string(e.Status), Rail: e.Rail, ParentID: e.ParentID,
		}
	}
	return out, nil
}

func (l *Ledger) Post(ctx context.Context, cmd api.PostTransaction) (api.Result, error) {
	entries := make([]domain.Entry, len(cmd.Entries))
	for i, e := range cmd.Entries {
		entries[i] = domain.Entry{AccountID: e.AccountID, Amount: e.Amount, Direction: domain.EntryDirection(e.Direction)}
	}
	return result(l.svc.Post(ctx, domain.Transaction{
		IdempotencyKey: cmd.IdempotencyKey,
		Kind:           domain.TransactionKind(cmd.Kind),
		Status:         domain.TransactionStatus(cmd.Status),
		Rail:           cmd.Rail,
		ExpiresAt:      cmd.ExpiresAt,
		Entries:        entries,
	}))
}

func (l *Ledger) Reverse(ctx context.Context, cmd api.Reverse) (api.Result, error) {
	return result(l.svc.Reverse(ctx, application.ReverseCommand{
		Kind: cmd.Kind, OriginalID: cmd.TransactionID, Amount: cmd.Amount, IdempotencyKey: cmd.IdempotencyKey,
	}))
}

func (l *Ledger) Capture(ctx context.Context, cmd api.Capture) (api.Result, error) {
	return result(l.svc.Capture(ctx, application.CaptureCommand{
		HoldID: cmd.HoldID, Amount: cmd.Amount, Final: cmd.Final, IdempotencyKey: cmd.IdempotencyKey,
	}))
}

func (l *Ledger) Void(ctx context.Context, cmd api.Void) (api.Result, error) {
	return result(l.svc.Void(ctx, application.VoidCommand{
		HoldID: cmd.HoldID, Reason: cmd.Reason, IdempotencyKey: cmd.IdempotencyKey,
	}))
}

func (l *Ledger) GetTransaction(ctx context.Context, id uuid.UUID) (api.Transaction, error) {
	t, err := l.svc.GetTransaction(ctx, id)
	return transaction(t), err
}
