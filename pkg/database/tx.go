package database

import (
	"context"
	"database/sql"

	"github.com/irfanali17899/fintech/pkg/errdefs"
)

type TxRunner interface {
	InTx(ctx context.Context, fn func(tx *sql.Tx) error) error
}

// InTx runs fn inside a transaction. Commit on nil, rollback on error or panic.
func InTx(ctx context.Context, db *sql.DB, fn func(tx *sql.Tx) error) (err error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return errdefs.Internalf(err, "begin tx")
	}

	defer func() {
		if p := recover(); p != nil {
			_ = tx.Rollback()
			panic(p)
		}
		if err != nil {
			_ = tx.Rollback()
			return
		}
		err = tx.Commit()
	}()

	return fn(tx)
}

func (d *DB) InTx(ctx context.Context, fn func(tx *sql.Tx) error) error {
	return InTx(ctx, d.DB, fn)
}
