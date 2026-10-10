package application_test

import (
	"context"
	"errors"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/irfanali17899/fintech/internal/ledger"
	"github.com/irfanali17899/fintech/internal/ledger/internal/application"
	"github.com/irfanali17899/fintech/internal/ledger/internal/domain"
	"github.com/irfanali17899/fintech/internal/ledger/internal/persistence"
	"github.com/irfanali17899/fintech/internal/ledger/ports"
	"github.com/irfanali17899/fintech/internal/rails"
	"github.com/irfanali17899/fintech/pkg/database"
	"github.com/irfanali17899/fintech/pkg/errdefs"
)

// product vocabulary used by these tests; the ledger itself has no opinion
const (
	kindTopup    domain.TransactionKind = "topup"
	kindTransfer domain.TransactionKind = "transfer"
	kindPayment  domain.TransactionKind = "payment"
	kindRefund   domain.TransactionKind = "refund"
)

// ---- fixture ----

type fixture struct {
	ctx      context.Context
	ledger   *application.LedgerService
	accounts *application.AccountService
	admin    *application.AdminService
}

func setup(t *testing.T) fixture {
	t.Helper()
	db := connectTestDB(t)
	return setupWith(t, db, rails.New(db))
}

var migrateOnce sync.Once

// connectTestDB connects and brings both schemas up to date once per run, so a
// stale local database fails here, loudly, rather than in a vague query.
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
	var merr error
	migrateOnce.Do(func() {
		ctx := context.Background()
		if merr = ledger.Migrate(ctx, db.DB, dsn); merr == nil {
			merr = rails.Migrate(ctx, db.DB, dsn)
		}
	})
	if merr != nil {
		t.Fatalf("migrate: %v", merr)
	}
	return db
}

