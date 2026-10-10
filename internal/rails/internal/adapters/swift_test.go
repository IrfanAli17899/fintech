package adapters_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/irfanali17899/fintech/internal/ledger/ports"
	"github.com/irfanali17899/fintech/internal/rails/internal/adapters"
	"github.com/irfanali17899/fintech/internal/rails/internal/domain"
	"github.com/irfanali17899/fintech/internal/rails/internal/persistence"
)

func setupSwift(t *testing.T) (*adapters.SwiftRail, *persistence.AttemptRepo) {
	t.Helper()
	db := connectTestDB(t) // extract from card_test's setup if it's inline there
	repo := persistence.NewAttemptRepo(db.DB)
	return adapters.NewSwiftRail(repo, 72*time.Hour), repo
}

func initiateSwift(t *testing.T, s *adapters.SwiftRail, amount int64) string {
	t.Helper()
	res, err := s.Initiate(context.Background(), ports.RailInitiateRequest{
		TransactionID: uuid.New(), Amount: amount, Currency: "USD", Kind: "topup",
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Settlement != ports.SettlementDeferred {
		t.Fatalf("settlement = %s", res.Settlement)
	}
	return res.Ref
}

func TestSwift_SettlesNormalAmount(t *testing.T) {
	s, repo := setupSwift(t)
	ref := initiateSwift(t, s, 1000)
	ctx := context.Background()

	if err := s.Settle(ctx, ref); err != nil {
		t.Fatal(err)
	}
	a, _ := repo.Get(ctx, ref)
	if a.State != domain.AttemptSettled {
		t.Fatalf("state = %s", a.State)
	}
}

func TestSwift_BouncesMagicAmount(t *testing.T) {
	s, repo := setupSwift(t)
	ref := initiateSwift(t, s, 1099)
	ctx := context.Background()

	var bounce *ports.RailBounce
	if err := s.Settle(ctx, ref); !errors.As(err, &bounce) {
		t.Fatalf("want RailBounce, got %v", err)
	}
	if bounce.Reason == "" {
		t.Fatal("bounce carries no reason")
	}
	a, _ := repo.Get(ctx, ref)
	if a.State != domain.AttemptBounced || a.Reason == nil {
		t.Fatalf("%+v", a)
	}
	// a bounced attempt can't be settled afterwards; the magic amount still
	// routes to the bounce branch, which now fails the state transition
	if err := s.Settle(ctx, ref); !errors.Is(err, domain.ErrWrongState) {
		t.Fatalf("second settle: want ErrWrongState, got %v", err)
	}
}

func TestSwift_NeverReversible(t *testing.T) {
	s, _ := setupSwift(t)
	ref := initiateSwift(t, s, 1000)
	if err := s.Reverse(context.Background(), ref, 1000); !errors.Is(err, domain.ErrNotReversible) {
		t.Fatalf("want ErrNotReversible, got %v", err)
	}
}
