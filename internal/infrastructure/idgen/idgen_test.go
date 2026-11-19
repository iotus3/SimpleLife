package idgen

import (
	"testing"
	"time"

	"github.com/iotus3/simplelife/app/internal/domain/shared"
)

func TestGeneratorProducesDistinctIdentifiers(t *testing.T) {
	var ids shared.IDGen = New()
	seen := make(map[shared.ID]bool, 1024)
	for i := 0; i < 1024; i++ {
		id := ids.New()
		if id.IsZero() {
			t.Fatal("an identifier is the zero value")
		}
		if seen[id] {
			t.Fatalf("identifier %q was generated twice", id)
		}
		seen[id] = true
	}
}

func TestGeneratorUsesTheClockItIsGiven(t *testing.T) {
	fixed := time.Date(2026, time.November, 17, 9, 30, 0, 0, time.Local)
	early := Generator{Now: func() time.Time { return fixed }}.New()
	late := Generator{Now: func() time.Time { return fixed.Add(time.Hour) }}.New()
	if early.IsZero() || late.IsZero() {
		t.Fatal("an identifier is the zero value")
	}
	// The identifier carries its timestamp, so an identifier made from an earlier
	// clock must sort first.
	if !(early < late) {
		t.Errorf("identifiers do not follow the clock: %q then %q", early, late)
	}
}

func TestGeneratorWithoutAClockUsesTheSystem(t *testing.T) {
	id := Generator{}.New()
	if id.IsZero() {
		t.Fatal("the identifier is the zero value")
	}
}
