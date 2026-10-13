// Package codes lists every error code the ledger can return to a caller.
// It is a leaf with no dependencies, so consumers can branch on a code
// (errdefs.Is(err, codes.InsufficientFunds)) without importing anything else
// from the ledger. Codes are part of the contract: renaming one is a breaking
// change.
package codes

const (
	AccountExists            = "ACCOUNT_EXISTS"
	AccountNotFound          = "ACCOUNT_NOT_FOUND"
	InvalidAccountType       = "INVALID_ACCOUNT_TYPE"
	InvalidOwnerType         = "INVALID_OWNER_TYPE"
	InvalidCurrency          = "INVALID_CURRENCY"
	OwnerIDRequired          = "OWNER_ID_REQUIRED"
	TransactionNotFound      = "TRANSACTION_NOT_FOUND"
	TransactionUnbalanced    = "TRANSACTION_UNBALANCED"
	InvalidTransactionKind   = "INVALID_TRANSACTION_KIND"
	IdempotencyKeyRequired   = "IDEMPOTENCY_KEY_REQUIRED"
	TooFewEntries            = "TOO_FEW_ENTRIES"
	InvalidEntryDirection    = "INVALID_ENTRY_DIRECTION"
	InvalidAmount            = "INVALID_AMOUNT"
	AmountTooLarge           = "AMOUNT_TOO_LARGE"
	CurrencyMismatch         = "CURRENCY_MISMATCH"
	AccountNetZero           = "ACCOUNT_NET_ZERO"
	InsufficientFunds        = "INSUFFICIENT_FUNDS"
	TransactionNotReversible = "TRANSACTION_NOT_REVERSIBLE"
	ReversalExceedsOriginal  = "REVERSAL_EXCEEDS_ORIGINAL"
	InvalidTransactionStatus = "INVALID_TRANSACTION_STATUS"
	TransactionNotPending    = "TRANSACTION_NOT_PENDING"
	CaptureExceedsHold       = "CAPTURE_EXCEEDS_HOLD"
	UnknownRail              = "UNKNOWN_RAIL"
	RailFailed               = "RAIL_FAILED"
	RailNotReversible        = "RAIL_NOT_REVERSIBLE"
	RailManagedHold          = "RAIL_MANAGED_HOLD"
	InvalidRelation          = "INVALID_RELATION"
	RelationWithoutParent    = "RELATION_WITHOUT_PARENT"
	RailCannotCancel         = "RAIL_CANNOT_CANCEL"
	HoldAlreadySettling      = "HOLD_ALREADY_SETTLING"
	IdempotencyKeyReused     = "IDEMPOTENCY_KEY_REUSED"
)
