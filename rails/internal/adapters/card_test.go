package adapters_test

import (
	"context"
	"errors"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/irfanali17899/fintech/ledger/ports"
	"github.com/irfanali17899/fintech/platform/database"
	"github.com/irfanali17899/fintech/rails"
	"github.com/irfanali17899/fintech/rails/internal/adapters"
	"github.com/irfanali17899/fintech/rails/internal/domain"
	"github.com/irfanali17899/fintech/rails/internal/persistence"
)

type repos struct {
	attempts *persistence.AttemptRepo
	outcomes *persistence.OutcomeRepo
	store    adapters.Store
}

var migrateOnce sync.Once

// connectTestDB connects and brings the rails schema up to date once per run,
// so a stale local database fails here, loudly, rather than in a vague query.
func connectTestDB(t *testing.T) *database.DB {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		dsn = "postgres://fintech:fintech@localhost:5432/fintech_test?sslmode=disable"
	}
	db, err := database.Connect(context.Background(), dsn)
	if err != nil {
		t.Skipf("test database not available (make test-db): %v", err)
	}
	t.Cleanup(func() { db.Close() })
	var merr error
	migrateOnce.Do(func() { merr = rails.Migrate(context.Background(), db.DB, dsn) })
	if merr != nil {
		t.Fatalf("migrate rails: %v", merr)
	}
	return db
}

func newRepos(db *database.DB) repos {
	a, o := persistence.NewAttemptRepo(db.DB), persistence.NewOutcomeRepo(db.DB)
	return repos{attempts: a, outcomes: o, store: adapters.NewStore(db, a, o)}
}

