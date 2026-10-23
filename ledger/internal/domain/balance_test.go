package domain

import "testing"

func TestAccountType_Balance(t *testing.T) {
	sums := AccountSums{Debits: 300, Credits: 1000, PendingDebits: 200, PendingCredits: 500}

	tests := []struct {
		typ  AccountType
		want Balance
	}{
		{AccountLiability, Balance{Posted: 700, PendingIn: 500, PendingOut: 200, Available: 500}},
		{AccountRevenue, Balance{Posted: 700, PendingIn: 500, PendingOut: 200, Available: 500}},
		{AccountAsset, Balance{Posted: -700, PendingIn: 200, PendingOut: 500, Available: -1200}},
		{AccountExpense, Balance{Posted: -700, PendingIn: 200, PendingOut: 500, Available: -1200}},
	}
	for _, tc := range tests {
		t.Run(string(tc.typ), func(t *testing.T) {
			got := tc.typ.Balance(sums)
			if got.Posted != tc.want.Posted || got.PendingIn != tc.want.PendingIn ||
				got.PendingOut != tc.want.PendingOut || got.Available != tc.want.Available {
				t.Fatalf("got %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestAccountType_Balance_UnknownTypePanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("expected panic")
		}
	}()
	AccountType("bogus").Balance(AccountSums{})
}
