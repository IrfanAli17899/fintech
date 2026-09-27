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

// ---- fixture ----

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
		OwnerType: domain.OwnerCustomer, OwnerID: uuid.NewString(), Currency: "PKR", Type: typ,
	})
	must(t, err)
	return acc.ID
}

func (f fixture) balance(t *testing.T, id uuid.UUID) domain.Balance {
	t.Helper()
	b, err := f.accounts.GetAccountBalance(f.ctx, id)
	must(t, err)
	return *b
}

func (f fixture) post(t *testing.T, tr domain.Transaction) *domain.Transaction {
	t.Helper()
	created, _, err := f.ledger.Post(f.ctx, tr)
	must(t, err)
	return created
}

func (f fixture) verifyClean(t *testing.T) {
	t.Helper()
	ms, err := f.admin.VerifyBalances(f.ctx)
	must(t, err)
	if len(ms) != 0 {
		t.Fatalf("balance cache drift: %+v", ms)
	}
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

func hold(from, to uuid.UUID, amt int64, key string) domain.Transaction {
	x := transfer(from, to, amt, key)
	x.Kind = domain.TransactionKindPayment
	x.Status = domain.TransactionStatusPending
	return x
}

// ---- story 2: posting ----

func TestPost_Success(t *testing.T) {
	f := setup(t)
	cash := f.account(t, domain.AccountAsset)
	ali := f.account(t, domain.AccountLiability)

	got := f.post(t, transfer(cash, ali, 100000, uuid.NewString()))
	if got.Status != domain.TransactionStatusPosted || got.PostedAt == nil {
		t.Fatalf("status=%s posted_at=%v", got.Status, got.PostedAt)
	}
	if len(got.Entries) != 2 {
		t.Fatalf("entries = %d", len(got.Entries))
	}
	back, err := f.ledger.GetTransaction(f.ctx, got.ID)
	must(t, err)
	if len(back.Entries) != 2 {
		t.Fatalf("GetTransaction entries = %d", len(back.Entries))
	}
	if b := f.balance(t, ali); b.Posted != 100000 {
		t.Fatalf("ali posted = %d", b.Posted)
	}
	f.verifyClean(t)
}

func TestPost_UnknownAccount(t *testing.T) {
	f := setup(t)
	cash := f.account(t, domain.AccountAsset)
	_, _, err := f.ledger.Post(f.ctx, transfer(cash, uuid.New(), 100, uuid.NewString()))
	if !errdefs.Is(err, domain.CodeAccountNotFound) {
		t.Fatalf("want ACCOUNT_NOT_FOUND, got %v", err)
	}
}

func TestPost_CurrencyMismatch(t *testing.T) {
	f := setup(t)
	pkr := f.account(t, domain.AccountAsset)
	usd, err := f.accounts.CreateAccount(f.ctx, domain.Account{
		OwnerType: domain.OwnerCustomer, OwnerID: uuid.NewString(), Currency: "USD", Type: domain.AccountLiability,
	})
	must(t, err)
	_, _, err = f.ledger.Post(f.ctx, transfer(pkr, usd.ID, 100, uuid.NewString()))
	if !errdefs.Is(err, domain.CodeCurrencyMismatch) {
		t.Fatalf("want CURRENCY_MISMATCH, got %v", err)
	}
}

// ---- story 7: idempotency ----

func TestPost_ReplayReturnsOriginal(t *testing.T) {
	f := setup(t)
	cash := f.account(t, domain.AccountAsset)
	ali := f.account(t, domain.AccountLiability)
	key := uuid.NewString()

	first, replayed, err := f.ledger.Post(f.ctx, transfer(cash, ali, 100, key))
	must(t, err)
	if replayed {
		t.Fatal("first post marked as replay")
	}
	second, replayed, err := f.ledger.Post(f.ctx, transfer(cash, ali, 100, key))
	must(t, err)
	if !replayed || second.ID != first.ID {
		t.Fatalf("replay=%v id match=%v", replayed, second.ID == first.ID)
	}
	if b := f.balance(t, ali); b.Posted != 100 {
		t.Fatalf("replay double-posted: %d", b.Posted)
	}
}

func TestPost_KeyReusedWithDifferentPayload(t *testing.T) {
	f := setup(t)
	cash := f.account(t, domain.AccountAsset)
	ali := f.account(t, domain.AccountLiability)
	key := uuid.NewString()

	f.post(t, transfer(cash, ali, 100, key))
	_, _, err := f.ledger.Post(f.ctx, transfer(cash, ali, 101, key))
	if !errdefs.Is(err, domain.CodeIdempotencyKeyReused) {
		t.Fatalf("want IDEMPOTENCY_KEY_REUSED, got %v", err)
	}
}

func TestPost_ConcurrentSameKeyCreatesOne(t *testing.T) {
	f := setup(t)
	cash := f.account(t, domain.AccountAsset)
	ali := f.account(t, domain.AccountLiability)
	key := uuid.NewString()

	var (
		wg  sync.WaitGroup
		mu  sync.Mutex
		ids = map[uuid.UUID]bool{}
	)
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			got, _, err := f.ledger.Post(f.ctx, transfer(cash, ali, 100, key))
			if err != nil {
				t.Errorf("post: %v", err)
				return
			}
			mu.Lock()
			ids[got.ID] = true
			mu.Unlock()
		}()
	}
	wg.Wait()
	if len(ids) != 1 {
		t.Fatalf("want 1 transaction, got %d", len(ids))
	}
	if b := f.balance(t, ali); b.Posted != 100 {
		t.Fatalf("ali posted = %d, want 100", b.Posted)
	}
}

