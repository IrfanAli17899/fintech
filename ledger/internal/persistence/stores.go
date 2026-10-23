package persistence

import (
	"context"
	"database/sql"

	"github.com/irfanali17899/fintech/platform/database"
)

// Stores is every ledger repo, bound to one connection pool or one transaction.
// It lives here rather than in application/port because the Postgres unit of
// work builds it, and port already imports this package for the repo types.
type Stores struct {
	Accounts     *AccountRepo
	Transactions *TransactionRepo
	Entries      *EntryRepo
	Balances     *BalanceRepo
	Keys         *IdempotencyRepo
}

func NewStores(db database.DBTX) Stores {
	return Stores{
		Accounts:     NewAccountRepo(db),
		Transactions: NewTransactionRepo(db),
		Entries:      NewEntryRepo(db),
		Balances:     NewBalanceRepo(db),
		Keys:         NewIdempotencyRepo(db),
	}
}

func (s Stores) WithTx(tx *sql.Tx) Stores {
	return Stores{
		Accounts:     s.Accounts.WithTx(tx),
		Transactions: s.Transactions.WithTx(tx),
		Entries:      s.Entries.WithTx(tx),
		Balances:     s.Balances.WithTx(tx),
		Keys:         s.Keys.WithTx(tx),
	}
}

// UnitOfWork is the Postgres application/port.UnitOfWork: one database
// transaction, every store bound to it.
type UnitOfWork struct {
	tx     database.TxRunner
	stores Stores
}

func NewUnitOfWork(tx database.TxRunner, stores Stores) *UnitOfWork {
	return &UnitOfWork{tx: tx, stores: stores}
}

func (u *UnitOfWork) Do(ctx context.Context, fn func(Stores) error) error {
	return u.tx.InTx(ctx, func(tx *sql.Tx) error { return fn(u.stores.WithTx(tx)) })
}
