// Package power keeps the machine awake while a pomodoro runs. A platform that cannot hold the
// request reports an error the caller ignores: not preventing sleep is no reason to refuse a
// pomodoro.
package power

import (
	"github.com/vivaui/viva/app"

	"github.com/iotus3/simplelife/app/internal/domain/shared"
)

// Guard is the sleep-lock adapter.
type Guard struct {
	// hold asks the platform to stay awake. It is nil in the application, where
	// the system is asked, and replaced by a recorder in the tests: a test must
	// not hold the sleep lock of the machine that runs it.
	hold func(reason string) (func(), error)
}

// Hold asks the system not to sleep until the returned function is called.
func (g Guard) Hold(reason string) (func(), error) {
	hold := g.hold
	if hold == nil {
		hold = app.PreventSleep
	}
	return hold(reason)
}

var _ shared.SleepGuard = Guard{}