func setupWith(t *testing.T, db *database.DB, reg ports.RailRegistry) fixture {
	t.Helper()
	acc := persistence.NewAccountRepo(db.DB)
	txn := persistence.NewTransactionRepo(db.DB)
	ent := persistence.NewEntryRepo(db.DB)
	bal := persistence.NewBalanceRepo(db.DB)
	idem := persistence.NewIdempotencyRepo(db.DB)

	return fixture{
		ctx:      context.Background(),
		ledger:   application.NewLedgerService(db, acc, txn, ent, bal, idem, reg),
		accounts: application.NewAccountService(db, acc, bal, ent),
		admin:    application.NewAdminService(db, bal),
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

// applyAll runs the outcome worker until a run applies nothing, as of `at`.
// Rows that keep failing back off and stop the loop instead of spinning it.
func (f fixture) applyAll(t *testing.T, at time.Time) application.OutcomeStats {
	t.Helper()
	var total application.OutcomeStats
	for {
		st, err := f.ledger.ApplyOutcomes(f.ctx, at, 100)
		must(t, err)
		total.Claimed += st.Claimed
		total.Settled += st.Settled
		total.Bounced += st.Bounced
		total.Cancelled += st.Cancelled
		total.Superseded += st.Superseded
		total.Refunded += st.Refunded
		total.Refused += st.Refused
		total.Failed += st.Failed
		total.Diverged += st.Diverged
		if st.Applied() == 0 {
			return total
		}
	}
}

// drain applies everything earlier tests left due in the shared database, so
// a test's counts cover only its own outcomes.
func (f fixture) drain(t *testing.T) {
	t.Helper()
	f.applyAll(t, time.Now().Add(365*24*time.Hour))
}

func transfer(from, to uuid.UUID, amt int64, key string) domain.Transaction {
	return domain.Transaction{
		Kind:           kindTransfer,
		IdempotencyKey: key,
		Entries: []domain.Entry{
			{AccountID: from, Amount: amt, Direction: domain.EntryDirectionDebit},
			{AccountID: to, Amount: amt, Direction: domain.EntryDirectionCredit},
		},
	}
}

func hold(from, to uuid.UUID, amt int64, key string) domain.Transaction {
	x := transfer(from, to, amt, key)
	x.Kind = kindPayment
	x.Status = domain.TransactionStatusPending
	return x
}

func holdExpiring(from, to uuid.UUID, amt int64, at time.Time) domain.Transaction {
	h := hold(from, to, amt, uuid.NewString())
	h.ExpiresAt = &at
	return h
}

func cardHold(t *testing.T, f fixture) *domain.Transaction {
	t.Helper()
	cash := f.account(t, domain.AccountAsset)
	ali := f.account(t, domain.AccountLiability)
	x := transfer(cash, ali, 5000, uuid.NewString())
	x.Kind, x.Rail = kindTopup, "card"
	return f.post(t, x)
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
		Kind: kindPayment, IdempotencyKey: uuid.NewString(),
		Entries: []domain.Entry{
			{AccountID: ali, Amount: 120, Direction: domain.EntryDirectionDebit},
			{AccountID: shop, Amount: 115, Direction: domain.EntryDirectionCredit},
			{AccountID: fees, Amount: 5, Direction: domain.EntryDirectionCredit},
		},
	})

	r1, _, err := f.ledger.Reverse(f.ctx, application.ReverseCommand{Kind: string(kindRefund), OriginalID: pay.ID, Amount: 50, IdempotencyKey: uuid.NewString()})
	must(t, err)
	if r1.ParentID == nil || *r1.ParentID != pay.ID {
		t.Fatal("reversal not linked to original")
	}
	if b := f.balance(t, shop); b.Posted != 115-48 {
		t.Fatalf("shop after partial = %d, want 67", b.Posted)
	}

	_, _, err = f.ledger.Reverse(f.ctx, application.ReverseCommand{Kind: string(kindRefund), OriginalID: pay.ID, IdempotencyKey: uuid.NewString()})
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

	_, _, err = f.ledger.Reverse(f.ctx, application.ReverseCommand{Kind: string(kindRefund), OriginalID: pay.ID, Amount: 1, IdempotencyKey: uuid.NewString()})
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

	_, _, err := f.ledger.Reverse(f.ctx, application.ReverseCommand{Kind: string(kindRefund), OriginalID: pay.ID, Amount: 101, IdempotencyKey: uuid.NewString()})
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
			_, _, err := f.ledger.Reverse(f.ctx, application.ReverseCommand{Kind: string(kindRefund), OriginalID: pay.ID, Amount: 70, IdempotencyKey: uuid.NewString()})
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
		x.Kind, x.Status = kindTopup, domain.TransactionStatusPending
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

// ---- story 13: capture and void ----

func TestCapture_Full(t *testing.T) {
	f := setup(t)
	cash := f.account(t, domain.AccountAsset)
	ali := f.account(t, domain.AccountLiability)
	shop := f.account(t, domain.AccountLiability)
	f.post(t, transfer(cash, ali, 1000, uuid.NewString()))
	h := f.post(t, hold(ali, shop, 500, uuid.NewString()))

	cap, replayed, err := f.ledger.Capture(f.ctx, application.CaptureCommand{HoldID: h.ID, IdempotencyKey: uuid.NewString()})
	must(t, err)
	if replayed || cap.Status != domain.TransactionStatusPosted || cap.ParentID == nil || *cap.ParentID != h.ID {
		t.Fatalf("capture: %+v", cap)
	}
	if cap.Kind != h.Kind {
		t.Fatalf("capture kind = %s, want %s", cap.Kind, h.Kind)
	}
	holdNow, err := f.ledger.GetTransaction(f.ctx, h.ID)
	must(t, err)
	if holdNow.Status != domain.TransactionStatusVoided {
		t.Fatalf("hold status = %s, want voided", holdNow.Status)
	}
	a := f.balance(t, ali)
	if a.Posted != 500 || a.PendingOut != 0 || a.Available != 500 {
		t.Fatalf("ali: %+v", a)
	}
	s := f.balance(t, shop)
	if s.Posted != 500 || s.PendingIn != 0 {
		t.Fatalf("shop: %+v", s)
	}
	f.verifyClean(t)
}

func TestCapture_PartialKeepsRestReserved(t *testing.T) {
	f := setup(t)
	cash := f.account(t, domain.AccountAsset)
	ali := f.account(t, domain.AccountLiability)
	shop := f.account(t, domain.AccountLiability)
	f.post(t, transfer(cash, ali, 1000, uuid.NewString()))
	h := f.post(t, hold(ali, shop, 500, uuid.NewString()))

	cap, _, err := f.ledger.Capture(f.ctx, application.CaptureCommand{HoldID: h.ID, Amount: 300, IdempotencyKey: uuid.NewString()})
	must(t, err)
	if cap.Total() != 300 {
		t.Fatalf("capture total = %d, want 300", cap.Total())
	}

	a := f.balance(t, ali)
	if a.Posted != 700 || a.PendingOut != 200 || a.Available != 500 {
		t.Fatalf("ali: only the captured portion should be released: %+v", a)
	}
	if s := f.balance(t, shop); s.Posted != 300 || s.PendingIn != 200 {
		t.Fatalf("shop: %+v", s)
	}

	holdNow, err := f.ledger.GetTransaction(f.ctx, h.ID)
	must(t, err)
	if holdNow.Status != domain.TransactionStatusPending {
		t.Fatalf("hold should stay pending after partial capture, got %s", holdNow.Status)
	}
	f.verifyClean(t)
}

func TestCapture_ExceedsHold(t *testing.T) {
	f := setup(t)
	cash := f.account(t, domain.AccountAsset)
	ali := f.account(t, domain.AccountLiability)
	shop := f.account(t, domain.AccountLiability)
	f.post(t, transfer(cash, ali, 1000, uuid.NewString()))
	h := f.post(t, hold(ali, shop, 500, uuid.NewString()))

	_, _, err := f.ledger.Capture(f.ctx, application.CaptureCommand{HoldID: h.ID, Amount: 501, IdempotencyKey: uuid.NewString()})
	if !errdefs.Is(err, domain.CodeCaptureExceedsHold) {
		t.Fatalf("want CAPTURE_EXCEEDS_HOLD, got %v", err)
	}
	if b := f.balance(t, ali); b.PendingOut != 500 {
		t.Fatalf("hold disturbed by rejected capture: %+v", b)
	}
}

func TestCapture_NotPending(t *testing.T) {
	f := setup(t)
	cash := f.account(t, domain.AccountAsset)
	ali := f.account(t, domain.AccountLiability)
	shop := f.account(t, domain.AccountLiability)
	f.post(t, transfer(cash, ali, 1000, uuid.NewString()))
	posted := f.post(t, transfer(ali, shop, 100, uuid.NewString()))

	_, _, err := f.ledger.Capture(f.ctx, application.CaptureCommand{HoldID: posted.ID, IdempotencyKey: uuid.NewString()})
	if !errdefs.Is(err, domain.CodeTransactionNotPending) {
		t.Fatalf("capture posted: want TRANSACTION_NOT_PENDING, got %v", err)
	}
	_, _, err = f.ledger.Void(f.ctx, application.VoidCommand{HoldID: posted.ID, IdempotencyKey: uuid.NewString()})
	if !errdefs.Is(err, domain.CodeTransactionNotPending) {
		t.Fatalf("void posted: want TRANSACTION_NOT_PENDING, got %v", err)
	}
}

func TestVoid_ReleasesHold(t *testing.T) {
	f := setup(t)
	cash := f.account(t, domain.AccountAsset)
	ali := f.account(t, domain.AccountLiability)
	shop := f.account(t, domain.AccountLiability)
	f.post(t, transfer(cash, ali, 1000, uuid.NewString()))
	h := f.post(t, hold(ali, shop, 500, uuid.NewString()))

	v, _, err := f.ledger.Void(f.ctx, application.VoidCommand{HoldID: h.ID, IdempotencyKey: uuid.NewString()})
	must(t, err)
	if v.ID != h.ID || v.Status != domain.TransactionStatusVoided {
		t.Fatalf("void: %+v", v)
	}
	a := f.balance(t, ali)
	if a.Posted != 1000 || a.PendingOut != 0 || a.Available != 1000 {
		t.Fatalf("ali after void: %+v", a)
	}
	if s := f.balance(t, shop); s.Posted != 0 || s.PendingIn != 0 {
		t.Fatalf("shop after void: %+v", s)
	}
	_, _, err = f.ledger.Capture(f.ctx, application.CaptureCommand{HoldID: h.ID, IdempotencyKey: uuid.NewString()})
	if !errdefs.Is(err, domain.CodeTransactionNotPending) {
		t.Fatalf("want TRANSACTION_NOT_PENDING, got %v", err)
	}
	f.verifyClean(t)
}

func TestCapture_ReplayReturnsCapture(t *testing.T) {
	f := setup(t)
	cash := f.account(t, domain.AccountAsset)
	ali := f.account(t, domain.AccountLiability)
	shop := f.account(t, domain.AccountLiability)
	f.post(t, transfer(cash, ali, 1000, uuid.NewString()))
	h := f.post(t, hold(ali, shop, 500, uuid.NewString()))
	key := uuid.NewString()

	first, _, err := f.ledger.Capture(f.ctx, application.CaptureCommand{HoldID: h.ID, Amount: 300, IdempotencyKey: key})
	must(t, err)
	second, replayed, err := f.ledger.Capture(f.ctx, application.CaptureCommand{HoldID: h.ID, Amount: 300, IdempotencyKey: key})
	must(t, err)
	if !replayed || second.ID != first.ID {
		t.Fatalf("replay=%v same=%v", replayed, second.ID == first.ID)
	}
	if b := f.balance(t, shop); b.Posted != 300 {
		t.Fatalf("replay double-captured: %d", b.Posted)
	}
}

func TestCapture_ConcurrentCapturesOnlyOneWins(t *testing.T) {
	f := setup(t)
	cash := f.account(t, domain.AccountAsset)
	ali := f.account(t, domain.AccountLiability)
	shop := f.account(t, domain.AccountLiability)
	f.post(t, transfer(cash, ali, 1000, uuid.NewString()))
	h := f.post(t, hold(ali, shop, 500, uuid.NewString()))

	var (
		wg      sync.WaitGroup
		mu      sync.Mutex
		ok, bad int
	)
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _, err := f.ledger.Capture(f.ctx, application.CaptureCommand{HoldID: h.ID, IdempotencyKey: uuid.NewString()})
			mu.Lock()
			defer mu.Unlock()
			switch {
			case err == nil:
				ok++
			case errdefs.Is(err, domain.CodeTransactionNotPending):
				bad++
			default:
				t.Errorf("unexpected: %v", err)
			}
		}()
	}
	wg.Wait()
	if ok != 1 || bad != 4 {
		t.Fatalf("want 1 capture / 4 rejected, got %d / %d", ok, bad)
	}
	if b := f.balance(t, shop); b.Posted != 500 {
		t.Fatalf("shop posted = %d, want 500", b.Posted)
	}
	f.verifyClean(t)
}

func TestCapture_TwiceThenExhausted(t *testing.T) {
	f := setup(t)
	cash := f.account(t, domain.AccountAsset)
	ali := f.account(t, domain.AccountLiability)
	shop := f.account(t, domain.AccountLiability)
	f.post(t, transfer(cash, ali, 1000, uuid.NewString()))
	h := f.post(t, hold(ali, shop, 500, uuid.NewString()))

	_, _, err := f.ledger.Capture(f.ctx, application.CaptureCommand{HoldID: h.ID, Amount: 300, IdempotencyKey: uuid.NewString()})
	must(t, err)
	_, _, err = f.ledger.Capture(f.ctx, application.CaptureCommand{HoldID: h.ID, IdempotencyKey: uuid.NewString()})
	must(t, err)

	holdNow, err := f.ledger.GetTransaction(f.ctx, h.ID)
	must(t, err)
	if holdNow.Status != domain.TransactionStatusVoided {
		t.Fatalf("fully captured hold should be voided, got %s", holdNow.Status)
	}
	a := f.balance(t, ali)
	if a.Posted != 500 || a.PendingOut != 0 || a.Available != 500 {
		t.Fatalf("ali: %+v", a)
	}
	if s := f.balance(t, shop); s.Posted != 500 || s.PendingIn != 0 {
		t.Fatalf("shop: %+v", s)
	}
	_, _, err = f.ledger.Capture(f.ctx, application.CaptureCommand{HoldID: h.ID, Amount: 1, IdempotencyKey: uuid.NewString()})
	if !errdefs.Is(err, domain.CodeTransactionNotPending) {
		t.Fatalf("want TRANSACTION_NOT_PENDING, got %v", err)
	}
	f.verifyClean(t)
}

