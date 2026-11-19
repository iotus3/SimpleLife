package focus_test

import (
	"testing"
	"time"

	"github.com/iotus3/simplelife/app/internal/domain/focus"
	"github.com/iotus3/simplelife/app/internal/domain/shared"
)

// economy builds the defaults: half a yuan per minute, half penalty.
func economy(t *testing.T, enabled bool) focus.EconomySettings {
	t.Helper()
	e := focus.DefaultEconomySettings(shared.CNY)
	e.Enabled = enabled
	e.RewardAccountID = "01J0ACCOUNTPOT00000000000A"
	e.RewardCategoryID = "01J0CATEGORYAWARD0000000A"
	e.PenaltyCategoryID = "01J0CATEGORYFINE00000000A"
	if err := e.Validate(); err != nil {
		t.Fatalf("default economy settings must be valid: %v", err)
	}
	return e
}

// completedFocus returns a finished focus session that ran for d from 09:00.
func completedFocus(t *testing.T, d time.Duration, interruptions int) *focus.Session {
	t.Helper()
	s, err := focus.Start("01J0FOCUS000000000000001", "", focus.KindFocus, 25*time.Minute, at("2026-11-17 09:00:00"))
	if err != nil {
		t.Fatal(err)
	}
	s.Interruptions = interruptions
	if err := s.Complete(at("2026-11-17 09:00:00").Add(d)); err != nil {
		t.Fatalf("Complete: %v", err)
	}
	return s
}

func canceledFocus(t *testing.T, elapsed, planned time.Duration) *focus.Session {
	t.Helper()
	s, err := focus.Start("01J0FOCUS000000000000002", "", focus.KindFocus, planned, at("2026-11-17 09:00:00"))
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Cancel(at("2026-11-17 09:00:00").Add(elapsed), true); err != nil {
		t.Fatalf("Cancel: %v", err)
	}
	return s
}

func TestDefaultEconomySettingsMatchTheDesign(t *testing.T) {
	e := focus.DefaultEconomySettings(shared.CNY)
	if e.Enabled {
		t.Fatal("the economy must be off by default")
	}
	if e.Rate.Minor() != 50 {
		t.Fatalf("rate = %d, want 50 minor units (¥0.50/min)", e.Rate.Minor())
	}
	if e.PenaltyRatioPermille != 500 || e.PenaltyMinMinutes != 5 {
		t.Fatalf("penalty defaults changed: %+v", e)
	}
	if got, want := e.RewardCapDaily.Minor(), int64(8*25*50); got != want {
		t.Fatalf("reward cap = %d, want %d", got, want)
	}
	if got, want := e.PenaltyCapDaily.Minor(), int64(4*25*50*500/1000); got != want {
		t.Fatalf("penalty cap = %d, want %d", got, want)
	}
	usd := focus.DefaultEconomySettings(shared.USD)
	if usd.Rate.Currency() != shared.USD {
		t.Fatal("the default rate must follow the account currency")
	}
}

func TestRewardFormulaCases(t *testing.T) {
	// The formula table, case by case.
	cases := []struct {
		name          string
		elapsed       time.Duration
		interruptions int
		wantMinor     int64
	}{
		{"25 分钟无中断", 25 * time.Minute, 0, 1250},
		{"25 分钟中断 1 次", 25 * time.Minute, 1, 1000},
		{"40 分钟中断 3 次", 40 * time.Minute, 3, 1200},
		{"25 分 59 秒", 25*time.Minute + 59*time.Second, 0, 1250},
		{"不足 1 分钟", 59 * time.Second, 0, 0},
	}
	e := economy(t, true)
	for _, c := range cases {
		s := completedFocus(t, c.elapsed, c.interruptions)
		out, err := e.Reward(s, shared.ZeroMoney(shared.CNY))
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		if out.Amount.Minor() != c.wantMinor {
			t.Fatalf("%s: reward = %d, want %d", c.name, out.Amount.Minor(), c.wantMinor)
		}
	}
}

