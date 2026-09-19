package services_test

import (
	"context"
	"errors"
	"os"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/irfanali17899/fintech/internal/ledger/internal/domain"
	"github.com/irfanali17899/fintech/internal/ledger/internal/repositories"
	"github.com/irfanali17899/fintech/internal/ledger/internal/services"
	"github.com/irfanali17899/fintech/pkg/database"
	"github.com/irfanali17899/fintech/pkg/errdefs"
)

type fixture struct {
	ctx      context.Context
	ledger   *services.LedgerService
	accounts *services.AccountService
	admin    *services.AdminService
}

func setup(t *testing.T) fixture {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		dsn = "postgres://fintech:fintech@localhost:5432/fintech?sslmode=disable"
	}
	ctx := context.Background()
	db, err := database.Connect(ctx, dsn)
	if err != nil {
		t.Skipf("database not available: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	acc := repositories.NewAccountRepo(db.DB)
	txn := repositories.NewTransactionRepo(db.DB)
	ent := repositories.NewEntryRepo(db.DB)
	bal := repositories.NewBalanceRepo(db.DB)
	idem := repositories.NewIdempotencyRepo(db.DB)

	return fixture{
		ctx:      ctx,
		ledger:   services.NewLedgerService(db, acc, txn, ent, bal, idem),
		accounts: services.NewAccountService(db, acc, bal),
		admin:    services.NewAdminService(db, bal),
	}
}

// must fails the test with the underlying cause when err is an errdefs internal error.
func must(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		return
	}
	var e *errdefs.Error
	if errors.As(err, &e) && e.Cause != nil {
		t.Fatalf("%v: %v", err, e.Cause)
	}
	t.Fatal(err)
}

func (f fixture) account(t *testing.T, typ domain.AccountType) uuid.UUID {
	t.Helper()
	acc, err := f.accounts.CreateAccount(f.ctx, domain.Account{
		OwnerType: domain.OwnerCustomer,
		OwnerID:   uuid.NewString(),
		Currency:  "PKR",
		Type:      typ,
	})
	must(t, err)
	return acc.ID
}

func (f fixture) balance(t *testing.T, id uuid.UUID) int64 {
	t.Helper()
	b, err := f.accounts.GetAccountBalance(f.ctx, id)
	must(t, err)
	return b.Amount
}

func transfer(from, to uuid.UUID, amt int64, key string) domain.Transaction {
	return domain.Transaction{
		Kind:           domain.TransactionKindTransfer,
		IdempotencyKey: key,
		Entries: []domain.Entry{
			{AccountID: from, Amount: amt, Direction: domain.EntryDirectionDebit},
			{AccountID: to, Amount: amt, Direction: domain.EntryDirectionCredit},
		},
	}
}

func (f fixture) post(t *testing.T, tr domain.Transaction) *domain.Transaction {
	t.Helper()
	created, _, err := f.ledger.Post(f.ctx, tr)
	must(t, err)
	return created
}

func TestPost_InsufficientFunds(t *testing.T) {
	f := setup(t)
	cash := f.account(t, domain.AccountAsset)
	ali := f.account(t, domain.AccountLiability)
	sara := f.account(t, domain.AccountLiability)

	f.post(t, transfer(cash, ali, 1000, uuid.NewString()))

	_, _, err := f.ledger.Post(f.ctx, transfer(ali, sara, 1001, uuid.NewString()))
	if !errdefs.Is(err, domain.CodeInsufficientFunds) {
		t.Fatalf("want INSUFFICIENT_FUNDS, got %v", err)
	}
	if got := f.balance(t, ali); got != 1000 {
		t.Fatalf("ali balance changed on rejected transfer: %d", got)
	}
}

func TestPost_AssetMayGoNegative(t *testing.T) {
	f := setup(t)
	cash := f.account(t, domain.AccountAsset)
	ali := f.account(t, domain.AccountLiability)

	f.post(t, transfer(cash, ali, 1000, uuid.NewString()))
	if got := f.balance(t, cash); got != 1000 {
		t.Fatalf("cash normal balance = %d, want 1000", got)
	}
}

func TestPost_ConcurrentTransfersCannotOverdraw(t *testing.T) {
	f := setup(t)
	cash := f.account(t, domain.AccountAsset)
	ali := f.account(t, domain.AccountLiability)
	sara := f.account(t, domain.AccountLiability)

	f.post(t, transfer(cash, ali, 1000, uuid.NewString()))

	const n = 20
	var (
		wg      sync.WaitGroup
		mu      sync.Mutex
		ok, bad int
	)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _, err := f.ledger.Post(f.ctx, transfer(ali, sara, 100, uuid.NewString()))
			mu.Lock()
			defer mu.Unlock()
			switch {
			case err == nil:
				ok++
			case errdefs.Is(err, domain.CodeInsufficientFunds):
				bad++
			default:
				t.Errorf("unexpected error: %v", err)
			}
		}()
	}
	wg.Wait()

	if ok != 10 || bad != 10 {
		t.Fatalf("want 10 ok / 10 rejected, got %d / %d", ok, bad)
	}
	if got := f.balance(t, ali); got != 0 {
		t.Fatalf("ali balance = %d, want 0", got)
	}
	if got := f.balance(t, sara); got != 1000 {
		t.Fatalf("sara balance = %d, want 1000", got)
	}
	ms, err := f.admin.VerifyBalances(f.ctx)
	must(t, err)
	if len(ms) != 0 {
		t.Fatalf("cache drift after concurrent posts: %+v", ms)
	}
}

func TestPost_OppositeDirectionsDoNotDeadlock(t *testing.T) {
	f := setup(t)
	cash := f.account(t, domain.AccountAsset)
	a := f.account(t, domain.AccountLiability)
	b := f.account(t, domain.AccountLiability)
	f.post(t, transfer(cash, a, 10000, uuid.NewString()))
	f.post(t, transfer(cash, b, 10000, uuid.NewString()))

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			if _, _, err := f.ledger.Post(f.ctx, transfer(a, b, 10, uuid.NewString())); err != nil {
				t.Errorf("a→b: %v", err)
			}
		}()
		go func() {
			defer wg.Done()
			if _, _, err := f.ledger.Post(f.ctx, transfer(b, a, 10, uuid.NewString())); err != nil {
				t.Errorf("b→a: %v", err)
			}
		}()
	}
	wg.Wait()

	if total := f.balance(t, a) + f.balance(t, b); total != 20000 {
		t.Fatalf("money created or destroyed: total %d", total)
	}
}
