package ingest_test

import (
	"strings"
	"testing"

	app "github.com/iotus3/simplelife/app/internal/application/ingest"
	"github.com/iotus3/simplelife/app/internal/apptest/harness"
	"github.com/iotus3/simplelife/app/internal/apptest/store"
	"github.com/iotus3/simplelife/app/internal/domain/export"
	"github.com/iotus3/simplelife/app/internal/domain/ledger"
	"github.com/iotus3/simplelife/app/internal/domain/shared"
	task "github.com/iotus3/simplelife/app/internal/domain/task"
)

// zhHeaders is the interface's side of the import: the labels a Chinese export
// writes for each column. Only a few are given, which also proves a file that
// carries a subset of the columns still imports.
type zhHeaders struct{}

func (zhHeaders) Header(table, column string) []string {
	labels := map[string]map[string]string{
		"tasks": {
			"title": "标题", "status": "状态", "due_date": "截止日期",
			"estimate_pomodoros": "预估番茄", "tags": "标签", "notes": "备注",
		},
		"entries": {
			"kind": "类型", "amount": "金额", "currency": "币种",
			"account": "账户", "category": "分类", "occurred_at": "发生时间",
		},
		"templates": {
			"task": "待办标题", "rule": "触发规则", "enabled": "启用",
			"estimate": "预估番茄", "focus_minutes": "一个番茄的时长",
			"due_offset": "到期偏移（天）",
		},
	}
	if byTable, ok := labels[table]; ok {
		if label, ok := byTable[column]; ok {
			return []string{label}
		}
	}
	return nil
}

type fixture struct {
	svc   *app.Service
	store *store.Store
	clock *harness.Clock
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	st := store.New()
	clk := harness.NewClock(harness.At("2026-11-17 09:00:00"))
	svc := app.New(st.Tasks(), st.Entries(), st.Accounts(), st.Categories(), st.Budgets(),
		st.Templates(), clk, harness.NewIDs())
	return &fixture{svc: svc, store: st, clock: clk}
}

// document builds one table the way the decoder hands it over.
func document(headers []string, rows ...[]string) export.Document {
	return export.Document{Headers: headers, Rows: rows}
}

// importInto runs one import with the Chinese header labels, which is what a
// Chinese export writes.
func (f *fixture) importInto(table string, document export.Document) (app.Report, error) {
	return f.svc.Import(table, document, zhHeaders{})
}

