package presentation

import (
	"testing"

	"github.com/vivaui/viva/app"

	"github.com/iotus3/simplelife/app/internal/application/settings"
)

// Closing hides the window only when there is somewhere to come back from: a status item is
// created while the window opens and it can fail. Hiding without one leaves the program running
// with no window and no icon, which is what a user reports as "impossible to close".
func TestClosingHidesOnlyWhenTheStatusAreaExists(t *testing.T) {
	fixture := newFixture(t)
	state := fixture.state

	if err := state.deps.Settings.SetCloseAction(settings.CloseTray); err != nil {
		t.Fatalf("choose the tray: %v", err)
	}
	if state.shouldHideOnClose() {
		t.Fatal("without a status item, closing must quit instead of hiding the window")
	}

	// A status item exists: now the choice the user made is honoured.
	state.statusItem = &app.StatusItem{}
	if !state.shouldHideOnClose() {
		t.Fatal("with a status item, closing must hide the window")
	}

	// And the other choice still quits, status item or not.
	if err := state.deps.Settings.SetCloseAction(settings.CloseExit); err != nil {
		t.Fatalf("choose quitting: %v", err)
	}
	if state.shouldHideOnClose() {
		t.Fatal("choosing to quit must never hide the window")
	}
	state.statusItem = nil
	if state.shouldHideOnClose() {
		t.Fatal("choosing to quit must never hide the window")
	}
}
