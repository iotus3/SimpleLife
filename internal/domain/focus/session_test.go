package focus_test

import (
	"testing"
	"time"

	"github.com/iotus3/simplelife/app/internal/domain/focus"
	"github.com/iotus3/simplelife/app/internal/domain/shared"
)

func at(s string) time.Time {
	t, err := time.ParseInLocation("2006-01-02 15:04:05", s, time.Local)
	if err != nil {
		panic(err)
	}
	return t
}

func mustStart(t *testing.T, planned time.Duration) *focus.Session {
	t.Helper()
	s, err := focus.Start("01J0FOCUS000000000000001", "01J0TASKNEW0000000000000A", focus.KindFocus, planned, at("2026-11-17 09:00:00"))
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	return s
}

func TestStartValidates(t *testing.T) {
	now := at("2026-11-17 09:00:00")
	if _, err := focus.Start("", "", focus.KindFocus, time.Minute, now); !shared.IsKind(err, shared.KindInvalid) {
		t.Fatalf("empty id must be invalid, got %v", err)
	}
	if _, err := focus.Start("01J0FOCUS000000000000001", "", focus.Kind(9), time.Minute, now); !shared.IsKind(err, shared.KindInvalid) {
		t.Fatalf("unknown kind must be invalid, got %v", err)
	}
	if _, err := focus.Start("01J0FOCUS000000000000001", "", focus.KindFocus, -time.Minute, now); !shared.IsKind(err, shared.KindInvalid) {
		t.Fatalf("negative planned must be invalid, got %v", err)
	}
	s, err := focus.Start("01J0FOCUS000000000000001", "", focus.KindFocus, 0, now)
	if err != nil {
		t.Fatal(err)
	}
	if !s.IsCountUp() || s.Phase != focus.PhaseRunning {
		t.Fatalf("unexpected session: %+v", s)
	}
}

func TestCountdownUsesWallClockNotTicks(t *testing.T) {
	s := mustStart(t, 25*time.Minute)
	if got := s.Remaining(at("2026-11-17 09:10:00")); got != 15*time.Minute {
		t.Fatalf("Remaining = %v, want 15m", got)
	}
	if got := s.Elapsed(at("2026-11-17 09:10:00")); got != 10*time.Minute {
		t.Fatalf("Elapsed = %v, want 10m", got)
	}
	if got := s.Progress(at("2026-11-17 09:05:00")); got != 0.2 {
		t.Fatalf("Progress = %v, want 0.2", got)
	}
	if s.Expired(at("2026-11-17 09:24:59")) {
		t.Fatal("must not be expired one second early")
	}
	if !s.Expired(at("2026-11-17 09:25:00")) {
		t.Fatal("must be expired at the deadline")
	}
	if got := s.Remaining(at("2026-11-17 12:00:00")); got != 0 {
		t.Fatalf("Remaining must clamp at zero, got %v", got)
	}
	if got := s.Progress(at("2026-11-17 12:00:00")); got != 1 {
		t.Fatalf("Progress must clamp at one, got %v", got)
	}
}

func TestPauseSurvivesLongSleep(t *testing.T) {
	// The regression this guards: counting down with ticks makes a paused or
	// suspended app lose time. Here the clock jumps an hour while paused and the
	// remaining duration must not move by a millisecond.
	s := mustStart(t, 25*time.Minute)
	if err := s.Pause(at("2026-11-17 09:10:00")); err != nil {
		t.Fatal(err)
	}
	if got := s.Remaining(at("2026-11-17 10:10:00")); got != 15*time.Minute {
		t.Fatalf("Remaining while paused = %v, want 15m", got)
	}
	if err := s.Resume(at("2026-11-17 10:10:00")); err != nil {
		t.Fatal(err)
	}
	if got := s.Remaining(at("2026-11-17 10:10:00")); got != 15*time.Minute {
		t.Fatalf("Remaining right after resume = %v, want 15m", got)
	}
	if got := s.Remaining(at("2026-11-17 10:15:00")); got != 10*time.Minute {
		t.Fatalf("Remaining five minutes later = %v, want 10m", got)
	}
	if got := s.Elapsed(at("2026-11-17 10:15:00")); got != 15*time.Minute {
		t.Fatalf("Elapsed = %v, want 15m (the paused hour does not count)", got)
	}
}

