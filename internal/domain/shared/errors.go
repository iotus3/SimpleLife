// Package shared holds the value objects and ports every bounded context of
// SimpleLife depends on: identifiers, money, local dates, the clock, and the
// transaction boundary. It imports nothing but the standard library.
package shared

import (
	"errors"
	"fmt"
)

// Kind classifies a domain error so the application and presentation layers can
// react without knowing every concrete failure.
type Kind uint8

const (
	KindInvalid Kind = iota + 1
	KindNotFound
	KindConflict
	KindUnsupported
)

func (k Kind) String() string {
	switch k {
	case KindInvalid:
		return "invalid"
	case KindNotFound:
		return "notfound"
	case KindConflict:
		return "conflict"
	case KindUnsupported:
		return "unsupported"
	}
	return "unknown"
}

// Error is the only error type the domain returns. Code is a stable machine
// readable identifier ("task.title.length"); Msg is for developers; Field names
// the form field a validation error belongs to, if any.
type Error struct {
	Kind  Kind
	Code  string
	Msg   string
	Field string
	Err   error
}

func (e *Error) Error() string {
	s := e.Code + ": " + e.Msg
	if e.Field != "" {
		s += " (field=" + e.Field + ")"
	}
	if e.Err != nil {
		s += ": " + e.Err.Error()
	}
	return s
}

func (e *Error) Unwrap() error { return e.Err }

// Invalid reports user input the domain refuses.
func Invalid(code, field, msg string) *Error {
	return &Error{Kind: KindInvalid, Code: code, Msg: msg, Field: field}
}

// NotFound reports a missing aggregate.
func NotFound(code, msg string) *Error {
	return &Error{Kind: KindNotFound, Code: code, Msg: msg}
}

// Conflict reports a state or uniqueness conflict.
func Conflict(code, msg string) *Error {
	return &Error{Kind: KindConflict, Code: code, Msg: msg}
}

// Unsupported reports a capability the current build target does not have. It is
// never used to fake success.
func Unsupported(code, msg string) *Error {
	return &Error{Kind: KindUnsupported, Code: code, Msg: msg}
}

// WithField returns a copy of err carrying field, so one validation helper can
// serve several form fields.
func (e *Error) WithField(field string) *Error {
	cp := *e
	cp.Field = field
	return &cp
}

// Wrap attaches an underlying cause, keeping the kind and code.
func (e *Error) Wrap(err error) *Error {
	cp := *e
	cp.Err = err
	return &cp
}

// IsKind reports whether err (or anything it wraps) is a domain error of kind k.
func IsKind(err error, k Kind) bool {
	var de *Error
	if errors.As(err, &de) {
		return de.Kind == k
	}
	return false
}

// Code returns the stable code of a domain error, or "" for any other error.
func Code(err error) string {
	var de *Error
	if errors.As(err, &de) {
		return de.Code
	}
	return ""
}

// Field returns the form field of a domain error, or "".
func Field(err error) string {
	var de *Error
	if errors.As(err, &de) {
		return de.Field
	}
	return ""
}

// ErrDuplicate is returned by a repository when a unique index rejects a write.
// The use cases translate it into a conflict (or into "already done" where the
// write is idempotent by design).
var ErrDuplicate = errors.New("shared: duplicate key")

// ErrUnsupportedPlatform reports a capability the build target lacks.
var ErrUnsupportedPlatform = errors.New("shared: unsupported on this platform")

// ErrNotFound is the repository-level "no such row".
var ErrNotFound = errors.New("shared: not found")

// Dupf wraps ErrDuplicate with context, so errors.Is(err, ErrDuplicate) holds.
func Dupf(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrDuplicate, fmt.Sprintf(format, args...))
}
