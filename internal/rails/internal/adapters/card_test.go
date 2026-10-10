package adapters_test

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/irfanali17899/fintech/internal/ledger/ports"
	"github.com/irfanali17899/fintech/internal/rails/internal/adapters"
	"github.com/irfanali17899/fintech/internal/rails/internal/domain"
	"github.com/irfanali17899/fintech/internal/rails/internal/persistence"
	"github.com/irfanali17899/fintech/pkg/database"
)

func setup(t *testing.T) (*adapters.CardRail, *persistence.AttemptRepo) {
	t.Helper()
	db := connectTestDB(t)
	repo := persistence.NewAttemptRepo(db.DB)
	return adapters.NewCardRail(repo, 48*time.Hour), repo
}

func connectTestDB(t *testing.T) *database.DB {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		dsn = "postgres://fintech:fintech@localhost:5432/fintech?sslmode=disable"
	}
	db, err := database.Connect(context.Background(), dsn)
	if err != nil {
		t.Skipf("database not available: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func initiateCard(t *testing.T, c *adapters.CardRail) string {
	t.Helper()
	res, err := c.Initiate(context.Background(), ports.RailInitiateRequest{
		TransactionID: uuid.New(), Amount: 1000, Currency: "PKR", Kind: "topup",
	})
	if err != nil {
		t.Fatal(err)
	}
	return res.Ref
}

func TestCard_InitiateIsDeferredAndRecorded(t *testing.T) {
	c, repo := setup(t)
	ref := initiateCard(t, c)

	a, err := repo.Get(context.Background(), ref)
	if err != nil {
		t.Fatal(err)
	}
	if a.State != domain.AttemptInitiated || a.Amount != 1000 || time.Until(a.SettleAfter) < 47*time.Hour {
		t.Fatalf("%+v", a)
	}
}

func TestCard_SettleOnce(t *testing.T) {
	c, repo := setup(t)
	ref := initiateCard(t, c)
	ctx := context.Background()

	if err := c.Settle(ctx, ref); err != nil {
		t.Fatal(err)
	}
	a, _ := repo.Get(ctx, ref)
	if a.State != domain.AttemptSettled {
		t.Fatalf("state = %s", a.State)
	}
	if err := c.Settle(ctx, ref); !errors.Is(err, domain.ErrWrongState) {
		t.Fatalf("second settle: want ErrWrongState, got %v", err)
	}
}

func TestCard_ReverseOnlyAfterSettle(t *testing.T) {
	c, repo := setup(t)
	ref := initiateCard(t, c)
	ctx := context.Background()

	if err := c.Reverse(ctx, ref, 1000); !errors.Is(err, domain.ErrWrongState) {
		t.Fatalf("reverse before settle: want ErrWrongState, got %v", err)
	}
	if err := c.Settle(ctx, ref); err != nil {
		t.Fatal(err)
	}
	if err := c.Reverse(ctx, ref, 1000); err != nil {
		t.Fatal(err)
	}
	a, _ := repo.Get(ctx, ref)
	if a.State != domain.AttemptReversed || a.Reason == nil {
		t.Fatalf("%+v", a)
	}
}
