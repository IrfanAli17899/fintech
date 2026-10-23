package domain

import (
	"testing"

	"github.com/irfanali17899/fintech/platform/errdefs"
)

func TestAccount_Validate(t *testing.T) {
	valid := Account{OwnerType: OwnerCustomer, OwnerID: "ali", Currency: "PKR", Type: AccountLiability}

	tests := []struct {
		name     string
		mutate   func(*Account)
		wantCode string
	}{
		{"valid", func(*Account) {}, ""},
		{"bad type", func(a *Account) { a.Type = "savings" }, CodeInvalidAccountType},
		{"bad owner type", func(a *Account) { a.OwnerType = "robot" }, CodeInvalidOwnerType},
		{"empty owner id", func(a *Account) { a.OwnerID = "" }, CodeOwnerIDRequired},
		{"lowercase currency", func(a *Account) { a.Currency = "pkr" }, CodeInvalidCurrency},
		{"short currency", func(a *Account) { a.Currency = "PK" }, CodeInvalidCurrency},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			a := valid
			tc.mutate(&a)
			err := a.Validate()
			if tc.wantCode == "" && err != nil {
				t.Fatalf("expected valid, got %v", err)
			}
			if tc.wantCode != "" && !errdefs.Is(err, tc.wantCode) {
				t.Fatalf("want %s, got %v", tc.wantCode, err)
			}
		})
	}
}
