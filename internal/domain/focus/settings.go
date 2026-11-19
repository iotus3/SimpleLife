package focus

import (
	"time"

	"github.com/iotus3/simplelife/app/internal/domain/shared"
)

// Durations are bounded so the UI sliders and the keyboard input agree, and so no
// single session can swallow a day.
const (
	FocusMin = time.Minute
	FocusMax = 180 * time.Minute

	ShortBreakMin = time.Minute
	ShortBreakMax = 60 * time.Minute

	LongBreakMin = time.Minute
	LongBreakMax = 120 * time.Minute

	LongBreakEveryMin = 2
	LongBreakEveryMax = 12
)

// TimerSettings is a pure value object: defaults, bounds and the derived break
// rule live here so the UI only binds numbers.
type TimerSettings struct {
	FocusDuration  time.Duration
	ShortBreak     time.Duration
	LongBreak      time.Duration
	LongBreakEvery int
	AutoStartBreak bool
	AutoStartFocus bool
}

// DefaultTimerSettings is the classic 25 / 5 / 15 with a long break every 4th
// pomodoro. Both auto-start switches default to off: starting a break for the
// user while they are in a meeting is worse than one extra click.
func DefaultTimerSettings() TimerSettings {
	return TimerSettings{
		FocusDuration:  25 * time.Minute,
		ShortBreak:     5 * time.Minute,
		LongBreak:      15 * time.Minute,
		LongBreakEvery: 4,
	}
}

// Validate checks every field. The form calls it before saving, so an out-of-range
// value can never reach the database.
func (s TimerSettings) Validate() error {
	if s.FocusDuration < FocusMin || s.FocusDuration > FocusMax {
		return shared.Invalid("focus.settings.focus", "focus_duration", "focus duration must be 1..180 minutes")
	}
	if s.ShortBreak < ShortBreakMin || s.ShortBreak > ShortBreakMax {
		return shared.Invalid("focus.settings.short_break", "short_break", "short break must be 1..60 minutes")
	}
	if s.LongBreak < LongBreakMin || s.LongBreak > LongBreakMax {
		return shared.Invalid("focus.settings.long_break", "long_break", "long break must be 1..120 minutes")
	}
	if s.LongBreakEvery < LongBreakEveryMin || s.LongBreakEvery > LongBreakEveryMax {
		return shared.Invalid("focus.settings.long_break_every", "long_break_every", "long break interval must be 2..12")
	}
	return nil
}

// DurationFor returns the configured length of a session kind.
func (s TimerSettings) DurationFor(k Kind) time.Duration {
	switch k {
	case KindFocus:
		return s.FocusDuration
	case KindShortBreak:
		return s.ShortBreak
	case KindLongBreak:
		return s.LongBreak
	}
	return 0
}

// NextBreakKind decides which break follows the given number of focus sessions completed
// today: every Nth becomes a long break. The count comes from the store, so a restart does
// not reset the cycle.
func (s TimerSettings) NextBreakKind(completedFocusToday int) Kind {
	every := s.LongBreakEvery
	if every < LongBreakEveryMin {
		every = LongBreakEveryMin
	}
	if completedFocusToday > 0 && completedFocusToday%every == 0 {
		return KindLongBreak
	}
	return KindShortBreak
}