func TestVoid_AfterPartialReleasesRest(t *testing.T) {
	f := setup(t)
	cash := f.account(t, domain.AccountAsset)
	ali := f.account(t, domain.AccountLiability)
	shop := f.account(t, domain.AccountLiability)
	f.post(t, transfer(cash, ali, 1000, uuid.NewString()))
	h := f.post(t, hold(ali, shop, 500, uuid.NewString()))

	_, _, err := f.ledger.Capture(f.ctx, application.CaptureCommand{HoldID: h.ID, Amount: 300, IdempotencyKey: uuid.NewString()})
	must(t, err)
	_, _, err = f.ledger.Void(f.ctx, application.VoidCommand{HoldID: h.ID, IdempotencyKey: uuid.NewString()})
	must(t, err)

	a := f.balance(t, ali)
	if a.Posted != 700 || a.PendingOut != 0 || a.Available != 700 {
		t.Fatalf("ali: captured 300 stays, rest released: %+v", a)
	}
	if s := f.balance(t, shop); s.Posted != 300 || s.PendingIn != 0 {
		t.Fatalf("shop: %+v", s)
	}
	f.verifyClean(t)
}

// ---- story 14: expiry ----

func TestExpireHolds_VoidsPastDue(t *testing.T) {
	f := setup(t)
	cash := f.account(t, domain.AccountAsset)
	ali := f.account(t, domain.AccountLiability)
	shop := f.account(t, domain.AccountLiability)
	f.post(t, transfer(cash, ali, 1000, uuid.NewString()))
	h := f.post(t, holdExpiring(ali, shop, 500, time.Now().Add(-time.Minute)))

	n, err := f.ledger.ExpireHolds(f.ctx, time.Now(), 100)
	must(t, err)
	if n < 1 {
		t.Fatalf("expired %d, want at least 1", n)
	}
	got, err := f.ledger.GetTransaction(f.ctx, h.ID)
	must(t, err)
	if got.Status != domain.TransactionStatusVoided {
		t.Fatalf("status = %s, want voided", got.Status)
	}
	if b := f.balance(t, ali); b.PendingOut != 0 || b.Available != 1000 {
		t.Fatalf("reservation not released: %+v", b)
	}
	f.verifyClean(t)
}

func TestExpireHolds_LeavesFutureAlone(t *testing.T) {
	f := setup(t)
	cash := f.account(t, domain.AccountAsset)
	ali := f.account(t, domain.AccountLiability)
	shop := f.account(t, domain.AccountLiability)
	f.post(t, transfer(cash, ali, 1000, uuid.NewString()))
	h := f.post(t, holdExpiring(ali, shop, 500, time.Now().Add(time.Hour)))

	_, err := f.ledger.ExpireHolds(f.ctx, time.Now(), 100)
	must(t, err)
	got, err := f.ledger.GetTransaction(f.ctx, h.ID)
	must(t, err)
	if got.Status != domain.TransactionStatusPending {
		t.Fatalf("future hold touched: %s", got.Status)
	}
}

func TestExpireHolds_PartiallyCapturedReleasesRest(t *testing.T) {
	f := setup(t)
	cash := f.account(t, domain.AccountAsset)
	ali := f.account(t, domain.AccountLiability)
	shop := f.account(t, domain.AccountLiability)
	f.post(t, transfer(cash, ali, 1000, uuid.NewString()))
	h := f.post(t, holdExpiring(ali, shop, 500, time.Now().Add(-time.Minute)))
	_, _, err := f.ledger.Capture(f.ctx, application.CaptureCommand{HoldID: h.ID, Amount: 300, IdempotencyKey: uuid.NewString()})
	must(t, err)

	_, err = f.ledger.ExpireHolds(f.ctx, time.Now(), 100)
	must(t, err)
	a := f.balance(t, ali)
	if a.Posted != 700 || a.PendingOut != 0 || a.Available != 700 {
		t.Fatalf("ali: %+v", a)
	}
	f.verifyClean(t)
}

func TestExpireHolds_ConcurrentWorkersVoidEachOnce(t *testing.T) {
	f := setup(t)
	cash := f.account(t, domain.AccountAsset)
	shop := f.account(t, domain.AccountLiability)
	const n = 20
	ids := make([]uuid.UUID, n)
	for i := range ids {
		w := f.account(t, domain.AccountLiability)
		f.post(t, transfer(cash, w, 100, uuid.NewString()))
		ids[i] = f.post(t, holdExpiring(w, shop, 100, time.Now().Add(-time.Minute))).ID
	}

	var wg sync.WaitGroup
	var total atomic.Int64
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				n, err := f.ledger.ExpireHolds(f.ctx, time.Now(), 5)
				if err != nil {
					t.Errorf("expire: %v", err)
					return
				}
				if n == 0 {
					return
				}
				total.Add(int64(n))
			}
		}()
	}
	wg.Wait()

	for _, id := range ids {
		got, err := f.ledger.GetTransaction(f.ctx, id)
		must(t, err)
		if got.Status != domain.TransactionStatusVoided {
			t.Fatalf("hold %s not voided", id)
		}
	}
	f.verifyClean(t)
}

func TestExpireHolds_LeavesRailBackedAlone(t *testing.T) {
	f := setup(t)
	h := cardHold(t, f)

	_, err := f.ledger.ExpireHolds(f.ctx, h.ExpiresAt.Add(time.Minute), 1000)
	must(t, err)
	got, err := f.ledger.GetTransaction(f.ctx, h.ID)
	must(t, err)
	if got.Status != domain.TransactionStatusPending {
		t.Fatalf("rail-backed hold expired by the clock: %s", got.Status)
	}
}

// ---- story 15: rails ----

func TestPost_UnknownRail(t *testing.T) {
	f := setup(t)
	cash := f.account(t, domain.AccountAsset)
	ali := f.account(t, domain.AccountLiability)

	x := transfer(cash, ali, 100, uuid.NewString())
	x.Rail = "carrier-pigeon"
	_, _, err := f.ledger.Post(f.ctx, x)
	if !errdefs.Is(err, domain.CodeUnknownRail) {
		t.Fatalf("want UNKNOWN_RAIL, got %v", err)
	}
}

func TestPost_InternalRailIsImmediate(t *testing.T) {
	f := setup(t)
	cash := f.account(t, domain.AccountAsset)
	ali := f.account(t, domain.AccountLiability)

	got := f.post(t, transfer(cash, ali, 100, uuid.NewString()))
	if got.Rail != domain.RailInternal || got.RailRef == "" || got.Status != domain.TransactionStatusPosted {
		t.Fatalf("%+v", got)
	}
}

func TestPost_DeferredRailLandsPending(t *testing.T) {
	deferred := fakeRail{
		name:  "fake",
		props: ports.RailProperties{SettlementDelay: 48 * time.Hour, CanBounce: true},
		res:   ports.RailInitiateResult{Ref: "prov-123", Settlement: ports.SettlementDeferred},
	}
	f := setupWith(t, connectTestDB(t), fakeRegistry{"internal": instantInternal, "fake": deferred})
	cash := f.account(t, domain.AccountAsset)
	ali := f.account(t, domain.AccountLiability)

	x := transfer(cash, ali, 100, uuid.NewString())
	x.Kind, x.Rail = kindTopup, "fake"
	got := f.post(t, x)

	if got.Status != domain.TransactionStatusPending || got.RailRef != "prov-123" {
		t.Fatalf("%+v", got)
	}
	if got.ExpiresAt == nil {
		t.Fatal("deferred rail must set an expiry")
	}
	want := time.Now().Add(48*time.Hour + domain.SettlementGrace)
	if d := got.ExpiresAt.Sub(want); d < -time.Minute || d > time.Minute {
		t.Fatalf("expires_at = %v, want ≈ %v", got.ExpiresAt, want)
	}
	if b := f.balance(t, ali); b.PendingIn != 100 || b.Posted != 0 {
		t.Fatalf("ali: %+v", b)
	}
	f.verifyClean(t)
}

