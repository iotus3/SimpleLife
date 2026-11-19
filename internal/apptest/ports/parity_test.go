package ports_test

import (
	"errors"
	"fmt"
	"reflect"
	"testing"
	"time"

	focusapp "github.com/iotus3/simplelife/app/internal/application/focus"
	ledgerapp "github.com/iotus3/simplelife/app/internal/application/ledger"
	taskapp "github.com/iotus3/simplelife/app/internal/application/task"
	templateapp "github.com/iotus3/simplelife/app/internal/application/template"
	"github.com/iotus3/simplelife/app/internal/apptest/harness"
	"github.com/iotus3/simplelife/app/internal/apptest/ports"
	focus "github.com/iotus3/simplelife/app/internal/domain/focus"
	"github.com/iotus3/simplelife/app/internal/domain/ledger"
	"github.com/iotus3/simplelife/app/internal/domain/shared"
	task "github.com/iotus3/simplelife/app/internal/domain/task"
	"github.com/iotus3/simplelife/app/internal/domain/template"
	"github.com/iotus3/simplelife/app/internal/infrastructure/persistence/sqlite"
)

// backbone is one implementation under test: a port set plus the services wired to
// it, so a scenario can be written once and run twice.
type backbone struct {
	name     string
	set      ports.Set
	clock    *harness.Clock
	tasks    *taskapp.Service
	focus    *focusapp.Service
	money    *ledgerapp.Service
	template *templateapp.Service
}

// settingsLiteral keeps the timer out of the settings store: the parity suite is
// about persistence, not about the settings form.
type settingsLiteral struct{}

func (settingsLiteral) Timer() focus.TimerSettings { return focus.DefaultTimerSettings() }
func (settingsLiteral) Economy() focus.EconomySettings {
	return focus.DefaultEconomySettings(shared.CNY)
}

func backbones(t *testing.T) []*backbone {
	t.Helper()
	clk := harness.NewClock(harness.At("2026-11-17 08:00:00"))

	memory := ports.Memory()
	db, err := sqlite.Open(t.TempDir() + "/parity.db")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	out := make([]*backbone, 0, 2)
	for _, set := range []ports.Set{memory, ports.SQLite(db)} {
		ids := harness.NewIDs()
		money := ledgerapp.New(set.Entries(), set.Accounts(), set.Categories(), set.Budgets(), set, clk, ids)
		tasks := taskapp.New(set.Tasks(), set, clk, ids)
		foc := focusapp.New(set.Sessions(), set.Tasks(), set.Entries(), money, set, clk, ids, settingsLiteral{}, nil, nil)
		out = append(out, &backbone{
			name:     fmt.Sprintf("%T", set),
			set:      set,
			clock:    clk,
			tasks:    tasks,
			focus:    foc,
			money:    money,
			template: templateapp.New(set.Templates(), set.Tasks(), set, clk, ids),
		})
	}
	return out
}

// TestPortsAgree runs the same scenarios against both implementations and compares
// the results. Anything that differs is an ambiguity in the port, not a cosmetic
// mismatch: the app cannot behave the same on both when the store disagrees.
func TestPortsAgree(t *testing.T) {
	results := make(map[string][]string)
	for _, b := range backbones(t) {
		t.Run(b.name, func(t *testing.T) {
			results[b.name] = runScenarios(t, b)
		})
	}
	if len(results) != 2 {
		t.Fatalf("expected two backbones, got %d", len(results))
	}
	var first string
	for name := range results {
		if first == "" {
			first = name
			continue
		}
		if !reflect.DeepEqual(results[first], results[name]) {
			t.Fatalf("the two stores disagree:\n%s: %v\n%s: %v", first, results[first], name, results[name])
		}
	}
}

