package domain

import (
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