func TestPauseResumeStateMachine(t *testing.T) {
	s := mustStart(t, 25*time.Minute)
	if err := s.Resume(at("2026-11-17 09:01:00")); !shared.IsKind(err, shared.KindConflict) {
		t.Fatalf("resuming a running session must conflict, got %v", err)
	}
	if err := s.Pause(at("2026-11-17 09:10:00")); err != nil {
		t.Fatal(err)
	}
	if err := s.Pause(at("2026-11-17 09:11:00")); !shared.IsKind(err, shared.KindConflict) {
		t.Fatalf("pausing twice must conflict, got %v", err)
	}
	s.Complete(at("2026-11-17 09:12:00"))
	if err := s.Resume(at("2026-11-17 09:13:00")); !shared.IsKind(err, shared.KindConflict) {
		t.Fatalf("resuming a settled session must conflict, got %v", err)
	}
}

func TestCompleteFreezesElapsedAndRefusesEmpty(t *testing.T) {
	s := mustStart(t, 25*time.Minute)
	if err := s.Complete(at("2026-11-17 09:00:00")); !shared.IsKind(err, shared.KindConflict) {
		t.Fatalf("completing at zero elapsed must conflict, got %v", err)
	}
	if got := shared.Code(s.Complete(at("2026-11-17 09:00:00"))); got != "focus.complete.empty" {
		t.Fatalf("code = %q", got)
	}
	if err := s.Complete(at("2026-11-17 09:25:00")); err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if s.Phase != focus.PhaseDone {
		t.Fatalf("phase = %v", s.Phase)
	}
	// Elapsed is frozen at EndedAt: reading it later must not keep growing.
	if got, want := s.Elapsed(at("2026-11-17 18:00:00")), 25*time.Minute; got != want {
		t.Fatalf("Elapsed after completion = %v, want %v", got, want)
	}
	if got := s.CompletedFocusMinutes(); got != 25 {
		t.Fatalf("CompletedFocusMinutes = %d, want 25", got)
	}
	if err := s.Complete(at("2026-11-17 09:26:00")); !shared.IsKind(err, shared.KindConflict) {
		t.Fatalf("completing twice must conflict, got %v", err)
	}
}

func TestCompleteWhilePausedBanksThePause(t *testing.T) {
	s := mustStart(t, 25*time.Minute)
	if err := s.Pause(at("2026-11-17 09:10:00")); err != nil {
		t.Fatal(err)
	}
	if err := s.Complete(at("2026-11-17 09:20:00")); err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if got := s.Elapsed(at("2026-11-17 09:20:00")); got != 10*time.Minute {
		t.Fatalf("Elapsed = %v, want 10m", got)
	}
	if !s.PausedAt.IsZero() {
		t.Fatal("a settled session must not stay paused")
	}
	if got := s.PausedTotal; got != 10*time.Minute {
		t.Fatalf("PausedTotal = %v, want 10m", got)
	}
}

func TestCancelCountsInterruptions(t *testing.T) {
	s := mustStart(t, 25*time.Minute)
	if err := s.Cancel(at("2026-11-17 09:08:00"), true); err != nil {
		t.Fatal(err)
	}
	if s.Phase != focus.PhaseCanceled || s.Interruptions != 1 {
		t.Fatalf("unexpected session: %+v", s)
	}
	if got := s.CompletedFocusMinutes(); got != 0 {
		t.Fatalf("a canceled session credits no minutes, got %d", got)
	}
	if err := s.Cancel(at("2026-11-17 09:09:00"), true); !shared.IsKind(err, shared.KindConflict) {
		t.Fatalf("canceling twice must conflict, got %v", err)
	}

	other := mustStart(t, 25*time.Minute)
	if err := other.Cancel(at("2026-11-17 09:08:00"), false); err != nil {
		t.Fatal(err)
	}
	if other.Interruptions != 0 {
		t.Fatalf("an uninterrupted cancel must not bump the counter, got %d", other.Interruptions)
	}
}

