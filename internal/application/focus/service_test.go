package focus_test

import (
	"errors"
	"testing"
	"time"

	focusapp "github.com/iotus3/simplelife/app/internal/application/focus"
	ledgerapp "github.com/iotus3/simplelife/app/internal/application/ledger"
	taskapp "github.com/iotus3/simplelife/app/internal/application/task"
	"github.com/iotus3/simplelife/app/internal/apptest/harness"
	"github.com/iotus3/simplelife/app/internal/apptest/store"
	"github.com/iotus3/simplelife/app/internal/contracts"
	"github.com/iotus3/simplelife/app/internal/domain/focus"
	"github.com/iotus3/simplelife/app/internal/domain/ledger"
	"github.com/iotus3/simplelife/app/internal/domain/shared"
	task "github.com/iotus3/simplelife/app/internal/domain/task"
)

// stubSettings is the two-method Settings port the focus service needs.
type stubSettings struct {
	timer   focus.TimerSettings
	economy focus.EconomySettings
}

func (s stubSettings) Timer() focus.TimerSettings     { return s.timer }
func (s stubSettings) Economy() focus.EconomySettings { return s.economy }

type fixture struct {
	svc     *focusapp.Service
	tasks   *taskapp.Service
	money   *ledgerapp.Service
	store   *store.Store
	clock   *harness.Clock
	set     *stubSettings
	account shared.ID
	task    shared.ID
}

// newFixture wires the real application services against the in-memory store, so
// the economy assertions check what the user would actually see in the ledger.
func newFixture(t *testing.T, enabled bool) *fixture {
	t.Helper()
	st := store.New()
	clk := harness.NewClock(harness.At("2026-11-17 09:00:00"))
	ids := harness.NewIDs()
	tasks := taskapp.New(st.Tasks(), st, clk, ids)
	money := ledgerapp.New(st.Entries(), st.Accounts(), st.Categories(), st.Budgets(), st, clk, ids)

	econ := focus.DefaultEconomySettings(shared.CNY)
	econ.Enabled = enabled
	cash, err := money.CreateAccount("现金", ledger.AccountCash, shared.NewMoney(0, shared.CNY))
	if err != nil {
		t.Fatal(err)
	}
	pot, err := st.Accounts().Get(cash.ID)
	if err != nil {
		t.Fatal(err)
	}
	award, err := money.CreateCategory("专注奖励", ledger.CategoryIncome, "")
	if err != nil {
		t.Fatal(err)
	}
	fine, err := money.CreateCategory("专注罚金", ledger.CategoryExpense, "")
	if err != nil {
		t.Fatal(err)
	}
	econ.RewardAccountID = pot.ID
	econ.RewardCategoryID = award.ID
	econ.PenaltyCategoryID = fine.ID

	set := &stubSettings{timer: focus.DefaultTimerSettings(), economy: econ}
	svc := focusapp.New(st.Sessions(), st.Tasks(), st.Entries(), money, st, clk, ids, set, shared.NoopNotifier{}, shared.NoopSleepGuard{})

	tk, err := tasks.Create(taskapp.CreateInput{Title: "写代码", DueDate: shared.DateOf(clk.Now()), Estimate: 2})
	if err != nil {
		t.Fatal(err)
	}
	return &fixture{svc: svc, tasks: tasks, money: money, store: st, clock: clk, set: set, account: pot.ID, task: tk.ID}
}

func TestStartMarksTheTaskInProgressAndRefusesASecond(t *testing.T) {
	f := newFixture(t, false)
	session, err := f.svc.Start(focusapp.StartInput{Kind: focus.KindFocus, TaskID: f.task})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	if session.Planned != 25*time.Minute {
		t.Fatalf("planned = %v, want the configured 25m", session.Planned)
	}
	stored, err := f.store.Tasks().Get(f.task)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Status != task.StatusDoing {
		t.Fatalf("task status = %v, want doing", stored.Status)
	}
	if _, err := f.svc.Start(focusapp.StartInput{Kind: focus.KindFocus}); !shared.IsKind(err, shared.KindConflict) {
		t.Fatalf("a second active session must conflict, got %v", err)
	}
	if got := shared.Code(secondStart(f.svc)); got != "focus.start.already_active" {
		t.Fatalf("code = %q", got)
	}
}