// runScenarios exercises the queries whose semantics are easiest to get subtly
// wrong, and returns one line per observation.
func runScenarios(t *testing.T, b *backbone) []string {
	t.Helper()
	var out []string
	note := func(format string, args ...any) { out = append(out, fmt.Sprintf(format, args...)) }

	day := shared.DateOf(b.clock.Now())

	// Tasks: one due today, one due yesterday, one undated, one archived.
	today, err := b.tasks.Create(taskapp.CreateInput{Title: "今天的", DueDate: day, Estimate: 2, Tags: []string{"work"}})
	if err != nil {
		t.Fatalf("%s: create today: %v", b.name, err)
	}
	overdue, err := b.tasks.Create(taskapp.CreateInput{Title: "逾期的", DueDate: day.AddDays(-1)})
	if err != nil {
		t.Fatalf("%s: create overdue: %v", b.name, err)
	}
	undated, err := b.tasks.Create(taskapp.CreateInput{Title: "未排期"})
	if err != nil {
		t.Fatalf("%s: create undated: %v", b.name, err)
	}
	archived, err := b.tasks.Create(taskapp.CreateInput{Title: "归档的", DueDate: day})
	if err != nil {
		t.Fatalf("%s: create archived: %v", b.name, err)
	}
	if err := b.tasks.Delete(archived.ID); err != nil {
		t.Fatalf("%s: delete: %v", b.name, err)
	}

	note("task.Today=%s", mustTitles(t, b, "today", func() ([]*task.Task, error) { return b.tasks.Today(day) }))
	note("task.History=%s", mustTitles(t, b, "history", func() ([]*task.Task, error) { return b.tasks.History(day) }))
	if n, err := b.tasks.OverdueCount(); err != nil {
		t.Fatalf("%s: overdue count: %v", b.name, err)
	} else {
		note("task.OverdueCount=%d", n)
	}
	if n, err := b.tasks.OpenCount(); err != nil {
		t.Fatalf("%s: open count: %v", b.name, err)
	} else {
		note("task.OpenCount=%d", n)
	}
	groups, err := b.tasks.HistoryGroups(day)
	if err != nil {
		t.Fatalf("%s: history groups: %v", b.name, err)
	}
	for _, g := range groups {
		note("task.Group(%s)=%s", g.Day, titlesOf(g.Tasks))
	}
	note("task.Filter(archived)=%s", mustTitles(t, b, "archived", func() ([]*task.Task, error) {
		return b.tasks.List(task.Filter{Statuses: []task.Status{task.StatusArchived}})
	}))

	// Completing the overdue task must move it out of the overdue count.
	if _, err := b.tasks.Complete(overdue.ID); err != nil {
		t.Fatalf("%s: complete: %v", b.name, err)
	}
	if n, err := b.tasks.OverdueCount(); err != nil {
		t.Fatalf("%s: overdue count after complete: %v", b.name, err)
	} else {
		note("task.OverdueCountAfter=%d", n)
	}

	// Sorting: the three open tasks must come back in the same order for both.
	for _, order := range []task.Ordering{task.OrderManual, task.OrderDue, task.OrderPriority, task.OrderCreated} {
		got, err := b.tasks.List(task.Filter{Order: order})
		if err != nil {
			t.Fatalf("%s: list order %v: %v", b.name, order, err)
		}
		note("task.Order(%v)=%s", order, titlesOf(got))
	}

	// Money: two accounts, three entries, one transfer, one budget.
	cash, err := b.money.CreateAccount("现金", ledger.AccountCash, shared.NewMoney(10000, shared.CNY))
	if err != nil {
		t.Fatalf("%s: create account: %v", b.name, err)
	}
	bank, err := b.money.CreateAccount("银行卡", ledger.AccountDebitCard, shared.NewMoney(500000, shared.CNY))
	if err != nil {
		t.Fatalf("%s: create account: %v", b.name, err)
	}
	dining, err := b.money.CreateCategory("餐饮", ledger.CategoryExpense, "")
	if err != nil {
		t.Fatalf("%s: create category: %v", b.name, err)
	}
	salary, err := b.money.CreateCategory("工资", ledger.CategoryIncome, "")
	if err != nil {
		t.Fatalf("%s: create category: %v", b.name, err)
	}
	if _, err := b.money.CreateEntry(ledgerapp.EntryInput{
		Kind: ledger.EntryExpense, Amount: shared.NewMoney(3550, shared.CNY),
		AccountID: cash.ID, CategoryID: dining.ID, OccurredAt: b.clock.Now(), Note: "午饭",
	}); err != nil {
		t.Fatalf("%s: expense: %v", b.name, err)
	}
	if _, err := b.money.CreateEntry(ledgerapp.EntryInput{
		Kind: ledger.EntryIncome, Amount: shared.NewMoney(500000, shared.CNY),
		AccountID: bank.ID, CategoryID: salary.ID, OccurredAt: b.clock.Now(), Note: "工资",
	}); err != nil {
		t.Fatalf("%s: income: %v", b.name, err)
	}
	if _, err := b.money.CreateEntry(ledgerapp.EntryInput{
		Kind: ledger.EntryTransfer, Amount: shared.NewMoney(20000, shared.CNY),
		AccountID: bank.ID, ToAccountID: cash.ID, OccurredAt: b.clock.Now(), Note: "取现",
	}); err != nil {
		t.Fatalf("%s: transfer: %v", b.name, err)
	}
	if _, err := b.money.UpsertBudget(dining.ID, day.AsMonth(), shared.NewMoney(100000, shared.CNY)); err != nil {
		t.Fatalf("%s: budget: %v", b.name, err)
	}

	month := day.AsMonth()
	summary, err := b.money.Summarise(month)
	if err != nil {
		t.Fatalf("%s: summarise: %v", b.name, err)
	}
	note("money.Income=%v", summary.Income)
	note("money.Expense=%v", summary.Expense)
	note("money.Net=%v", summary.Net)
	note("money.Count=%d", summary.EntryCount)

	balances, err := b.money.Balances(false)
	if err != nil {
		t.Fatalf("%s: balances: %v", b.name, err)
	}
	for _, bal := range balances {
		note("money.Balance(%s)=%d", bal.Account.Name, bal.Balance.Minor())
	}
	spending, err := b.money.CategorySpending(month)
	if err != nil {
		t.Fatalf("%s: spending: %v", b.name, err)
	}
	for _, s := range spending {
		budget := int64(0)
		if s.Budget != nil {
			budget = s.Budget.Limit.Minor()
		}
		note("money.Spent(%s)=%d/%d", s.Category.Name, s.Spent.Minor(), budget)
	}
	recent, err := b.money.Recent(2)
	if err != nil {
		t.Fatalf("%s: recent: %v", b.name, err)
	}
	note("money.Recent=%d", len(recent))
	found, err := b.money.Search("午饭", 20)
	if err != nil {
		t.Fatalf("%s: search: %v", b.name, err)
	}
	note("money.Search=%d", len(found))

	// Focus: complete a pomodoro on today's task and check the counters.
	if _, err := b.focus.Start(focusapp.StartInput{Kind: focus.KindFocus, TaskID: today.ID}); err != nil {
		t.Fatalf("%s: start: %v", b.name, err)
	}
	b.clock.Advance(25 * time.Minute)
	if _, _, err := b.focus.Complete(); err != nil {
		t.Fatalf("%s: complete focus: %v", b.name, err)
	}
	stats, err := b.focus.TodayStats()
	if err != nil {
		t.Fatalf("%s: today stats: %v", b.name, err)
	}
	note("focus.Stats=%d/%d/%d", stats.CompletedFocus, stats.FocusMinutes, stats.Interruptions)
	recentSessions, err := b.focus.ListRecent(5)
	if err != nil {
		t.Fatalf("%s: recent sessions: %v", b.name, err)
	}
	note("focus.Recent=%d", len(recentSessions))
	refresh, err := b.tasks.Get(today.ID)
	if err != nil {
		t.Fatalf("%s: reload task: %v", b.name, err)
	}
	note("focus.TaskPomodoros=%d status=%s", refresh.DonePomodoros, refresh.Status)

	// The economy is off by default, so no booking may be written.
	booked, err := b.money.List(ledger.EntryFilter{Origin: []ledger.Origin{ledger.OriginFocusReward}})
	if err != nil {
		t.Fatalf("%s: list bookings: %v", b.name, err)
	}
	note("focus.Bookings=%d", len(booked))

	// Templates: save one, expand it twice, compare the run counters and logs.
	if _, err := b.template.Save(templateapp.SaveInput{
		Name: "晨间", Enabled: true,
		Rule: template.TriggerRule{Kind: template.TriggerDaily},
		Task: template.Task{Title: "写日记", Estimate: 1},
	}); err != nil {
		t.Fatalf("%s: save template: %v", b.name, err)
	}
	first, err := b.template.Expand(day)
	if err != nil {
		t.Fatalf("%s: expand: %v", b.name, err)
	}
	note("template.First=%d/%d/%d", first.Created, first.Skipped, len(first.Failed))
	second, err := b.template.Expand(day)
	if err != nil {
		t.Fatalf("%s: expand again: %v", b.name, err)
	}
	note("template.Second=%d/%d/%d", second.Created, second.Skipped, len(second.Failed))
	note("template.Today=%s", mustTitles(t, b, "today after expand", func() ([]*task.Task, error) { return b.tasks.Today(day) }))
	logs, err := b.template.RecentLogs(5)
	if err != nil {
		t.Fatalf("%s: template logs: %v", b.name, err)
	}
	note("template.Logs=%d", len(logs))

	// Settings: the same key round-trips, and a missing key is the same error.
	if err := b.set.Settings().Set("language", "en"); err != nil {
		t.Fatalf("%s: set setting: %v", b.name, err)
	}
	value, err := b.set.Settings().Get("language")
	if err != nil || value != "en" {
		t.Fatalf("%s: get setting: %q %v", b.name, value, err)
	}
	if _, err := b.set.Settings().Get("missing"); !errors.Is(err, shared.ErrNotFound) {
		t.Fatalf("%s: a missing setting must be ErrNotFound, got %v", b.name, err)
	}
	_ = undated
	return out
}

// mustTitles renders a task list for the comparison log, failing the test if the
// query itself errored.
func mustTitles(t *testing.T, b *backbone, label string, fn func() ([]*task.Task, error)) string {
	t.Helper()
	list, err := fn()
	if err != nil {
		t.Fatalf("%s: %s: %v", b.name, label, err)
	}
	return titlesOf(list)
}

func titlesOf(tasks []*task.Task) string {
	out := make([]string, len(tasks))
	for i, tk := range tasks {
		out[i] = tk.Title
	}
	return fmt.Sprint(out)
}
