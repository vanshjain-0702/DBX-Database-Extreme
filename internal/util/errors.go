package util

import (
	"errors"
	"fmt"
)

// Error code constants.
const (
	ErrCodeOK              = 0
	ErrCodeNotFound        = 1
	ErrCodeWrongType       = 2
	ErrCodeOutOfRange      = 3
	ErrCodeSyntax          = 4
	ErrCodeAuth            = 5
	ErrCodePermission      = 6
	ErrCodeQuotaExceeded   = 7
	ErrCodeTimeout         = 8
	ErrCodeRateLimit       = 9
	ErrCodeTransactionAbort = 10
	ErrCodeRedirect        = 11
	ErrCodeNotLeader       = 12
	ErrCodeInternal        = 99
)

// DBXError is a typed error for DBX.
type DBXError struct {
	Code    int
	Message string
	Cause   error
}

func (e *DBXError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("DBX[%d]: %s: %v", e.Code, e.Message, e.Cause)
	}
	return fmt.Sprintf("DBX[%d]: %s", e.Code, e.Message)
}

func (e *DBXError) Unwrap() error { return e.Cause }

// Common errors.
var (
	ErrNotFound        = &DBXError{Code: ErrCodeNotFound, Message: "key not found"}
	ErrWrongType       = &DBXError{Code: ErrCodeWrongType, Message: "WRONGTYPE operation against a key holding the wrong kind of value"}
	ErrOutOfRange      = &DBXError{Code: ErrCodeOutOfRange, Message: "value is not an integer or out of range"}
	ErrSyntax          = &DBXError{Code: ErrCodeSyntax, Message: "syntax error"}
	ErrAuth            = &DBXError{Code: ErrCodeAuth, Message: "NOAUTH authentication required"}
	ErrPermission      = &DBXError{Code: ErrCodePermission, Message: "NOPERM permission denied"}
	ErrQuotaExceeded   = &DBXError{Code: ErrCodeQuotaExceeded, Message: "quota exceeded"}
	ErrTimeout         = &DBXError{Code: ErrCodeTimeout, Message: "operation timed out"}
	ErrRateLimit       = &DBXError{Code: ErrCodeRateLimit, Message: "rate limit exceeded"}
	ErrTransactionAbort = &DBXError{Code: ErrCodeTransactionAbort, Message: "EXECABORT transaction discarded"}
	ErrNotLeader       = &DBXError{Code: ErrCodeNotLeader, Message: "not cluster leader"}
)

// Newf creates a new DBXError with a formatted message.
func Newf(code int, format string, args ...any) *DBXError {
	return &DBXError{Code: code, Message: fmt.Sprintf(format, args...)}
}

// Wrap wraps an existing error.
func Wrap(code int, msg string, cause error) *DBXError {
	return &DBXError{Code: code, Message: msg, Cause: cause}
}

// IsNotFound returns true if the error is a not-found error.
func IsNotFound(err error) bool {
	var e *DBXError
	if errors.As(err, &e) {
		return e.Code == ErrCodeNotFound
	}
	return false
}

// IsWrongType returns true if the error is a wrong-type error.
func IsWrongType(err error) bool {
	var e *DBXError
	if errors.As(err, &e) {
		return e.Code == ErrCodeWrongType
	}
	return false
}

// RedirectError carries MOVED redirect information.
type RedirectError struct {
	Slot int
	Addr string
}

func (e *RedirectError) Error() string {
	return fmt.Sprintf("MOVED %d %s", e.Slot, e.Addr)
}
