// Package focus holds the pomodoro session state machine. A session stores timestamps, not a
// remaining-seconds counter, so sleeping or restarting cannot make it drift.
package focus

import (
	"time"

	"github.com/iotus3/simplelife/app/internal/domain/shared"
)

// Kind is what a session counts.
type Kind uint8

const (
	KindFocus Kind = iota + 1
	KindShortBreak
	KindLongBreak
)

// String returns the stable storage key.
func (k Kind) String() string {
	switch k {
	case KindFocus:
		return "focus"
	case KindShortBreak:
		return "short_break"
	case KindLongBreak:
		return "long_break"
	}
	return "unknown"
}

// ParseKind is the inverse of String.
func ParseKind(s string) (Kind, error) {
	switch s {
	case "focus":
		return KindFocus, nil
	case "short_break":
		return KindShortBreak, nil
	case "long_break":
		return KindLongBreak, nil
	}
	return 0, shared.Invalid("focus.kind.parse", "", "unknown session kind: "+s)
}

// Phase is where a session sits in its lifecycle.
type Phase uint8

const (
	PhaseRunning Phase = iota + 1
	PhasePaused
	PhaseDone
	PhaseCanceled
)

// String returns the stable storage key.
func (p Phase) String() string {
	switch p {
	case PhaseRunning:
		return "running"
	case PhasePaused:
		return "paused"
	case PhaseDone:
		return "done"
	case PhaseCanceled:
		return "canceled"
	}
	return "unknown"
}

// ParsePhase is the inverse of String.
func ParsePhase(s string) (Phase, error) {
	switch s {
	case "running":
		return PhaseRunning, nil
	case "paused":
		return PhasePaused, nil
	case "done":
		return PhaseDone, nil
	case "canceled":
		return PhaseCanceled, nil
	}
	return 0, shared.Invalid("focus.phase.parse", "", "unknown session phase: "+s)
}

// IsActive reports whether the session still occupies the single active slot.
func (p Phase) IsActive() bool { return p == PhaseRunning || p == PhasePaused }

// IsFinal reports whether the session has been settled.
func (p Phase) IsFinal() bool { return p == PhaseDone || p == PhaseCanceled }

// SuspiciousAfter is how long an active session may hang before the app asks the
// user what happened instead of silently crediting a huge pomodoro count. A
// closed laptop overnight is the case this exists for.
const SuspiciousAfter = 2 * time.Hour

// Session is the aggregate root. All fields are unexported-by-convention: the
// application layer reads them, but only the methods below may change state.
type Session struct {
	ID            shared.ID
	TaskID        shared.ID // empty = unbound focus
	PredecessorID shared.ID // previous session, used to pair focus with its break
	Kind          Kind
	Phase         Phase
	Planned       time.Duration // 0 = count-up stopwatch
	StartedAt     time.Time
	EndedAt       time.Time // zero while active
	PausedTotal   time.Duration
	PausedAt      time.Time // non-zero = currently paused
	Interruptions int
	Note          string
}

// Start opens a session. planned may be 0 for a count-up timer.
func Start(id shared.ID, taskID shared.ID, kind Kind, planned time.Duration, now time.Time) (*Session, error) {
	if id.IsZero() {
		return nil, shared.Invalid("focus.id.empty", "", "session id is required")
	}
	if kind != KindFocus && kind != KindShortBreak && kind != KindLongBreak {
		return nil, shared.Invalid("focus.kind.range", "", "unknown session kind")
	}
	if planned < 0 {
		return nil, shared.Invalid("focus.planned.range", "", "planned duration must not be negative")
	}
	return &Session{
		ID:        id,
		TaskID:    taskID,
		Kind:      kind,
		Phase:     PhaseRunning,
		Planned:   planned,
		StartedAt: now,
	}, nil
}

// IsActive reports whether the session can still be paused, resumed or settled.
func (s *Session) IsActive() bool { return s.Phase.IsActive() }

// IsCountUp reports a stopwatch session (no planned duration).
func (s *Session) IsCountUp() bool { return s.Planned <= 0 }

// elapsedAt is the single source of truth for the timer.
func (s *Session) elapsedAt(now time.Time) time.Duration {
	end := now
	if !s.EndedAt.IsZero() {
		end = s.EndedAt
	}
	d := end.Sub(s.StartedAt) - s.PausedTotal
	if !s.PausedAt.IsZero() {
		d -= end.Sub(s.PausedAt) // subtract the pause that is still open
	}
	if d < 0 {
		// The wall clock was moved backwards. Clamp instead of returning a
		// negative duration, which would make Remaining exceed Planned.
		return 0
	}
	return d
}

// Elapsed is the real time invested. It may exceed Planned when the app was
// frozen at the deadline and settled later.
func (s *Session) Elapsed(now time.Time) time.Duration { return s.elapsedAt(now) }

// ElapsedMinutes is the whole minutes invested, floored. Money is computed from
// this so a 25:59 pomodoro pays for 25 minutes, not 26.
func (s *Session) ElapsedMinutes(now time.Time) int {
	return int(s.elapsedAt(now) / time.Minute)
}

