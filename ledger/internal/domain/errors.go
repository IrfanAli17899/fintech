package domain

import (
	"github.com/google/uuid"
	"github.com/irfanali17899/fintech/ledger/codes"
	"github.com/irfanali17899/fintech/platform/errdefs"
)

var (
	// accounts
	CodeAccountExists      = codes.AccountExists
	CodeAccountNotFound    = codes.AccountNotFound
	CodeInvalidAccountType = codes.InvalidAccountType
	CodeInvalidOwnerType   = codes.InvalidOwnerType
	CodeInvalidCurrency    = codes.InvalidCurrency
	CodeOwnerIDRequired    = codes.OwnerIDRequired

	// transactions
	CodeTransactionNotFound      = codes.TransactionNotFound
	CodeTransactionUnbalanced    = codes.TransactionUnbalanced
	CodeInvalidTransactionKind   = codes.InvalidTransactionKind
	CodeIdempotencyKeyRequired   = codes.IdempotencyKeyRequired
	CodeTooFewEntries            = codes.TooFewEntries
	CodeInvalidEntryDirection    = codes.InvalidEntryDirection
	CodeInvalidAmount            = codes.InvalidAmount
	CodeAmountTooLarge           = codes.AmountTooLarge
	CodeCurrencyMismatch         = codes.CurrencyMismatch
	CodeAccountNetZero           = codes.AccountNetZero
	CodeInsufficientFunds        = codes.InsufficientFunds
	CodeTransactionNotReversible = codes.TransactionNotReversible
	CodeReversalExceedsOriginal  = codes.ReversalExceedsOriginal
	CodeInvalidTransactionStatus = codes.InvalidTransactionStatus
	CodeTransactionNotPending    = codes.TransactionNotPending
	CodeCaptureExceedsHold       = codes.CaptureExceedsHold
	CodeUnknownRail              = codes.UnknownRail
	CodeRailFailed               = codes.RailFailed
	CodeRailNotReversible        = codes.RailNotReversible
	CodeRailManagedHold          = codes.RailManagedHold
	CodeInvalidRelation          = codes.InvalidRelation
	CodeRelationWithoutParent    = codes.RelationWithoutParent
	CodeRailCannotCancel         = codes.RailCannotCancel
	CodeHoldAlreadySettling      = codes.HoldAlreadySettling

	// idempotency
	CodeIdempotencyKeyExists   = "IDEMPOTENCY_KEY_EXISTS"    // internal: never reaches a caller
	CodeIdempotencyKeyNotFound = "IDEMPOTENCY_KEY_NOT_FOUND" // internal: never reaches a caller
	CodeIdempotencyKeyReused   = codes.IdempotencyKeyReused
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

func ErrInsufficientFunds(id uuid.UUID, available, requested int64) *errdefs.Error {
	return errdefs.Validationf(CodeInsufficientFunds,
		"account %s has %d available, %d requested", id, available, requested)
}

func ErrTransactionNotReversible(id uuid.UUID, status TransactionStatus) *errdefs.Error {
	return errdefs.Validationf(CodeTransactionNotReversible,
		"transaction %s is %s; only posted transactions can be reversed", id, status)
}

func ErrReversalExceedsOriginal(id uuid.UUID, remaining, requested int64) *errdefs.Error {
	return errdefs.Validationf(CodeReversalExceedsOriginal,
		"transaction %s has %d remaining to reverse, %d requested", id, remaining, requested)
}

func ErrInvalidTransactionStatus(s TransactionStatus) *errdefs.Error {
	return errdefs.Validationf(CodeInvalidTransactionStatus,
		"transaction status %q is not valid for posting; use pending or posted", s)
}

func ErrTransactionNotPending(id uuid.UUID, status TransactionStatus) *errdefs.Error {
	return errdefs.Validationf(CodeTransactionNotPending,
		"transaction %s is %s; only pending transactions can be captured or voided", id, status)
}

func ErrCaptureExceedsHold(id uuid.UUID, held, requested int64) *errdefs.Error {
	return errdefs.Validationf(CodeCaptureExceedsHold,
		"hold %s is for %d, capture of %d requested", id, held, requested)
}

func ErrUnknownRail(name string) *errdefs.Error {
	return errdefs.Validationf(CodeUnknownRail, "rail %q is not available", name)
}

func ErrRailFailed(name, op string, cause error) *errdefs.Error {
	e := errdefs.Internalf(cause, "rail %s failed to %s", name, op)
	e.Code = CodeRailFailed
	return e
}

func ErrRailNotReversible(id uuid.UUID, rail string) *errdefs.Error {
	return errdefs.Validationf(CodeRailNotReversible,
		"transaction %s was sent via %s, which does not support reversal", id, rail)
}

func ErrRailManagedHold(id uuid.UUID, rail string) *errdefs.Error {
	return errdefs.Validationf(CodeRailManagedHold,
		"transaction %s is held by %s; only the rail can settle or cancel it", id, rail)
}

func ErrRailCannotCancel(id uuid.UUID, rail string) *errdefs.Error {
	return errdefs.Validationf(CodeRailCannotCancel,
		"transaction %s is pending on %s, which does not support cancellation; it will settle or bounce", id, rail)
}

func ErrHoldAlreadySettling(id uuid.UUID, rail string) *errdefs.Error {
	return errdefs.Conflictf(CodeHoldAlreadySettling,
		"transaction %s has already been settled or bounced by %s; it can no longer be cancelled", id, rail)
}

// Internal signal from the repo that the key already exists; the service
// turns this into a replay or an ErrIdempotencyKeyReused. Never reaches clients.
func ErrIdempotencyKeyExists(key string) *errdefs.Error {
	return errdefs.Conflictf(CodeIdempotencyKeyExists, "idempotency key %q already exists", key)
}

func ErrIdempotencyKeyNotFound(key string) *errdefs.Error {
	return errdefs.NotFoundf(CodeIdempotencyKeyNotFound, "idempotency key %q not found", key)
}

// Same key, different payload — a client bug that must surface.
func ErrIdempotencyKeyReused(key string) *errdefs.Error {
	return errdefs.Validationf(CodeIdempotencyKeyReused,
		"idempotency key %q was already used with a different request", key)
}

func ErrInvalidRelation(r TransactionRelation) *errdefs.Error {
	return errdefs.Validationf(CodeInvalidRelation, "relation %q is not valid", r)
}

func ErrRelationWithoutParent(r TransactionRelation) *errdefs.Error {
	return errdefs.Validationf(CodeRelationWithoutParent,
		"relation %q requires a parent transaction", r)
}
