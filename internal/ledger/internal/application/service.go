package application

import (
	"github.com/irfanali17899/fintech/internal/ledger/internal/persistence"
	"github.com/irfanali17899/fintech/internal/ledger/ports"
	"github.com/irfanali17899/fintech/pkg/database"
)

// Service is the ledger's application layer: every use case is a method on it,
// one file per use case. It only holds dependencies; what callers may reach is
// decided by the api interfaces in front of it, not by splitting this type.
type Service struct {
	txRunner        database.TxRunner
	accRepo         *persistence.AccountRepo
	transactionRepo *persistence.TransactionRepo
	entryRepo       *persistence.EntryRepo
	balanceRepo     *persistence.BalanceRepo
	idemRepo        *persistence.IdempotencyRepo
	rails           ports.RailRegistry
}

func NewService(
	txRunner database.TxRunner,
	accRepo *persistence.AccountRepo,
	transactionRepo *persistence.TransactionRepo,
	entryRepo *persistence.EntryRepo,
	balanceRepo *persistence.BalanceRepo,
	idemRepo *persistence.IdempotencyRepo,
	rails ports.RailRegistry,
) *Service {
	return &Service{
		txRunner:        txRunner,
		accRepo:         accRepo,
		transactionRepo: transactionRepo,
		entryRepo:       entryRepo,
		balanceRepo:     balanceRepo,
		idemRepo:        idemRepo,
		rails:           rails,
	}
}
