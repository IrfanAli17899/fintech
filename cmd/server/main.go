package main

import (
	"context"
	"log"
	"net/http"
	"os"

	"github.com/irfanali17899/fintech/internal/ledger"
	"github.com/irfanali17899/fintech/pkg/database"
)

func main() {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		dbURL = "postgres://fintech:fintech@localhost:5432/fintech?sslmode=disable"
	}
	ctx := context.Background()

	db, err := database.Connect(ctx, dbURL)
	if err != nil {
		log.Fatalf("database initialization failed: %v", err)
	}
	defer db.Close()

	if err := database.RunMigrations(ctx, dbURL); err != nil {
		log.Fatalf("database migrations failed: %v", err)
	}

	mux := http.NewServeMux()

	ledgerMod := ledger.New(db)
	if err := ledgerMod.Seed(ctx); err != nil {
		log.Fatalf("seeding failed: %v", err)
	}
	ledgerMod.RegisterRoutes(mux)

	server := &http.Server{
		Addr:    ":4000",
		Handler: mux,
	}

	log.Println("listening on :4000")
	if err := server.ListenAndServe(); err != nil {
		log.Fatal(err)
	}
}
