package domain

import (
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/irfanali17899/fintech/pkg/errdefs"
)

// product vocabulary used by these tests; the ledger itself has no opinion
const (
	kindTopup  TransactionKind = "topup"
	kindRefund TransactionKind = "refund"
)

func ptr[T any](v T) *T { return &v }

func entry(acc uuid.UUID, amt int64, dir EntryDirection) Entry {
	return Entry{AccountID: acc, Amount: amt, Direction: dir}
}

func validTxn(a, b uuid.UUID) Transaction {
	return Transaction{
		Kind:           kindTopup,
		Status:         TransactionStatusPosted,
		IdempotencyKey: "k1",
		Entries: []Entry{
			entry(a, 1000, EntryDirectionDebit),
			entry(b, 1000, EntryDirectionCredit),
		},
	}
}

func TestTransaction_Validate(t *testing.T) {
	a, b := uuid.New(), uuid.New()

	tests := []struct {
		name     string
		mutate   func(*Transaction)
		wantCode string
	}{
		{"valid posted", func(*Transaction) {}, ""},
		{"valid pending", func(x *Transaction) { x.Status = TransactionStatusPending }, ""},
		{"any product kind is valid", func(x *Transaction) { x.Kind = "pharmacy_sale" }, ""},
		{"empty kind", func(x *Transaction) { x.Kind = "" }, CodeInvalidTransactionKind},
		{"kind too long", func(x *Transaction) { x.Kind = TransactionKind(strings.Repeat("x", 65)) }, CodeInvalidTransactionKind},
		{"empty status", func(x *Transaction) { x.Status = "" }, CodeInvalidTransactionStatus},
		{"voided not allowed as input", func(x *Transaction) { x.Status = TransactionStatusVoided }, CodeInvalidTransactionStatus},
		{"relation without parent", func(x *Transaction) { x.Relation = ptr(RelationReverses) }, CodeRelationWithoutParent},
		{"bad relation", func(x *Transaction) {
			x.ParentID, x.Relation = ptr(uuid.New()), ptr(TransactionRelation("owns"))
		}, CodeInvalidRelation},
		{"valid relation with parent", func(x *Transaction) {
			x.ParentID, x.Relation = ptr(uuid.New()), ptr(RelationCaptures)
		}, ""},
		{"missing key", func(x *Transaction) { x.IdempotencyKey = "" }, CodeIdempotencyKeyRequired},
		{"one entry", func(x *Transaction) { x.Entries = x.Entries[:1] }, CodeTooFewEntries},
		{"bad direction", func(x *Transaction) { x.Entries[0].Direction = "up" }, CodeInvalidEntryDirection},
		{"zero amount", func(x *Transaction) { x.Entries[0].Amount = 0 }, CodeInvalidAmount},
		{"negative amount", func(x *Transaction) { x.Entries[1].Amount = -5 }, CodeInvalidAmount},
		{"amount too large", func(x *Transaction) {
			x.Entries[0].Amount, x.Entries[1].Amount = MaxEntryAmount+1, MaxEntryAmount+1
		}, CodeAmountTooLarge},
		{"unbalanced", func(x *Transaction) { x.Entries[1].Amount = 999 }, CodeTransactionUnbalanced},
		{"account nets to zero", func(x *Transaction) {
			x.Entries = []Entry{
				entry(a, 300, EntryDirectionDebit), entry(a, 300, EntryDirectionCredit),
				entry(b, 100, EntryDirectionDebit), entry(uuid.New(), 100, EntryDirectionCredit),
			}
		}, CodeAccountNetZero},
		{"three legs balanced", func(x *Transaction) {
			x.Entries = []Entry{
				entry(a, 1000, EntryDirectionDebit),
				entry(b, 900, EntryDirectionCredit),
				entry(uuid.New(), 100, EntryDirectionCredit),
			}
		}, ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			x := validTxn(a, b)
			tc.mutate(&x)
			err := x.Validate()
			if tc.wantCode == "" && err != nil {
				t.Fatalf("expected valid, got %v", err)
			}
			if tc.wantCode != "" && !errdefs.Is(err, tc.wantCode) {
				t.Fatalf("want %s, got %v", tc.wantCode, err)
			}
		})
	}
}