func setupCard(t *testing.T) (*adapters.CardRail, repos) {
	t.Helper()
	r := newRepos(connectTestDB(t))
	return adapters.NewCardRail(r.store, 48*time.Hour), r
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

func TestCard_InitiateRecordsAttemptAndSettleOutcome(t *testing.T) {
	c, r := setupCard(t)
	ref := initiateCard(t, c)
	ctx := context.Background()

	a, err := r.attempts.Get(ctx, ref)
	if err != nil {
		t.Fatal(err)
	}
	if a.State != domain.AttemptInitiated || a.Amount != 1000 || time.Until(a.SettleAfter) < 47*time.Hour {
		t.Fatalf("%+v", a)
	}
	o, err := r.outcomes.Get(ctx, ref, domain.OutcomeSettle, "")
	if err != nil {
		t.Fatal(err)
	}
	if o.Decided() || o.AppliedAt != nil {
		t.Fatalf("settle outcome should be undecided and unapplied: %+v", o)
	}
}

func TestCard_SettleDecidesOnceAndIsIdempotent(t *testing.T) {
	c, r := setupCard(t)
	ref := initiateCard(t, c)
	ctx := context.Background()

	if err := c.Settle(ctx, ref); err != nil {
		t.Fatal(err)
	}
	a, _ := r.attempts.Get(ctx, ref)
	o, _ := r.outcomes.Get(ctx, ref, domain.OutcomeSettle, "")
	if a.State != domain.AttemptSettled || o.State != domain.AttemptSettled {
		t.Fatalf("attempt=%s outcome=%s", a.State, o.State)
	}
	if err := c.Settle(ctx, ref); err != nil {
		t.Fatalf("second settle must be a no-op, got %v", err)
	}
}

func TestCard_CancelBeforeSettleSupersedesSettle(t *testing.T) {
	c, r := setupCard(t)
	ref := initiateCard(t, c)
	ctx := context.Background()

	if err := c.Cancel(ctx, ref); err != nil {
		t.Fatal(err)
	}
	a, _ := r.attempts.Get(ctx, ref)
	cancel, _ := r.outcomes.Get(ctx, ref, domain.OutcomeCancel, "")
	settle, _ := r.outcomes.Get(ctx, ref, domain.OutcomeSettle, "")
	if a.State != domain.AttemptCancelled || cancel.State != domain.AttemptCancelled || settle.State != domain.AttemptCancelled {
		t.Fatalf("attempt=%s cancel=%s settle=%s", a.State, cancel.State, settle.State)
	}
	if err := c.Cancel(ctx, ref); err != nil {
		t.Fatalf("second cancel must be a no-op, got %v", err)
	}
	if err := c.Settle(ctx, ref); !errors.Is(err, domain.ErrWrongState) {
		t.Fatalf("settle after cancel: want ErrWrongState, got %v", err)
	}
}

func TestCard_RefundsOnlyAfterSettleAndMultiplePartials(t *testing.T) {
	c, r := setupCard(t)
	ref := initiateCard(t, c)
	ctx := context.Background()

	if err := c.Reverse(ctx, ref, "k1", 400); !errors.Is(err, domain.ErrWrongState) {
		t.Fatalf("refund before settle: want ErrWrongState, got %v", err)
	}
	if err := c.Settle(ctx, ref); err != nil {
		t.Fatal(err)
	}
	if err := c.Reverse(ctx, ref, "k1", 400); err != nil {
		t.Fatal(err)
	}
	if err := c.Reverse(ctx, ref, "k2", 300); err != nil {
		t.Fatal(err)
	}
	if err := c.Reverse(ctx, ref, "k1", 400); err != nil {
		t.Fatalf("repeat with same key must be a no-op, got %v", err)
	}
	for _, k := range []string{"k1", "k2"} {
		o, err := r.outcomes.Get(ctx, ref, domain.OutcomeRefund, k)
		if err != nil {
			t.Fatal(err)
		}
		if o.State != domain.AttemptSettled || o.AppliedAt != nil {
			t.Fatalf("refund %s: %+v", k, o)
		}
	}
	a, _ := r.attempts.Get(ctx, ref)
	if a.State != domain.AttemptSettled {
		t.Fatalf("refunds must not change the attempt's state: %s", a.State)
	}
}

func TestCard_ConcurrentSettleAndCancelOneWins(t *testing.T) {
	for i := 0; i < 20; i++ {
		c, r := setupCard(t)
		ref := initiateCard(t, c)
		ctx := context.Background()

		var wg sync.WaitGroup
		var settleErr, cancelErr error
		wg.Add(2)
		go func() { defer wg.Done(); settleErr = c.Settle(ctx, ref) }()
		go func() { defer wg.Done(); cancelErr = c.Cancel(ctx, ref) }()
		wg.Wait()

		if (settleErr == nil) == (cancelErr == nil) {
			t.Fatalf("exactly one must win: settle=%v cancel=%v", settleErr, cancelErr)
		}
		a, _ := r.attempts.Get(ctx, ref)
		settle, _ := r.outcomes.Get(ctx, ref, domain.OutcomeSettle, "")
		if settle.State != a.State {
			t.Fatalf("attempt %s but settle outcome %s", a.State, settle.State)
		}
		if cancelErr != nil && !errors.Is(cancelErr, ports.ErrRailAlreadyDecided) {
			t.Fatalf("losing cancel: want ErrRailAlreadyDecided, got %v", cancelErr)
		}
	}
}

func TestOutcomes_FailureBacksOffAndReleasesLease(t *testing.T) {
	c, r := setupCard(t)
	ref := initiateCard(t, c)
	ctx := context.Background()

	if err := r.outcomes.RecordFailure(ctx, ref, domain.OutcomeSettle, "", "provider down"); err != nil {
		t.Fatal(err)
	}
	o, _ := r.outcomes.Get(ctx, ref, domain.OutcomeSettle, "")
	if o.AttemptCount != 1 || o.ClaimedUntil != nil || o.NextAttemptAt == nil || o.LastError == nil {
		t.Fatalf("%+v", o)
	}
	if d := time.Until(*o.NextAttemptAt); d < 30*time.Second || d > 2*time.Minute {
		t.Fatalf("first backoff should be about a minute, got %v", d)
	}
	for i := 0; i < 15; i++ {
		_ = r.outcomes.RecordFailure(ctx, ref, domain.OutcomeSettle, "", "provider down")
	}
	o, _ = r.outcomes.Get(ctx, ref, domain.OutcomeSettle, "")
	if d := time.Until(*o.NextAttemptAt); d > 6*time.Hour+time.Minute {
		t.Fatalf("backoff must cap at 6h, got %v", d)
	}
}
