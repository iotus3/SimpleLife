// Package idgen adapts the domain's identifier function to the IDGen port and stamps each id
// with the wall clock.
package idgen

import (
	"time"

	"github.com/iotus3/simplelife/app/internal/domain/shared"
)

// Generator makes identifiers from the current time.
type Generator struct {
	Now func() time.Time
}

// New returns a generator that reads the system clock.
func New() Generator { return Generator{Now: time.Now} }

// New returns the next identifier.
func (g Generator) New() shared.ID {
	now := time.Now
	if g.Now != nil {
		now = g.Now
	}
	id, err := shared.NewID(now())
	if err != nil {
		// shared.NewID only fails when the random source fails, which on this
		// platform means the machine is broken beyond continuing.
		panic("idgen: " + err.Error())
	}
	return id
}

var _ shared.IDGen = Generator{}
