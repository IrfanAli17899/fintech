package adapters

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/irfanali17899/fintech/internal/ledger/ports"
	"github.com/irfanali17899/fintech/internal/rails/internal/domain"
	"github.com/irfanali17899/fintech/internal/rails/internal/persistence"
	"github.com/irfanali17899/fintech/pkg/database"
)

type Registry struct {
	rails    map[string]ports.Rail
	attempts *persistence.AttemptRepo
	outcomes *persistence.OutcomeRepo
	tx       database.TxRunner
}

func NewRegistry(tx database.TxRunner, attempts *persistence.AttemptRepo, outcomes *persistence.OutcomeRepo, rails ...ports.Rail) *Registry {
	r := &Registry{tx: tx, attempts: attempts, outcomes: outcomes, rails: make(map[string]ports.Rail, len(rails))}
	for _, x := range rails {
		r.rails[x.Name()] = x
	}
	return r
}

func (r *Registry) Get(name string) (ports.Rail, bool) {
	x, ok := r.rails[name]
	return x, ok
}

func (r *Registry) ClaimDue(ctx context.Context, now time.Time, lease time.Duration, limit int) ([]ports.Outcome, error) {
	var claimed []domain.Outcome
	err := r.tx.InTx(ctx, func(tx *sql.Tx) error {
		var err error
		claimed, err = r.outcomes.WithTx(tx).ClaimDue(ctx, now, lease, limit)
		return err
	})
	if err != nil {
		return nil, err
	}

	refs := make([]string, len(claimed))
	for i, o := range claimed {
		refs[i] = o.Ref
	}
	attempts, err := r.attempts.ByRefs(ctx, refs)
	if err != nil {
		return nil, err
	}

	out := make([]ports.Outcome, 0, len(claimed))
	for _, o := range claimed {
		a := attempts[o.Ref] // outcomes.ref is a foreign key, so it is present
		d, ok := decisions[o.State]
		if !ok {
			return nil, fmt.Errorf("outcome %s/%s in unexpected state %s", o.Ref, o.Kind, o.State)
		}
		out = append(out, ports.Outcome{
			Rail:          a.Rail,
			Ref:           o.Ref,
			Kind:          ports.OutcomeKind(o.Kind),
			Key:           o.Key,
			TransactionID: a.TransactionID,
			Amount:        o.Amount,
			Decision:      d,
			Reason:        deref(o.Reason),
		})
	}
	return out, nil
}

var decisions = map[domain.AttemptState]ports.Decision{
	domain.AttemptInitiated: ports.DecisionUndecided,
	domain.AttemptSettled:   ports.DecisionSettled,
	domain.AttemptBounced:   ports.DecisionBounced,
	domain.AttemptCancelled: ports.DecisionCancelled,
}

func (r *Registry) MarkApplied(ctx context.Context, o ports.Outcome) error {
	_, err := r.outcomes.MarkApplied(ctx, o.Ref, domain.OutcomeKind(o.Kind), o.Key)
	return err
}

func (r *Registry) RecordFailure(ctx context.Context, o ports.Outcome, cause error) error {
	return r.outcomes.RecordFailure(ctx, o.Ref, domain.OutcomeKind(o.Kind), o.Key, cause.Error())
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
