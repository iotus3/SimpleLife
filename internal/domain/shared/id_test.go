package shared

import (
	"testing"
	"time"
)

func TestNewIDIsValidAndSortsByTime(t *testing.T) {
	base := time.Date(2026, 11, 17, 9, 0, 0, 0, time.UTC)
	first, err := NewID(base)
	if err != nil {
		t.Fatal(err)
	}
	later, err := NewID(base.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if !first.Valid() {
		t.Fatalf("generated id %q is not valid", first)
	}
	if len(first) != idLen {
		t.Fatalf("id length = %d, want %d", len(first), idLen)
	}
	// Lexicographic order must follow creation order: that is the whole point of
	// the timestamp prefix.
	if !(first < later) {
		t.Fatalf("%q should sort before %q", first, later)
	}
}

func TestNewIDIsUnique(t *testing.T) {
	now := time.Now()
	seen := make(map[ID]struct{}, 512)
	for i := 0; i < 512; i++ {
		id, err := NewID(now)
		if err != nil {
			t.Fatal(err)
		}
		if _, dup := seen[id]; dup {
			t.Fatalf("duplicate id %q after %d draws", id, i)
		}
		seen[id] = struct{}{}
	}
}

func TestIDValidRejectsGarbage(t *testing.T) {
	cases := []string{
		"",
		"short",
		"0123456789012345678901234",   // 25 chars
		"012345678901234567890123456", // 27 chars
		"0123456789012345678901234I",  // I is not in Crockford
		"0123456789012345678901234U",  // U is not in Crockford
		"0123456789012345678901234a",  // lowercase
	}
	for _, c := range cases {
		if ID(c).Valid() {
			t.Errorf("%q must not be a valid id", c)
		}
	}
}
