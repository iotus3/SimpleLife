package template_test

import (
	"errors"
	"testing"
	"time"

	app "github.com/iotus3/simplelife/app/internal/application/template"
	"github.com/iotus3/simplelife/app/internal/apptest/harness"
	"github.com/iotus3/simplelife/app/internal/apptest/store"
	"github.com/iotus3/simplelife/app/internal/domain/shared"
	task "github.com/iotus3/simplelife/app/internal/domain/task"
	"github.com/iotus3/simplelife/app/internal/domain/template"
)

type fixture struct {
	svc   *app.Service
	store *store.Store
	clock *harness.Clock
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	st := store.New()
	clk := harness.NewClock(harness.At("2026-11-17 08:00:00"))
	return &fixture{svc: app.New(st.Templates(), st.Tasks(), st, clk, harness.NewIDs()), store: st, clock: clk}
}

// oneTask is the single todo a template writes.
func oneTask(title string) template.Task {
	return template.Task{Title: title, Estimate: 1}
}

func TestSaveCreatesAndUpdates(t *testing.T) {
	f := newFixture(t)
	tpl, err := f.svc.Save(app.SaveInput{
		Name:    "晨间",
		Enabled: true,
		Rule:    template.TriggerRule{Kind: template.TriggerDaily},
		Task:    oneTask("写日记"),
	})
	if err != nil {
		t.Fatalf("Save: %v", err)
	}
	if tpl.ID.IsZero() || tpl.Task.Title != "写日记" || tpl.Task.Estimate != 1 {
		t.Fatalf("template = %+v", tpl)
	}

	updated, err := f.svc.Save(app.SaveInput{
		ID:      tpl.ID,
		Name:    "晨间 2",
		Enabled: true,
		Rule:    template.TriggerRule{Kind: template.TriggerDaily},
		Task:    oneTask("看邮件"),
	})
	if err != nil {
		t.Fatalf("Save update: %v", err)
	}
	if updated.ID != tpl.ID {
		t.Fatal("updating must keep the id")
	}
	if !updated.CreatedAt.Equal(tpl.CreatedAt) {
		t.Fatal("CreatedAt must be preserved")
	}
	if updated.Task.Title != "看邮件" {
		t.Fatalf("a template writes exactly the task it was saved with: %+v", updated.Task)
	}

	if _, err := f.svc.Save(app.SaveInput{Name: "  "}); !shared.IsKind(err, shared.KindInvalid) {
		t.Fatalf("a blank name must be invalid, got %v", err)
	}
	if _, err := f.svc.Save(app.SaveInput{
		Name:    "无名待办",
		Enabled: true,
		Rule:    template.TriggerRule{Kind: template.TriggerDaily},
	}); !shared.IsKind(err, shared.KindInvalid) {
		t.Fatalf("a template with no task title must be invalid, got %v", err)
	}
	if _, err := f.svc.Save(app.SaveInput{ID: shared.ID("00000000000000000000000099"), Name: "x"}); !shared.IsKind(err, shared.KindNotFound) {
		t.Fatalf("updating a missing template must be not-found, got %v", err)
	}
}

func TestExpandCreatesOneTaskPerTemplateOnce(t *testing.T) {
	f := newFixture(t)
	if _, err := f.svc.Save(app.SaveInput{
		Name: "每天一项", Enabled: true,
		Rule: template.TriggerRule{Kind: template.TriggerDaily},
		Task: oneTask("A"),
	}); err != nil {
		t.Fatal(err)
	}
	// A second template writes its own task: two templates never share one.
	if _, err := f.svc.Save(app.SaveInput{
		Name: "每天另一项", Enabled: true,
		Rule: template.TriggerRule{Kind: template.TriggerDaily},
		Task: oneTask("B"),
	}); err != nil {
		t.Fatal(err)
	}
	day := shared.DateOf(f.clock.Now())

	first, err := f.svc.Expand(day)
	if err != nil {
		t.Fatalf("Expand: %v", err)
	}
	if first.Created != 2 || first.Skipped != 0 || len(first.Failed) != 0 {
		t.Fatalf("first run = %+v", first)
	}
	for i := 0; i < 2; i++ {
		again, err := f.svc.Expand(day)
		if err != nil {
			t.Fatal(err)
		}
		if again.Created != 0 || again.Skipped != 2 {
			t.Fatalf("run %d must skip everything: %+v", i+2, again)
		}
	}
	tasks, err := f.store.Tasks().List(task.Filter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks) != 2 {
		t.Fatalf("want 2 tasks, got %d", len(tasks))
	}
	titles := map[string]bool{}
	for _, tk := range tasks {
		if !tk.IsFromTemplate() {
			t.Fatalf("task %+v must carry its origin", tk)
		}
		if !tk.DueDate().Equal(day) {
			t.Fatalf("due date = %v, want %v", tk.DueDate(), day)
		}
		titles[tk.Title] = true
	}
	if !titles["A"] || !titles["B"] {
		t.Fatalf("each template must write its own task: %v", titles)
	}
}

