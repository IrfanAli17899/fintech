package ledger

import (
	"context"
	"net/http"
	"time"

	"github.com/irfanali17899/fintech/internal/ledger/internal/handlers"
	"github.com/irfanali17899/fintech/internal/ledger/internal/repositories"
	"github.com/irfanali17899/fintech/internal/ledger/internal/services"
	"github.com/irfanali17899/fintech/pkg/database"
	"github.com/irfanali17899/fintech/pkg/scheduler"
)

type Module struct {
	db       *database.DB
	accounts *services.AccountService
	jobs     []*scheduler.Loop
	handlers []interface{ RegisterRoutes(*http.ServeMux) }
}

func New(db *database.DB) *Module {
	accRepo := repositories.NewAccountRepo(db)
	txnRepo := repositories.NewTransactionRepo(db)
	entryRepo := repositories.NewEntryRepo(db)
	balanceRepo := repositories.NewBalanceRepo(db)
	idemRepo := repositories.NewIdempotencyRepo(db)

	accSvc := services.NewAccountService(db, accRepo, balanceRepo)
	ledgerSvc := services.NewLedgerService(db, accRepo, txnRepo, entryRepo, balanceRepo, idemRepo)
	adminSvc := services.NewAdminService(db, balanceRepo)

	return &Module{
		db:       db,
		accounts: accSvc,
		jobs: []*scheduler.Loop{
			scheduler.New("expire-holds", time.Minute, ledgerSvc.ExpirerJob(500)),
		},
		handlers: []interface{ RegisterRoutes(*http.ServeMux) }{
			handlers.NewAccountHandler(accSvc),
			handlers.NewTransactionHandler(ledgerSvc),
			handlers.NewAdminHandler(adminSvc),
		},
	}
}

func (m *Module) RegisterRoutes(mux *http.ServeMux) {
	for _, h := range m.handlers {
		h.RegisterRoutes(mux)
	}
}

func (m *Module) Seed(ctx context.Context) error {
	return m.accounts.SeedSystem(ctx)
}

func (m *Module) Start(ctx context.Context) {
	for _, j := range m.jobs {
		j.Start(ctx)
	}
}

func (m *Module) Stop() {
	for _, j := range m.jobs {
		j.Stop()
	}
}