func TestTransaction_AccountIDs_SortedUnique(t *testing.T) {
	a, b := uuid.New(), uuid.New()
	x := Transaction{Entries: []Entry{
		entry(b, 1, EntryDirectionDebit), entry(a, 1, EntryDirectionCredit), entry(b, 1, EntryDirectionDebit),
	}}
	ids := x.AccountIDs()
	if len(ids) != 2 {
		t.Fatalf("want 2 unique ids, got %d", len(ids))
	}
	if string(ids[0][:]) > string(ids[1][:]) {
		t.Fatal("ids not sorted")
	}
}

func TestTransaction_Hash_OrderInsensitive(t *testing.T) {
	a, b := uuid.New(), uuid.New()
	x := validTxn(a, b)
	y := validTxn(a, b)
	y.Entries[0], y.Entries[1] = y.Entries[1], y.Entries[0]
	if x.Hash() != y.Hash() {
		t.Fatal("entry order changed the hash")
	}
	y.Entries[0].Amount++
	y.Entries[1].Amount++
	if x.Hash() == y.Hash() {
		t.Fatal("different payload produced same hash")
	}
}

func TestTransaction_SumsByAccount_RoutesByStatus(t *testing.T) {
	a, b := uuid.New(), uuid.New()

	posted := validTxn(a, b).SumsByAccount()
	if posted[a].Debits != 1000 || posted[b].Credits != 1000 || posted[a].PendingDebits != 0 {
		t.Fatalf("posted sums wrong: %+v", posted)
	}

	p := validTxn(a, b)
	p.Status = TransactionStatusPending
	pending := p.SumsByAccount()
	if pending[a].PendingDebits != 1000 || pending[b].PendingCredits != 1000 || pending[a].Debits != 0 {
		t.Fatalf("pending sums wrong: %+v", pending)
	}
}

func TestTransaction_Reverse(t *testing.T) {
	ali, shop, fees := uuid.New(), uuid.New(), uuid.New()
	payment := Transaction{Entries: []Entry{
		entry(ali, 120, EntryDirectionDebit),
		entry(shop, 115, EntryDirectionCredit),
		entry(fees, 5, EntryDirectionCredit),
	}}

	byAccount := func(es []Entry) map[uuid.UUID]Entry {
		m := make(map[uuid.UUID]Entry)
		for _, e := range es {
			m[e.AccountID] = e
		}
		return m
	}

	t.Run("full reversal flips every leg exactly", func(t *testing.T) {
		got := byAccount(payment.Reverse(120, nil))
		if got[ali].Amount != 120 || got[ali].Direction != EntryDirectionCredit ||
			got[shop].Amount != 115 || got[shop].Direction != EntryDirectionDebit ||
			got[fees].Amount != 5 || got[fees].Direction != EntryDirectionDebit {
			t.Fatalf("%+v", got)
		}
	})

	t.Run("partial scales and largest leg absorbs rounding", func(t *testing.T) {
		got := byAccount(payment.Reverse(50, nil))
		if got[ali].Amount != 50 || got[shop].Amount != 48 || got[fees].Amount != 2 {
			t.Fatalf("%+v", got)
		}
	})

	t.Run("reverse the rest after a partial is exact per leg", func(t *testing.T) {
		already := map[uuid.UUID]int64{ali: 50, shop: 48, fees: 2}
		got := byAccount(payment.Reverse(70, already))
		if got[ali].Amount != 70 || got[shop].Amount != 67 || got[fees].Amount != 3 {
			t.Fatalf("%+v", got)
		}
	})

	t.Run("result is balanced", func(t *testing.T) {
		for _, amt := range []int64{1, 7, 50, 119, 120} {
			rev := Transaction{Kind: kindRefund, Status: TransactionStatusPosted,
				IdempotencyKey: "k", Entries: payment.Reverse(amt, nil)}
			if err := rev.Validate(); err != nil {
				t.Fatalf("reverse(%d) not valid: %v", amt, err)
			}
		}
	})
}
