package domain

import (
	"github.com/google/uuid"
	"github.com/irfanali17899/fintech/pkg/errdefs"
)

var (
	// accounts
	CodeAccountExists      = "ACCOUNT_EXISTS"
	CodeAccountNotFound    = "ACCOUNT_NOT_FOUND"
	CodeInvalidAccountType = "INVALID_ACCOUNT_TYPE"
	CodeInvalidOwnerType   = "INVALID_OWNER_TYPE"
	CodeInvalidCurrency    = "INVALID_CURRENCY"
	CodeOwnerIDRequired    = "OWNER_ID_REQUIRED"

	// transactions
	CodeTransactionExists      = "TRANSACTION_EXISTS"
	CodeTransactionNotFound    = "TRANSACTION_NOT_FOUND"
	CodeTransactionUnbalanced  = "TRANSACTION_UNBALANCED"
	CodeInvalidTransactionKind = "INVALID_TRANSACTION_KIND"
	CodeIdempotencyKeyRequired = "IDEMPOTENCY_KEY_REQUIRED"
	CodeTooFewEntries          = "TOO_FEW_ENTRIES"
	CodeInvalidEntryDirection  = "INVALID_ENTRY_DIRECTION"
	CodeInvalidAmount          = "INVALID_AMOUNT"
	CodeAmountTooLarge         = "AMOUNT_TOO_LARGE"
	CodeCurrencyMismatch       = "CURRENCY_MISMATCH"
	CodeAccountNetZero         = "ACCOUNT_NET_ZERO"
)

// --- accounts ---

func ErrAccountExists(a Account) *errdefs.Error {
	return errdefs.Conflictf(CodeAccountExists, "%s %s already has a %s %s account",
		a.OwnerType, a.OwnerID, a.Currency, a.Type)
}
func ErrAccountNotFound(id uuid.UUID) *errdefs.Error {
	return errdefs.NotFoundf(CodeAccountNotFound, "account %s not found", id)
}
func ErrInvalidAccountType(t AccountType) *errdefs.Error {
	return errdefs.Validationf(CodeInvalidAccountType, "account type %q is not valid", t)
}
func ErrInvalidOwnerType(o OwnerType) *errdefs.Error {
	return errdefs.Validationf(CodeInvalidOwnerType, "owner type %q is not valid", o)
}
func ErrInvalidCurrency(c string) *errdefs.Error {
	return errdefs.Validationf(CodeInvalidCurrency, "currency %q must be 3 uppercase letters", c)
}
func ErrOwnerIDRequired() *errdefs.Error {
	return errdefs.Validationf(CodeOwnerIDRequired, "owner id is required")
}

// --- transactions ---

func ErrTransactionExists(key string) *errdefs.Error {
	return errdefs.Conflictf(CodeTransactionExists,
		"transaction with idempotency key %q already exists", key)
}

func ErrTransactionNotFound(id uuid.UUID) *errdefs.Error {
	return errdefs.NotFoundf(CodeTransactionNotFound, "transaction %s not found", id)
}

func ErrTransactionUnbalanced(debits, credits int64) *errdefs.Error {
	return errdefs.Validationf(CodeTransactionUnbalanced,
		"debits (%d) must equal credits (%d)", debits, credits)
}

func ErrInvalidTransactionKind(k TransactionKind) *errdefs.Error {
	return errdefs.Validationf(CodeInvalidTransactionKind, "transaction kind %q is not valid", k)
}

func ErrIdempotencyKeyRequired() *errdefs.Error {
	return errdefs.Validationf(CodeIdempotencyKeyRequired, "idempotency key is required")
}

func ErrTooFewEntries(n int) *errdefs.Error {
	return errdefs.Validationf(CodeTooFewEntries,
		"transaction needs at least 2 entries, got %d", n)
}

func ErrInvalidEntryDirection(d EntryDirection) *errdefs.Error {
	return errdefs.Validationf(CodeInvalidEntryDirection, "entry direction %q is not valid", d)
}

func ErrInvalidAmount(a int64) *errdefs.Error {
	return errdefs.Validationf(CodeInvalidAmount, "amount must be a positive integer, got %d", a)
}

func ErrAmountTooLarge(a int64) *errdefs.Error {
	return errdefs.Validationf(CodeAmountTooLarge,
		"amount %d exceeds maximum of %d minor units", a, MaxEntryAmount)
}

func ErrCurrencyMismatch(accountID uuid.UUID, got, want string) *errdefs.Error {
	return errdefs.Validationf(CodeCurrencyMismatch,
		"account %s is %s but transaction is %s", accountID, want, got)
}

func ErrAccountNetZero(id uuid.UUID) *errdefs.Error {
	return errdefs.Validationf(CodeAccountNetZero,
		"account %s has no net effect in this transaction", id)
}
