package domain

import "errors"

var (
	ErrAttemptNotFound = errors.New("rail attempt not found")
	ErrNotReversible   = errors.New("rail does not support reversal")
	ErrWrongState      = errors.New("rail attempt is not in the expected state")
	ErrBounced         = errors.New("rail transfer bounced")
)
