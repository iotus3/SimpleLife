package dashboard_test

import (
	"errors"
	"testing"
	"time"

	"github.com/iotus3/simplelife/app/internal/application/dashboard"
	focusapp "github.com/iotus3/simplelife/app/internal/application/focus"
	ledgerapp "github.com/iotus3/simplelife/app/internal/application/ledger"
	taskapp "github.com/iotus3/simplelife/app/internal/application/task"
	"github.com/iotus3/simplelife/app/internal/apptest/harness"
	"github.com/iotus3/simplelife/app/internal/apptest/store"
	focus "github.com/iotus3/simplelife/app/internal/domain/focus"
	"github.com/iotus3/simplelife/app/internal/domain/ledger"
	"github.com/iotus3/simplelife/app/internal/domain/shared"
	task "github.com/iotus3/simplelife/app/internal/domain/task"
)

type fixture struct {
	svc   *dashboard.Service
	tasks *taskapp.Service
	focus *focusapp.Service
	money *ledgerapp.Service
	store *store.Store
	clock *harness.Clock
	ids   *harness.IDs
}

// settings is a literal Settings so the test controls the timer without a store.
type settings struct{}

func (settings) Timer() focus.TimerSettings     { return focus.DefaultTimerSettings() }
func (settings) Economy() focus.EconomySettings { return focus.DefaultEconomySettings(shared.CNY) }

func newFixture(t *testing.T) *fixture {
	t.Helper()
	st := store.New()
	clk := harness.NewClock(harness.At("2026-11-17 09:00:00"))
	ids := harness.NewIDs()
	tasks := taskapp.New(st.Tasks(), st, clk, ids)
	money := ledgerapp.New(st.Entries(), st.Accounts(), st.Categories(), st.Budgets(), st, clk, ids)
	foc := focusapp.New(st.Sessions(), st.Tasks(), st.Entries(), money, st, clk, ids, settings{}, nil, nil)
	return &fixture{
		svc:   dashboard.New(tasks, foc, money, clk),
		tasks: tasks, focus: foc, money: money, store: st, clock: clk, ids: ids,
	}
}

func TestLoadEmptyDatabase(t *testing.T) {
	f := newFixture(t)
	ov, err := f.svc.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !ov.Day.Equal(shared.MustParseLocalDate("2026-11-17")) {
		t.Fatalf("day = %v", ov.Day)
	}
	if len(ov.TodayTasks) != 0 || ov.OpenTasks != 0 || ov.DoneToday != 0 || ov.OverdueTasks != 0 {
		t.Fatalf("overview = %+v", ov)
	}
	if ov.ActiveSession != nil || ov.CompletedFocus != 0 || ov.FocusMinutes != 0 {
		t.Fatalf("focus = %+v", ov)
	}
	if len(ov.Income) != 0 || len(ov.Expense) != 0 {
		t.Fatalf("money = %+v / %+v", ov.Income, ov.Expense)
	}
}

func TestLoadCountsTodayOnly(t *testing.T) {
	f := newFixture(t)
	mk := func(title string, due shared.LocalDate) *task.Task {
		t.Helper()
		tk, err := f.tasks.Create(taskapp.CreateInput{Title: title, DueDate: due, Estimate: 2})
		if err != nil {
			t.Fatalf("Create %q: %v", title, err)
		}
		return tk
	}
	mk("今天 A", shared.MustParseLocalDate("2026-11-17"))
	mk("今天 B", shared.MustParseLocalDate("2026-11-17"))
	mk("明天", shared.MustParseLocalDate("2026-11-18"))
	overdue := mk("昨天", shared.MustParseLocalDate("2026-11-16"))

	if _, err := f.tasks.Complete(overdue.ID); err != nil {
		t.Fatalf("Complete: %v", err)
	}

	ov, err := f.svc.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(ov.TodayTasks) != 2 {
		t.Fatalf("today = %d", len(ov.TodayTasks))
	}
	if ov.OpenTasks != 2 || ov.PlannedPomodoros != 4 {
		t.Fatalf("open = %d, planned = %d", ov.OpenTasks, ov.PlannedPomodoros)
	}
	if ov.OverdueTasks != 0 {
		t.Fatalf("the overdue task is done, so overdue must be 0, got %d", ov.OverdueTasks)
	}
	// The completion happened "now", so it is also a task done today — even though
	// its due date is yesterday. The counter follows CompletedAt, not the due date.
	if ov.DoneToday != 1 {
		t.Fatalf("doneToday = %d", ov.DoneToday)
	}
}