func TestCountUpNeverExpires(t *testing.T) {
	s, err := focus.Start("01J0FOCUS000000000000001", "", focus.KindFocus, 0, at("2026-11-17 09:00:00"))
	if err != nil {
		t.Fatal(err)
	}
	if s.Expired(at("2026-11-17 20:00:00")) {
		t.Fatal("a count-up session must never expire")
	}
	if got := s.Remaining(at("2026-11-17 20:00:00")); got != 0 {
		t.Fatalf("Remaining = %v, want 0", got)
	}
	if got := s.Progress(at("2026-11-17 20:00:00")); got != 0 {
		t.Fatalf("Progress = %v, want 0 for a count-up session", got)
	}
	if got := s.ElapsedMinutes(at("2026-11-17 09:11:30")); got != 11 {
		t.Fatalf("ElapsedMinutes = %d, want 11 (floored)", got)
	}
}

func TestClockGoingBackwardsClamps(t *testing.T) {
	s := mustStart(t, 25*time.Minute)
	if got := s.Elapsed(at("2026-11-17 08:00:00")); got != 0 {
		t.Fatalf("Elapsed must clamp at zero, got %v", got)
	}
	if got := s.Remaining(at("2026-11-17 08:00:00")); got != 25*time.Minute {
		t.Fatalf("Remaining must not exceed Planned, got %v", got)
	}
	if got := s.Progress(at("2026-11-17 08:00:00")); got != 0 {
		t.Fatalf("Progress must clamp at zero, got %v", got)
	}
}

func TestSuspiciousAfterTwoHours(t *testing.T) {
	s := mustStart(t, 25*time.Minute)
	if s.Suspicious(at("2026-11-17 10:00:00")) {
		t.Fatal("an hour old session is not suspicious")
	}
	if !s.Suspicious(at("2026-11-17 11:30:00")) {
		t.Fatal("a session older than two hours must be flagged")
	}
	if err := s.Complete(at("2026-11-17 11:30:00")); err != nil {
		t.Fatal(err)
	}
	if s.Suspicious(at("2026-11-17 20:00:00")) {
		t.Fatal("a settled session is never suspicious")
	}
}

func TestKindAndPhaseParsing(t *testing.T) {
	for _, k := range []focus.Kind{focus.KindFocus, focus.KindShortBreak, focus.KindLongBreak} {
		back, err := focus.ParseKind(k.String())
		if err != nil || back != k {
			t.Fatalf("kind round trip failed: %v %v", back, err)
		}
	}
	if _, err := focus.ParseKind("nope"); !shared.IsKind(err, shared.KindInvalid) {
		t.Fatalf("unknown kind must be invalid, got %v", err)
	}
	if got := focus.Kind(99).String(); got != "unknown" {
		t.Fatalf("String = %q", got)
	}
	for _, p := range []focus.Phase{focus.PhaseRunning, focus.PhasePaused, focus.PhaseDone, focus.PhaseCanceled} {
		back, err := focus.ParsePhase(p.String())
		if err != nil || back != p {
			t.Fatalf("phase round trip failed: %v %v", back, err)
		}
	}
	if _, err := focus.ParsePhase("nope"); !shared.IsKind(err, shared.KindInvalid) {
		t.Fatalf("unknown phase must be invalid, got %v", err)
	}
	if got := focus.Phase(99).String(); got != "unknown" {
		t.Fatalf("String = %q", got)
	}
	if !focus.PhaseRunning.IsActive() || !focus.PhasePaused.IsActive() {
		t.Fatal("running and paused are active")
	}
	if focus.PhaseDone.IsActive() || focus.PhaseCanceled.IsActive() {
		t.Fatal("settled phases are not active")
	}
	if !focus.PhaseDone.IsFinal() || !focus.PhaseCanceled.IsFinal() {
		t.Fatal("IsFinal is wrong")
	}
}

func TestSetNoteAndIsActive(t *testing.T) {
	s := mustStart(t, 25*time.Minute)
	s.SetNote("备注")
	if s.Note != "备注" {
		t.Fatalf("note = %q", s.Note)
	}
	if !s.IsActive() {
		t.Fatal("a fresh session is active")
	}
	if err := s.Cancel(at("2026-11-17 09:01:00"), false); err != nil {
		t.Fatal(err)
	}
	if s.IsActive() {
		t.Fatal("a canceled session is not active")
	}
}