func TestRewardIsCappedByTheDayRemainder(t *testing.T) {
	e := economy(t, true)
	s := completedFocus(t, 25*time.Minute, 0)
	// Already rewarded 9 000 of the 10 000 cap: only 1 000 is left.
	out, err := e.Reward(s, shared.NewMoney(9000, shared.CNY))
	if err != nil {
		t.Fatal(err)
	}
	if out.Amount.Minor() != 1000 || !out.CappedByDaily {
		t.Fatalf("outcome = %+v, want 1000 capped", out)
	}
	// Ninth pomodoro of the day: the cap is already used up.
	out, err = e.Reward(s, shared.NewMoney(10000, shared.CNY))
	if err != nil {
		t.Fatal(err)
	}
	if out.Applicable() || out.Reason != focus.ReasonCapReached {
		t.Fatalf("outcome = %+v, want cap reached", out)
	}
}

func TestPenaltyFormulaCases(t *testing.T) {
	cases := []struct {
		name      string
		elapsed   time.Duration
		planned   time.Duration
		wantMinor int64
		reason    string
	}{
		{"计划 25 分钟第 8 分钟放弃", 8 * time.Minute, 25 * time.Minute, 625, ""},
		{"计划 25 分钟第 3 分钟放弃", 3 * time.Minute, 25 * time.Minute, 0, focus.ReasonBelowMinimum},
		{"正计时取消", 60 * time.Minute, 0, 0, focus.ReasonCountUp},
		{"计划 1 小时第 30 分钟放弃", 30 * time.Minute, time.Hour, 1500, ""},
	}
	e := economy(t, true)
	for _, c := range cases {
		s := canceledFocus(t, c.elapsed, c.planned)
		out, err := e.Penalty(s, shared.ZeroMoney(shared.CNY))
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		if out.Amount.Minor() != c.wantMinor {
			t.Fatalf("%s: penalty = %d, want %d", c.name, out.Amount.Minor(), c.wantMinor)
		}
		if c.reason != "" && out.Reason != c.reason {
			t.Fatalf("%s: reason = %q, want %q", c.name, out.Reason, c.reason)
		}
	}
}

func TestPenaltyUsesPlannedDurationNotElapsed(t *testing.T) {
	// Bailing at minute 6 and at minute 24 of a 25 minute plan must cost the same;
	// otherwise the rational move is to quit just before the end.
	e := economy(t, true)
	early := canceledFocus(t, 6*time.Minute, 25*time.Minute)
	late := canceledFocus(t, 24*time.Minute, 25*time.Minute)
	a, err := e.Penalty(early, shared.ZeroMoney(shared.CNY))
	if err != nil {
		t.Fatal(err)
	}
	b, err := e.Penalty(late, shared.ZeroMoney(shared.CNY))
	if err != nil {
		t.Fatal(err)
	}
	if a.Amount.Minor() != b.Amount.Minor() {
		t.Fatalf("penalty depends on elapsed time: %d vs %d", a.Amount.Minor(), b.Amount.Minor())
	}
}

func TestPenaltyIsCapped(t *testing.T) {
	e := economy(t, true)
	s := canceledFocus(t, 8*time.Minute, 25*time.Minute)
	out, err := e.Penalty(s, shared.NewMoney(e.PenaltyCapDaily.Minor()-100, shared.CNY))
	if err != nil {
		t.Fatal(err)
	}
	if out.Amount.Minor() != 100 || !out.CappedByDaily {
		t.Fatalf("outcome = %+v, want 100 capped", out)
	}
	out, err = e.Penalty(s, e.PenaltyCapDaily)
	if err != nil {
		t.Fatal(err)
	}
	if out.Applicable() || out.Reason != focus.ReasonCapReached {
		t.Fatalf("outcome = %+v, want cap reached", out)
	}
}