func TestLoadCountsOverdue(t *testing.T) {
	f := newFixture(t)
	if _, err := f.tasks.Create(taskapp.CreateInput{Title: "拖了", DueDate: shared.MustParseLocalDate("2026-11-15")}); err != nil {
		t.Fatal(err)
	}
	ov, err := f.svc.Load()
	if err != nil {
		t.Fatal(err)
	}
	if ov.OverdueTasks != 1 {
		t.Fatalf("overdue = %d", ov.OverdueTasks)
	}
}

func TestLoadIgnoresYesterdaysCompletions(t *testing.T) {
	f := newFixture(t)
	tk, err := f.tasks.Create(taskapp.CreateInput{Title: "昨天做完的", DueDate: shared.MustParseLocalDate("2026-11-16")})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.tasks.Complete(tk.ID); err != nil {
		t.Fatal(err)
	}
	// Move the clock to the next day: the completion is now historical.
	f.clock.Set(harness.At("2026-11-18 09:00:00"))
	ov, err := f.svc.Load()
	if err != nil {
		t.Fatal(err)
	}
	if ov.DoneToday != 0 {
		t.Fatalf("doneToday = %d, want 0", ov.DoneToday)
	}
}

func TestLoadReflectsFocusAndMoney(t *testing.T) {
	f := newFixture(t)
	acct, err := ledger.NewAccount(f.ids.New(), "现金", ledger.AccountCash, shared.NewMoney(0, shared.CNY))
	if err != nil {
		t.Fatal(err)
	}
	if err := f.store.Accounts().Create(acct); err != nil {
		t.Fatal(err)
	}
	expenseCat, err := ledger.NewCategory(f.ids.New(), "餐饮", ledger.CategoryExpense)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.store.Categories().Create(expenseCat); err != nil {
		t.Fatal(err)
	}
	incomeCat, err := ledger.NewCategory(f.ids.New(), "工资", ledger.CategoryIncome)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.store.Categories().Create(incomeCat); err != nil {
		t.Fatal(err)
	}
	if _, err := f.money.CreateEntry(ledgerapp.EntryInput{
		Kind: ledger.EntryExpense, Amount: shared.NewMoney(2500, shared.CNY),
		AccountID: acct.ID, CategoryID: expenseCat.ID, OccurredAt: f.clock.Now(),
	}); err != nil {
		t.Fatalf("CreateEntry: %v", err)
	}
	if _, err := f.money.CreateEntry(ledgerapp.EntryInput{
		Kind: ledger.EntryIncome, Amount: shared.NewMoney(100000, shared.CNY),
		AccountID: acct.ID, CategoryID: incomeCat.ID, OccurredAt: f.clock.Now(),
	}); err != nil {
		t.Fatalf("CreateEntry: %v", err)
	}

	tk, err := f.tasks.Create(taskapp.CreateInput{Title: "写代码", DueDate: shared.MustParseLocalDate("2026-11-17")})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.focus.Start(focusapp.StartInput{Kind: focus.KindFocus, TaskID: tk.ID}); err != nil {
		t.Fatalf("Start: %v", err)
	}

	ov, err := f.svc.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if ov.ActiveSession == nil || ov.ActiveSession.TaskID != tk.ID {
		t.Fatalf("active = %+v", ov.ActiveSession)
	}
	if ov.OpenTasks != 1 {
		t.Fatalf("a task being focused is still open: %d", ov.OpenTasks)
	}
	if len(ov.Expense) != 1 || ov.Expense[0].Amount.Minor() == 0 {
		t.Fatalf("expense = %+v", ov.Expense)
	}
	if ov.Expense[0].Amount.Minor() != 2500 || ov.Income[0].Amount.Minor() != 100000 {
		t.Fatalf("income = %+v expense = %+v", ov.Income, ov.Expense)
	}

	// Finish the session so the stats path is exercised.
	f.clock.Advance(25 * time.Minute)
	if _, _, err := f.focus.Complete(); err != nil {
		t.Fatalf("Complete: %v", err)
	}
	ov, err = f.svc.Load()
	if err != nil {
		t.Fatal(err)
	}
	if ov.CompletedFocus != 1 || ov.FocusMinutes != 25 {
		t.Fatalf("stats = %d / %d", ov.CompletedFocus, ov.FocusMinutes)
	}
	if ov.ActiveSession != nil {
		t.Fatal("no session must be active after completing")
	}
}

