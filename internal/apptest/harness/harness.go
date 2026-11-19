// Package harness provides the two doubles every application test needs: a clock and an
// id generator the test controls.
package harness

import (
	"fmt"
	"time"

	"github.com/iotus3/simplelife/app/internal/domain/shared"
)

// Clock is a controllable clock.
type Clock struct {
	now time.Time
}

// NewClock builds a clock frozen at the given moment (local time).
func NewClock(at time.Time) *Clock { return &Clock{now: at} }

// Now implements shared.Clock.
func (c *Clock) Now() time.Time { return c.now }

// Advance moves the clock forward.
func (c *Clock) Advance(d time.Duration) { c.now = c.now.Add(d) }

// Set moves the clock to an absolute moment.
func (c *Clock) Set(at time.Time) { c.now = at }

// At parses "2006-01-02 15:04:05" in local time; tests read better with it.
func At(s string) time.Time {
	t, err := time.ParseInLocation("2006-01-02 15:04:05", s, time.Local)
	if err != nil {
		panic(err)
	}
	return t
}

// IDs mints sequential identifiers so assertions can refer to them.
type IDs struct {
	n int
}

// NewIDs builds an id generator starting at one.
func NewIDs() *IDs { return &IDs{} }

// New implements shared.IDGen. The result is 26 digits, which satisfies the same
// Crockford alphabet the real generator uses.
func (g *IDs) New() shared.ID {
	g.n++
	return shared.ID(fmt.Sprintf("%026d", g.n))
}