// ---- story 8: overdraft ----

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
	if b := f.balance(t, ali); b.Posted != 1000 {
		t.Fatalf("balance changed on rejected transfer: %d", b.Posted)
	}
}

func TestPost_AssetMayGoNegative(t *testing.T) {
	f := setup(t)
	cash := f.account(t, domain.AccountAsset)
	ali := f.account(t, domain.AccountLiability)
	f.post(t, transfer(cash, ali, 1000, uuid.NewString()))
	if b := f.balance(t, cash); b.Posted != 1000 {
		t.Fatalf("cash normal balance = %d, want 1000", b.Posted)
	}
}

func TestPost_ConcurrentTransfersCannotOverdraw(t *testing.T) {
	f := setup(t)
	cash := f.account(t, domain.AccountAsset)
	ali := f.account(t, domain.AccountLiability)
	sara := f.account(t, domain.AccountLiability)
	f.post(t, transfer(cash, ali, 1000, uuid.NewString()))

	var (
		wg      sync.WaitGroup
		mu      sync.Mutex
		ok, bad int
	)
	for i := 0; i < 20; i++ {
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
	if b := f.balance(t, ali); b.Posted != 0 {
		t.Fatalf("ali posted = %d, want 0", b.Posted)
	}
	f.verifyClean(t)
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
	if total := f.balance(t, a).Posted + f.balance(t, b).Posted; total != 20000 {
		t.Fatalf("money created or destroyed: %d", total)
	}
}

// ---- stories 9–10: reversals ----

func TestReverse_PartialThenRest(t *testing.T) {
	f := setup(t)
	cash := f.account(t, domain.AccountAsset)
	ali := f.account(t, domain.AccountLiability)
	shop := f.account(t, domain.AccountLiability)
	fees := f.account(t, domain.AccountRevenue)

	f.post(t, transfer(cash, ali, 120, uuid.NewString()))
	pay := f.post(t, domain.Transaction{
		Kind: domain.TransactionKindPayment, IdempotencyKey: uuid.NewString(),
		Entries: []domain.Entry{
			{AccountID: ali, Amount: 120, Direction: domain.EntryDirectionDebit},
			{AccountID: shop, Amount: 115, Direction: domain.EntryDirectionCredit},
			{AccountID: fees, Amount: 5, Direction: domain.EntryDirectionCredit},
		},
	})

	r1, _, err := f.ledger.Reverse(f.ctx, services.ReverseCommand{OriginalID: pay.ID, Amount: 50, IdempotencyKey: uuid.NewString()})
	must(t, err)
	if r1.ParentID == nil || *r1.ParentID != pay.ID {
		t.Fatal("reversal not linked to original")
	}
	if b := f.balance(t, shop); b.Posted != 115-48 {
		t.Fatalf("shop after partial = %d, want 67", b.Posted)
	}

	_, _, err = f.ledger.Reverse(f.ctx, services.ReverseCommand{OriginalID: pay.ID, IdempotencyKey: uuid.NewString()})
	must(t, err)

	orig, err := f.ledger.GetTransaction(f.ctx, pay.ID)
	must(t, err)
	if orig.Status != domain.TransactionStatusPosted {
		t.Fatalf("original status = %s, want posted (reversal never mutates the original)", orig.Status)
	}
	if b := f.balance(t, ali); b.Posted != 120 {
		t.Fatalf("ali should be whole again, got %d", b.Posted)
	}
	if b := f.balance(t, shop); b.Posted != 0 {
		t.Fatalf("shop should be back to 0, got %d", b.Posted)
	}

	_, _, err = f.ledger.Reverse(f.ctx, services.ReverseCommand{OriginalID: pay.ID, Amount: 1, IdempotencyKey: uuid.NewString()})
	if !errdefs.Is(err, domain.CodeReversalExceedsOriginal) {
		t.Fatalf("want REVERSAL_EXCEEDS_ORIGINAL, got %v", err)
	}
	f.verifyClean(t)
}

func TestReverse_ExceedsOriginal(t *testing.T) {
	f := setup(t)
	cash := f.account(t, domain.AccountAsset)
	ali := f.account(t, domain.AccountLiability)
	sara := f.account(t, domain.AccountLiability)
	f.post(t, transfer(cash, ali, 100, uuid.NewString()))
	pay := f.post(t, transfer(ali, sara, 100, uuid.NewString()))

	_, _, err := f.ledger.Reverse(f.ctx, services.ReverseCommand{OriginalID: pay.ID, Amount: 101, IdempotencyKey: uuid.NewString()})
	if !errdefs.Is(err, domain.CodeReversalExceedsOriginal) {
		t.Fatalf("want REVERSAL_EXCEEDS_ORIGINAL, got %v", err)
	}
}

func TestReverse_ConcurrentPartialsCannotExceed(t *testing.T) {
	f := setup(t)
	cash := f.account(t, domain.AccountAsset)
	ali := f.account(t, domain.AccountLiability)
	sara := f.account(t, domain.AccountLiability)
	f.post(t, transfer(cash, ali, 120, uuid.NewString()))
	pay := f.post(t, transfer(ali, sara, 120, uuid.NewString()))

	var (
		wg      sync.WaitGroup
		mu      sync.Mutex
		ok, bad int
	)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _, err := f.ledger.Reverse(f.ctx, services.ReverseCommand{OriginalID: pay.ID, Amount: 70, IdempotencyKey: uuid.NewString()})
			mu.Lock()
			defer mu.Unlock()
			switch {
			case err == nil:
				ok++
			case errdefs.Is(err, domain.CodeReversalExceedsOriginal):
				bad++
			default:
				t.Errorf("unexpected: %v", err)
			}
		}()
	}
	wg.Wait()
	if ok != 1 || bad != 1 {
		t.Fatalf("want exactly one of two 70-reversals against 120, got ok=%d bad=%d", ok, bad)
	}
}