func TestPost_RailFailureRejects(t *testing.T) {
	broken := fakeRail{name: "fake", err: errors.New("provider down")}
	f := setupWith(t, connectTestDB(t), fakeRegistry{"internal": instantInternal, "fake": broken})
	cash := f.account(t, domain.AccountAsset)
	ali := f.account(t, domain.AccountLiability)

	x := transfer(cash, ali, 100, uuid.NewString())
	x.Rail = "fake"
	_, _, err := f.ledger.Post(f.ctx, x)
	if !errdefs.Is(err, domain.CodeRailFailed) {
		t.Fatalf("want RAIL_FAILED, got %v", err)
	}
	if b := f.balance(t, ali); b.Posted != 0 || b.PendingIn != 0 {
		t.Fatalf("nothing should have been written: %+v", b)
	}
}

func TestPost_RejectedPostNeverReachesRail(t *testing.T) {
	broken := fakeRail{name: "fake", err: errors.New("rail must not be called")}
	f := setupWith(t, connectTestDB(t), fakeRegistry{"internal": instantInternal, "fake": broken})
	ali := f.account(t, domain.AccountLiability)
	shop := f.account(t, domain.AccountLiability)

	x := transfer(ali, shop, 100, uuid.NewString())
	x.Rail = "fake"
	_, _, err := f.ledger.Post(f.ctx, x)
	if !errdefs.Is(err, domain.CodeInsufficientFunds) {
		t.Fatalf("want INSUFFICIENT_FUNDS before the rail is asked, got %v", err)
	}
}

func TestReverse_IrreversibleRailRejected(t *testing.T) {
	oneWay := fakeRail{
		name:  "fake",
		props: ports.RailProperties{Instant: true, Reversible: false},
		res:   ports.RailInitiateResult{Ref: "r-1", Settlement: ports.SettlementImmediate},
	}
	f := setupWith(t, connectTestDB(t), fakeRegistry{"internal": instantInternal, "fake": oneWay})
	cash := f.account(t, domain.AccountAsset)
	ali := f.account(t, domain.AccountLiability)

	x := transfer(cash, ali, 100, uuid.NewString())
	x.Kind, x.Rail = kindTopup, "fake"
	posted := f.post(t, x)

	_, _, err := f.ledger.Reverse(f.ctx, application.ReverseCommand{Kind: string(kindRefund), OriginalID: posted.ID, IdempotencyKey: uuid.NewString()})
	if !errdefs.Is(err, domain.CodeRailNotReversible) {
		t.Fatalf("want RAIL_NOT_REVERSIBLE, got %v", err)
	}
}

func TestReverse_RailRefusalLeavesReservationForReconciler(t *testing.T) {
	refusing := fakeRail{
		name:   "fake",
		props:  ports.RailProperties{Instant: true, Reversible: true},
		res:    ports.RailInitiateResult{Ref: "r-1", Settlement: ports.SettlementImmediate},
		revErr: errors.New("provider refused"),
	}
	f := setupWith(t, connectTestDB(t), fakeRegistry{"internal": instantInternal, "fake": refusing})
	cash := f.account(t, domain.AccountAsset)
	ali := f.account(t, domain.AccountLiability)

	x := transfer(cash, ali, 100, uuid.NewString())
	x.Kind, x.Rail = kindTopup, "fake"
	posted := f.post(t, x)

	refund, _, err := f.ledger.Reverse(f.ctx, application.ReverseCommand{Kind: string(kindRefund), OriginalID: posted.ID, IdempotencyKey: uuid.NewString()})
	if !errdefs.Is(err, domain.CodeRailFailed) {
		t.Fatalf("want RAIL_FAILED, got %v", err)
	}
	// the pending refund was committed before the rail was asked; it stays
	// reserved with no outcome, which is exactly what the reconciler flags
	if refund == nil || refund.Status != domain.TransactionStatusPending {
		t.Fatalf("refund should be returned pending: %+v", refund)
	}
	if b := f.balance(t, ali); b.Posted != 100 || b.PendingOut != 100 || b.Available != 0 {
		t.Fatalf("reservation must remain: %+v", b)
	}
	f.verifyClean(t)
}

// ---- story 16: RTP rail ----

func TestPost_RaastIsImmediateAndFinal(t *testing.T) {
	f := setup(t)
	cash := f.account(t, domain.AccountAsset)
	ali := f.account(t, domain.AccountLiability)

	x := transfer(cash, ali, 100, uuid.NewString())
	x.Kind, x.Rail = kindTopup, "raast"
	got := f.post(t, x)

	if got.Status != domain.TransactionStatusPosted || !strings.HasPrefix(got.RailRef, "RAAST-") {
		t.Fatalf("%+v", got)
	}
	if b := f.balance(t, ali); b.Posted != 100 || b.PendingIn != 0 {
		t.Fatalf("ali: %+v", b)
	}

	_, _, err := f.ledger.Reverse(f.ctx, application.ReverseCommand{Kind: string(kindRefund), OriginalID: got.ID, IdempotencyKey: uuid.NewString()})
	if !errdefs.Is(err, domain.CodeRailNotReversible) {
		t.Fatalf("want RAIL_NOT_REVERSIBLE, got %v", err)
	}
}

func TestPost_RTPRailRejectsWrongCurrency(t *testing.T) {
	f := setup(t)
	usdCash, err := f.accounts.CreateAccount(f.ctx, domain.Account{
		OwnerType: domain.OwnerSystem, OwnerID: uuid.NewString(), Currency: "USD", Type: domain.AccountAsset,
	})
	must(t, err)
	usdAli, err := f.accounts.CreateAccount(f.ctx, domain.Account{
		OwnerType: domain.OwnerCustomer, OwnerID: uuid.NewString(), Currency: "USD", Type: domain.AccountLiability,
	})
	must(t, err)

	x := transfer(usdCash.ID, usdAli.ID, 100, uuid.NewString())
	x.Kind, x.Rail = kindTopup, "raast"
	_, _, err = f.ledger.Post(f.ctx, x)
	if !errdefs.Is(err, domain.CodeRailFailed) {
		t.Fatalf("want RAIL_FAILED, got %v", err)
	}
}

// ---- stories 17–18: card and swift land pending ----

func TestPost_CardTopupLandsPending(t *testing.T) {
	f := setup(t)
	h := cardHold(t, f)
	if h.Status != domain.TransactionStatusPending || !strings.HasPrefix(h.RailRef, "CARD-") || h.ExpiresAt == nil {
		t.Fatalf("%+v", h)
	}
	f.verifyClean(t)
}

func TestVoidAndCapture_RailManagedHold(t *testing.T) {
	f := setup(t)
	h := cardHold(t, f)

	// capture is the rail's job
	_, _, err := f.ledger.Capture(f.ctx, application.CaptureCommand{HoldID: h.ID, IdempotencyKey: uuid.NewString()})
	if !errdefs.Is(err, domain.CodeRailManagedHold) {
		t.Fatalf("capture: want RAIL_MANAGED_HOLD, got %v", err)
	}
	// void goes through the rail's cancel: the hold comes back still pending
	v, _, err := f.ledger.Void(f.ctx, application.VoidCommand{HoldID: h.ID, IdempotencyKey: uuid.NewString()})
	must(t, err)
	if v.Status != domain.TransactionStatusPending {
		t.Fatalf("void must not resolve a rail hold directly: %s", v.Status)
	}
}

// ---- story 21: two-phase outcomes ----

