package export_test

import (
	"strings"
	"testing"
	"time"

	app "github.com/iotus3/simplelife/app/internal/application/export"
	"github.com/iotus3/simplelife/app/internal/apptest/harness"
	"github.com/iotus3/simplelife/app/internal/apptest/store"
	"github.com/iotus3/simplelife/app/internal/domain/export"
	"github.com/iotus3/simplelife/app/internal/domain/focus"
	"github.com/iotus3/simplelife/app/internal/domain/ledger"
	"github.com/iotus3/simplelife/app/internal/domain/shared"
	task "github.com/iotus3/simplelife/app/internal/domain/task"
	"github.com/iotus3/simplelife/app/internal/domain/template"
)

// labels is a stub translator: the real one lives in presentation/i18n, and the
// application layer must not depend on it.
type labels struct{}

func (labels) Sheet(table string) string       { return "sheet:" + table }
func (labels) Column(table, col string) string { return "col:" + table + "." + col }

type fixture struct {
	svc   *app.Service
	store *store.Store
	clock *harness.Clock
	ids   *harness.IDs
	task  shared.ID
	acct  shared.ID
	cat   shared.ID
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	st := store.New()
	clk := harness.NewClock(harness.At("2026-11-17 12:00:00"))
	ids := harness.NewIDs()
	svc := app.New(st.Tasks(), st.Sessions(), st.Entries(), st.Accounts(), st.Categories(), st.Budgets(), st.Templates(), clk)

	acct, err := ledger.NewAccount(ids.New(), "现金", ledger.AccountCash, shared.NewMoney(10000, shared.CNY))
	if err != nil {
		t.Fatal(err)
	}
	if err := st.Accounts().Create(acct); err != nil {
		t.Fatal(err)
	}
	cat, err := ledger.NewCategory(ids.New(), "餐饮", ledger.CategoryExpense)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.Categories().Create(cat); err != nil {
		t.Fatal(err)
	}
	tk, err := task.New(ids.New(), "写方案", clk.Now())
	if err != nil {
		t.Fatal(err)
	}
	if err := tk.SetDueDate(shared.DateOf(clk.Now()), clk.Now()); err != nil {
		t.Fatal(err)
	}
	if err := tk.SetTags([]string{"work", "urgent"}, clk.Now()); err != nil {
		t.Fatal(err)
	}
	if err := st.Tasks().Create(tk); err != nil {
		t.Fatal(err)
	}
	return &fixture{svc: svc, store: st, clock: clk, ids: ids, task: tk.ID, acct: acct.ID, cat: cat.ID}
}

func (f *fixture) addEntry(t *testing.T, amountMinor int64, note string) {
	t.Helper()
	e, err := ledger.NewEntry(f.ids.New(), f.clock.Now(), ledger.EntryInput{
		Kind:       ledger.EntryExpense,
		Amount:     shared.NewMoney(amountMinor, shared.CNY),
		AccountID:  f.acct,
		CategoryID: f.cat,
		OccurredAt: f.clock.Now(),
		Note:       note,
	})
	if err != nil {
		t.Fatalf("NewEntry: %v", err)
	}
	if err := f.store.Entries().Create(e); err != nil {
		t.Fatalf("Create entry: %v", err)
	}
}

func TestBuildTasksSheet(t *testing.T) {
	f := newFixture(t)
	plan := export.Plan{
		Tables: []export.TableSelection{{Key: "tasks", Columns: []string{"id", "title", "status", "due_date", "tags"}}},
		Layout: export.LayoutCSV,
	}
	sheets, err := f.svc.Build(plan, labels{})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if len(sheets) != 1 {
		t.Fatalf("want one sheet, got %d", len(sheets))
	}
	sheet := sheets[0]
	if sheet.Name != "sheet:tasks" || sheet.FileBase != "tasks" {
		t.Fatalf("sheet = %+v", sheet.Name)
	}
	want := []string{"col:tasks.id", "col:tasks.title", "col:tasks.status", "col:tasks.due_date", "col:tasks.tags"}
	for i := range want {
		if sheet.Headers[i] != want[i] {
			t.Fatalf("headers = %v", sheet.Headers)
		}
	}
	if len(sheet.Rows) != 1 {
		t.Fatalf("rows = %v", sheet.Rows)
	}
	row := sheet.Rows[0]
	if row[1] != "写方案" {
		t.Fatalf("title cell = %q", row[1])
	}
	if row[2] != "todo" {
		t.Fatalf("status must be the stable english key, got %q", row[2])
	}
	if row[3] != "2026-11-17" {
		t.Fatalf("due date must stay ISO, got %q", row[3])
	}
	if row[4] != "work,urgent" {
		t.Fatalf("tags cell = %q", row[4])
	}
}

