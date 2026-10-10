package ledger

import (
	"context"
	"net/http"
	"time"

	"github.com/irfanali17899/fintech/internal/ledger/internal/application"
	"github.com/irfanali17899/fintech/internal/ledger/internal/persistence"
	"github.com/irfanali17899/fintech/internal/ledger/internal/transport/http"
	"github.com/irfanali17899/fintech/internal/ledger/ports"
	"github.com/irfanali17899/fintech/pkg/database"
	"github.com/irfanali17899/fintech/pkg/scheduler"
)

type Module struct {
	db       *database.DB
	accounts *application.AccountService
	jobs     []*scheduler.Loop
	handlers []interface{ RegisterRoutes(*http.ServeMux) }
}

func New(db *database.DB, rails ports.RailRegistry) *Module {
	accRepo := persistence.NewAccountRepo(db)
	txnRepo := persistence.NewTransactionRepo(db)
	entryRepo := persistence.NewEntryRepo(db)
	balanceRepo := persistence.NewBalanceRepo(db)
	idemRepo := persistence.NewIdempotencyRepo(db)

	accSvc := application.NewAccountService(db, accRepo, balanceRepo, entryRepo)
	ledgerSvc := application.NewLedgerService(db, accRepo, txnRepo, entryRepo, balanceRepo, idemRepo, rails)
	adminSvc := application.NewAdminService(db, balanceRepo)

	return &Module{
		db:       db,
		accounts: accSvc,
		jobs: []*scheduler.Loop{
			scheduler.New("expire-holds", time.Minute, ledgerSvc.ExpirerJob(500)),
			scheduler.New("settlement", time.Minute, ledgerSvc.SettleJob(500)),
		},
		handlers: []interface{ RegisterRoutes(*http.ServeMux) }{
			transporthttp.NewAccountHandler(accSvc),
			transporthttp.NewTransactionHandler(ledgerSvc),
			transporthttp.NewAdminHandler(adminSvc),
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
