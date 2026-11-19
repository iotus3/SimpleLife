package sqlite_test

import (
	"errors"
	"testing"
	"time"

	"github.com/iotus3/simplelife/app/internal/domain/shared"
	task "github.com/iotus3/simplelife/app/internal/domain/task"
	"github.com/iotus3/simplelife/app/internal/infrastructure/persistence/sqlite"
)

func TestTaskRoundTrip(t *testing.T) {
	db := openFile(t)
	now := clock().Now()
	tk := sampleTask(t, "0000000000TASK0000000001", "写方案", shared.MustParseLocalDate("2026-11-17"), now)
	if err := db.Tasks().Create(tk); err != nil {
		t.Fatalf("Create: %v", err)
	}
	got, err := db.Tasks().Get(tk.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Title != tk.Title || got.Status != tk.Status || got.SortKey != tk.SortKey {
		t.Fatalf("task = %+v", got)
	}
	if !got.CreatedAt.Equal(tk.CreatedAt) || !got.UpdatedAt.Equal(tk.UpdatedAt) {
		t.Fatalf("timestamps lost: %+v", got)
	}
	// Millisecond precision is the storage resolution; nothing may be truncated.
	if got.CreatedAt.Nanosecond()/int(time.Millisecond) != tk.CreatedAt.Nanosecond()/int(time.Millisecond) {
		t.Fatalf("sub-millisecond drift: %v vs %v", got.CreatedAt, tk.CreatedAt)
	}
	if !got.DueAt.Equal(tk.DueAt) {
		t.Fatalf("due = %v, want %v", got.DueAt, tk.DueAt)
	}
	if len(got.Tags) != 1 || got.Tags[0] != "work" {
		t.Fatalf("tags = %v", got.Tags)
	}
	if !got.CompletedAt.IsZero() || !got.OriginDate.IsZero() || got.OriginTemplateID != "" {
		t.Fatalf("unset fields must stay unset: %+v", got)
	}
}

func TestTaskUpdateAndDelete(t *testing.T) {
	db := openFile(t)
	clk := clock()
	tk := sampleTask(t, "0000000000TASK0000000002", "写方案", shared.MustParseLocalDate("2026-11-17"), clk.Now())
	if err := db.Tasks().Create(tk); err != nil {
		t.Fatal(err)
	}

	clk.Advance(time.Hour)
	if err := tk.SetNotes("细节", clk.Now()); err != nil {
		t.Fatal(err)
	}
	if err := tk.SetPriority(2, clk.Now()); err != nil {
		t.Fatal(err)
	}
	if err := tk.SetEstimate(3, clk.Now()); err != nil {
		t.Fatal(err)
	}
	// The counter only accumulates while the task is still open, so it comes first.
	if err := tk.RecordPomodoro(clk.Now()); err != nil {
		t.Fatal(err)
	}
	if err := tk.Complete(clk.Now()); err != nil {
		t.Fatal(err)
	}
	if err := db.Tasks().Update(tk); err != nil {
		t.Fatalf("Update: %v", err)
	}
	got, err := db.Tasks().Get(tk.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Notes != "细节" || got.Priority != 2 || got.EstimatePomodoros != 3 || got.DonePomodoros != 1 {
		t.Fatalf("task = %+v", got)
	}
	if got.Status != task.StatusDone || got.CompletedAt.IsZero() {
		t.Fatalf("completion lost: %+v", got)
	}

	if err := db.Tasks().Delete(tk.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := db.Tasks().Get(tk.ID); !shared.IsKind(err, shared.KindNotFound) {
		t.Fatalf("a deleted task must be not-found, got %v", err)
	}
	if err := db.Tasks().Delete(tk.ID); !shared.IsKind(err, shared.KindNotFound) {
		t.Fatalf("deleting twice must be not-found, got %v", err)
	}
	if err := db.Tasks().Update(tk); !shared.IsKind(err, shared.KindNotFound) {
		t.Fatalf("updating a deleted task must be not-found, got %v", err)
	}
}

func TestTaskListFilters(t *testing.T) {
	db := openFile(t)
	clk := clock()
	now := clk.Now()
	day := shared.MustParseLocalDate("2026-11-17")

	todayTask := sampleTask(t, "0000000000TASK0000000011", "今天的", day, now)
	pastTask := sampleTask(t, "0000000000TASK0000000012", "昨天的", day.AddDays(-1), now)
	futureTask := sampleTask(t, "0000000000TASK0000000013", "明天的", day.AddDays(1), now)
	undated := sampleTask(t, "0000000000TASK0000000014", "未排期", shared.LocalDate{}, now)
	if err := undated.SetNotes("含 Note 关键字", now); err != nil {
		t.Fatal(err)
	}
	archived := sampleTask(t, "0000000000TASK0000000015", "归档的", day, now)
	if err := archived.Archive(now); err != nil {
		t.Fatal(err)
	}
	for _, tk := range []*task.Task{todayTask, pastTask, futureTask, undated, archived} {
		if err := db.Tasks().Create(tk); err != nil {
			t.Fatalf("Create %q: %v", tk.Title, err)
		}
	}

	t.Run("today", func(t *testing.T) {
		got, err := db.Tasks().List(task.Today(day))
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 1 || got[0].ID != todayTask.ID {
			t.Fatalf("today = %+v", titles(got))
		}
	})
	t.Run("history", func(t *testing.T) {
		got, err := db.Tasks().List(task.History(day))
		if err != nil {
			t.Fatal(err)
		}
		// Past, future and undated — but not today's task and not the archived one.
		if len(got) != 3 {
			t.Fatalf("history = %v", titles(got))
		}
		for _, tk := range got {
			if tk.ID == todayTask.ID || tk.ID == archived.ID {
				t.Fatalf("history leaked %q", tk.Title)
			}
		}
	})
	t.Run("overdue", func(t *testing.T) {
		got, err := db.Tasks().List(task.Overdue(now))
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 1 || got[0].ID != pastTask.ID {
			t.Fatalf("overdue = %v", titles(got))
		}
		n, err := db.Tasks().Count(task.Overdue(now))
		if err != nil || n != 1 {
			t.Fatalf("count = %d, %v", n, err)
		}
	})
	t.Run("statuses", func(t *testing.T) {
		got, err := db.Tasks().List(task.Filter{Statuses: []task.Status{task.StatusArchived}, IncludeArchived: true})
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 1 || got[0].ID != archived.ID {
			t.Fatalf("archived = %v", titles(got))
		}
	})
	t.Run("include archived", func(t *testing.T) {
		got, err := db.Tasks().List(task.Filter{IncludeArchived: true})
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 5 {
			t.Fatalf("all = %v", titles(got))
		}
	})
	t.Run("undated only", func(t *testing.T) {
		no := false
		got, err := db.Tasks().List(task.Filter{HasDue: &no})
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 1 || got[0].ID != undated.ID {
			t.Fatalf("undated = %v", titles(got))
		}
		yes := true
		got, err = db.Tasks().List(task.Filter{HasDue: &yes})
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 3 {
			t.Fatalf("dated = %v", titles(got))
		}
	})
	t.Run("search is case insensitive over title and notes", func(t *testing.T) {
		got, err := db.Tasks().List(task.Filter{Search: "note"})
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 1 || got[0].ID != undated.ID {
			t.Fatalf("notes search = %v", titles(got))
		}
		got, err = db.Tasks().List(task.Filter{Search: "未排"})
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 1 {
			t.Fatalf("title search = %v", titles(got))
		}
	})
	t.Run("search escapes wildcards", func(t *testing.T) {
		pct := sampleTask(t, "0000000000TASK0000000016", "省下 50% 预算", shared.LocalDate{}, now)
		if err := db.Tasks().Create(pct); err != nil {
			t.Fatal(err)
		}
		got, err := db.Tasks().List(task.Filter{Search: "50%"})
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 1 || got[0].ID != pct.ID {
			t.Fatalf("literal %% search = %v", titles(got))
		}
		got, err = db.Tasks().List(task.Filter{Search: "%"})
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 1 {
			t.Fatalf("a bare %% must match only the literal: %v", titles(got))
		}
	})
	t.Run("tag", func(t *testing.T) {
		got, err := db.Tasks().List(task.Filter{Tag: "work"})
		if err != nil {
			t.Fatal(err)
		}
		// Every sample task carries the tag; the archived one is excluded silently.
		if len(got) != 5 {
			t.Fatalf("tag = %v", titles(got))
		}
		got, err = db.Tasks().List(task.Filter{Tag: "missing"})
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 0 {
			t.Fatalf("unknown tag = %v", titles(got))
		}
	})
	t.Run("paging", func(t *testing.T) {
		all, err := db.Tasks().List(task.Filter{Order: task.OrderCreated})
		if err != nil {
			t.Fatal(err)
		}
		page, err := db.Tasks().List(task.Filter{Order: task.OrderCreated, Limit: 2, Offset: 1})
		if err != nil {
			t.Fatal(err)
		}
		if len(page) != 2 || page[0].ID != all[1].ID || page[1].ID != all[2].ID {
			t.Fatalf("page = %v", titles(page))
		}
		if rest, err := db.Tasks().List(task.Filter{Offset: 99}); err != nil || len(rest) != 0 {
			t.Fatalf("offset past the end = %v, %v", titles(rest), err)
		}
	})
	t.Run("count open", func(t *testing.T) {
		n, err := db.Tasks().CountOpen()
		if err != nil || n != 5 {
			t.Fatalf("open = %d, %v", n, err)
		}
	})
}

func TestTaskOrdering(t *testing.T) {
	db := openFile(t)
	clk := clock()
	now := clk.Now()
	day := shared.MustParseLocalDate("2026-11-17")

	// Created in an order that differs from every sort order, so a bug cannot pass
	// by accident.
	first := sampleTask(t, "0000000000TASK0000000021", "b-later", day.AddDays(1), now)
	clk.Advance(time.Minute)
	second := sampleTask(t, "0000000000TASK0000000022", "a-sooner", day, clk.Now())
	clk.Advance(time.Minute)
	third := sampleTask(t, "0000000000TASK0000000023", "c-undated", shared.LocalDate{}, clk.Now())
	if err := second.SetPriority(1, clk.Now()); err != nil {
		t.Fatal(err)
	}
	if err := third.SetPriority(3, clk.Now()); err != nil {
		t.Fatal(err)
	}
	for _, tk := range []*task.Task{first, second, third} {
		if err := db.Tasks().Create(tk); err != nil {
			t.Fatal(err)
		}
	}

	manual, err := db.Tasks().List(task.Filter{Order: task.OrderManual})
	if err != nil {
		t.Fatal(err)
	}
	want := []shared.ID{second.ID, first.ID, third.ID} // a-sooner < b-later < c-undated
	assertOrder(t, "manual", manual, want)

	byDue, err := db.Tasks().List(task.Filter{Order: task.OrderDue})
	if err != nil {
		t.Fatal(err)
	}
	assertOrder(t, "due", byDue, []shared.ID{second.ID, first.ID, third.ID})

	byPriority, err := db.Tasks().List(task.Filter{Order: task.OrderPriority})
	if err != nil {
		t.Fatal(err)
	}
	assertOrder(t, "priority", byPriority, []shared.ID{second.ID, third.ID, first.ID})

	byCreated, err := db.Tasks().List(task.Filter{Order: task.OrderCreated})
	if err != nil {
		t.Fatal(err)
	}
	assertOrder(t, "created", byCreated, []shared.ID{third.ID, second.ID, first.ID})
}

func TestTaskOccurrenceIsUniqueInTheDatabase(t *testing.T) {
	// The expander's fast path can be wrong (a race, a crash between check and
	// insert); the unique index is what actually holds the invariant.
	db := openFile(t)
	now := clock().Now()
	day := shared.MustParseLocalDate("2026-11-17")
	origin := task.TemplateOrigin{TemplateID: "0000000000TMPL0000000001", Day: day, Seq: 0}

	first, err := task.NewFromTemplate("0000000000TASK0000000031", task.FromTemplateSpec{Title: "A"}, origin, now)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Tasks().Create(first); err != nil {
		t.Fatalf("Create: %v", err)
	}
	second, err := task.NewFromTemplate("0000000000TASK0000000032", task.FromTemplateSpec{Title: "A"}, origin, now)
	if err != nil {
		t.Fatal(err)
	}
	err = db.Tasks().Create(second)
	if !errors.Is(err, shared.ErrDuplicate) {
		t.Fatalf("a duplicate occurrence must be a duplicate error, got %v", err)
	}

	exists, err := db.Tasks().ExistsOccurrence(shared.ID(origin.TemplateID), day, 0)
	if err != nil || !exists {
		t.Fatalf("ExistsOccurrence = %v, %v", exists, err)
	}
	if exists, err := db.Tasks().ExistsOccurrence(shared.ID(origin.TemplateID), day.AddDays(1), 0); err != nil || exists {
		t.Fatalf("another day must not exist: %v, %v", exists, err)
	}

	// Two ordinary tasks never collide, even with the same (empty) origin.
	for i, id := range []shared.ID{"0000000000TASK0000000033", "0000000000TASK0000000034"} {
		tk := sampleTask(t, id, "普通任务", day, now)
		if err := db.Tasks().Create(tk); err != nil {
			t.Fatalf("ordinary task %d: %v", i, err)
		}
	}
}

func TestTaskParentAndFocusHistory(t *testing.T) {
	db := openFile(t)
	clk := clock()
	now := clk.Now()
	parent := sampleTask(t, "0000000000TASK0000000041", "父任务", shared.LocalDate{}, now)
	child := sampleTask(t, "0000000000TASK0000000042", "子任务", shared.LocalDate{}, now)
	if err := child.AttachParent(parent, now); err != nil {
		t.Fatal(err)
	}
	for _, tk := range []*task.Task{parent, child} {
		if err := db.Tasks().Create(tk); err != nil {
			t.Fatal(err)
		}
	}

	children, err := db.Tasks().List(task.Children(parent.ID))
	if err != nil {
		t.Fatal(err)
	}
	if len(children) != 1 || children[0].ID != child.ID {
		t.Fatalf("children = %v", titles(children))
	}

	topLevel, err := db.Tasks().List(task.Filter{ParentID: ptrID("")})
	if err != nil {
		t.Fatal(err)
	}
	if len(topLevel) != 1 || topLevel[0].ID != parent.ID {
		t.Fatalf("top level = %v", titles(topLevel))
	}

	if has, err := db.Tasks().HasFocusHistory(parent.ID); err != nil || has {
		t.Fatalf("no session yet: %v, %v", has, err)
	}
	session := sampleSession(t, "0000000000SESS0000000001", parent.ID, now, 25*time.Minute)
	if err := db.Sessions().Create(session); err != nil {
		t.Fatal(err)
	}
	if has, err := db.Tasks().HasFocusHistory(parent.ID); err != nil || !has {
		t.Fatalf("after a session: %v, %v", has, err)
	}
}

func TestTaskCreateRejectsADuplicateID(t *testing.T) {
	db := openFile(t)
	now := clock().Now()
	tk := sampleTask(t, "0000000000TASK0000000051", "写方案", shared.LocalDate{}, now)
	if err := db.Tasks().Create(tk); err != nil {
		t.Fatal(err)
	}
	if err := db.Tasks().Create(tk); !errors.Is(err, shared.ErrDuplicate) {
		t.Fatalf("a duplicate id must be reported, got %v", err)
	}
}

func TestTaskGetMissing(t *testing.T) {
	db := openMemory(t)
	if _, err := db.Tasks().Get("0000000000TASK0000000099"); !shared.IsKind(err, shared.KindNotFound) {
		t.Fatalf("a missing task must be not-found, got %v", err)
	}
}

func ptrID(s string) *shared.ID {
	id := shared.ID(s)
	return &id
}

func titles(tasks []*task.Task) []string {
	out := make([]string, len(tasks))
	for i, tk := range tasks {
		out[i] = tk.Title
	}
	return out
}

func assertOrder(t *testing.T, label string, got []*task.Task, want []shared.ID) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s: got %d tasks, want %d", label, len(got), len(want))
	}
	for i := range want {
		if got[i].ID != want[i] {
			t.Fatalf("%s: position %d = %q, want %q (order = %v)", label, i, got[i].Title, want[i], titles(got))
		}
	}
}

var _ = sqlite.SchemaVersion

// The per-task pomodoro length has its own column (migration 4). It must survive a
// write and a read, and a row written before the column existed reads as zero —
// which the page renders as "the focus page's length".
func TestTaskFocusMinutesRoundTrip(t *testing.T) {
	db := openFile(t)
	now := clock().Now()
	tk := sampleTask(t, "0000000000TASK00000000F1", "写方案", shared.MustParseLocalDate("2026-11-17"), now)
	if err := tk.SetFocusMinutes(10, now); err != nil {
		t.Fatalf("SetFocusMinutes: %v", err)
	}
	if err := db.Tasks().Create(tk); err != nil {
		t.Fatalf("Create: %v", err)
	}
	got, err := db.Tasks().Get(tk.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.FocusMinutes != 10 {
		t.Fatalf("FocusMinutes = %d after the round trip, want 10", got.FocusMinutes)
	}

	if err := tk.SetFocusMinutes(0, now); err != nil {
		t.Fatalf("clear: %v", err)
	}
	if err := db.Tasks().Update(tk); err != nil {
		t.Fatalf("Update: %v", err)
	}
	got, err = db.Tasks().Get(tk.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.FocusMinutes != 0 {
		t.Fatalf("FocusMinutes = %d after clearing, want 0", got.FocusMinutes)
	}

	// A row inserted without the column (as the migration leaves an old file) reads
	// as the default rather than as an error.
	other := sampleTask(t, "0000000000TASK00000000F2", "旧任务", shared.MustParseLocalDate("2026-11-17"), now)
	if err := db.Tasks().Create(other); err != nil {
		t.Fatalf("Create: %v", err)
	}
	raw, err := db.Tasks().Get(other.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if raw.FocusMinutes != 0 {
		t.Fatalf("a task with no length reads %d, want 0", raw.FocusMinutes)
	}
}