func TestImportTasksReadsLabelsAndRawKeys(t *testing.T) {
	f := newFixture(t)
	// The title column uses the raw key and the rest use Chinese labels, which is
	// what a file that traveled through a spreadsheet looks like.
	doc := document(
		[]string{"id", "标题", "状态", "截止日期", "预估番茄", "标签", "备注"},
		[]string{"00000000000000000000000001", "写周报", "done", "2026-11-20", "3", "work, 晨间", "三行"},
	)
	report, err := f.importInto("tasks", doc)
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	if report.Created != 1 || report.Skipped != 0 || len(report.Failed) != 0 {
		t.Fatalf("report = %+v", report)
	}
	tasks, err := f.store.Tasks().List(task.Filter{IncludeArchived: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks) != 1 {
		t.Fatalf("want one task, got %d", len(tasks))
	}
	restored := tasks[0]
	if restored.Title != "写周报" || restored.Status != task.StatusDone {
		t.Fatalf("task = %+v", restored)
	}
	if !restored.DueDate().Equal(shared.MustParseLocalDate("2026-11-20")) {
		t.Fatalf("due = %v", restored.DueDate())
	}
	if restored.EstimatePomodoros != 3 || restored.Notes != "三行" {
		t.Fatalf("estimates or notes lost: %+v", restored)
	}
	if len(restored.Tags) != 2 || restored.Tags[0] != "work" || restored.Tags[1] != "晨间" {
		t.Fatalf("tags = %v", restored.Tags)
	}
	if restored.CompletedAt.IsZero() {
		t.Fatal("a done task must carry a completion date")
	}
}

func TestImportTasksSkipsWhatIsAlreadyThere(t *testing.T) {
	f := newFixture(t)
	doc := document([]string{"id", "title"}, []string{"00000000000000000000000001", "写周报"})
	first, err := f.importInto("tasks", doc)
	if err != nil {
		t.Fatal(err)
	}
	if first.Created != 1 {
		t.Fatalf("first import = %+v", first)
	}
	// Importing the same file again must add nothing: the id is the key.
	second, err := f.importInto("tasks", doc)
	if err != nil {
		t.Fatal(err)
	}
	if second.Created != 0 || second.Skipped != 1 {
		t.Fatalf("second import = %+v", second)
	}
	tasks, _ := f.store.Tasks().List(task.Filter{})
	if len(tasks) != 1 {
		t.Fatalf("want one task after two imports, got %d", len(tasks))
	}
}

func TestImportReportsBadRowsAndKeepsGoing(t *testing.T) {
	f := newFixture(t)
	doc := document(
		[]string{"id", "title", "priority"},
		[]string{"00000000000000000000000001", "好的", "2"},
		[]string{"00000000000000000000000002", "  ", "2"},
		[]string{"00000000000000000000000003", "坏优先级", "9"},
		[]string{"00000000000000000000000004", "也好", "1"},
	)
	report, err := f.importInto("tasks", doc)
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	if report.Created != 2 || len(report.Failed) != 2 {
		t.Fatalf("report = %+v", report)
	}
	// Rows are counted the way a person counts in the file: the header is row 1.
	if report.Failed[0].Row != 3 || report.Failed[0].Code != "task.title.length" {
		t.Fatalf("first failure = %+v", report.Failed[0])
	}
	if report.Failed[1].Row != 4 || report.Failed[1].Code != "import.number.range" {
		t.Fatalf("second failure = %+v", report.Failed[1])
	}
}

func TestImportEntriesResolvesNames(t *testing.T) {
	f := newFixture(t)
	account, err := ledger.NewAccount("000000000000000000000000A1", "钱包", ledger.AccountCash,
		shared.NewMoney(0, shared.CNY))
	if err != nil {
		t.Fatal(err)
	}
	if err := f.store.Accounts().Create(account); err != nil {
		t.Fatal(err)
	}
	category, err := ledger.NewCategory("000000000000000000000000B1", "餐饮", ledger.CategoryExpense)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.store.Categories().Create(category); err != nil {
		t.Fatal(err)
	}

	doc := document(
		[]string{"id", "kind", "amount", "currency", "account", "category", "occurred_at", "note"},
		[]string{"000000000000000000000000C1", "expense", "-12.50", "CNY", "钱包", "餐饮", "2026-11-16 20:15", "晚饭"},
		[]string{"000000000000000000000000C2", "expense", "-8.00", "CNY", "不存在的账户", "餐饮", "2026-11-16 21:00", ""},
	)
	report, err := f.importInto("entries", doc)
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	if report.Created != 1 || len(report.Failed) != 1 {
		t.Fatalf("report = %+v", report)
	}
	if report.Failed[0].Code != "import.entry.account" {
		t.Fatalf("an unknown account must be a failure, not a new account: %+v", report.Failed[0])
	}
	entries, err := f.store.Entries().List(ledger.EntryFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("want one entry, got %d", len(entries))
	}
	entry := entries[0]
	if entry.Amount.Minor() != 1250 || entry.Amount.Currency() != shared.CNY {
		t.Fatalf("amount = %d %v", entry.Amount.Minor(), entry.Amount.Currency())
	}
	if entry.Kind != ledger.EntryExpense || entry.AccountID != account.ID || entry.CategoryID != category.ID {
		t.Fatalf("entry = %+v", entry)
	}
	if entry.Note != "晚饭" || entry.OccurredAt.Day() != 16 {
		t.Fatalf("the row's own values must survive: %+v", entry)
	}
}

func TestImportTemplatesRebuildsTheRule(t *testing.T) {
	f := newFixture(t)
	doc := document(
		[]string{"name", "启用", "触发规则", "待办标题", "预估番茄", "一个番茄的时长", "due_offset"},
		[]string{"每周复盘", "true", "weekdays:1,5", "写周报", "2", "45", "1"},
		// The last row leaves the length out: a file exported before the column
		// existed must still import, and the template then has no length of its own.
		[]string{"每天喝水", "false", "daily", "喝水", "0", "", "0"},
	)
	report, err := f.importInto("templates", doc)
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	if report.Created != 2 || len(report.Failed) != 0 {
		t.Fatalf("report = %+v", report)
	}
	templates, err := f.store.Templates().List()
	if err != nil {
		t.Fatal(err)
	}
	if len(templates) != 2 {
		t.Fatalf("want two templates, got %d", len(templates))
	}
	var weekly, daily bool
	for _, tpl := range templates {
		switch tpl.Name {
		case "每周复盘":
			weekly = true
			if !tpl.Enabled || tpl.Task.Title != "写周报" || tpl.Task.Estimate != 2 || tpl.Task.DueOffsetDays != 1 {
				t.Fatalf("template = %+v", tpl)
			}
			if tpl.Task.FocusMinutes != 45 {
				t.Fatalf("the pomodoro length was lost on import: %+v", tpl.Task)
			}
			if len(tpl.Rule.Weekdays) != 2 || !tpl.Rule.Matches(shared.MustParseLocalDate("2026-11-20")) {
				t.Fatalf("rule = %+v", tpl.Rule)
			}
		case "每天喝水":
			daily = true
			if tpl.Enabled {
				t.Fatal("a disabled template must stay disabled")
			}
			if tpl.Task.FocusMinutes != 0 {
				t.Fatalf("an empty length must stay empty: %+v", tpl.Task)
			}
		}
	}
	if !weekly || !daily {
		t.Fatalf("templates = %+v", templates)
	}
}

func TestImportRefusesWhatItCannotRestore(t *testing.T) {
	f := newFixture(t)
	for _, table := range []string{"sessions", "accounts", "nope"} {
		if _, err := f.importInto(table, document([]string{"id"}, []string{"x"})); !shared.IsKind(err, shared.KindInvalid) {
			t.Fatalf("%s must be refused, got %v", table, err)
		}
	}
}

func TestImportRefusesAFileOfAnotherTable(t *testing.T) {
	f := newFixture(t)
	doc := document([]string{"颜色", "味道"}, []string{"红", "甜"})
	_, err := f.importInto("tasks", doc)
	if !shared.IsKind(err, shared.KindInvalid) || !strings.Contains(err.Error(), "import.header") {
		t.Fatalf("a file with no known column must be refused, got %v", err)
	}
}

func TestImportableMatchesTables(t *testing.T) {
	got := strings.Join(app.Tables(), ",")
	if got != "categories,tasks,entries,budgets,templates" {
		t.Fatalf("Tables() = %s", got)
	}
	for _, key := range app.Tables() {
		if !app.Importable(key) {
			t.Fatalf("%s must be importable", key)
		}
	}
	if app.Importable("sessions") {
		t.Fatal("focus history is not importable")
	}
}