func TestOutcomes_CardTopupSettles(t *testing.T) {
	f := setup(t)
	f.drain(t)
	h := cardHold(t, f)
	ali := h.Entries[1].AccountID

	f.applyAll(t, time.Now())
	if got, _ := f.ledger.GetTransaction(f.ctx, h.ID); got.Status != domain.TransactionStatusPending {
		t.Fatalf("settled early: %s", got.Status)
	}

	st := f.applyAll(t, time.Now().Add(72*time.Hour))
	if st.Diverged != 0 {
		t.Fatalf("divergence: %+v", st)
	}
	got, err := f.ledger.GetTransaction(f.ctx, h.ID)
	must(t, err)
	if got.Status != domain.TransactionStatusVoided {
		t.Fatalf("hold after settle = %s", got.Status)
	}
	if b := f.balance(t, ali); b.Posted != 5000 || b.PendingIn != 0 || b.Available != 5000 {
		t.Fatalf("ali: %+v", b)
	}
	f.verifyClean(t)
}

func TestOutcomes_SwiftBounceVoidsWithReason(t *testing.T) {
	f := setup(t)
	f.drain(t)
	cash := f.account(t, domain.AccountAsset)
	ali := f.account(t, domain.AccountLiability)
	x := transfer(cash, ali, 1099, uuid.NewString()) // magic amount bounces
	x.Kind, x.Rail = kindTopup, "swift"
	h := f.post(t, x)

	st := f.applyAll(t, time.Now().Add(96*time.Hour))
	if st.Bounced < 1 || st.Diverged != 0 {
		t.Fatalf("%+v", st)
	}
	got, err := f.ledger.GetTransaction(f.ctx, h.ID)
	must(t, err)
	if got.Status != domain.TransactionStatusVoided || got.StatusReason == nil || !strings.HasPrefix(*got.StatusReason, "bounced:") {
		t.Fatalf("%+v", got)
	}
	if b := f.balance(t, ali); b.PendingIn != 0 || b.Posted != 0 {
		t.Fatalf("reservation not released: %+v", b)
	}
	f.verifyClean(t)
}

func TestOutcomes_SettledSwiftIsFinal(t *testing.T) {
	f := setup(t)
	cash := f.account(t, domain.AccountAsset)
	ali := f.account(t, domain.AccountLiability)
	x := transfer(cash, ali, 1000, uuid.NewString())
	x.Kind, x.Rail = kindTopup, "swift"
	h := f.post(t, x)
	f.applyAll(t, time.Now().Add(96*time.Hour))

	if b := f.balance(t, ali); b.Posted != 1000 {
		t.Fatalf("ali: %+v", b)
	}
	kids, err := f.ledger.Children(f.ctx, h.ID)
	must(t, err)
	if len(kids) != 1 {
		t.Fatalf("children = %d", len(kids))
	}
	_, _, err = f.ledger.Reverse(f.ctx, application.ReverseCommand{Kind: string(kindRefund), OriginalID: kids[0].ID, IdempotencyKey: uuid.NewString()})
	if !errdefs.Is(err, domain.CodeRailNotReversible) {
		t.Fatalf("want RAIL_NOT_REVERSIBLE, got %v", err)
	}
}

func TestOutcomes_VoidOnCardHoldGoesThroughCancel(t *testing.T) {
	f := setup(t)
	f.drain(t)
	h := cardHold(t, f)
	ali := h.Entries[1].AccountID

	_, _, err := f.ledger.Void(f.ctx, application.VoidCommand{HoldID: h.ID, IdempotencyKey: uuid.NewString()})
	must(t, err)

	st := f.applyAll(t, time.Now())
	if st.Cancelled != 1 || st.Diverged != 0 {
		t.Fatalf("%+v", st)
	}
	got, err := f.ledger.GetTransaction(f.ctx, h.ID)
	must(t, err)
	if got.Status != domain.TransactionStatusVoided || got.StatusReason == nil || *got.StatusReason != "cancelled at rail" {
		t.Fatalf("%+v", got)
	}
	if b := f.balance(t, ali); b.PendingIn != 0 || b.Posted != 0 {
		t.Fatalf("ali: %+v", b)
	}

	// the superseded settle outcome never touches it later
	st = f.applyAll(t, time.Now().Add(72*time.Hour))
	if st.Settled != 0 || st.Diverged != 0 {
		t.Fatalf("cancelled hold touched later: %+v", st)
	}
	f.verifyClean(t)
}

func TestOutcomes_SwiftHoldCannotBeCancelled(t *testing.T) {
	f := setup(t)
	cash := f.account(t, domain.AccountAsset)
	ali := f.account(t, domain.AccountLiability)
	x := transfer(cash, ali, 1000, uuid.NewString())
	x.Kind, x.Rail = kindTopup, "swift"
	h := f.post(t, x)

	_, _, err := f.ledger.Void(f.ctx, application.VoidCommand{HoldID: h.ID, IdempotencyKey: uuid.NewString()})
	if !errdefs.Is(err, domain.CodeRailCannotCancel) {
		t.Fatalf("want RAIL_CANNOT_CANCEL, got %v", err)
	}
}

func TestOutcomes_CardRefundsArePendingThenSettle(t *testing.T) {
	f := setup(t)
	f.drain(t)
	h := cardHold(t, f)
	ali := h.Entries[1].AccountID
	f.applyAll(t, time.Now().Add(72*time.Hour))
	kids, err := f.ledger.Children(f.ctx, h.ID)
	must(t, err)
	settled := kids[0]

	r1, _, err := f.ledger.Reverse(f.ctx, application.ReverseCommand{Kind: string(kindRefund), OriginalID: settled.ID, Amount: 3000, IdempotencyKey: uuid.NewString()})
	must(t, err)
	r2, _, err := f.ledger.Reverse(f.ctx, application.ReverseCommand{Kind: string(kindRefund), OriginalID: settled.ID, Amount: 1000, IdempotencyKey: uuid.NewString()})
	must(t, err)
	if r1.Status != domain.TransactionStatusPending || r2.Status != domain.TransactionStatusPending {
		t.Fatalf("rail refunds must be pending: %s %s", r1.Status, r2.Status)
	}
	if r1.Rail != "card" || r1.RailRef != settled.RailRef {
		t.Fatalf("refund must carry the original's rail: %+v", r1)
	}
	if b := f.balance(t, ali); b.Posted != 5000 || b.PendingOut != 4000 || b.Available != 1000 {
		t.Fatalf("ali while refunds pending: %+v", b)
	}
	// pending refunds already count: a third that would exceed is rejected
	_, _, err = f.ledger.Reverse(f.ctx, application.ReverseCommand{Kind: string(kindRefund), OriginalID: settled.ID, Amount: 1500, IdempotencyKey: uuid.NewString()})
	if !errdefs.Is(err, domain.CodeReversalExceedsOriginal) {
		t.Fatalf("want REVERSAL_EXCEEDS_ORIGINAL, got %v", err)
	}

	st := f.applyAll(t, time.Now())
	if st.Refunded != 2 || st.Diverged != 0 {
		t.Fatalf("%+v", st)
	}
	if b := f.balance(t, ali); b.Posted != 1000 || b.PendingOut != 0 || b.Available != 1000 {
		t.Fatalf("ali after refunds: %+v", b)
	}
	f.verifyClean(t)
}

func TestOutcomes_CrashBetweenPhasesIsFinishedNextRun(t *testing.T) {
	f := setup(t)
	f.drain(t)
	h := cardHold(t, f)
	ali := h.Entries[1].AccountID

	// phase 1 happened (the rail decided), phase 2 "crashed" before applying
	rail, _ := rails.New(connectTestDB(t)).Get("card")
	must(t, rail.Settle(f.ctx, h.RailRef))

	// decided-but-unapplied is due regardless of settle_after
	st := f.applyAll(t, time.Now())
	if st.Settled != 1 || st.Diverged != 0 {
		t.Fatalf("%+v", st)
	}
	if b := f.balance(t, ali); b.Posted != 5000 || b.PendingIn != 0 {
		t.Fatalf("ali: %+v", b)
	}
	f.verifyClean(t)
}