// Remaining is clamped to [0, Planned]. A count-up session returns 0.
func (s *Session) Remaining(now time.Time) time.Duration {
	if s.Planned <= 0 {
		return 0
	}
	if r := s.Planned - s.elapsedAt(now); r > 0 {
		return r
	}
	return 0
}

// Progress is the fraction of Planned already spent, in [0,1]. A count-up session
// returns 0; the UI shows the elapsed duration instead of a ring.
func (s *Session) Progress(now time.Time) float64 {
	if s.Planned <= 0 {
		return 0
	}
	p := float64(s.elapsedAt(now)) / float64(s.Planned)
	if p > 1 {
		return 1
	}
	if p < 0 {
		return 0
	}
	return p
}

// Expired reports that a countdown session has reached its planned duration. A
// count-up session never expires: it runs until the user stops it.
func (s *Session) Expired(now time.Time) bool {
	return s.Planned > 0 && s.Remaining(now) == 0
}

// Suspicious reports an active session that has outlived the sanity threshold, so
// the app can ask "did you finish, or did the machine sleep?" instead of guessing.
func (s *Session) Suspicious(now time.Time) bool {
	return s.IsActive() && s.elapsedAt(now) > SuspiciousAfter
}

// Pause stops the clock. Only a running session can pause.
func (s *Session) Pause(now time.Time) error {
	if s.Phase != PhaseRunning {
		return shared.Conflict("focus.pause.not_running", "only a running session can pause")
	}
	s.Phase = PhasePaused
	s.PausedAt = now
	return nil
}

// Resume restarts the clock and banks the pause that just ended.
func (s *Session) Resume(now time.Time) error {
	if s.Phase != PhasePaused {
		return shared.Conflict("focus.resume.not_paused", "only a paused session can resume")
	}
	if !s.PausedAt.IsZero() {
		if delta := now.Sub(s.PausedAt); delta > 0 {
			s.PausedTotal += delta
		}
	}
	s.PausedAt = time.Time{}
	s.Phase = PhaseRunning
	return nil
}

// Complete settles the session as done. A session that never ran a single second
// cannot be completed — that would credit a pomodoro the user never did.
func (s *Session) Complete(now time.Time) error {
	if s.Phase.IsFinal() {
		return shared.Conflict("focus.complete.final", "session is already settled")
	}
	if s.elapsedAt(now) <= 0 {
		return shared.Conflict("focus.complete.empty", "a session with no elapsed time cannot be completed")
	}
	s.closeAt(now)
	s.Phase = PhaseDone
	return nil
}

// Cancel settles the session as abandoned. interrupted marks a cancel the user
// caused (which lowers the reward quality of a later session and raises the
// interruption count); a cancel triggered by "start over" also counts.
func (s *Session) Cancel(now time.Time, interrupted bool) error {
	if s.Phase.IsFinal() {
		return shared.Conflict("focus.cancel.final", "session is already settled")
	}
	s.closeAt(now)
	s.Phase = PhaseCanceled
	if interrupted {
		s.Interruptions++
	}
	return nil
}

// closeAt freezes the clock: it folds an open pause into PausedTotal and stamps
// EndedAt, so every later Elapsed call returns the same value.
func (s *Session) closeAt(now time.Time) {
	if !s.PausedAt.IsZero() {
		if delta := now.Sub(s.PausedAt); delta > 0 {
			s.PausedTotal += delta
		}
		s.PausedAt = time.Time{}
	}
	if now.Before(s.StartedAt) {
		now = s.StartedAt
	}
	s.EndedAt = now
}

// SetNote stores the free-form note, trimmed by the caller.
func (s *Session) SetNote(note string) { s.Note = note }

// CompletedFocusMinutes is the credited length of a finished focus session.
func (s *Session) CompletedFocusMinutes() int {
	if s.Kind != KindFocus || s.Phase != PhaseDone {
		return 0
	}
	return int(s.Elapsed(s.EndedAt) / time.Minute)
}

// Repository is the persistence port for sessions.
type Repository interface {
	Create(s *Session) error
	Update(s *Session) error
	Get(id shared.ID) (*Session, error)
	// Active returns the running or paused session, at most one (the store
	// enforces it with a partial unique index).
	Active() (*Session, error)
	// ListByRange returns sessions whose StartedAt falls in [from, to), newest
	// first.
	ListByRange(from, to time.Time) ([]*Session, error)
	ListRecent(limit int) ([]*Session, error)
	// FocusCountBetween counts completed focus sessions starting in [from, to).
	FocusCountBetween(from, to time.Time) (int, error)
	// FocusMinutesBetween sums credited focus minutes in [from, to).
	FocusMinutesBetween(from, to time.Time) (int, error)
	// TaskFocusMinutes sums credited minutes for one task.
	TaskFocusMinutes(taskID shared.ID) (int, error)
}
