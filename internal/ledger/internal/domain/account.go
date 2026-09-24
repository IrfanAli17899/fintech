package domain

import (
	"fmt"
	"time"

	"github.com/google/uuid"
)

type AccountType string
type OwnerType string

const (
	OwnerCustomer OwnerType = "customer"
	OwnerSystem   OwnerType = "system"
	OwnerMerchant OwnerType = "merchant"

	AccountAsset     AccountType = "asset"
	AccountLiability AccountType = "liability"
	AccountRevenue   AccountType = "revenue"
	AccountExpense   AccountType = "expense"
)

var validAccountType = map[AccountType]bool{
	AccountAsset: true, AccountExpense: true, AccountLiability: true, AccountRevenue: true,
}

var validOwnerType = map[OwnerType]bool{
	OwnerCustomer: true, OwnerMerchant: true, OwnerSystem: true,
}

type Account struct {
	ID        uuid.UUID
	OwnerType OwnerType
	OwnerID   string
	Currency  string
	Type      AccountType
	CreatedAt time.Time
}

// Balance derives the account's balance figures from raw sums, signed by its
// normal balance. "In" is the direction that increases the account, "out" the
// direction that decreases it. Available deducts only outgoing holds: incoming
// pending funds are a promise, not money.
// AccountLiability, AccountRevenue: grow on credits so that's why its credits - debits
// AccountAsset, AccountExpense: grow on debits so that's why its debits - credits
func (t AccountType) Balance(s AccountSums) Balance {
	switch t {
	case AccountLiability, AccountRevenue:
		posted := s.Credits - s.Debits
		return Balance{
			Posted:     posted,
			PendingIn:  s.PendingCredits,
			PendingOut: s.PendingDebits,
			Available:  posted - s.PendingDebits,
		}
	case AccountAsset, AccountExpense:
		posted := s.Debits - s.Credits
		return Balance{
			Posted:     posted,
			PendingIn:  s.PendingDebits,
			PendingOut: s.PendingCredits,
			Available:  posted - s.PendingCredits,
		}
	default:
		panic(fmt.Sprintf("unknown account type %q", t))
	}
}
func (t AccountType) Valid() bool {
	return validAccountType[t]
}

func (o OwnerType) Valid() bool {
	return validOwnerType[o]
}

func ValidCurrency(currency string) bool {
	if len(currency) != 3 {
		return false
	}

	for i := 0; i < len(currency); i++ {
		char := currency[i]
		if char < 'A' || char > 'Z' {
			return false
		}
	}

	return true
}

func (a Account) Validate() error {
	switch {
	case !a.Type.Valid():
		return ErrInvalidAccountType(a.Type)
	case !a.OwnerType.Valid():
		return ErrInvalidOwnerType(a.OwnerType)
	case !ValidCurrency(a.Currency):
		return ErrInvalidCurrency(a.Currency)
	case a.OwnerID == "":
		return ErrOwnerIDRequired()
	}
	return nil
}
