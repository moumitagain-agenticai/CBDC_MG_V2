package domain

import "github.com/fineract/cacti-bridge/pkg/flog"
import (
	"errors"
	"fmt"
)

// ErrorCode is a stable, machine-readable identifier for a class of error.
// It is safe to expose to API clients and is mapped to an HTTP status in the
// API layer (see internal/adapters/api).
type ErrorCode string

const (
	CodeValidation   ErrorCode = "VALIDATION_ERROR"
	CodeNotFound     ErrorCode = "NOT_FOUND"
	CodeConflict     ErrorCode = "CONFLICT"
	CodeUnauthorized ErrorCode = "UNAUTHORIZED"
	CodeUpstream     ErrorCode = "UPSTREAM_ERROR"
	CodeTimeout      ErrorCode = "TIMEOUT"
	CodeCircuitOpen  ErrorCode = "CIRCUIT_OPEN"
	CodeInternal     ErrorCode = "INTERNAL_ERROR"
)

// DomainError is the single error type used across the connector. Every failure
// that reaches the API boundary is a *DomainError so it can be mapped to a
// stable HTTP status and error code without leaking internal detail.
type DomainError struct {
	Code    ErrorCode
	Message string
	// Err is the wrapped underlying cause (never serialized to clients).
	Err error
}

func (e *DomainError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("%s: %s: %v", e.Code, e.Message, e.Err)
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

// Unwrap enables errors.Is / errors.As traversal to the underlying cause.
func (e *DomainError) Unwrap() error { return e.Err }

// newError constructs a *DomainError.
func newError(code ErrorCode, msg string, cause error) *DomainError {
	return &DomainError{Code: code, Message: msg, Err: cause}
}

// Constructors for each error class. Using these keeps error creation uniform.
func NewValidationError(msg string, cause error) *DomainError {
	return newError(CodeValidation, msg, cause)
}
func NewNotFoundError(msg string, cause error) *DomainError {
	return newError(CodeNotFound, msg, cause)
}
func NewConflictError(msg string, cause error) *DomainError {
	return newError(CodeConflict, msg, cause)
}
func NewUnauthorizedError(msg string, cause error) *DomainError {
	return newError(CodeUnauthorized, msg, cause)
}
func NewUpstreamError(msg string, cause error) *DomainError {
	return newError(CodeUpstream, msg, cause)
}
func NewTimeoutError(msg string, cause error) *DomainError {
	return newError(CodeTimeout, msg, cause)
}
func NewCircuitOpenError(msg string, cause error) *DomainError {
	return newError(CodeCircuitOpen, msg, cause)
}
func NewInternalError(msg string, cause error) *DomainError {
	return newError(CodeInternal, msg, cause)
}

// AsDomainError extracts a *DomainError from err, or wraps it as an internal
// error if it is not already a DomainError. It never returns nil for a non-nil
// input, which makes it safe to call at the API boundary.
func AsDomainError(err error) *DomainError {
	if err == nil {
		return nil
	}
	var de *DomainError
	if errors.As(err, &de) {
		return de
	}
	return NewInternalError("unexpected error", err)
}

// flogMarker registers this source file with the Logrus per-file logger,
// producing logs/10_errors.log at runtime.
var _ = func() bool { flog.For("10_errors").Info("source file initialized"); return true }()