func TestStartUsesConfiguredBreakDuration(t *testing.T) {
	f := newFixture(t, false)
	session, err := f.svc.Start(focusapp.StartInput{Kind: focus.KindShortBreak})
	if err != nil {
		t.Fatal(err)
	}
	if session.Planned != 5*time.Minute {
		t.Fatalf("planned = %v, want 5m", session.Planned)
	}
	other := newFixture(t, false)
	if _, err := other.svc.Start(focusapp.StartInput{Kind: focus.KindFocus, Planned: 50 * time.Minute}); err != nil {
		t.Fatal(err)
	}
	active, err := other.svc.Active()
	if err != nil {
		t.Fatal(err)
	}
	if active.Planned != 50*time.Minute {
		t.Fatalf("an explicit planned duration must win, got %v", active.Planned)
	}
}

func TestCompleteCreditsTheTaskAndBooksTheReward(t *testing.T) {
	f := newFixture(t, true)
	if _, err := f.svc.Start(focusapp.StartInput{Kind: focus.KindFocus, TaskID: f.task}); err != nil {
		t.Fatal(err)
	}
	f.clock.Advance(25 * time.Minute)
	session, outcome, err := f.svc.Complete()
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if session.Phase != focus.PhaseDone {
		t.Fatalf("phase = %v", session.Phase)
	}
	if outcome.Amount.Minor() != 1250 {
		t.Fatalf("reward = %d, want 1250 (¥12.50)", outcome.Amount.Minor())
	}
	stored, err := f.store.Tasks().Get(f.task)
	if err != nil {
		t.Fatal(err)
	}
	if stored.DonePomodoros != 1 {
		t.Fatalf("DonePomodoros = %d, want 1", stored.DonePomodoros)
	}
	entries, err := f.store.Entries().List(ledger.EntryFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("want exactly one booking, got %d", len(entries))
	}
	entry := entries[0]
	if entry.Origin != ledger.OriginFocusReward || entry.FocusSessionID != session.ID {
		t.Fatalf("booking = %+v", entry)
	}
	if entry.Kind != ledger.EntryIncome || entry.Amount.Minor() != 1250 {
		t.Fatalf("booking amount = %v %d", entry.Kind, entry.Amount.Minor())
	}
	if entry.AccountID != f.account {
		t.Fatalf("booking account = %v, want %v", entry.AccountID, f.account)
	}
}

func TestCompleteWithEconomyOffBooksNothing(t *testing.T) {
	f := newFixture(t, false)
	if _, err := f.svc.Start(focusapp.StartInput{Kind: focus.KindFocus, TaskID: f.task}); err != nil {
		t.Fatal(err)
	}
	f.clock.Advance(25 * time.Minute)
	_, outcome, err := f.svc.Complete()
	if err != nil {
		t.Fatal(err)
	}
	if outcome.Reason != focus.ReasonDisabled {
		t.Fatalf("reason = %q", outcome.Reason)
	}
	entries, _ := f.store.Entries().List(ledger.EntryFilter{})
	if len(entries) != 0 {
		t.Fatalf("a disabled economy must not book, got %d entries", len(entries))
	}
}

func TestCompleteUnboundTaskStillBooks(t *testing.T) {
	f := newFixture(t, true)
	if _, err := f.svc.Start(focusapp.StartInput{Kind: focus.KindFocus}); err != nil {
		t.Fatal(err)
	}
	f.clock.Advance(25 * time.Minute)
	_, outcome, err := f.svc.Complete()
	if err != nil {
		t.Fatal(err)
	}
	if outcome.Amount.Minor() != 1250 {
		t.Fatalf("free focus must still be rewarded, got %d", outcome.Amount.Minor())
	}
}