var errBoom = errors.New("boom")

// The breakers embed a working facade and fail one method: Go has no method
// overloading in fakes either, so embedding is the only way to fail two methods
// of the same name on different ports without duplicating every method.
type failTaskList struct{ store.TaskRepo }

func (failTaskList) List(task.Filter) ([]*task.Task, error) { return nil, errBoom }

type failSessionRange struct{ store.SessionRepo }

func (failSessionRange) ListByRange(time.Time, time.Time) ([]*focus.Session, error) {
	return nil, errBoom
}

type failSessionActive struct{ store.SessionRepo }

func (failSessionActive) Active() (*focus.Session, error) { return nil, errBoom }

type failEntryList struct{ store.EntryRepo }

func (failEntryList) List(ledger.EntryFilter) ([]*ledger.Entry, error) { return nil, errBoom }

func TestLoadReportsBrokenSections(t *testing.T) {
	// Each broken port must surface its error instead of showing a zero that looks
	// like real data.
	cases := []struct {
		name   string
		build  func(f *fixture) *dashboard.Service
		broken string
	}{
		{
			name: "tasks",
			build: func(f *fixture) *dashboard.Service {
				broken := taskapp.New(failTaskList{f.store.Tasks()}, f.store, f.clock, f.ids)
				return dashboard.New(broken, f.focus, f.money, f.clock)
			},
		},
		{
			name: "sessions",
			build: func(f *fixture) *dashboard.Service {
				broken := focusapp.New(failSessionRange{f.store.Sessions()}, f.store.Tasks(), f.store.Entries(), f.money, f.store, f.clock, f.ids, settings{}, nil, nil)
				return dashboard.New(f.tasks, broken, f.money, f.clock)
			},
		},
		{
			name: "active",
			build: func(f *fixture) *dashboard.Service {
				broken := focusapp.New(failSessionActive{f.store.Sessions()}, f.store.Tasks(), f.store.Entries(), f.money, f.store, f.clock, f.ids, settings{}, nil, nil)
				return dashboard.New(f.tasks, broken, f.money, f.clock)
			},
		},
		{
			name: "money",
			build: func(f *fixture) *dashboard.Service {
				broken := ledgerapp.New(failEntryList{f.store.Entries()}, f.store.Accounts(), f.store.Categories(), f.store.Budgets(), f.store, f.clock, f.ids)
				return dashboard.New(f.tasks, f.focus, broken, f.clock)
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture(t)
			svc := tc.build(f)
			ov, err := svc.Load()
			if err == nil {
				t.Fatalf("%s: a broken port must be reported, got %+v", tc.name, ov)
			}
			if !ov.Day.Equal(shared.MustParseLocalDate("2026-11-17")) {
				t.Fatalf("the day must still be set: %v", ov.Day)
			}
		})
	}
}
