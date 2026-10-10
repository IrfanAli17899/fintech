package adapters

import (
	"context"
	"database/sql"
	"time"

	"github.com/irfanali17899/fintech/internal/ledger/ports"
	"github.com/irfanali17899/fintech/internal/rails/internal/persistence"
	"github.com/irfanali17899/fintech/pkg/database"
)

type Registry struct {
	rails    map[string]ports.Rail
	attempts *persistence.AttemptRepo
	tx       database.TxRunner
}

func NewRegistry(tx database.TxRunner, attempts *persistence.AttemptRepo, rails ...ports.Rail) *Registry {
	r := &Registry{tx: tx, attempts: attempts, rails: make(map[string]ports.Rail, len(rails))}
	for _, x := range rails {
		r.rails[x.Name()] = x
	}
	return r
}

func (r *Registry) Get(name string) (ports.Rail, bool) {
	x, ok := r.rails[name]
	return x, ok
}

func (r *Registry) Due(ctx context.Context, now time.Time, limit int) ([]ports.DueAttempt, error) {
	var out []ports.DueAttempt
	err := r.tx.InTx(ctx, func(tx *sql.Tx) error {
		due, err := r.attempts.WithTx(tx).Due(ctx, now, limit)
		if err != nil {
			return err
		}
		for _, a := range due {
			out = append(out, ports.DueAttempt{Rail: a.Rail, Ref: a.Ref, TransactionID: a.TransactionID})
		}
		return nil
	})
	return out, err
}
