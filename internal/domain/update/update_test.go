package update_test

import (
	"testing"

	"github.com/iotus3/simplelife/app/internal/domain/update"
)

func TestCompareOrdersNumericParts(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"1.0.0", "1.0.0", 0},
		{"v1.0.0", "1.0.0", 0},
		{"1.2", "1.2.0", 0},
		{"0.9.0", "0.10.0", -1},
		{"0.10.0", "0.9.0", 1},
		{"1.0.0", "1.0.1", -1},
		{"2.0.0", "1.99.99", 1},
		{"1.2.0-rc1", "1.2.0", 0},
		{"1.2.0+build7", "1.2.0", 0},
		// Unreadable versions never claim an order.
		{"", "1.0.0", 0},
		{"1.0.0", "next", 0},
		{"1.x.0", "1.0.0", 0},
	}
	for _, c := range cases {
		if got := update.Compare(c.a, c.b); got != c.want {
			t.Fatalf("Compare(%q, %q) = %d, want %d", c.a, c.b, got, c.want)
		}
	}
}

func TestNewerRefusesToGuess(t *testing.T) {
	if !update.Newer("0.9.0", "0.10.0") {
		t.Fatal("0.10.0 is newer than 0.9.0")
	}
	if update.Newer("0.10.0", "0.9.0") {
		t.Fatal("a downgrade must not be offered as an update")
	}
	if update.Newer("1.0.0", "1.0.0") {
		t.Fatal("the same version is not an update")
	}
	// A build with no version (a source build) and a garbled release both stay
	// quiet: telling the user to install something unknown is worse than silence.
	if update.Newer("", "1.0.0") || update.Newer("1.0.0", "") || update.Newer("1.0.0", "newest") {
		t.Fatal("an unreadable version must never produce an update")
	}
}
