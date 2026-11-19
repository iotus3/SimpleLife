package presentation

import (
	"testing"
	"time"

	"github.com/iotus3/simplelife/app/internal/presentation/i18n"
)

// The focus page offers the three usual pomodoros and a custom length, and a session
// started without a choice lasts 25 minutes — not whatever the settings happen to say.
func TestFocusPageOffersTheUsualLengths(t *testing.T) {
	fixture := newFixture(t)
	session := fixture.openWindow(t)
	fixture.state.route = RouteFocus
	fixture.state.markAll()
	settle(t, session)

	zh := func(id i18n.MessageID, args ...any) string { return i18n.T(i18n.ZhCN, id, args...) }
	if got := fixture.state.focusMinutes(); got != defaultFocusMinutes {
		t.Fatalf("a fresh focus page starts at %d minutes, want %d", got, defaultFocusMinutes)
	}
	// A select paints the entry it holds, so the choices are checked on the list and
	// the painted one: "25 分钟" is the length this page starts with.
	want := []int{25, 35, 55, focusDurationCustom}
	if len(focusLengthChoices) != len(want) {
		t.Fatalf("the lengths offered are %v, want %v", focusLengthChoices, want)
	}
	for index, minutes := range want {
		if focusLengthChoices[index] != minutes {
			t.Fatalf("the lengths offered are %v, want %v", focusLengthChoices, want)
		}
	}
	if !session.HasText(zh(i18n.MsgFocusDurationValue, defaultFocusMinutes)) {
		t.Errorf("the chosen length is not %d minutes: %v", defaultFocusMinutes, session.Texts())
	}

	fixture.state.focusMin = 35
	if got := fixture.state.focusPlanned(); got != 35*time.Minute {
		t.Fatalf("choosing 35 planned %v", got)
	}
	// A typed length out of bounds falls back to the default rather than refusing to
	// start a session the user asked for.
	fixture.state.focusMin = focusDurationCustom
	fixture.state.focusCustom = "100000"
	if got := fixture.state.focusMinutes(); got != defaultFocusMinutes {
		t.Fatalf("an out-of-bounds custom length planned %d minutes, want %d", got, defaultFocusMinutes)
	}
	fixture.state.focusCustom = "45"
	if got := fixture.state.focusMinutes(); got != 45 {
		t.Fatalf("a typed 45 planned %d minutes", got)
	}
}
