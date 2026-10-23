package errdefs

import (
	"errors"
	"fmt"
)

// Category classifies an error for transport mapping. It is the ONLY thing the
// grpc layer needs to know about a domain error — services never import grpc
// status codes into their domain.
type Category string

const (
	Validation      Category = "validation"      // → InvalidArgument / 400
	Unauthenticated Category = "unauthenticated" // → Unauthenticated / 401
	Forbidden       Category = "forbidden"       // → PermissionDenied / 403
	NotFound        Category = "not_found"       // → NotFound / 404
	Conflict        Category = "conflict"        // → AlreadyExists / 409
	Internal        Category = "internal"        // → Internal / 500
)

// Error is a domain failure with a machine-readable code (e.g. VENDOR_NOT_FOUND)
// carried to clients via the x-error-code trailer.
type Error struct {
	Code     string
	Category Category
	Message  string
	Cause    error
}

func (e *Error) Error() string { return fmt.Sprintf("%s: %s", e.Code, e.Message) }
func (e *Error) Unwrap() error { return e.Cause }

func New(code string, category Category, message string) *Error {
	return &Error{Code: code, Category: category, Message: message}
}

func Is(err error, code string) bool {
	var e *Error
	return errors.As(err, &e) && e.Code == code
}

// Convenience constructors — one per category, so call sites read as intent.

func Internalf(cause error, format string, a ...any) *Error {
	return &Error{Code: string(Internal), Category: Internal,
		Message: fmt.Sprintf(format, a...), Cause: cause}
}

func Validationf(code, format string, a ...any) *Error {
	return New(code, Validation, fmt.Sprintf(format, a...))
}

func Unauthenticatedf(code, format string, a ...any) *Error {
	return New(code, Unauthenticated, fmt.Sprintf(format, a...))
}

func Forbiddenf(code, format string, a ...any) *Error {
	return New(code, Forbidden, fmt.Sprintf(format, a...))
}

func NotFoundf(code, format string, a ...any) *Error {
	return New(code, NotFound, fmt.Sprintf(format, a...))
}

func Conflictf(code, format string, a ...any) *Error {
	return New(code, Conflict, fmt.Sprintf(format, a...))
}
