package shared

import (
	"errors"
	"testing"
)

func TestDomainErrorKindCodeAndField(t *testing.T) {
	err := Invalid("task.title.length", "title", "title must be 1..200 runes")
	if !IsKind(err, KindInvalid) {
		t.Error("IsKind(invalid) failed")
	}
	if IsKind(err, KindConflict) {
		t.Error("IsKind must distinguish kinds")
	}
	if Code(err) != "task.title.length" {
		t.Errorf("Code = %q", Code(err))
	}
	if Field(err) != "title" {
		t.Errorf("Field = %q", Field(err))
	}
	if !IsKind(err.WithField("notes"), KindInvalid) || Field(err.WithField("notes")) != "notes" {
		t.Error("WithField must return a copy with the new field")
	}
	if Field(err) != "title" {
		t.Error("WithField must not mutate the original")
	}
}

func TestWrappedCauseIsPreserved(t *testing.T) {
	cause := errors.New("disk full")
	err := Conflict("store.write", "cannot write").Wrap(cause)
	if !errors.Is(err, cause) {
		t.Error("Unwrap must expose the cause")
	}
	if !IsKind(err, KindConflict) {
		t.Error("wrapping must keep the kind")
	}
}

func TestDuplicateIsDetectable(t *testing.T) {
	err := Dupf("task occurrence %s", "2026-11-17")
	if !errors.Is(err, ErrDuplicate) {
		t.Fatal("Dupf must wrap ErrDuplicate")
	}
	if Code(err) != "" {
		t.Error("a non-domain error has no code")
	}
}