func TestOutcomes_TwoWorkersApplyEachOnce(t *testing.T) {
	f := setup(t)
	f.drain(t)
	cash := f.account(t, domain.AccountAsset)
	const n = 20
	ids := make([]uuid.UUID, n)
	for i := range ids {
		w := f.account(t, domain.AccountLiability)
		x := transfer(cash, w, 100, uuid.NewString())
		x.Kind, x.Rail = kindTopup, "card"
		ids[i] = f.post(t, x).ID
	}

	later := time.Now().Add(72 * time.Hour)
	var wg sync.WaitGroup
	var diverged atomic.Int64
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				st, err := f.ledger.ApplyOutcomes(f.ctx, later, 5)
				if err != nil {
					t.Errorf("apply: %v", err)
					return
				}
				diverged.Add(int64(st.Diverged))
				if st.Claimed == 0 {
					return
				}
			}
		}()
	}
	wg.Wait()
	if diverged.Load() != 0 {
		t.Fatalf("divergence under two workers: %d", diverged.Load())
	}
	for _, id := range ids {
		got, err := f.ledger.GetTransaction(f.ctx, id)
		must(t, err)
		if got.Status != domain.TransactionStatusVoided {
			t.Fatalf("hold %s not settled", id)
		}
	}
	f.verifyClean(t)
}

func TestOutcomes_SwiftVoidRefusalDoesNotSpendKey(t *testing.T) {
	f := setup(t)
	cash := f.account(t, domain.AccountAsset)
	ali := f.account(t, domain.AccountLiability)
	x := transfer(cash, ali, 1000, uuid.NewString())
	x.Kind, x.Rail = kindTopup, "swift"
	h := f.post(t, x)

	cmd := application.VoidCommand{HoldID: h.ID, IdempotencyKey: uuid.NewString()}
	for i := 0; i < 2; i++ {
		// a spent key would replay the hold with no error on the second call
		if _, _, err := f.ledger.Void(f.ctx, cmd); !errdefs.Is(err, domain.CodeRailCannotCancel) {
			t.Fatalf("call %d: want RAIL_CANNOT_CANCEL, got %v", i+1, err)
		}
	}
}

func TestOutcomes_CancelLosingToSettlementIsAConflict(t *testing.T) {
	f := setup(t)
	f.drain(t)
	h := cardHold(t, f)

	rail, _ := rails.New(connectTestDB(t)).Get("card")
	must(t, rail.Settle(f.ctx, h.RailRef))

	_, _, err := f.ledger.Void(f.ctx, application.VoidCommand{HoldID: h.ID, IdempotencyKey: uuid.NewString()})
	if !errdefs.Is(err, domain.CodeHoldAlreadySettling) {
		t.Fatalf("want HOLD_ALREADY_SETTLING, got %v", err)
	}
	if st := f.applyAll(t, time.Now()); st.Settled != 1 || st.Diverged != 0 {
		t.Fatalf("the settlement must still apply: %+v", st)
	}
	f.verifyClean(t)
}

func deferredFake(calls *railCalls) fakeRail {
	return fakeRail{
		name:  "fake",
		props: ports.RailProperties{SettlementDelay: time.Hour, Reversible: true, CanCancel: true},
		res:   ports.RailInitiateResult{Ref: "r-" + uuid.NewString(), Settlement: ports.SettlementDeferred},
		calls: calls,
	}
}

func TestVoid_ReplayAfterRailTimeoutAsksAgain(t *testing.T) {
	calls := &railCalls{fail: 1}
	f := setupWith(t, connectTestDB(t), fakeRegistry{"internal": instantInternal, "fake": deferredFake(calls)})
	cash := f.account(t, domain.AccountAsset)
	ali := f.account(t, domain.AccountLiability)
	x := transfer(cash, ali, 100, uuid.NewString())
	x.Kind, x.Rail = kindTopup, "fake"
	h := f.post(t, x)

	cmd := application.VoidCommand{HoldID: h.ID, IdempotencyKey: uuid.NewString()}
	if _, _, err := f.ledger.Void(f.ctx, cmd); !errdefs.Is(err, domain.CodeRailFailed) {
		t.Fatalf("first call: want RAIL_FAILED, got %v", err)
	}
	got, replayed, err := f.ledger.Void(f.ctx, cmd)
	must(t, err)
	if !replayed || got.Status != domain.TransactionStatusPending || calls.cancel != 2 {
		t.Fatalf("retry must reach the rail again: replayed=%v status=%s cancel calls=%d", replayed, got.Status, calls.cancel)
	}
}

func TestReverse_ReplayAfterRailTimeoutAsksAgain(t *testing.T) {
	calls := &railCalls{fail: 1}
	instantFake := fakeRail{
		name:  "fake",
		props: ports.RailProperties{Instant: true, Reversible: true},
		res:   ports.RailInitiateResult{Ref: "r-1", Settlement: ports.SettlementImmediate},
		calls: calls,
	}
	f := setupWith(t, connectTestDB(t), fakeRegistry{"internal": instantInternal, "fake": instantFake})
	cash := f.account(t, domain.AccountAsset)
	ali := f.account(t, domain.AccountLiability)
	x := transfer(cash, ali, 100, uuid.NewString())
	x.Kind, x.Rail = kindTopup, "fake"
	posted := f.post(t, x)

	cmd := application.ReverseCommand{Kind: string(kindRefund), OriginalID: posted.ID, IdempotencyKey: uuid.NewString()}
	if _, _, err := f.ledger.Reverse(f.ctx, cmd); !errdefs.Is(err, domain.CodeRailFailed) {
		t.Fatalf("first call: want RAIL_FAILED, got %v", err)
	}
	got, replayed, err := f.ledger.Reverse(f.ctx, cmd)
	must(t, err)
	if !replayed || got.Status != domain.TransactionStatusPending || calls.reverse != 2 {
		t.Fatalf("retry must reach the rail again: replayed=%v status=%s reverse calls=%d", replayed, got.Status, calls.reverse)
	}
	if b := f.balance(t, ali); b.PendingOut != 100 || b.Available != 0 {
		t.Fatalf("refund must stay reserved until the rail decides: %+v", b)
	}
	f.verifyClean(t)
}

// ---- story 21b: refund settlement ----

// settledCardPayment returns the posted capture child of a settled card topup
// and the liability account it credited.
func settledCardPayment(t *testing.T, f fixture) (*domain.Transaction, uuid.UUID) {
	t.Helper()
	h := cardHold(t, f)
	f.applyAll(t, time.Now().Add(72*time.Hour))
	kids, err := f.ledger.Children(f.ctx, h.ID)
	must(t, err)
	if len(kids) != 1 {
		t.Fatalf("children = %d", len(kids))
	}
	return &kids[0], h.Entries[1].AccountID
}

func TestRefund_SettlesInPlace(t *testing.T) {
	f := setup(t)
	f.drain(t)
	settled, _ := settledCardPayment(t, f)

	r, _, err := f.ledger.Reverse(f.ctx, application.ReverseCommand{Kind: string(kindRefund), OriginalID: settled.ID, Amount: 2000, IdempotencyKey: uuid.NewString()})
	must(t, err)
	if st := f.applyAll(t, time.Now()); st.Refunded != 1 || st.Diverged != 0 {
		t.Fatalf("%+v", st)
	}

	got, err := f.ledger.GetTransaction(f.ctx, r.ID)
	must(t, err)
	if got.Status != domain.TransactionStatusPosted || got.PostedAt == nil {
		t.Fatalf("refund child must post in place: %+v", got)
	}
	if kids, _ := f.ledger.Children(f.ctx, r.ID); len(kids) != 0 {
		t.Fatalf("refund child must have no children, has %d", len(kids))
	}
	f.verifyClean(t)
}

func TestRefund_SettledRefundsKeepCounting(t *testing.T) {
	f := setup(t)
	f.drain(t)
	settled, ali := settledCardPayment(t, f)

	_, _, err := f.ledger.Reverse(f.ctx, application.ReverseCommand{Kind: string(kindRefund), OriginalID: settled.ID, Amount: 4000, IdempotencyKey: uuid.NewString()})
	must(t, err)
	f.applyAll(t, time.Now())

	// enough money that only the refund limit, not funds, can stop the next one
	cash := f.account(t, domain.AccountAsset)
	f.post(t, transfer(cash, ali, 10000, uuid.NewString()))

	_, _, err = f.ledger.Reverse(f.ctx, application.ReverseCommand{Kind: string(kindRefund), OriginalID: settled.ID, Amount: 1001, IdempotencyKey: uuid.NewString()})
	if !errdefs.Is(err, domain.CodeReversalExceedsOriginal) {
		t.Fatalf("5000 settled, 4000 refunded: 1001 more must be rejected, got %v", err)
	}
	_, _, err = f.ledger.Reverse(f.ctx, application.ReverseCommand{Kind: string(kindRefund), OriginalID: settled.ID, IdempotencyKey: uuid.NewString()})
	must(t, err) // the remaining 1000 is still refundable
	f.verifyClean(t)
}

