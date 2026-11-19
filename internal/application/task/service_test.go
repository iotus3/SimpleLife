package task_test

import (
	"testing"
	"time"

	app "github.com/iotus3/simplelife/app/internal/application/task"
	"github.com/iotus3/simplelife/app/internal/apptest/harness"
	"github.com/iotus3/simplelife/app/internal/apptest/store"
	"github.com/iotus3/simplelife/app/internal/domain/focus"
	"github.com/iotus3/simplelife/app/internal/domain/shared"
	domain "github.com/iotus3/simplelife/app/internal/domain/task"
)

func newService(t *testing.T) (*app.Service, *store.Store, *harness.Clock) {
	t.Helper()
	st := store.New()
	clk := harness.NewClock(harness.At("2026-11-17 09:00:00"))
	return app.New(st.Tasks(), st, clk, harness.NewIDs()), st, clk
}

func TestCreateStoresAValidatedTask(t *testing.T) {
	svc, st, clk := newService(t)
	tk, err := svc.Create(app.CreateInput{
		Title:    "  写方案  ",
		Notes:    "细节",
		Priority: 1,
		DueDate:  shared.MustParseLocalDate("2026-11-17"),
		Estimate: 3,
		Tags:     []string{"Work"},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if tk.Title != "写方案" || tk.Priority != 1 || tk.EstimatePomodoros != 3 {
		t.Fatalf("unexpected task: %+v", tk)
	}
	if !tk.DueDate().Equal(shared.MustParseLocalDate("2026-11-17")) {
		t.Fatalf("due date = %v", tk.DueDate())
	}
	stored, err := st.Tasks().Get(tk.ID)
	if err != nil {
		t.Fatalf("the task must be persisted: %v", err)
	}
	if !stored.CreatedAt.Equal(clk.Now()) {
		t.Fatal("CreatedAt must come from the injected clock")
	}
}

func TestCreateRejectsBadInputAndStoresNothing(t *testing.T) {
	svc, st, _ := newService(t)
	if _, err := svc.Create(app.CreateInput{Title: "   "}); !shared.IsKind(err, shared.KindInvalid) {
		t.Fatalf("blank title must be invalid, got %v", err)
	}
	if _, err := svc.Create(app.CreateInput{Title: "x", Priority: 9}); !shared.IsKind(err, shared.KindInvalid) {
		t.Fatalf("out-of-range priority must be invalid, got %v", err)
	}
	if _, err := svc.Create(app.CreateInput{Title: "x", Estimate: 99}); !shared.IsKind(err, shared.KindInvalid) {
		t.Fatalf("out-of-range estimate must be invalid, got %v", err)
	}
	all, err := st.Tasks().List(domain.Filter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 0 {
		t.Fatalf("a rejected create must store nothing, got %d tasks", len(all))
	}
}

func TestCreateSubtaskLinksToParent(t *testing.T) {
	svc, _, _ := newService(t)
	parent, err := svc.Create(app.CreateInput{Title: "父"})
	if err != nil {
		t.Fatal(err)
	}
	child, err := svc.Create(app.CreateInput{Title: "子", ParentID: parent.ID})
	if err != nil {
		t.Fatalf("Create subtask: %v", err)
	}
	if child.ParentID != parent.ID {
		t.Fatalf("parent = %v", child.ParentID)
	}
	kids, err := svc.Children(parent.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(kids) != 1 || kids[0].ID != child.ID {
		t.Fatalf("children = %+v", kids)
	}
	if _, err := svc.Create(app.CreateInput{Title: "孙", ParentID: child.ID}); !shared.IsKind(err, shared.KindInvalid) {
		t.Fatalf("a third level must be refused, got %v", err)
	}
	missing := shared.ID("00000000000000000000000099")
	if _, err := svc.Create(app.CreateInput{Title: "x", ParentID: missing}); !shared.IsKind(err, shared.KindNotFound) {
		t.Fatalf("a missing parent must be not-found, got %v", err)
	}
}

func TestUpdateAppliesAllFields(t *testing.T) {
	svc, _, clk := newService(t)
	tk, _ := svc.Create(app.CreateInput{Title: "旧", Priority: 1})
	clk.Advance(time.Minute)
	updated, err := svc.Update(app.UpdateInput{
		ID:       tk.ID,
		Title:    "新",
		Notes:    "备注",
		Priority: 3,
		DueDate:  shared.MustParseLocalDate("2026-11-20"),
		Estimate: 4,
		Tags:     []string{"a"},
	})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if updated.Title != "新" || updated.Priority != 3 || updated.EstimatePomodoros != 4 {
		t.Fatalf("unexpected task: %+v", updated)
	}
	if !updated.UpdatedAt.Equal(clk.Now()) {
		t.Fatal("UpdatedAt must advance")
	}
}

func TestUpdateDoesNotWriteWhenValidationFails(t *testing.T) {
	svc, st, _ := newService(t)
	tk, _ := svc.Create(app.CreateInput{Title: "原始", Priority: 1})
	if _, err := svc.Update(app.UpdateInput{ID: tk.ID, Title: "原始", Priority: 1, Estimate: 99}); !shared.IsKind(err, shared.KindInvalid) {
		t.Fatalf("bad estimate must be invalid, got %v", err)
	}
	stored, _ := st.Tasks().Get(tk.ID)
	if stored.EstimatePomodoros != 0 {
		t.Fatalf("a failed update must not be stored, got %+v", stored)
	}
	if _, err := svc.Update(app.UpdateInput{ID: "00000000000000000000000099", Title: "x"}); !shared.IsKind(err, shared.KindNotFound) {
		t.Fatalf("a missing task must be not-found, got %v", err)
	}
}

func TestCompleteAndReopen(t *testing.T) {
	svc, _, _ := newService(t)
	tk, _ := svc.Create(app.CreateInput{Title: "x"})
	done, err := svc.Complete(tk.ID)
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if done.Status != domain.StatusDone {
		t.Fatalf("status = %v", done.Status)
	}
	// Idempotent: a second click is not an error.
	if _, err := svc.Complete(tk.ID); err != nil {
		t.Fatalf("double Complete must be tolerated, got %v", err)
	}
	reopened, err := svc.Reopen(tk.ID)
	if err != nil {
		t.Fatalf("Reopen: %v", err)
	}
	if reopened.Status != domain.StatusTodo {
		t.Fatalf("status = %v", reopened.Status)
	}
	if _, err := svc.Reopen(tk.ID); !shared.IsKind(err, shared.KindConflict) {
		t.Fatalf("reopening a todo must conflict, got %v", err)
	}
}

func TestDeleteArchivesWhenHistoryExists(t *testing.T) {
	svc, st, clk := newService(t)
	tk, _ := svc.Create(app.CreateInput{Title: "x"})
	session, err := focusSession(st, tk.ID, clk.Now())
	if err != nil {
		t.Fatalf("seed session: %v", err)
	}
	_ = session
	if err := svc.Delete(tk.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	stored, err := st.Tasks().Get(tk.ID)
	if err != nil {
		t.Fatalf("a task with focus history must still exist: %v", err)
	}
	if stored.Status != domain.StatusArchived {
		t.Fatalf("status = %v, want archived", stored.Status)
	}
}

func TestDeleteRemovesCleanTasks(t *testing.T) {
	svc, st, _ := newService(t)
	tk, _ := svc.Create(app.CreateInput{Title: "x"})
	if err := svc.Delete(tk.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := st.Tasks().Get(tk.ID); !shared.IsKind(err, shared.KindNotFound) {
		t.Fatalf("a clean task must be removed, got %v", err)
	}
}

func TestDeleteAlwaysArchivesGeneratedTasks(t *testing.T) {
	// A generated task keeps its occurrence key, so it must survive as an archived
	// row: otherwise the expander would recreate it on the next start.
	svc, st, _ := newService(t)
	now := harness.At("2026-11-17 09:00:00")
	generated, err := domain.NewFromTemplate(
		"00000000000000000000000050",
		domain.FromTemplateSpec{Title: "模板任务"},
		domain.TemplateOrigin{TemplateID: "00000000000000000000000051", Day: shared.DateOf(now), Seq: 0},
		now,
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.Tasks().Create(generated); err != nil {
		t.Fatal(err)
	}
	if err := svc.Delete(generated.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	stored, err := st.Tasks().Get(generated.ID)
	if err != nil {
		t.Fatalf("the generated task must be archived, not removed: %v", err)
	}
	if stored.Status != domain.StatusArchived {
		t.Fatalf("status = %v", stored.Status)
	}
}

func TestMoveKeepsOrderStable(t *testing.T) {
	svc, _, _ := newService(t)
	a, _ := svc.Create(app.CreateInput{Title: "a"})
	b, _ := svc.Create(app.CreateInput{Title: "b"})
	c, _ := svc.Create(app.CreateInput{Title: "c"})

	// Put c first, then b between c and a.
	if err := svc.Move(app.MoveInput{ID: c.ID, NextID: a.ID}); err != nil {
		t.Fatalf("Move: %v", err)
	}
	if err := svc.Move(app.MoveInput{ID: b.ID, PrevID: c.ID, NextID: a.ID}); err != nil {
		t.Fatalf("Move: %v", err)
	}
	tasks, err := svc.Today(shared.MustParseLocalDate("2026-11-17"))
	if err != nil {
		t.Fatal(err)
	}
	// None of the tasks has a due date, so they are not in "today"; read them raw.
	tasks, err = svc.List(domain.Filter{Order: domain.OrderManual})
	if err != nil {
		t.Fatal(err)
	}
	got := []shared.ID{tasks[0].ID, tasks[1].ID, tasks[2].ID}
	want := []shared.ID{c.ID, b.ID, a.ID}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("order = %v, want %v", got, want)
		}
	}
}

func TestMoveAppendsAtTheEnd(t *testing.T) {
	svc, _, _ := newService(t)
	a, _ := svc.Create(app.CreateInput{Title: "a"})
	b, _ := svc.Create(app.CreateInput{Title: "b"})
	if err := svc.Move(app.MoveInput{ID: a.ID, PrevID: b.ID}); err != nil {
		t.Fatalf("Move: %v", err)
	}
	tasks, _ := svc.List(domain.Filter{Order: domain.OrderManual})
	if len(tasks) != 2 || tasks[0].ID != b.ID || tasks[1].ID != a.ID {
		t.Fatalf("order = %+v", tasks)
	}
}

func TestMoveRecoversFromDenseKeys(t *testing.T) {
	// Two neighbours with nothing between them: the service normalises the group
	// and retries instead of failing the user's drag.
	svc, st, clk := newService(t)
	a, _ := svc.Create(app.CreateInput{Title: "a"})
	b, _ := svc.Create(app.CreateInput{Title: "b"})
	if err := a.SetSortKey("0", clk.Now()); err != nil {
		t.Fatal(err)
	}
	if err := st.Tasks().Update(a); err != nil {
		t.Fatal(err)
	}
	first, err := domain.RankBetween("", "0")
	if err == nil {
		t.Fatalf("the fixture is wrong: %q was placeable", first)
	}
	// Insert a third task at the head: it must succeed through renumbering.
	c, err := svc.Create(app.CreateInput{Title: "c"})
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.Move(app.MoveInput{ID: c.ID, NextID: a.ID}); err != nil {
		t.Fatalf("Move must recover from a dense key: %v", err)
	}
	tasks, _ := svc.List(domain.Filter{Order: domain.OrderManual})
	if len(tasks) != 3 || tasks[0].ID != c.ID {
		t.Fatalf("order = %+v", tasks)
	}
	_ = b
}

func TestTodayAndHistoryPartitionTheData(t *testing.T) {
	svc, _, _ := newService(t)
	day := shared.MustParseLocalDate("2026-11-17")
	today, _ := svc.Create(app.CreateInput{Title: "今天", DueDate: day})
	tomorrow, _ := svc.Create(app.CreateInput{Title: "明天", DueDate: day.AddDays(1)})
	undated, _ := svc.Create(app.CreateInput{Title: "未排期"})

	todays, err := svc.Today(day)
	if err != nil {
		t.Fatal(err)
	}
	if len(todays) != 1 || todays[0].ID != today.ID {
		t.Fatalf("today view = %+v", todays)
	}
	history, err := svc.History(day)
	if err != nil {
		t.Fatal(err)
	}
	if len(history) != 2 {
		t.Fatalf("history view = %+v", history)
	}
	seen := map[shared.ID]bool{}
	for _, h := range history {
		seen[h.ID] = true
	}
	if !seen[tomorrow.ID] || !seen[undated.ID] {
		t.Fatalf("history must hold the future and undated tasks: %+v", history)
	}
}

func TestHistoryGroupsOrderAndUndatedLast(t *testing.T) {
	svc, _, _ := newService(t)
	day := shared.MustParseLocalDate("2026-11-17")
	_, _ = svc.Create(app.CreateInput{Title: "昨天", DueDate: day.AddDays(-1)})
	_, _ = svc.Create(app.CreateInput{Title: "明天", DueDate: day.AddDays(1)})
	_, _ = svc.Create(app.CreateInput{Title: "未排期"})
	_, _ = svc.Create(app.CreateInput{Title: "今天", DueDate: day})

	groups, err := svc.HistoryGroups(day)
	if err != nil {
		t.Fatalf("HistoryGroups: %v", err)
	}
	if len(groups) != 3 {
		t.Fatalf("want 3 groups, got %d: %+v", len(groups), groups)
	}
	if !groups[0].Day.Equal(day.AddDays(1)) {
		t.Fatalf("the newest date must come first, got %v", groups[0].Day)
	}
	if !groups[1].Day.Equal(day.AddDays(-1)) {
		t.Fatalf("second group = %v", groups[1].Day)
	}
	if !groups[2].Day.IsZero() {
		t.Fatalf("the undated group must be last, got %v", groups[2].Day)
	}
}

func TestOverdueCountIgnoresDoneTasks(t *testing.T) {
	svc, _, _ := newService(t)
	yesterday := shared.MustParseLocalDate("2026-11-16")
	late, _ := svc.Create(app.CreateInput{Title: "逾期", DueDate: yesterday})
	if n, err := svc.OverdueCount(); err != nil || n != 1 {
		t.Fatalf("overdue = %d, %v", n, err)
	}
	if _, err := svc.Complete(late.ID); err != nil {
		t.Fatal(err)
	}
	if n, err := svc.OverdueCount(); err != nil || n != 0 {
		t.Fatalf("a done task is not overdue: %d, %v", n, err)
	}
}

func TestOpenCountAndStartDoing(t *testing.T) {
	svc, _, _ := newService(t)
	tk, _ := svc.Create(app.CreateInput{Title: "x"})
	if n, err := svc.OpenCount(); err != nil || n != 1 {
		t.Fatalf("open = %d, %v", n, err)
	}
	doing, err := svc.StartDoing(tk.ID)
	if err != nil {
		t.Fatalf("StartDoing: %v", err)
	}
	if doing.Status != domain.StatusDoing {
		t.Fatalf("status = %v", doing.Status)
	}
	if _, err := svc.Complete(tk.ID); err != nil {
		t.Fatal(err)
	}
	if n, err := svc.OpenCount(); err != nil || n != 0 {
		t.Fatalf("open = %d, %v", n, err)
	}
}

func TestRestoreBringsBackArchivedTask(t *testing.T) {
	svc, st, _ := newService(t)
	now := harness.At("2026-11-17 09:00:00")
	generated, err := domain.NewFromTemplate(
		"00000000000000000000000060",
		domain.FromTemplateSpec{Title: "模板任务"},
		domain.TemplateOrigin{TemplateID: "00000000000000000000000061", Day: shared.DateOf(now), Seq: 1},
		now,
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.Tasks().Create(generated); err != nil {
		t.Fatal(err)
	}
	if err := svc.Delete(generated.ID); err != nil {
		t.Fatal(err)
	}
	restored, err := svc.Restore(generated.ID)
	if err != nil {
		t.Fatalf("Restore: %v", err)
	}
	if restored.Status != domain.StatusTodo {
		t.Fatalf("status = %v", restored.Status)
	}
	if !restored.IsFromTemplate() {
		t.Fatal("restoring must keep the occurrence key")
	}
}

// focusSession seeds a finished session that references the task, so the delete
// guard has something to find.
func focusSession(st *store.Store, taskID shared.ID, now time.Time) (shared.ID, error) {
	session, err := focus.Start("00000000000000000000000077", taskID, focus.KindFocus, 25*time.Minute, now)
	if err != nil {
		return "", err
	}
	if err := session.Complete(now.Add(25 * time.Minute)); err != nil {
		return "", err
	}
	if err := st.Sessions().Create(session); err != nil {
		return "", err
	}
	return session.ID, nil
}

func TestGetReturnsTheStoredTask(t *testing.T) {
	svc, _, _ := newService(t)
	created, err := svc.Create(app.CreateInput{Title: "写方案"})
	if err != nil {
		t.Fatal(err)
	}
	got, err := svc.Get(created.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.ID != created.ID || got.Title != "写方案" {
		t.Fatalf("task = %+v", got)
	}
	if _, err := svc.Get(shared.ID("00000000000000000000000099")); !shared.IsKind(err, shared.KindNotFound) {
		t.Fatalf("a missing task must be not-found, got %v", err)
	}
}

func TestUpdateRejectsAnOutOfRangePriority(t *testing.T) {
	// The range is checked in the service, before the aggregate is touched, so the
	// stored row keeps its old value.
	svc, _, _ := newService(t)
	created, err := svc.Create(app.CreateInput{Title: "写方案", Priority: 2})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Update(app.UpdateInput{ID: created.ID, Title: "写方案", Priority: 9}); !shared.IsKind(err, shared.KindInvalid) {
		t.Fatalf("priority 9 must be invalid, got %v", err)
	}
	if _, err := svc.Update(app.UpdateInput{ID: created.ID, Title: "写方案", Priority: -1}); !shared.IsKind(err, shared.KindInvalid) {
		t.Fatalf("priority -1 must be invalid, got %v", err)
	}
	stored, err := svc.Get(created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Priority != 2 {
		t.Fatalf("a rejected update must not write: %+v", stored)
	}
	if _, err := svc.Update(app.UpdateInput{ID: shared.ID("00000000000000000000000099"), Title: "x"}); !shared.IsKind(err, shared.KindNotFound) {
		t.Fatalf("updating a missing task must be not-found, got %v", err)
	}
}

func TestMoveRejectsAnUnknownNeighbour(t *testing.T) {
	svc, _, _ := newService(t)
	a, err := svc.Create(app.CreateInput{Title: "a"})
	if err != nil {
		t.Fatal(err)
	}
	missing := shared.ID("00000000000000000000000099")
	if err := svc.Move(app.MoveInput{ID: a.ID, NextID: missing}); !shared.IsKind(err, shared.KindNotFound) {
		t.Fatalf("an unknown next neighbour must be not-found, got %v", err)
	}
	if err := svc.Move(app.MoveInput{ID: a.ID, PrevID: missing}); !shared.IsKind(err, shared.KindNotFound) {
		t.Fatalf("an unknown prev neighbour must be not-found, got %v", err)
	}
	if err := svc.Move(app.MoveInput{ID: missing, PrevID: a.ID}); !shared.IsKind(err, shared.KindNotFound) {
		t.Fatalf("moving a missing task must be not-found, got %v", err)
	}
}

func TestMoveRenumbersSubtaskGroupsSeparately(t *testing.T) {
	// A dense parent group must be renumbered without touching the root group: the
	// two are separate sort scopes, and renumbering across them would scramble the
	// root order the user just arranged.
	svc, st, clk := newService(t)
	parent, err := svc.Create(app.CreateInput{Title: "父"})
	if err != nil {
		t.Fatal(err)
	}
	rootA, _ := svc.Create(app.CreateInput{Title: "根 A"})
	rootB, _ := svc.Create(app.CreateInput{Title: "根 B"})
	rootKeys := map[shared.ID]string{rootA.ID: rootA.SortKey, rootB.ID: rootB.SortKey}

	childA, err := svc.Create(app.CreateInput{Title: "子 A", ParentID: parent.ID})
	if err != nil {
		t.Fatal(err)
	}
	childB, err := svc.Create(app.CreateInput{Title: "子 B", ParentID: parent.ID})
	if err != nil {
		t.Fatal(err)
	}
	// Squeeze the two existing children into adjacent keys so nothing fits between
	// them: that is the only state that reaches the renumbering path.
	for _, pair := range []struct {
		tk  *domain.Task
		key string
	}{{childA, "0"}, {childB, "1"}} {
		if err := pair.tk.SetSortKey(pair.key, clk.Now()); err != nil {
			t.Fatal(err)
		}
		if err := st.Tasks().Update(pair.tk); err != nil {
			t.Fatal(err)
		}
	}
	childC, err := svc.Create(app.CreateInput{Title: "子 C", ParentID: parent.ID})
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.Move(app.MoveInput{ID: childC.ID, PrevID: childA.ID, NextID: childB.ID}); err != nil {
		t.Fatalf("Move in a dense child group: %v", err)
	}
	children, err := svc.Children(parent.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(children) != 3 {
		t.Fatalf("children = %+v", children)
	}
	order := map[shared.ID]bool{}
	for _, c := range children {
		order[c.ID] = true
	}
	for _, id := range []shared.ID{childA.ID, childB.ID, childC.ID} {
		if !order[id] {
			t.Fatalf("child %v vanished", id)
		}
	}
	roots, err := svc.List(domain.Filter{Order: domain.OrderManual})
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range roots {
		if r.ParentID != "" {
			continue
		}
		if want, ok := rootKeys[r.ID]; ok && r.SortKey != want {
			t.Fatalf("root %q sort key changed: %q -> %q", r.Title, want, r.SortKey)
		}
	}
}

// The estimate and the length of one pomodoro are separate answers: "four of them"
// and "ten minutes each". Both travel through create and update.
func TestCreateAndUpdateCarryThePomodoroLength(t *testing.T) {
	f, _, _ := newService(t)
	created, err := f.Create(app.CreateInput{Title: "写周报", FocusMinutes: 10})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if created.FocusMinutes != 10 {
		t.Fatalf("FocusMinutes = %d, want 10", created.FocusMinutes)
	}

	updated, err := f.Update(app.UpdateInput{ID: created.ID, Title: "写周报", FocusMinutes: 45})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if updated.FocusMinutes != 45 {
		t.Fatalf("FocusMinutes = %d after the edit, want 45", updated.FocusMinutes)
	}
	stored, err := f.Get(created.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if stored.FocusMinutes != 45 {
		t.Fatalf("the stored task carries %d, want 45", stored.FocusMinutes)
	}

	// An empty field is "the default", so the update above must be reversible.
	cleared, err := f.Update(app.UpdateInput{ID: created.ID, Title: "写周报"})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if cleared.FocusMinutes != 0 {
		t.Fatalf("FocusMinutes = %d after clearing, want 0", cleared.FocusMinutes)
	}
	if _, err := f.Create(app.CreateInput{Title: "太长", FocusMinutes: domain.FocusMinutesMax + 1}); !shared.IsKind(err, shared.KindInvalid) {
		t.Fatalf("an out-of-range length must be refused, got %v", err)
	}
}
