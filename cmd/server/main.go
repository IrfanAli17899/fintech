package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/irfanali17899/fintech/internal/ledger"
	"github.com/irfanali17899/fintech/pkg/database"
)

func main() {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		dbURL = "postgres://fintech:fintech@localhost:5432/fintech?sslmode=disable"
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	db, err := database.Connect(ctx, dbURL)
	if err != nil {
		log.Fatalf("database initialization failed: %v", err)
	}
	defer db.Close()

	if err := database.RunMigrations(ctx, dbURL); err != nil {
		log.Fatalf("database migrations failed: %v", err)
	}

	ledgerMod := ledger.New(db)
	if err := ledgerMod.Seed(ctx); err != nil {
		log.Fatalf("seeding failed: %v", err)
	}
	ledgerMod.Start(ctx)
	defer ledgerMod.Stop()

	mux := http.NewServeMux()
	ledgerMod.RegisterRoutes(mux)
	server := &http.Server{Addr: ":4000", Handler: mux}

	go func() {
		log.Println("listening on :4000")
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatal(err)
		}
	}()

	<-ctx.Done()
	log.Println("shutting down")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Printf("server shutdown: %v", err)
	}
}
