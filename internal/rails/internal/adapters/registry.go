package adapters

import (
	"context"
	"database/sql"
	"time"

	"github.com/irfanali17899/fintech/internal/ledger/api"
	"github.com/irfanali17899/fintech/internal/rails/internal/repositories"
	"github.com/irfanali17899/fintech/pkg/database"
)

type Registry struct {
	rails    map[string]api.Rail
	attempts *repositories.AttemptRepo
	tx       database.TxRunner
}

func NewRegistry(tx database.TxRunner, attempts *repositories.AttemptRepo, rails ...api.Rail) *Registry {
	r := &Registry{tx: tx, attempts: attempts, rails: make(map[string]api.Rail, len(rails))}
	for _, x := range rails {
		r.rails[x.Name()] = x
	}
	return r
}

func (r *Registry) Get(name string) (api.Rail, bool) {
	x, ok := r.rails[name]
	return x, ok
}

func (r *Registry) Due(ctx context.Context, now time.Time, limit int) ([]api.DueAttempt, error) {
	var out []api.DueAttempt
	err := r.tx.InTx(ctx, func(tx *sql.Tx) error {
		due, err := r.attempts.WithTx(tx).Due(ctx, now, limit)
		if err != nil {
			return err
		}
		for _, a := range due {
			out = append(out, api.DueAttempt{Rail: a.Rail, Ref: a.Ref, TransactionID: a.TransactionID})
		}
		return nil
	})
	return out, err
}