func TestCancelBooksThePenalty(t *testing.T) {
	f := newFixture(t, true)
	if _, err := f.svc.Start(focusapp.StartInput{Kind: focus.KindFocus, TaskID: f.task}); err != nil {
		t.Fatal(err)
	}
	f.clock.Advance(8 * time.Minute)
	session, outcome, err := f.svc.Cancel(true)
	if err != nil {
		t.Fatalf("Cancel: %v", err)
	}
	if session.Phase != focus.PhaseCanceled || session.Interruptions != 1 {
		t.Fatalf("session = %+v", session)
	}
	if outcome.Amount.Minor() != 625 {
		t.Fatalf("penalty = %d, want 625", outcome.Amount.Minor())
	}
	entries, _ := f.store.Entries().List(ledger.EntryFilter{})
	if len(entries) != 1 || entries[0].Kind != ledger.EntryExpense {
		t.Fatalf("penalty booking = %+v", entries)
	}
}

func TestCancelBeforeTheGracePeriodCostsNothing(t *testing.T) {
	f := newFixture(t, true)
	if _, err := f.svc.Start(focusapp.StartInput{Kind: focus.KindFocus, TaskID: f.task}); err != nil {
		t.Fatal(err)
	}
	f.clock.Advance(2 * time.Minute)
	_, outcome, err := f.svc.Cancel(true)
	if err != nil {
		t.Fatal(err)
	}
	if outcome.Applicable() || outcome.Reason != focus.ReasonBelowMinimum {
		t.Fatalf("outcome = %+v", outcome)
	}
	entries, _ := f.store.Entries().List(ledger.EntryFilter{})
	if len(entries) != 0 {
		t.Fatalf("no booking expected, got %d", len(entries))
	}
}

func TestCancelCountUpCostsNothing(t *testing.T) {
	// A stopwatch session has no plan to break, so cancelling it is free. The
	// session is created through the domain because the service always derives a
	// planned duration for a focus session.
	f := newFixture(t, true)
	session, err := focus.Start("00000000000000000000000088", "", focus.KindFocus, 0, f.clock.Now())
	if err != nil {
		t.Fatal(err)
	}
	if err := f.store.Sessions().Create(session); err != nil {
		t.Fatal(err)
	}
	f.clock.Advance(60 * time.Minute)
	_, outcome, err := f.svc.Cancel(false)
	if err != nil {
		t.Fatal(err)
	}
	if outcome.Reason != focus.ReasonCountUp {
		t.Fatalf("a count-up cancel must not be fined, got %+v", outcome)
	}
}