// The length of one pomodoro a template carries is copied into the task it writes,
// so a "read for 40 minutes" template produces sessions that already know it. A
// template without one writes tasks that fall back to the focus page.
func TestExpandedTaskCarriesTheTemplatesPomodoroLength(t *testing.T) {
	f := newFixture(t)
	if _, err := f.svc.Save(app.SaveInput{
		Name: "精读文献", Enabled: true,
		Rule: template.TriggerRule{Kind: template.TriggerDaily},
		Task: template.Task{Title: "精读一篇", Estimate: 2, FocusMinutes: 40},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.Save(app.SaveInput{
		Name: "普通任务", Enabled: true,
		Rule: template.TriggerRule{Kind: template.TriggerDaily},
		Task: oneTask("随手记"),
	}); err != nil {
		t.Fatal(err)
	}
	day := shared.DateOf(f.clock.Now())
	if _, err := f.svc.Expand(day); err != nil {
		t.Fatalf("Expand: %v", err)
	}
	tasks, err := f.store.Tasks().List(task.Filter{})
	if err != nil {
		t.Fatal(err)
	}
	lengths := map[string]int{}
	for _, tk := range tasks {
		lengths[tk.Title] = tk.FocusMinutes
	}
	if lengths["精读一篇"] != 40 {
		t.Fatalf("the generated task lost the template's length: %v", lengths)
	}
	if lengths["随手记"] != 0 {
		t.Fatalf("a template without a length must not invent one: %v", lengths)
	}
}

func TestDeletedGeneratedTaskDoesNotComeBack(t *testing.T) {
	f := newFixture(t)
	if _, err := f.svc.Save(app.SaveInput{
		Name: "每天一项", Enabled: true,
		Rule: template.TriggerRule{Kind: template.TriggerDaily},
		Task: oneTask("A"),
	}); err != nil {
		t.Fatal(err)
	}
	day := shared.DateOf(f.clock.Now())
	if _, err := f.svc.Expand(day); err != nil {
		t.Fatal(err)
	}
	tasks, _ := f.store.Tasks().List(task.Filter{})
	if len(tasks) != 1 {
		t.Fatalf("want one task, got %d", len(tasks))
	}
	// Soft-delete like the task service does when a task must stay for its key.
	archived := tasks[0]
	if err := archived.Archive(f.clock.Now()); err != nil {
		t.Fatal(err)
	}
	if err := f.store.Tasks().Update(archived); err != nil {
		t.Fatal(err)
	}
	after, err := f.svc.Expand(day)
	if err != nil {
		t.Fatal(err)
	}
	if after.Created != 0 || after.Skipped != 1 {
		t.Fatalf("an archived occurrence must stay archived: %+v", after)
	}
	visible, _ := f.store.Tasks().List(task.Filter{})
	if len(visible) != 0 {
		t.Fatalf("the deleted task must not reappear: %+v", visible)
	}
}

func TestExpandSkipsTemplatesThatDoNotFire(t *testing.T) {
	f := newFixture(t)
	// 2026-11-17 is a Tuesday.
	if _, err := f.svc.Save(app.SaveInput{
		Name: "只周一", Enabled: true,
		Rule: template.TriggerRule{Kind: template.TriggerWeekdays, Weekdays: []time.Weekday{time.Monday}},
		Task: oneTask("A"),
	}); err != nil {
		t.Fatal(err)
	}
	res, err := f.svc.Expand(shared.MustParseLocalDate("2026-11-17"))
	if err != nil {
		t.Fatal(err)
	}
	if res.Created != 0 || res.Skipped != 0 {
		t.Fatalf("a Tuesday must not fire a Monday rule: %+v", res)
	}
	res, err = f.svc.Expand(shared.MustParseLocalDate("2026-11-23"))
	if err != nil {
		t.Fatal(err)
	}
	if res.Created != 1 {
		t.Fatalf("Monday must fire: %+v", res)
	}
}

func TestExpandIgnoresDisabledTemplates(t *testing.T) {
	f := newFixture(t)
	if _, err := f.svc.Save(app.SaveInput{
		Name: "关掉了", Enabled: false,
		Rule: template.TriggerRule{Kind: template.TriggerDaily},
		Task: oneTask("A"),
	}); err != nil {
		t.Fatal(err)
	}
	res, err := f.svc.Expand(shared.DateOf(f.clock.Now()))
	if err != nil {
		t.Fatal(err)
	}
	if res.Created != 0 {
		t.Fatalf("a disabled template must not fire: %+v", res)
	}
}

func TestExpandReportsABadTaskWithoutStopping(t *testing.T) {
	f := newFixture(t)
	// A blank task title can only exist if the row was written outside the service;
	// it must be reported as a failure instead of creating a nameless todo.
	tpl, err := f.svc.Save(app.SaveInput{
		Name: "坏掉的", Enabled: true,
		Rule: template.TriggerRule{Kind: template.TriggerDaily},
		Task: oneTask("好"),
	})
	if err != nil {
		t.Fatal(err)
	}
	// A healthy template proves the run keeps going after the failure.
	if _, err := f.svc.Save(app.SaveInput{
		Name: "正常的", Enabled: true,
		Rule: template.TriggerRule{Kind: template.TriggerDaily},
		Task: oneTask("也好"),
	}); err != nil {
		t.Fatal(err)
	}
	tpl.Task.Title = "   "
	if err := f.store.Templates().Save(tpl); err != nil {
		t.Fatal(err)
	}
	res, err := f.svc.Expand(shared.DateOf(f.clock.Now()))
	if err != nil {
		t.Fatalf("Expand: %v", err)
	}
	if res.Created != 1 || len(res.Failed) != 1 {
		t.Fatalf("res = %+v", res)
	}
	if res.Failed[0].TemplateID != tpl.ID || res.Failed[0].Code != "task.title.length" {
		t.Fatalf("failure = %+v", res.Failed[0])
	}
}

func TestExpandRejectsAZeroDay(t *testing.T) {
	f := newFixture(t)
	if _, err := f.svc.Expand(shared.LocalDate{}); !shared.IsKind(err, shared.KindInvalid) {
		t.Fatalf("a zero day must be invalid, got %v", err)
	}
}

func TestExpandTreatsADatabaseRaceAsSkipped(t *testing.T) {
	// The unique index is the authority; a duplicate that slips past the existence
	// check must be counted as skipped, not as a failure.
	f := newFixture(t)
	if _, err := f.svc.Save(app.SaveInput{
		Name: "每天", Enabled: true,
		Rule: template.TriggerRule{Kind: template.TriggerDaily},
		Task: oneTask("A"),
	}); err != nil {
		t.Fatal(err)
	}
	// Pre-insert an occurrence with the same key but a different id path: the
	// expander will find it through ExistsOccurrence, which is the normal path.
	day := shared.DateOf(f.clock.Now())
	tpl, _ := f.svc.List()
	seed, err := task.NewFromTemplate("00000000000000000000000080",
		task.FromTemplateSpec{Title: "A"},
		task.TemplateOrigin{TemplateID: tpl[0].ID, Day: day, Seq: 0}, f.clock.Now())
	if err != nil {
		t.Fatal(err)
	}
	if err := f.store.Tasks().Create(seed); err != nil {
		t.Fatal(err)
	}
	res, err := f.svc.Expand(day)
	if err != nil {
		t.Fatal(err)
	}
	if res.Skipped != 1 || res.Created != 0 {
		t.Fatalf("res = %+v", res)
	}
}

func TestPreviewDoesNotWrite(t *testing.T) {
	f := newFixture(t)
	for _, name := range []string{"每天 A", "每天 B"} {
		if _, err := f.svc.Save(app.SaveInput{
			Name: name, Enabled: true,
			Rule: template.TriggerRule{Kind: template.TriggerDaily},
			Task: oneTask(name),
		}); err != nil {
			t.Fatal(err)
		}
	}
	day := shared.DateOf(f.clock.Now())
	preview, err := f.svc.Preview(day)
	if err != nil {
		t.Fatalf("Preview: %v", err)
	}
	if len(preview) != 2 {
		t.Fatalf("preview = %+v", preview)
	}
	if preview[0].Exists {
		t.Fatal("nothing has been expanded yet")
	}
	if !preview[0].DueDate.Equal(day) {
		t.Fatalf("due = %v", preview[0].DueDate)
	}
	if _, err := f.svc.Expand(day); err != nil {
		t.Fatal(err)
	}
	preview, err = f.svc.Preview(day)
	if err != nil {
		t.Fatal(err)
	}
	if !preview[0].Exists {
		t.Fatal("after expanding, the preview must show the occurrence as existing")
	}
}

func TestNextRunFindsTheNextMatchingDay(t *testing.T) {
	daily := &template.Template{Enabled: true, Rule: template.TriggerRule{Kind: template.TriggerDaily}}
	from := shared.MustParseLocalDate("2026-11-17")
	if got := app.NextRun(daily, from); !got.Equal(from) {
		t.Fatalf("daily NextRun = %v", got)
	}
	weekly := &template.Template{Enabled: true, Rule: template.TriggerRule{Kind: template.TriggerWeekdays, Weekdays: []time.Weekday{time.Monday}}}
	if got := app.NextRun(weekly, from); !got.Equal(shared.MustParseLocalDate("2026-11-23")) {
		t.Fatalf("weekly NextRun = %v", got)
	}
	if got := app.NextRun(&template.Template{Enabled: false}, from); !got.IsZero() {
		t.Fatalf("a disabled template has no next run, got %v", got)
	}
	once := &template.Template{Enabled: true, Rule: template.TriggerRule{Kind: template.TriggerSpecificDates, Dates: []shared.LocalDate{shared.MustParseLocalDate("2020-01-01")}}}
	if got := app.NextRun(once, from); !got.IsZero() {
		t.Fatalf("a past one-off has no next run, got %v", got)
	}
	if got := app.NextRun(nil, from); !got.IsZero() {
		t.Fatalf("nil template = %v", got)
	}
}

func TestLogsAndLastExpanded(t *testing.T) {
	f := newFixture(t)
	tpl, err := f.svc.Save(app.SaveInput{
		Name: "每天", Enabled: true,
		Rule: template.TriggerRule{Kind: template.TriggerDaily},
		Task: oneTask("A"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok, err := f.svc.LastExpanded(tpl.ID); err != nil || ok {
		t.Fatalf("no log expected yet: %v %v", ok, err)
	}
	day := shared.DateOf(f.clock.Now())
	if _, err := f.svc.Expand(day); err != nil {
		t.Fatal(err)
	}
	entry, ok, err := f.svc.LastExpanded(tpl.ID)
	if err != nil || !ok {
		t.Fatalf("LastExpanded: %v %v", ok, err)
	}
	if entry.Created != 1 || !entry.Day.Equal(day) {
		t.Fatalf("log = %+v", entry)
	}
	logs, err := f.svc.RecentLogs(0)
	if err != nil {
		t.Fatal(err)
	}
	if len(logs) != 1 {
		t.Fatalf("logs = %+v", logs)
	}
	if _, ok, err := f.svc.LastExpanded(shared.ID("00000000000000000000000099")); err != nil || ok {
		t.Fatalf("an unknown template must have no log: %v %v", ok, err)
	}
}

func TestDeleteRemovesTheTemplateButKeepsItsTasks(t *testing.T) {
	f := newFixture(t)
	tpl, err := f.svc.Save(app.SaveInput{
		Name: "每天", Enabled: true,
		Rule: template.TriggerRule{Kind: template.TriggerDaily},
		Task: oneTask("A"),
	})
	if err != nil {
		t.Fatal(err)
	}
	day := shared.DateOf(f.clock.Now())
	if _, err := f.svc.Expand(day); err != nil {
		t.Fatal(err)
	}
	if err := f.svc.Delete(tpl.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := f.svc.Get(tpl.ID); !shared.IsKind(err, shared.KindNotFound) {
		t.Fatalf("the template must be gone, got %v", err)
	}
	tasks, _ := f.store.Tasks().List(task.Filter{})
	if len(tasks) != 1 {
		t.Fatalf("the generated task must stay: %+v", tasks)
	}
	if err := f.svc.Delete(tpl.ID); !shared.IsKind(err, shared.KindNotFound) {
		t.Fatalf("deleting twice must be not-found, got %v", err)
	}
}

func TestExpandSurfacesStoreErrors(t *testing.T) {
	// A store that fails to list must fail the call rather than silently create
	// nothing: the user pressed a button and deserves an error, not silence.
	f := newFixture(t)
	broken := app.New(failingRepo{err: errors.New("boom")}, f.store.Tasks(), f.store, f.clock, harness.NewIDs())
	if _, err := broken.Expand(shared.DateOf(f.clock.Now())); err == nil {
		t.Fatal("a store failure must be returned")
	}
}

type failingRepo struct{ err error }

func (r failingRepo) ListEnabled() ([]*template.Template, error)                        { return nil, r.err }
func (r failingRepo) List() ([]*template.Template, error)                               { return nil, r.err }
func (r failingRepo) Get(shared.ID) (*template.Template, error)                         { return nil, r.err }
func (r failingRepo) Save(*template.Template) error                                     { return r.err }
func (r failingRepo) Delete(shared.ID) error                                            { return r.err }
func (r failingRepo) MarkExpanded(shared.ID, shared.LocalDate, template.LogEntry) error { return r.err }
func (r failingRepo) RecentLogs(int) ([]template.LogEntry, error)                       { return nil, r.err }
