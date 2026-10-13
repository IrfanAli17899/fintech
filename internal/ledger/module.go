package ledger

import (
	"context"
	"net/http"
	"time"

	"github.com/irfanali17899/fintech/internal/ledger/api"
	"github.com/irfanali17899/fintech/internal/ledger/internal/adapter"
	"github.com/irfanali17899/fintech/internal/ledger/internal/application"
	"github.com/irfanali17899/fintech/internal/ledger/internal/persistence"
	transporthttp "github.com/irfanali17899/fintech/internal/ledger/internal/transport/http"
	"github.com/irfanali17899/fintech/internal/ledger/ports"
	"github.com/irfanali17899/fintech/pkg/database"
	"github.com/irfanali17899/fintech/pkg/scheduler"
)

type Module struct {
	db       *database.DB
	svc      *application.Service
	ledger   *adapter.Ledger
	jobs     []*scheduler.Loop
	handlers []interface{ RegisterRoutes(*http.ServeMux) }
}

func New(db *database.DB, rails ports.RailRegistry) *Module {
	stores := persistence.NewStores(db)
	svc := application.NewService(persistence.NewUnitOfWork(db, stores), stores, rails)

	ledger := adapter.NewLedger(svc)
	return &Module{
		db:     db,
		svc:    svc,
		ledger: ledger,
		jobs: []*scheduler.Loop{
			scheduler.New("expire-holds", time.Minute, svc.ExpirerJob(500)),
			scheduler.New("outcomes", time.Minute, svc.OutcomesJob(20)),
			scheduler.New("reconcile", 15*time.Minute, svc.ReconcileJob(50)),
		},
		handlers: []interface{ RegisterRoutes(*http.ServeMux) }{
			transporthttp.NewAccountHandler(ledger),
			transporthttp.NewTransactionHandler(ledger),
			transporthttp.NewAdminHandler(adapter.NewAdmin(svc)),
		},
	}
}

// API is the ledger as other modules see it. There is deliberately no
// accessor for the admin surface: it is wired to this module's own transport
// and never handed to another module.
func (m *Module) API() api.Ledger { return m.ledger }

func (m *Module) RegisterRoutes(mux *http.ServeMux) {
	for _, h := range m.handlers {
		h.RegisterRoutes(mux)
	}
}

func (m *Module) Seed(ctx context.Context) error {
	return m.svc.SeedSystem(ctx)
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