func TestBookingFailureDoesNotLoseThePomodoro(t *testing.T) {
	// The session commits first and the money is written after, so a broken booker
	// must cost a warning, never the completion.
	st := store.New()
	clk := harness.NewClock(harness.At("2026-11-17 09:00:00"))
	ids := harness.NewIDs()
	tasks := taskapp.New(st.Tasks(), st, clk, ids)
	econ := focus.DefaultEconomySettings(shared.CNY)
	econ.Enabled = true
	econ.RewardAccountID = "00000000000000000000000011"
	econ.RewardCategoryID = "00000000000000000000000012"
	econ.PenaltyCategoryID = "00000000000000000000000013"
	set := &stubSettings{timer: focus.DefaultTimerSettings(), economy: econ}
	svc := focusapp.New(st.Sessions(), st.Tasks(), st.Entries(), failingBooker{}, st, clk, ids, set, shared.NoopNotifier{}, shared.NoopSleepGuard{})

	tk, err := tasks.Create(taskapp.CreateInput{Title: "x"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Start(focusapp.StartInput{Kind: focus.KindFocus, TaskID: tk.ID}); err != nil {
		t.Fatal(err)
	}
	clk.Advance(25 * time.Minute)
	session, outcome, err := svc.Complete()
	if err != nil {
		t.Fatalf("a booking failure must not fail the completion: %v", err)
	}
	if session.Phase != focus.PhaseDone {
		t.Fatalf("phase = %v", session.Phase)
	}
	if outcome.Reason != "focus.economy.booking_failed" {
		t.Fatalf("reason = %q", outcome.Reason)
	}
	stored, err := st.Tasks().Get(tk.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.DonePomodoros != 1 {
		t.Fatal("the pomodoro must still be credited")
	}
	entries, _ := st.Entries().List(ledger.EntryFilter{})
	if len(entries) != 0 {
		t.Fatalf("no entry expected, got %d", len(entries))
	}
}

// failingBooker simulates a ledger write that cannot happen (archived account).
type failingBooker struct{}

func (failingBooker) Book(contracts.BookingRequest) error {
	return errors.New("ledger: account archived")
}

func TestPauseAndResumeThroughTheService(t *testing.T) {
	f := newFixture(t, false)
	if _, err := f.svc.Start(focusapp.StartInput{Kind: focus.KindFocus, TaskID: f.task}); err != nil {
		t.Fatal(err)
	}
	f.clock.Advance(10 * time.Minute)
	paused, err := f.svc.Pause()
	if err != nil {
		t.Fatalf("Pause: %v", err)
	}
	if paused.Phase != focus.PhasePaused {
		t.Fatalf("phase = %v", paused.Phase)
	}
	f.clock.Advance(time.Hour)
	resumed, err := f.svc.Resume()
	if err != nil {
		t.Fatalf("Resume: %v", err)
	}
	if got := resumed.Remaining(f.clock.Now()); got != 15*time.Minute {
		t.Fatalf("Remaining = %v, want 15m", got)
	}
	if _, err := f.svc.Resume(); !shared.IsKind(err, shared.KindConflict) {
		t.Fatalf("resuming a running session must conflict, got %v", err)
	}
}

func TestSettleWithoutAnActiveSession(t *testing.T) {
	f := newFixture(t, false)
	if _, _, err := f.svc.Complete(); !shared.IsKind(err, shared.KindConflict) {
		t.Fatalf("Complete must conflict, got %v", err)
	}
	if _, _, err := f.svc.Cancel(true); !shared.IsKind(err, shared.KindConflict) {
		t.Fatalf("Cancel must conflict, got %v", err)
	}
	if _, err := f.svc.Pause(); !shared.IsKind(err, shared.KindConflict) {
		t.Fatalf("Pause must conflict, got %v", err)
	}
	if _, err := f.svc.Resume(); !shared.IsKind(err, shared.KindConflict) {
		t.Fatalf("Resume must conflict, got %v", err)
	}
	if active, err := f.svc.Active(); err != nil || active != nil {
		t.Fatalf("Active = %v, %v", active, err)
	}
}

func TestSuspiciousSessionAndRecovery(t *testing.T) {
	f := newFixture(t, false)
	if _, err := f.svc.Start(focusapp.StartInput{Kind: focus.KindFocus, TaskID: f.task}); err != nil {
		t.Fatal(err)
	}
	if s, err := f.svc.Suspicious(); err != nil || s != nil {
		t.Fatalf("a fresh session must not be suspicious: %v, %v", s, err)
	}
	f.clock.Advance(3 * time.Hour)
	suspicious, err := f.svc.Suspicious()
	if err != nil {
		t.Fatal(err)
	}
	if suspicious == nil {
		t.Fatal("a three-hour-old session must be suspicious")
	}

	kept, _, err := f.svc.Recover(focusapp.RecoverKeep)
	if err != nil {
		t.Fatalf("RecoverKeep: %v", err)
	}
	if kept == nil || kept.Phase != focus.PhaseRunning {
		t.Fatalf("keeping must leave the session running: %+v", kept)
	}

	settled, _, err := f.svc.Recover(focusapp.RecoverCredit)
	if err != nil {
		t.Fatalf("RecoverCredit: %v", err)
	}
	if settled.Phase != focus.PhaseDone {
		t.Fatalf("phase = %v", settled.Phase)
	}
	if _, _, err := f.svc.Recover(focusapp.RecoverAction(99)); !shared.IsKind(err, shared.KindInvalid) {
		t.Fatalf("an unknown recovery action must be invalid, got %v", err)
	}

	second := newFixture(t, false)
	if _, err := second.svc.Start(focusapp.StartInput{Kind: focus.KindFocus}); err != nil {
		t.Fatal(err)
	}
	second.clock.Advance(3 * time.Hour)
	discarded, _, err := second.svc.Recover(focusapp.RecoverDiscard)
	if err != nil {
		t.Fatalf("RecoverDiscard: %v", err)
	}
	if discarded.Phase != focus.PhaseCanceled {
		t.Fatalf("phase = %v", discarded.Phase)
	}
	if discarded.Interruptions != 0 {
		t.Fatal("a recovery discard must not count as an interruption")
	}
}

func TestTodayStatsAndNextBreak(t *testing.T) {
	f := newFixture(t, false)
	for i := 0; i < 4; i++ {
		if _, err := f.svc.Start(focusapp.StartInput{Kind: focus.KindFocus, TaskID: f.task}); err != nil {
			t.Fatal(err)
		}
		f.clock.Advance(25 * time.Minute)
		if _, _, err := f.svc.Complete(); err != nil {
			t.Fatal(err)
		}
		// A finished session completes its task (creditTask), and a done task cannot
		// be focused again until it is reopened — the same step the user takes by
		// clearing the tick in the list.
		if _, err := f.tasks.Reopen(f.task); err != nil {
			t.Fatal(err)
		}
	}
	stats, err := f.svc.TodayStats()
	if err != nil {
		t.Fatalf("TodayStats: %v", err)
	}
	if stats.CompletedFocus != 4 || stats.FocusMinutes != 100 {
		t.Fatalf("stats = %+v", stats)
	}
	if got := f.svc.NextBreakSuggestion(); got != focus.KindLongBreak {
		t.Fatalf("after four pomodoros the suggestion must be a long break, got %v", got)
	}
	recent, err := f.svc.ListRecent(0)
	if err != nil {
		t.Fatal(err)
	}
	if len(recent) != 4 {
		t.Fatalf("recent = %d", len(recent))
	}
}

func TestBookingIsNotDuplicated(t *testing.T) {
	// The unique index on focus_session_id is the real guard; the service checks
	// first so a retry is a calm no-op instead of a database error.
	f := newFixture(t, true)
	if _, err := f.svc.Start(focusapp.StartInput{Kind: focus.KindFocus, TaskID: f.task}); err != nil {
		t.Fatal(err)
	}
	f.clock.Advance(25 * time.Minute)
	if _, _, err := f.svc.Complete(); err != nil {
		t.Fatal(err)
	}
	// A second booking for the same session must be refused by the store, which is
	// what makes a replayed completion harmless.
	dup, err := ledger.NewEntry("00000000000000000000000099", f.clock.Now(), ledger.EntryInput{
		Kind:           ledger.EntryIncome,
		Amount:         shared.NewMoney(1250, shared.CNY),
		AccountID:      f.account,
		CategoryID:     f.set.economy.RewardCategoryID,
		Origin:         ledger.OriginFocusReward,
		FocusSessionID: mustActiveID(t, f),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := f.store.Entries().Create(dup); err == nil {
		t.Fatal("the store must refuse a second booking for one session")
	}
}

func TestTodayStatsCountsInterruptions(t *testing.T) {
	f := newFixture(t, false)
	if _, err := f.svc.Start(focusapp.StartInput{Kind: focus.KindFocus, TaskID: f.task}); err != nil {
		t.Fatal(err)
	}
	f.clock.Advance(5 * time.Minute)
	if _, _, err := f.svc.Cancel(true); err != nil {
		t.Fatal(err)
	}
	stats, err := f.svc.TodayStats()
	if err != nil {
		t.Fatal(err)
	}
	if stats.CompletedFocus != 0 || stats.Interruptions != 1 {
		t.Fatalf("stats = %+v", stats)
	}
}

func secondStart(svc *focusapp.Service) error {
	_, err := svc.Start(focusapp.StartInput{Kind: focus.KindFocus})
	return err
}

func mustActiveID(t *testing.T, f *fixture) shared.ID {
	t.Helper()
	recent, err := f.svc.ListRecent(1)
	if err != nil {
		t.Fatal(err)
	}
	if len(recent) != 1 {
		t.Fatalf("want one session, got %d", len(recent))
	}
	return recent[0].ID
}
