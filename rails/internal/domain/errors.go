package domain

import "errors"

var (
	ErrAttemptNotFound = errors.New("rail attempt not found")
	ErrNotReversible   = errors.New("rail does not support reversal")
	ErrWrongState      = errors.New("rail attempt is not in the expected state")
	ErrOutcomeNotFound = errors.New("rail outcome not found")
	ErrCannotCancel    = errors.New("rail does not support cancellation")
)
