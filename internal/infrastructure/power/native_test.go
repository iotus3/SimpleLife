package power

import (
	"errors"
	"testing"

	"github.com/iotus3/simplelife/app/internal/domain/shared"
)

func TestGuardReleasesWhatItHeld(t *testing.T) {
	var held []string
	released := 0
	var guard shared.SleepGuard = Guard{hold: func(reason string) (func(), error) {
		held = append(held, reason)
		return func() { released++ }, nil
	}}
	release, err := guard.Hold("pomodoro")
	if err != nil {
		t.Fatalf("hold: %v", err)
	}
	if len(held) != 1 || held[0] != "pomodoro" {
		t.Fatalf("held %v, want one request for a pomodoro", held)
	}
	release()
	if released != 1 {
		t.Fatalf("released %d times, want 1", released)
	}
}

func TestGuardReportsAPlatformThatCannotHold(t *testing.T) {
	want := errors.New("unsupported")
	guard := Guard{hold: func(string) (func(), error) { return nil, want }}
	if _, err := guard.Hold("pomodoro"); !errors.Is(err, want) {
		t.Fatalf("err is %v, want %v", err, want)
	}
}