func TestEconomyGuards(t *testing.T) {
	off := economy(t, false)
	s := completedFocus(t, 25*time.Minute, 0)
	if out, err := off.Reward(s, shared.ZeroMoney(shared.CNY)); err != nil || out.Reason != focus.ReasonDisabled {
		t.Fatalf("disabled economy must report %q, got %+v %v", focus.ReasonDisabled, out, err)
	}

	e := economy(t, true)
	breakSession, err := focus.Start("01J0FOCUS000000000000003", "", focus.KindShortBreak, 5*time.Minute, at("2026-11-17 09:00:00"))
	if err != nil {
		t.Fatal(err)
	}
	breakSession.Complete(at("2026-11-17 09:05:00"))
	if out, _ := e.Reward(breakSession, shared.ZeroMoney(shared.CNY)); out.Reason != focus.ReasonNotFocus {
		t.Fatalf("a break must not be rewarded, got %+v", out)
	}

	running := mustStart(t, 25*time.Minute)
	if out, _ := e.Reward(running, shared.ZeroMoney(shared.CNY)); out.Reason != focus.ReasonNotSettled {
		t.Fatalf("an active session must not be rewarded, got %+v", out)
	}
	if out, _ := e.Penalty(running, shared.ZeroMoney(shared.CNY)); out.Reason != focus.ReasonNotSettled {
		t.Fatalf("an active session must not be penalised, got %+v", out)
	}

	if _, err := e.Reward(s, shared.ZeroMoney(shared.USD)); err != shared.ErrCurrencyMismatch {
		t.Fatalf("a currency mismatch must be refused, got %v", err)
	}
	if _, err := e.Penalty(canceledFocus(t, 8*time.Minute, 25*time.Minute), shared.ZeroMoney(shared.USD)); err != shared.ErrCurrencyMismatch {
		t.Fatalf("a currency mismatch must be refused, got %v", err)
	}
}

func TestEconomyValidate(t *testing.T) {
	base := economy(t, true)

	bad := base
	bad.Rate = shared.NewMoney(0, shared.CNY)
	if !shared.IsKind(bad.Validate(), shared.KindInvalid) {
		t.Fatal("a zero rate must be invalid")
	}
	bad = base
	bad.Rate = shared.NewMoney(10001, shared.CNY)
	if !shared.IsKind(bad.Validate(), shared.KindInvalid) {
		t.Fatal("a rate above the ceiling must be invalid")
	}
	bad = base
	bad.Rate = shared.NewMoney(50, shared.Currency("EUR"))
	if !shared.IsKind(bad.Validate(), shared.KindInvalid) {
		t.Fatal("an unsupported currency must be invalid")
	}
	bad = base
	bad.PenaltyRatioPermille = 2001
	if !shared.IsKind(bad.Validate(), shared.KindInvalid) {
		t.Fatal("a penalty ratio above 2.0 must be invalid")
	}
	bad = base
	bad.PenaltyMinMinutes = 61
	if !shared.IsKind(bad.Validate(), shared.KindInvalid) {
		t.Fatal("a grace period above 60 minutes must be invalid")
	}
	bad = base
	bad.RewardCapDaily = shared.NewMoney(-1, shared.CNY)
	if !shared.IsKind(bad.Validate(), shared.KindInvalid) {
		t.Fatal("a negative cap must be invalid")
	}
	bad = base
	bad.PenaltyCapDaily = shared.NewMoney(100, shared.USD)
	if !shared.IsKind(bad.Validate(), shared.KindInvalid) {
		t.Fatal("a cap in another currency must be invalid")
	}
	bad = base
	bad.RewardAccountID = ""
	if !shared.IsKind(bad.Validate(), shared.KindInvalid) {
		t.Fatal("an enabled economy needs an account")
	}
	bad = base
	bad.PenaltyCategoryID = ""
	if !shared.IsKind(bad.Validate(), shared.KindInvalid) {
		t.Fatal("an enabled economy needs both categories")
	}
	disabled := bad
	disabled.Enabled = false
	if err := disabled.Validate(); err != nil {
		t.Fatalf("a disabled economy may omit the ids: %v", err)
	}
}