// ---- stories 11–12: holds and available balance ----

func TestPost_PendingDoesNotMoveMoney(t *testing.T) {
	f := setup(t)
	cash := f.account(t, domain.AccountAsset)
	ali := f.account(t, domain.AccountLiability)

	got := f.post(t, func() domain.Transaction {
		x := transfer(cash, ali, 5000, uuid.NewString())
		x.Kind, x.Status = domain.TransactionKindTopup, domain.TransactionStatusPending
		return x
	}())
	if got.Status != domain.TransactionStatusPending || got.PostedAt != nil {
		t.Fatalf("status=%s posted_at=%v", got.Status, got.PostedAt)
	}
	b := f.balance(t, ali)
	if b.Posted != 0 || b.PendingIn != 5000 || b.Available != 0 {
		t.Fatalf("ali: %+v", b)
	}
	f.verifyClean(t)
}

func TestPost_HoldReducesAvailable(t *testing.T) {
	f := setup(t)
	cash := f.account(t, domain.AccountAsset)
	ali := f.account(t, domain.AccountLiability)
	sara := f.account(t, domain.AccountLiability)

	f.post(t, transfer(cash, ali, 1000, uuid.NewString()))
	f.post(t, hold(ali, sara, 500, uuid.NewString()))

	b := f.balance(t, ali)
	if b.Posted != 1000 || b.PendingOut != 500 || b.Available != 500 {
		t.Fatalf("ali after hold: %+v", b)
	}
	s := f.balance(t, sara)
	if s.Posted != 0 || s.PendingIn != 500 || s.Available != 0 {
		t.Fatalf("sara: pending credit must not be available: %+v", s)
	}
}

