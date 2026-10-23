package adapters_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/irfanali17899/fintech/ledger/ports"
	"github.com/irfanali17899/fintech/rails/internal/adapters"
	"github.com/irfanali17899/fintech/rails/internal/domain"
)

func setupSwift(t *testing.T) (*adapters.SwiftRail, repos) {
	t.Helper()
	r := newRepos(connectTestDB(t))
	return adapters.NewSwiftRail(r.store, 72*time.Hour), r
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
	s, r := setupSwift(t)
	ref := initiateSwift(t, s, 1000)
	ctx := context.Background()

	if err := s.Settle(ctx, ref); err != nil {
		t.Fatal(err)
	}
	a, _ := r.attempts.Get(ctx, ref)
	o, _ := r.outcomes.Get(ctx, ref, domain.OutcomeSettle, "")
	if a.State != domain.AttemptSettled || o.State != domain.AttemptSettled {
		t.Fatalf("attempt=%s outcome=%s", a.State, o.State)
	}
}

func TestSwift_BouncesMagicAmount(t *testing.T) {
	s, r := setupSwift(t)
	ref := initiateSwift(t, s, 1099)
	ctx := context.Background()

	var bounce *ports.RailBounce
	if err := s.Settle(ctx, ref); !errors.As(err, &bounce) {
		t.Fatalf("want RailBounce, got %v", err)
	}
	if bounce.Reason == "" {
		t.Fatal("bounce carries no reason")
	}
	a, _ := r.attempts.Get(ctx, ref)
	o, _ := r.outcomes.Get(ctx, ref, domain.OutcomeSettle, "")
	if a.State != domain.AttemptBounced || o.State != domain.AttemptBounced || o.Reason == nil {
		t.Fatalf("attempt=%+v outcome=%+v", a, o)
	}
	if err := s.Settle(ctx, ref); !errors.Is(err, domain.ErrWrongState) {
		t.Fatalf("second settle: want ErrWrongState, got %v", err)
	}
}

func TestSwift_NeitherCancellableNorReversible(t *testing.T) {
	s, _ := setupSwift(t)
	ref := initiateSwift(t, s, 1000)
	ctx := context.Background()
	if err := s.Cancel(ctx, ref); !errors.Is(err, domain.ErrCannotCancel) {
		t.Fatalf("want ErrCannotCancel, got %v", err)
	}
	if err := s.Reverse(ctx, ref, "k", 1000); !errors.Is(err, domain.ErrNotReversible) {
		t.Fatalf("want ErrNotReversible, got %v", err)
	}
}