func TestBuildEntriesSheetKeepsValuesNeutral(t *testing.T) {
	f := newFixture(t)
	f.addEntry(t, 3550, "午饭")
	plan := export.Plan{
		Tables: []export.TableSelection{{Key: "entries", Columns: []string{"kind", "amount", "currency", "account", "category", "occurred_at", "origin"}}},
		Layout: export.LayoutCSV,
	}
	sheets, err := f.svc.Build(plan, labels{})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	row := sheets[0].Rows[0]
	if row[0] != "expense" {
		t.Fatalf("kind = %q, want the stable key", row[0])
	}
	if row[1] != "35.50" {
		t.Fatalf("amount = %q, want a plain decimal without a symbol", row[1])
	}
	if row[2] != "CNY" {
		t.Fatalf("currency = %q", row[2])
	}
	if row[3] != "现金" || row[4] != "餐饮" {
		t.Fatalf("names = %q / %q", row[3], row[4])
	}
	if !strings.HasPrefix(row[5], "2026-11-17 ") {
		t.Fatalf("occurred_at = %q, want an ISO-like timestamp", row[5])
	}
	if row[6] != "manual" {
		t.Fatalf("origin = %q", row[6])
	}
}

func TestBuildRespectsTheDateRange(t *testing.T) {
	f := newFixture(t)
	f.addEntry(t, 1000, "今天")
	f.clock.Advance(48 * time.Hour)
	f.addEntry(t, 2000, "后天")
	plan := export.Plan{
		Tables: []export.TableSelection{{Key: "entries", Columns: []string{"amount"}}},
		Range:  shared.DateRange{From: shared.DateOf(f.clock.Now()), To: shared.DateOf(f.clock.Now()).AddDays(1)},
		Layout: export.LayoutCSV,
	}
	sheets, err := f.svc.Build(plan, labels{})
	if err != nil {
		t.Fatal(err)
	}
	if len(sheets[0].Rows) != 1 || sheets[0].Rows[0][0] != "20.00" {
		t.Fatalf("rows = %v", sheets[0].Rows)
	}
}

func TestBuildSessionsSheetResolvesTaskTitles(t *testing.T) {
	f := newFixture(t)
	session, err := focus.Start("00000000000000000000000090", f.task, focus.KindFocus, 25*time.Minute, f.clock.Now())
	if err != nil {
		t.Fatal(err)
	}
	if err := session.Complete(f.clock.Now().Add(25 * time.Minute)); err != nil {
		t.Fatal(err)
	}
	if err := f.store.Sessions().Create(session); err != nil {
		t.Fatal(err)
	}
	plan := export.Plan{
		Tables: []export.TableSelection{{Key: "sessions", Columns: []string{"kind", "phase", "title", "planned_minutes", "elapsed_minutes", "interruptions"}}},
		Layout: export.LayoutXLSX,
	}
	sheets, err := f.svc.Build(plan, labels{})
	if err != nil {
		t.Fatal(err)
	}
	row := sheets[0].Rows[0]
	if row[0] != "focus" || row[1] != "done" || row[2] != "写方案" {
		t.Fatalf("row = %v", row)
	}
	if row[3] != "25" || row[4] != "25" || row[5] != "0" {
		t.Fatalf("row = %v", row)
	}
}

func TestBuildAccountsAndCategoriesAndBudgets(t *testing.T) {
	f := newFixture(t)
	f.addEntry(t, 3550, "午饭")
	budget, err := ledger.NewBudget("00000000000000000000000095", f.cat, shared.DateOf(f.clock.Now()).AsMonth(), shared.NewMoney(100000, shared.CNY))
	if err != nil {
		t.Fatal(err)
	}
	if err := f.store.Budgets().Upsert(budget); err != nil {
		t.Fatal(err)
	}
	plan := export.Plan{
		Tables: []export.TableSelection{
			{Key: "accounts", Columns: []string{"name", "kind", "currency", "balance", "archived"}},
			{Key: "categories", Columns: []string{"name", "direction", "parent"}},
			{Key: "budgets", Columns: []string{"month", "category", "limit", "currency"}},
		},
		Layout: export.LayoutXLSX,
	}
	sheets, err := f.svc.Build(plan, labels{})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if len(sheets) != 3 {
		t.Fatalf("want three sheets, got %d", len(sheets))
	}
	// 100.00 opening minus 35.50 spent.
	if got := sheets[0].Rows[0][3]; got != "64.50" {
		t.Fatalf("balance = %q, want 64.50", got)
	}
	if sheets[0].Rows[0][4] != "false" {
		t.Fatalf("archived = %q", sheets[0].Rows[0][4])
	}
	if sheets[1].Rows[0][1] != "expense" || sheets[1].Rows[0][2] != "" {
		t.Fatalf("category row = %v", sheets[1].Rows[0])
	}
	if sheets[2].Rows[0][0] != "2026-11" || sheets[2].Rows[0][1] != "餐饮" || sheets[2].Rows[0][2] != "1000.00" {
		t.Fatalf("budget row = %v", sheets[2].Rows[0])
	}
}