func TestPost_HoldThenOverdrawRejected(t *testing.T) {
	f := setup(t)
	cash := f.account(t, domain.AccountAsset)
	ali := f.account(t, domain.AccountLiability)
	sara := f.account(t, domain.AccountLiability)

	f.post(t, transfer(cash, ali, 1000, uuid.NewString()))
	f.post(t, hold(ali, sara, 500, uuid.NewString()))

	_, _, err := f.ledger.Post(f.ctx, transfer(ali, sara, 600, uuid.NewString()))
	if !errdefs.Is(err, domain.CodeInsufficientFunds) {
		t.Fatalf("want INSUFFICIENT_FUNDS, got %v", err)
	}
	f.post(t, transfer(ali, sara, 400, uuid.NewString()))
	if b := f.balance(t, ali); b.Available != 100 {
		t.Fatalf("ali available = %d, want 100", b.Available)
	}
}

func TestPost_ConcurrentHoldsCannotOverReserve(t *testing.T) {
	f := setup(t)
	cash := f.account(t, domain.AccountAsset)
	ali := f.account(t, domain.AccountLiability)
	sara := f.account(t, domain.AccountLiability)
	f.post(t, transfer(cash, ali, 1000, uuid.NewString()))

	var (
		wg      sync.WaitGroup
		mu      sync.Mutex
		ok, bad int
	)
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _, err := f.ledger.Post(f.ctx, hold(ali, sara, 100, uuid.NewString()))
			mu.Lock()
			defer mu.Unlock()
			switch {
			case err == nil:
				ok++
			case errdefs.Is(err, domain.CodeInsufficientFunds):
				bad++
			default:
				t.Errorf("unexpected: %v", err)
			}
		}()
	}
	wg.Wait()
	if ok != 10 || bad != 10 {
		t.Fatalf("want 10/10, got ok=%d bad=%d", ok, bad)
	}
	if b := f.balance(t, ali); b.Available != 0 || b.PendingOut != 1000 {
		t.Fatalf("ali: %+v", b)
	}
	f.verifyClean(t)
}

func TestPost_ConcurrentCreditsToOneMerchant(t *testing.T) {
	f := setup(t)
	cash := f.account(t, domain.AccountAsset)
	shop := f.account(t, domain.AccountLiability)

	const n = 50
	wallets := make([]uuid.UUID, n)
	for i := range wallets {
		wallets[i] = f.account(t, domain.AccountLiability)
		f.post(t, transfer(cash, wallets[i], 100, uuid.NewString()))
	}

	var wg sync.WaitGroup
	for _, w := range wallets {
		wg.Add(1)
		go func(w uuid.UUID) {
			defer wg.Done()
			if _, _, err := f.ledger.Post(f.ctx, transfer(w, shop, 100, uuid.NewString())); err != nil {
				t.Errorf("payment to shop: %v", err)
			}
		}(w)
	}
	wg.Wait()

	if b := f.balance(t, shop); b.Posted != n*100 {
		t.Fatalf("shop posted = %d, want %d", b.Posted, n*100)
	}
	f.verifyClean(t)
}
