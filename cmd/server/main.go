package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/irfanali17899/fintech/ledger"
	"github.com/irfanali17899/fintech/platform/database"
	"github.com/irfanali17899/fintech/platform/httpx"
	"github.com/irfanali17899/fintech/rails"
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
	if v := os.Getenv("DB_MAX_OPEN"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 {
			log.Fatalf("DB_MAX_OPEN must be a positive integer, got %q", v)
		}
		db.SetMaxOpenConns(n)
		db.SetMaxIdleConns(n)
	}

	if err := ledger.Migrate(ctx, db.DB, dbURL); err != nil {
		log.Fatalf("ledger database migrations failed: %v", err)
	}
	if err := rails.Migrate(ctx, db.DB, dbURL); err != nil {
		log.Fatalf("rails database migrations failed: %v", err)
	}

	ledgerMod := ledger.New(db, rails.New(db))
	if err := ledgerMod.Seed(ctx); err != nil {
		log.Fatalf("seeding failed: %v", err)
	}
	ledgerMod.Start(ctx)
	defer ledgerMod.Stop()

	mux := http.NewServeMux()
	ledgerMod.RegisterRoutes(mux)
	server := &http.Server{
		Addr:              ":4000",
		Handler:           httpx.WithTimeout(mux, 15*time.Second),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

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