func TestVoid_PendingRefundChildCannotBeCancelled(t *testing.T) {
	f := setup(t)
	f.drain(t)
	settled, _ := settledCardPayment(t, f)

	r, _, err := f.ledger.Reverse(f.ctx, application.ReverseCommand{Kind: string(kindRefund), OriginalID: settled.ID, Amount: 500, IdempotencyKey: uuid.NewString()})
	must(t, err)
	_, _, err = f.ledger.Void(f.ctx, application.VoidCommand{HoldID: r.ID, IdempotencyKey: uuid.NewString()})
	if !errdefs.Is(err, domain.CodeRailCannotCancel) {
		t.Fatalf("want RAIL_CANNOT_CANCEL, got %v", err)
	}
}

func TestSummary_SettlementLabels(t *testing.T) {
	f := setup(t)
	f.drain(t)
	cash := f.account(t, domain.AccountAsset)
	ali := f.account(t, domain.AccountLiability)
	shop := f.account(t, domain.AccountLiability)
	f.post(t, transfer(cash, ali, 10000, uuid.NewString()))

	label := func(id uuid.UUID) string {
		t.Helper()
		got, err := f.ledger.GetTransaction(f.ctx, id)
		must(t, err)
		return got.Summary.Settlement
	}

	card := cardHold(t, f)
	f.applyAll(t, time.Now().Add(72*time.Hour))
	if l := label(card.ID); l != "settled_via_children" {
		t.Fatalf("settled card hold: %s", l)
	}

	partial := f.post(t, hold(ali, shop, 500, uuid.NewString()))
	_, _, err := f.ledger.Capture(f.ctx, application.CaptureCommand{HoldID: partial.ID, Amount: 300, IdempotencyKey: uuid.NewString()})
	must(t, err)
	_, _, err = f.ledger.Void(f.ctx, application.VoidCommand{HoldID: partial.ID, IdempotencyKey: uuid.NewString()})
	must(t, err)
	if l := label(partial.ID); l != "partially_settled" {
		t.Fatalf("partly captured then voided: %s", l)
	}

	cancelled := f.post(t, hold(ali, shop, 200, uuid.NewString()))
	_, _, err = f.ledger.Void(f.ctx, application.VoidCommand{HoldID: cancelled.ID, IdempotencyKey: uuid.NewString()})
	must(t, err)
	if l := label(cancelled.ID); l != "voided" {
		t.Fatalf("voided with no captures: %s", l)
	}

	if l := label(f.post(t, transfer(ali, shop, 100, uuid.NewString())).ID); l != "posted" {
		t.Fatalf("plain transfer: %s", l)
	}
}

func TestHistory_CarriesPostedAt(t *testing.T) {
	f := setup(t)
	cash := f.account(t, domain.AccountAsset)
	ali := f.account(t, domain.AccountLiability)
	shop := f.account(t, domain.AccountLiability)
	f.post(t, transfer(cash, ali, 1000, uuid.NewString()))
	f.post(t, hold(ali, shop, 300, uuid.NewString()))

	h, err := f.accounts.History(f.ctx, ali, 0, 10)
	must(t, err)
	if len(h.Entries) != 2 {
		t.Fatalf("entries = %d", len(h.Entries))
	}
	pending, posted := h.Entries[0], h.Entries[1] // newest first
	if pending.PostedAt != nil || posted.PostedAt == nil {
		t.Fatalf("pending posted_at=%v, posted posted_at=%v", pending.PostedAt, posted.PostedAt)
	}
}

// ---- story 21c: reconciliation ----

// reconcileLater runs reconciliation an hour ahead, past the grace window, and
// returns the findings that touch ref or id.
func (f fixture) reconcileLater(t *testing.T, ref string, id uuid.UUID) []domain.Finding {
	t.Helper()
	rep, err := f.ledger.Reconcile(f.ctx, time.Now().Add(time.Hour))
	must(t, err)
	var out []domain.Finding
	for _, x := range rep.Findings {
		if x.Kind == domain.FindingTrialBalance {
			t.Fatalf("trial balance does not hold: %s", x.Detail)
		}
		if (ref != "" && x.Ref == ref) || (id != uuid.Nil && x.TransactionID == id) {
			out = append(out, x)
		}
	}
	return out
}

func hasFinding(fs []domain.Finding, kind domain.FindingKind, reason string) bool {
	for _, x := range fs {
		if x.Kind == kind && x.Reason == reason {
			return true
		}
	}
	return false
}

func TestReconcile_OrphanAttempt(t *testing.T) {
	f := setup(t)
	rail, _ := rails.New(connectTestDB(t)).Get("card")
	res, err := rail.Initiate(f.ctx, ports.RailInitiateRequest{TransactionID: uuid.New(), Amount: 700, Currency: "PKR", Kind: "topup"})
	must(t, err)

	rep, err := f.ledger.Reconcile(f.ctx, time.Now())
	must(t, err)
	for _, x := range rep.Findings {
		if x.Ref == res.Ref {
			t.Fatalf("an attempt inside the grace window may be a post in flight: %+v", x)
		}
	}
	if fs := f.reconcileLater(t, res.Ref, uuid.Nil); !hasFinding(fs, domain.FindingOrphanAttempt, "") {
		t.Fatalf("want orphan_attempt, got %+v", fs)
	}
}

func TestReconcile_VoidedButLive(t *testing.T) {
	f := setup(t)
	h := cardHold(t, f)
	// what an HTTP void did before the rail guards: the ledger lets go, the rail does not
	_, _, err := f.ledger.Void(f.ctx, application.VoidCommand{HoldID: h.ID, IdempotencyKey: uuid.NewString(), System: true})
	must(t, err)

	if fs := f.reconcileLater(t, h.RailRef, h.ID); !hasFinding(fs, domain.FindingDivergence, domain.ReasonVoidedButLive) {
		t.Fatalf("want voided_but_live, got %+v", fs)
	}
}

func TestReconcile_SettledNotCredited(t *testing.T) {
	f := setup(t)
	h := cardHold(t, f)
	rail, _ := rails.New(connectTestDB(t)).Get("card")
	must(t, rail.Settle(f.ctx, h.RailRef))
	_, _, err := f.ledger.Void(f.ctx, application.VoidCommand{HoldID: h.ID, IdempotencyKey: uuid.NewString(), System: true})
	must(t, err)

	fs := f.reconcileLater(t, h.RailRef, h.ID)
	if !hasFinding(fs, domain.FindingDivergence, domain.ReasonSettledNotCredited) || !hasFinding(fs, domain.FindingUnapplied, "") {
		t.Fatalf("want settled_not_credited and unapplied, got %+v", fs)
	}
}

func TestReconcile_UnappliedOnlyPastGrace(t *testing.T) {
	f := setup(t)
	h := cardHold(t, f)
	rail, _ := rails.New(connectTestDB(t)).Get("card")
	must(t, rail.Settle(f.ctx, h.RailRef)) // decided, worker not yet run

	rep, err := f.ledger.Reconcile(f.ctx, time.Now())
	must(t, err)
	for _, x := range rep.Findings {
		if x.Ref == h.RailRef {
			t.Fatalf("a fresh decision is in flight, not a finding: %+v", x)
		}
	}
	fs := f.reconcileLater(t, h.RailRef, h.ID)
	if !hasFinding(fs, domain.FindingUnapplied, "") || len(fs) != 1 {
		t.Fatalf("want only unapplied, got %+v", fs)
	}
}