func TestBuildTemplatesSheet(t *testing.T) {
	f := newFixture(t)
	tpl, err := template.New("00000000000000000000000096", "晨间", f.clock.Now())
	if err != nil {
		t.Fatal(err)
	}
	tpl.Enabled = true
	tpl.Rule = template.TriggerRule{Kind: template.TriggerWeekdays, Weekdays: []time.Weekday{time.Monday, time.Friday}}
	if err := tpl.SetTask(template.Task{Title: "写日记", Notes: "三行", Estimate: 2, FocusMinutes: 45, DueOffsetDays: 1}, f.clock.Now()); err != nil {
		t.Fatal(err)
	}
	if err := f.store.Templates().Save(tpl); err != nil {
		t.Fatal(err)
	}
	plan := export.Plan{
		Tables: []export.TableSelection{{Key: "templates", Columns: []string{"name", "enabled", "rule", "task", "notes", "estimate", "focus_minutes", "due_offset"}}},
		Layout: export.LayoutMarkdown,
	}
	sheets, err := f.svc.Build(plan, labels{})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	row := sheets[0].Rows[0]
	if row[0] != "晨间" || row[1] != "true" {
		t.Fatalf("row = %v", row)
	}
	if !strings.HasPrefix(row[2], "weekdays:") {
		t.Fatalf("rule = %q, want a stable machine form", row[2])
	}
	// The template's pomodoro length is exported as a plain number, so a file can be
	// read back into a template that carries it (ingest parses the same column).
	if row[3] != "写日记" || row[4] != "三行" || row[5] != "2" || row[6] != "45" || row[7] != "1" {
		t.Fatalf("task = %v", row[3:])
	}
}

func TestBuildRejectsAnInvalidPlan(t *testing.T) {
	f := newFixture(t)
	if _, err := f.svc.Build(export.Plan{}, labels{}); !shared.IsKind(err, shared.KindInvalid) {
		t.Fatalf("an empty plan must be invalid, got %v", err)
	}
	if _, err := f.svc.Build(export.Plan{
		Tables: []export.TableSelection{{Key: "tasks", Columns: []string{"nope"}}},
		Layout: export.LayoutCSV,
	}, labels{}); !shared.IsKind(err, shared.KindInvalid) {
		t.Fatalf("an unknown column must be invalid, got %v", err)
	}
}

func TestTodayUsesTheInjectedClock(t *testing.T) {
	f := newFixture(t)
	if got := f.svc.Today(); !got.Equal(shared.DateOf(f.clock.Now())) {
		t.Fatalf("Today = %v", got)
	}
}

func TestDescribeRuleForEveryKind(t *testing.T) {
	// The rule column must be reversible text, not a sentence: an export that
	// changes with the interface language cannot be re-imported.
	f := newFixture(t)
	rules := []template.TriggerRule{
		{Kind: template.TriggerDaily},
		{Kind: template.TriggerMonthly, DayOfMonth: 15},
		{Kind: template.TriggerSpecificDates, Dates: []shared.LocalDate{shared.MustParseLocalDate("2026-12-25")}},
		{Kind: template.TriggerEveryNDays, EveryNDays: 3, Anchor: shared.MustParseLocalDate("2026-11-17")},
		{Kind: template.TriggerKind(99)},
	}
	for _, rule := range rules {
		tpl, err := template.New(f.ids.New(), "x", f.clock.Now())
		if err != nil {
			t.Fatal(err)
		}
		// SetTask validates, so the rule is swapped in after the task exists.
		if err := tpl.SetTask(template.Task{Title: "y"}, f.clock.Now()); err != nil {
			t.Fatal(err)
		}
		tpl.Rule = rule
		if err := f.store.Templates().Save(tpl); err != nil {
			t.Fatal(err)
		}
	}
	plan := export.Plan{
		Tables: []export.TableSelection{{Key: "templates", Columns: []string{"rule"}}},
		Layout: export.LayoutCSV,
	}
	sheets, err := f.svc.Build(plan, labels{})
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, row := range sheets[0].Rows {
		seen[row[0]] = true
	}
	for _, want := range []string{"daily", "monthly:15", "dates:2026-12-25", "every_n_days:3:from:2026-11-17", "unknown"} {
		if !seen[want] {
			t.Fatalf("missing rule %q in %v", want, seen)
		}
	}
}