func TestQualityPermille(t *testing.T) {
	cases := map[int]int{0: focus.QualityPerfect, 1: focus.QualityOneBreak, 2: focus.QualityManyBreaks, 9: focus.QualityManyBreaks}
	for interruptions, want := range cases {
		if got := focus.QualityPermille(&focus.Session{Interruptions: interruptions}); got != want {
			t.Fatalf("%d interruptions: %d, want %d", interruptions, got, want)
		}
	}
}

func TestTimerSettingsDefaultsAndBounds(t *testing.T) {
	s := focus.DefaultTimerSettings()
	if s.FocusDuration != 25*time.Minute || s.ShortBreak != 5*time.Minute || s.LongBreak != 15*time.Minute || s.LongBreakEvery != 4 {
		t.Fatalf("defaults changed: %+v", s)
	}
	if s.AutoStartBreak || s.AutoStartFocus {
		t.Fatal("auto-start must default to off")
	}
	if err := s.Validate(); err != nil {
		t.Fatalf("defaults must be valid: %v", err)
	}
	if got := s.DurationFor(focus.KindFocus); got != 25*time.Minute {
		t.Fatalf("DurationFor(focus) = %v", got)
	}
	if got := s.DurationFor(focus.KindShortBreak); got != 5*time.Minute {
		t.Fatalf("DurationFor(short) = %v", got)
	}
	if got := s.DurationFor(focus.KindLongBreak); got != 15*time.Minute {
		t.Fatalf("DurationFor(long) = %v", got)
	}
	if got := s.DurationFor(focus.Kind(99)); got != 0 {
		t.Fatalf("DurationFor(unknown) = %v", got)
	}

	bad := []focus.TimerSettings{
		{FocusDuration: 0, ShortBreak: time.Minute, LongBreak: time.Minute, LongBreakEvery: 4},
		{FocusDuration: 181 * time.Minute, ShortBreak: time.Minute, LongBreak: time.Minute, LongBreakEvery: 4},
		{FocusDuration: time.Minute, ShortBreak: 0, LongBreak: time.Minute, LongBreakEvery: 4},
		{FocusDuration: time.Minute, ShortBreak: 61 * time.Minute, LongBreak: time.Minute, LongBreakEvery: 4},
		{FocusDuration: time.Minute, ShortBreak: time.Minute, LongBreak: 0, LongBreakEvery: 4},
		{FocusDuration: time.Minute, ShortBreak: time.Minute, LongBreak: 121 * time.Minute, LongBreakEvery: 4},
		{FocusDuration: time.Minute, ShortBreak: time.Minute, LongBreak: time.Minute, LongBreakEvery: 1},
		{FocusDuration: time.Minute, ShortBreak: time.Minute, LongBreak: time.Minute, LongBreakEvery: 13},
	}
	for i, b := range bad {
		if !shared.IsKind(b.Validate(), shared.KindInvalid) {
			t.Fatalf("case %d must be invalid", i)
		}
	}
}

func TestNextBreakKindCycle(t *testing.T) {
	s := focus.DefaultTimerSettings() // long break every 4
	cases := map[int]focus.Kind{
		0: focus.KindShortBreak,
		1: focus.KindShortBreak,
		2: focus.KindShortBreak,
		3: focus.KindShortBreak,
		4: focus.KindLongBreak,
		8: focus.KindLongBreak,
		9: focus.KindShortBreak,
	}
	for completed, want := range cases {
		if got := s.NextBreakKind(completed); got != want {
			t.Fatalf("%d completed: %v, want %v", completed, got, want)
		}
	}
}
