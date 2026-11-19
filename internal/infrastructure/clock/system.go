// Package clock is the wall clock the application runs on. Tests inject a clock
// of their own; nothing outside this package reads time.Now for business logic.
package clock

import (
	"time"

	"github.com/iotus3/simplelife/app/internal/domain/shared"
)

// System reports the machine's local time.
type System struct{}

// Now returns the current local time.
func (System) Now() time.Time { return time.Now() }

var _ shared.Clock = System{}
