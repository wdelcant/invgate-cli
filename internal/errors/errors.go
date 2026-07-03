// Package errors provides the AppError type used throughout invgate-cli
// for structured error handling with HTTP status codes, verbose context,
// and stdlib errors.Is/As compatibility.
package errors

import (
	stderrors "errors"
	"fmt"
	"sort"
	"strings"
)

// AppError is the canonical error type carrying an HTTP status code,
// a human-readable message, an optional wrapped cause, and a verbose
// context map printed only when --verbose is active.
type AppError struct {
	Code    int               // HTTP status code (0 for non-HTTP errors)
	Message string            // User-facing message
	Cause   error             // Wrapped underlying error (supports errors.Unwrap)
	Verbose map[string]any    // Extra context printed only with --verbose
}

// Error implements the error interface.
func (e *AppError) Error() string {
	if e.Code != 0 {
		return fmt.Sprintf("Error (%d): %s", e.Code, e.Message)
	}
	return fmt.Sprintf("Error: %s", e.Message)
}

// Unwrap returns the wrapped cause so errors.Is and errors.As traverse
// the error chain.
func (e *AppError) Unwrap() error {
	return e.Cause
}

// VerboseOutput renders the verbose context as a multi-line string.
// Keys are sorted alphabetically for deterministic output.
func (e *AppError) VerboseOutput() string {
	if len(e.Verbose) == 0 {
		return ""
	}
	keys := make([]string, 0, len(e.Verbose))
	for k := range e.Verbose {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	for _, k := range keys {
		b.WriteString(fmt.Sprintf("  %s: %v\n", k, e.Verbose[k]))
	}
	return strings.TrimRight(b.String(), "\n")
}

// NewError creates an AppError with the given code and message.
func NewError(code int, msg string) *AppError {
	return &AppError{Code: code, Message: msg}
}

// Wrap creates an AppError wrapping cause with the given code and message.
func Wrap(err error, code int, msg string) *AppError {
	return &AppError{Code: code, Message: msg, Cause: err}
}

// New creates a plain AppError with no code (code=0).
func New(msg string) *AppError {
	return &AppError{Message: msg}
}

// Errorf creates an AppError using fmt.Errorf-style formatting.
func Errorf(format string, args ...any) *AppError {
	return &AppError{Message: fmt.Sprintf(format, args...)}
}

// IsCode reports whether err is (or wraps) an AppError with the given code.
func IsCode(err error, code int) bool {
	var ae *AppError
	if stderrors.As(err, &ae) {
		return ae.Code == code
	}
	return false
}

// Is implements errors.Is so that AppError values match on identity.
func (e *AppError) Is(target error) bool {
	var ae *AppError
	if stderrors.As(target, &ae) {
		return e.Code == ae.Code && e.Message == ae.Message
	}
	return false
}

// FormatError renders an AppError for stderr output. When verbose is true
// the Verbose map is appended.
func FormatError(err error, verbose bool) string {
	var ae *AppError
	if !stderrors.As(err, &ae) {
		return fmt.Sprintf("Error: %s", err.Error())
	}
	out := ae.Error()
	if verbose {
		if v := ae.VerboseOutput(); v != "" {
			out += "\n" + v
		}
	}
	return out
}