func TestReconcile_Stuck(t *testing.T) {
	f := setup(t)
	h := cardHold(t, f)
	reg := rails.New(connectTestDB(t))
	o := ports.Outcome{Rail: "card", Ref: h.RailRef, Kind: ports.OutcomeSettle}
	for i := 0; i < 10; i++ {
		must(t, reg.RecordFailure(f.ctx, o, errors.New("provider down")))
	}
	if fs := f.reconcileLater(t, h.RailRef, h.ID); !hasFinding(fs, domain.FindingStuck, "") {
		t.Fatalf("want stuck, got %+v", fs)
	}
}

func TestReconcile_RefundNeverSent(t *testing.T) {
	calls := &railCalls{fail: 1000}
	instantFake := fakeRail{
		name:  "fake",
		props: ports.RailProperties{Instant: true, Reversible: true},
		res:   ports.RailInitiateResult{Ref: "r-" + uuid.NewString(), Settlement: ports.SettlementImmediate},
		calls: calls,
	}
	f := setupWith(t, connectTestDB(t), fakeRegistry{"internal": instantInternal, "fake": instantFake})
	cash := f.account(t, domain.AccountAsset)
	ali := f.account(t, domain.AccountLiability)
	x := transfer(cash, ali, 100, uuid.NewString())
	x.Kind, x.Rail = kindTopup, "fake"
	posted := f.post(t, x)

	child, _, err := f.ledger.Reverse(f.ctx, application.ReverseCommand{Kind: string(kindRefund), OriginalID: posted.ID, IdempotencyKey: uuid.NewString()})
	if !errdefs.Is(err, domain.CodeRailFailed) {
		t.Fatalf("want RAIL_FAILED, got %v", err)
	}
	if fs := f.reconcileLater(t, "", child.ID); !hasFinding(fs, domain.FindingDivergence, domain.ReasonRefundNeverSent) {
		t.Fatalf("want refund_never_sent, got %+v", fs)
	}
}

func TestReconcile_HealthyFlowsReportNothing(t *testing.T) {
	f := setup(t)
	f.drain(t)

	settled, _ := settledCardPayment(t, f)
	refund, _, err := f.ledger.Reverse(f.ctx, application.ReverseCommand{Kind: string(kindRefund), OriginalID: settled.ID, Amount: 1500, IdempotencyKey: uuid.NewString()})
	must(t, err)

	cancelled := cardHold(t, f)
	_, _, err = f.ledger.Void(f.ctx, application.VoidCommand{HoldID: cancelled.ID, IdempotencyKey: uuid.NewString()})
	must(t, err)

	cash := f.account(t, domain.AccountAsset)
	ali := f.account(t, domain.AccountLiability)
	x := transfer(cash, ali, 1099, uuid.NewString()) // swift bounce
	x.Kind, x.Rail = kindTopup, "swift"
	bounced := f.post(t, x)

	f.applyAll(t, time.Now().Add(96*time.Hour))

	for _, c := range []struct {
		ref string
		id  uuid.UUID
	}{
		{settled.RailRef, *settled.ParentID}, {"", refund.ID},
		{cancelled.RailRef, cancelled.ID}, {bounced.RailRef, bounced.ID},
	} {
		if fs := f.reconcileLater(t, c.ref, c.id); len(fs) != 0 {
			t.Fatalf("healthy flow reported: %+v", fs)
		}
	}
}

// ---- story 11b: kind is product-owned ----

func TestPost_KindIsProductOwned(t *testing.T) {
	f := setup(t)
	cash := f.account(t, domain.AccountAsset)
	ali := f.account(t, domain.AccountLiability)

	x := transfer(cash, ali, 100, uuid.NewString())
	x.Kind = "pharmacy_sale"
	if got := f.post(t, x); got.Kind != "pharmacy_sale" {
		t.Fatalf("kind = %s, want pharmacy_sale", got.Kind)
	}

	x = transfer(cash, ali, 100, uuid.NewString())
	x.Kind = ""
	if _, _, err := f.ledger.Post(f.ctx, x); !errdefs.Is(err, domain.CodeInvalidTransactionKind) {
		t.Fatalf("want INVALID_TRANSACTION_KIND, got %v", err)
	}
}

func TestReverse_ChildCarriesCallerKindAndRelation(t *testing.T) {
	f := setup(t)
	cash := f.account(t, domain.AccountAsset)
	ali := f.account(t, domain.AccountLiability)
	sara := f.account(t, domain.AccountLiability)
	f.post(t, transfer(cash, ali, 100, uuid.NewString()))
	pay := f.post(t, transfer(ali, sara, 100, uuid.NewString()))

	rev, _, err := f.ledger.Reverse(f.ctx, application.ReverseCommand{
		Kind: "pharmacy_return", OriginalID: pay.ID, IdempotencyKey: uuid.NewString(),
	})
	must(t, err)
	if rev.Kind != "pharmacy_return" || rev.Relation == nil || *rev.Relation != domain.RelationReverses {
		t.Fatalf("%+v", rev)
	}
}

// ---- story 20: summaries and history ----

func TestSummary_TracksCapturesAndReversals(t *testing.T) {
	f := setup(t)
	cash := f.account(t, domain.AccountAsset)
	ali := f.account(t, domain.AccountLiability)
	shop := f.account(t, domain.AccountLiability)
	f.post(t, transfer(cash, ali, 1000, uuid.NewString()))

	h := f.post(t, hold(ali, shop, 500, uuid.NewString()))
	if h.Summary.Total != 500 || h.Summary.Remaining != 500 {
		t.Fatalf("fresh summary: %+v", h.Summary)
	}

	cap, _, err := f.ledger.Capture(f.ctx, application.CaptureCommand{HoldID: h.ID, Amount: 300, IdempotencyKey: uuid.NewString()})
	must(t, err)
	got, err := f.ledger.GetTransaction(f.ctx, h.ID)
	must(t, err)
	if got.Summary.Captured != 300 || got.Summary.Remaining != 200 {
		t.Fatalf("after capture: %+v", got.Summary)
	}

	_, _, err = f.ledger.Reverse(f.ctx, application.ReverseCommand{Kind: string(kindRefund), OriginalID: cap.ID, Amount: 100, IdempotencyKey: uuid.NewString()})
	must(t, err)
	capNow, err := f.ledger.GetTransaction(f.ctx, cap.ID)
	must(t, err)
	if capNow.Summary.Total != 300 || capNow.Summary.Reversed != 100 || capNow.Summary.Remaining != 200 {
		t.Fatalf("capture after partial refund: %+v", capNow.Summary)
	}
}

func TestHistory_PagesNewestFirst(t *testing.T) {
	f := setup(t)
	cash := f.account(t, domain.AccountAsset)
	ali := f.account(t, domain.AccountLiability)
	for i := 0; i < 7; i++ {
		f.post(t, transfer(cash, ali, int64(100+i), uuid.NewString()))
	}

	var seen []int64
	var before int64
	pages := 0
	for {
		h, err := f.accounts.History(f.ctx, ali, before, 3)
		must(t, err)
		pages++
		for _, e := range h.Entries {
			seen = append(seen, e.Seq)
		}
		if h.NextBefore == 0 {
			break
		}
		before = h.NextBefore
	}

	if pages != 3 || len(seen) != 7 {
		t.Fatalf("pages=%d entries=%d", pages, len(seen))
	}
	for i := 1; i < len(seen); i++ {
		if seen[i] >= seen[i-1] {
			t.Fatalf("not strictly descending at %d: %v", i, seen)
		}
	}
	h, err := f.accounts.History(f.ctx, ali, 0, 1)
	must(t, err)
	if h.Entries[0].Amount != 106 || h.Entries[0].Direction != domain.EntryDirectionCredit {
		t.Fatalf("newest entry: %+v", h.Entries[0])
	}
}

func TestHistory_UnknownAccount(t *testing.T) {
	f := setup(t)
	_, err := f.accounts.History(f.ctx, uuid.New(), 0, 10)
	if !errdefs.Is(err, domain.CodeAccountNotFound) {
		t.Fatalf("want ACCOUNT_NOT_FOUND, got %v", err)
	}
}
