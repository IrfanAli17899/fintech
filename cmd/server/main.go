package main

import (
	"context"
	"database/sql"
	"log"
	"net/http"

	"github.com/irfanali17899/fintech/internal/handlers"
	"github.com/irfanali17899/fintech/internal/repositories"
	"github.com/irfanali17899/fintech/internal/services"
	"github.com/irfanali17899/fintech/pkg/database"
)

func main() {
	dbUrl := "postgres://fintech:fintech@localhost:5432/fintech?sslmode=disable"
	ctx := context.Background()
	db, err := database.Connect(ctx, dbUrl)

	if err != nil {
		log.Fatalf("database initialization failed: %v", err)
	}

	defer db.Close()

	err = database.RunMigrations(ctx, dbUrl)
	if err != nil {
		log.Fatalf("database migrations failed: %v", err)
	}

	mux := http.NewServeMux()

	// accounts wiring
	accRepo := repositories.NewAccountRepo(db.DB)
	accSvc := services.NewAccountService(accRepo)
	accHandler := handlers.NewAccountHandler(accSvc)
	accHandler.RegisterRoutes(mux)

	// transactions wiring
	txnRepo := repositories.NewTransactionRepo(db.DB)
	entryRepo := repositories.NewEntryRepo(db.DB)
	ledgerSvc := services.NewLedgerService(db, accRepo, txnRepo, entryRepo)
	txnHandler := handlers.NewTransactionHandler(ledgerSvc)
	txnHandler.RegisterRoutes(mux)

	// seeds
	err = database.InTx(ctx, db.DB, func(tx *sql.Tx) error {
		return accRepo.WithTx(tx).SeedAccounts(ctx)
	})

	if err != nil {
		log.Fatalf("database seeding failed: %v", err)
	}

	server := &http.Server{
		Addr:    ":4000",
		Handler: mux,
	}

	log.Println("listening on :4000")

	if err := server.ListenAndServe(); err != nil {
		log.Fatal(err)
	}

}
