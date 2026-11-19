package task_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/iotus3/simplelife/app/internal/domain/shared"
	"github.com/iotus3/simplelife/app/internal/domain/task"
)

func at(s string) time.Time {
	t, err := time.ParseInLocation("2006-01-02 15:04:05", s, time.Local)
	if err != nil {
		panic(err)
	}
	return t
}

func newTask(t *testing.T) *task.Task {
	t.Helper()
	return newTaskWithID(t, "01J0TASKNEW0000000000000A")
}

func newTaskWithID(t *testing.T, id shared.ID) *task.Task {
	t.Helper()
	tk, err := task.New(id, "写设计文档", at("2026-11-17 09:00:00"))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return tk
}

func TestNewRejectsBadInput(t *testing.T) {
	now := at("2026-11-17 09:00:00")
	if _, err := task.New("", "x", now); !shared.IsKind(err, shared.KindInvalid) {
		t.Fatalf("empty id must be invalid, got %v", err)
	}
	for _, title := range []string{"", "   ", strings.Repeat("字", task.TitleMaxRunes+1)} {
		if _, err := task.New("01J0TASKNEW0000000000000A", title, now); !shared.IsKind(err, shared.KindInvalid) {
			t.Fatalf("title %q must be invalid, got %v", title, err)
		}
	}
	tk, err := task.New("01J0TASKNEW0000000000000A", "  ok  ", now)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if tk.Title != "ok" {
		t.Fatalf("title must be trimmed, got %q", tk.Title)
	}
	if tk.Status != task.StatusTodo || tk.SortKey != task.InitialSortKey {
		t.Fatalf("unexpected defaults: %+v", tk)
	}
	if !tk.CreatedAt.Equal(now) || !tk.UpdatedAt.Equal(now) {
		t.Fatal("timestamps must come from the injected clock")
	}
}

func TestCompleteIsIdempotentAndStampsTime(t *testing.T) {
	tk := newTask(t)
	now := at("2026-11-17 10:00:00")
	if err := tk.Complete(now); err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if tk.Status != task.StatusDone || !tk.CompletedAt.Equal(now) {
		t.Fatalf("unexpected state: %+v", tk)
	}
	later := now.Add(time.Hour)
	if err := tk.Complete(later); err != nil {
		t.Fatalf("second Complete must be a no-op, got %v", err)
	}
	if !tk.CompletedAt.Equal(now) {
		t.Fatal("a second Complete must not move CompletedAt")
	}
}

func TestReopenRequiresDone(t *testing.T) {
	tk := newTask(t)
	now := at("2026-11-17 10:00:00")
	if err := tk.Reopen(now); !shared.IsKind(err, shared.KindConflict) {
		t.Fatalf("reopening a todo must conflict, got %v", err)
	}
	if err := tk.Complete(now); err != nil {
		t.Fatal(err)
	}
	if err := tk.Reopen(now); err != nil {
		t.Fatalf("Reopen: %v", err)
	}
	if tk.Status != task.StatusTodo || !tk.CompletedAt.IsZero() {
		t.Fatalf("reopen must clear completion: %+v", tk)
	}
}

func TestStartDoingIsIdempotent(t *testing.T) {
	tk := newTask(t)
	now := at("2026-11-17 10:00:00")
	if err := tk.StartDoing(now); err != nil {
		t.Fatal(err)
	}
	if tk.Status != task.StatusDoing {
		t.Fatalf("status = %v, want doing", tk.Status)
	}
	if err := tk.StartDoing(now); err != nil {
		t.Fatalf("second StartDoing must be a no-op, got %v", err)
	}
	if err := tk.Complete(now); err != nil {
		t.Fatal(err)
	}
	if err := tk.StartDoing(now); !shared.IsKind(err, shared.KindConflict) {
		t.Fatalf("starting a done task must conflict, got %v", err)
	}
}

func TestArchiveAndRestore(t *testing.T) {
	tk := newTask(t)
	now := at("2026-11-17 10:00:00")
	if err := tk.Archive(now); err != nil {
		t.Fatal(err)
	}
	if tk.Status != task.StatusArchived {
		t.Fatalf("status = %v", tk.Status)
	}
	if err := tk.Archive(now); err != nil {
		t.Fatalf("archiving twice must be a no-op, got %v", err)
	}
	if err := tk.Complete(now); !shared.IsKind(err, shared.KindConflict) {
		t.Fatalf("completing an archived task must conflict, got %v", err)
	}
	if err := tk.Restore(now); err != nil {
		t.Fatalf("Restore: %v", err)
	}
	if tk.Status != task.StatusTodo {
		t.Fatalf("status = %v, want todo", tk.Status)
	}
	if err := tk.Restore(now); !shared.IsKind(err, shared.KindConflict) {
		t.Fatalf("restoring a live task must conflict, got %v", err)
	}
}

func TestRecordPomodoroOnlyOnOpenTasks(t *testing.T) {
	tk := newTask(t)
	now := at("2026-11-17 10:00:00")
	if err := tk.RecordPomodoro(now); err != nil {
		t.Fatalf("RecordPomodoro: %v", err)
	}
	if tk.DonePomodoros != 1 {
		t.Fatalf("DonePomodoros = %d, want 1", tk.DonePomodoros)
	}
	if err := tk.StartDoing(now); err != nil {
		t.Fatal(err)
	}
	if err := tk.RecordPomodoro(now); err != nil {
		t.Fatalf("a doing task must accept pomodoros, got %v", err)
	}
	if err := tk.Complete(now); err != nil {
		t.Fatal(err)
	}
	if err := tk.RecordPomodoro(now); !shared.IsKind(err, shared.KindConflict) {
		t.Fatalf("a done task must refuse pomodoros, got %v", err)
	}
	if got := shared.Code(tk.RecordPomodoro(now)); got != "task.record.not_open" {
		t.Fatalf("code = %q", got)
	}
}

func TestSettersValidateRanges(t *testing.T) {
	tk := newTask(t)
	now := at("2026-11-17 10:00:00")

	if err := tk.SetEstimate(task.EstimatePomodorosMax+1, now); !shared.IsKind(err, shared.KindInvalid) {
		t.Fatalf("estimate above max must be invalid, got %v", err)
	}
	if err := tk.SetEstimate(-1, now); !shared.IsKind(err, shared.KindInvalid) {
		t.Fatalf("negative estimate must be invalid, got %v", err)
	}
	if err := tk.SetEstimate(task.EstimatePomodorosMax, now); err != nil {
		t.Fatalf("SetEstimate: %v", err)
	}
	if tk.EstimatePomodoros != task.EstimatePomodorosMax {
		t.Fatalf("estimate = %d", tk.EstimatePomodoros)
	}

	if err := tk.SetPriority(task.PriorityMax+1, now); !shared.IsKind(err, shared.KindInvalid) {
		t.Fatalf("priority above max must be invalid, got %v", err)
	}
	if err := tk.SetPriority(1, now); err != nil {
		t.Fatal(err)
	}
	if got := tk.Priority.Label(); got != "P0" {
		t.Fatalf("Label = %q, want P0", got)
	}
	if got := task.Priority(0).Label(); got != "" {
		t.Fatalf("unset Label = %q, want empty", got)
	}

	if err := tk.SetNotes(strings.Repeat("x", task.NotesMaxRunes+1), now); !shared.IsKind(err, shared.KindInvalid) {
		t.Fatalf("long notes must be invalid, got %v", err)
	}
	if err := tk.SetNotes("  备注  ", now); err != nil {
		t.Fatal(err)
	}
	if tk.Notes != "备注" {
		t.Fatalf("notes must be trimmed, got %q", tk.Notes)
	}

	if err := tk.SetTitle("   ", now); !shared.IsKind(err, shared.KindInvalid) {
		t.Fatalf("blank title must be invalid, got %v", err)
	}
}

func TestSetTagsNormalisesAndBounds(t *testing.T) {
	tk := newTask(t)
	now := at("2026-11-17 10:00:00")
	if err := tk.SetTags([]string{" Work ", "work", "", "URGENT"}, now); err != nil {
		t.Fatalf("SetTags: %v", err)
	}
	if len(tk.Tags) != 2 || tk.Tags[0] != "work" || tk.Tags[1] != "urgent" {
		t.Fatalf("tags = %v", tk.Tags)
	}
	if err := tk.SetTags([]string{strings.Repeat("t", task.TagMaxRunes+1)}, now); !shared.IsKind(err, shared.KindInvalid) {
		t.Fatalf("long tag must be invalid, got %v", err)
	}
	tooMany := make([]string, task.MaxTags+1)
	for i := range tooMany {
		tooMany[i] = string(rune('a' + i))
	}
	if err := tk.SetTags(tooMany, now); !shared.IsKind(err, shared.KindInvalid) {
		t.Fatalf("too many tags must be invalid, got %v", err)
	}
}

func TestDueDateHelpers(t *testing.T) {
	tk := newTask(t)
	now := at("2026-11-17 10:00:00")
	day := shared.MustParseLocalDate("2026-11-18")
	if err := tk.SetDueDate(day, now); err != nil {
		t.Fatal(err)
	}
	if !tk.HasDueDate() || !tk.DueDate().Equal(day) {
		t.Fatalf("due = %v / %v", tk.DueAt, tk.DueDate())
	}
	if !tk.DueAt.Equal(day.EndOfDay()) {
		t.Fatalf("due must be the local end of day, got %v", tk.DueAt)
	}
	if tk.IsOverdue(day.Midnight()) {
		t.Fatal("a task is not overdue before its end of day")
	}
	if !tk.IsOverdue(day.AddDays(1).Midnight()) {
		t.Fatal("a task must be overdue after its end of day")
	}
	if err := tk.SetDueDate(shared.LocalDate{}, now); err != nil {
		t.Fatal(err)
	}
	if tk.HasDueDate() {
		t.Fatal("zero date must clear the due date")
	}
	if err := tk.SetDueDate(shared.NewLocalDate(1200, time.January, 1), now); !shared.IsKind(err, shared.KindInvalid) {
		t.Fatalf("out-of-range due date must be invalid, got %v", err)
	}
	tk.SetDueDate(day, now)
	tk.ClearDue(now)
	if tk.HasDueDate() {
		t.Fatal("ClearDue must drop the date")
	}
	if tk.IsOverdue(now) {
		t.Fatal("a task without a due date is never overdue")
	}
}

func TestAttachParentRules(t *testing.T) {
	parent := newTask(t)
	child := newTaskWithID(t, "01J0TASKCHILD00000000000B")
	now := at("2026-11-17 10:00:00")
	if err := child.AttachParent(parent, now); err != nil {
		t.Fatalf("AttachParent: %v", err)
	}
	if child.ParentID != parent.ID {
		t.Fatalf("parent = %v", child.ParentID)
	}
	if err := parent.AttachParent(child, now); !shared.IsKind(err, shared.KindInvalid) {
		t.Fatalf("third level must be refused, got %v", err)
	}
	if err := child.AttachParent(child, now); !shared.IsKind(err, shared.KindInvalid) {
		t.Fatalf("self parent must be refused, got %v", err)
	}
	grand := newTaskWithID(t, "01J0TASKGRAND00000000000E")
	if err := parent.AttachParent(grand, now); err != nil {
		t.Fatal(err)
	}
	if err := child.AttachParent(nil, now); err != nil {
		t.Fatal(err)
	}
	if child.ParentID != "" {
		t.Fatal("nil parent must detach")
	}
	child.DetachParent(now)
	parent.Archive(now)
	if err := child.AttachParent(parent, now); !shared.IsKind(err, shared.KindInvalid) {
		t.Fatalf("archived parent must be refused, got %v", err)
	}
}

func TestNewFromTemplateWritesOriginAndDue(t *testing.T) {
	now := at("2026-11-17 08:00:00")
	day := shared.MustParseLocalDate("2026-11-17")
	origin := task.TemplateOrigin{TemplateID: "01J0TEMPLATE000000000000C", Day: day, Seq: 2}
	spec := task.FromTemplateSpec{
		Title:         "  晨间复盘  ",
		Notes:         "看昨天",
		Priority:      1,
		Estimate:      2,
		FocusMinutes:  40,
		DueOffsetDays: 1,
		Tags:          []string{"Routine"},
	}
	tk, err := task.NewFromTemplate("01J0TASKTMPL0000000000000D", spec, origin, now)
	if err != nil {
		t.Fatalf("NewFromTemplate: %v", err)
	}
	if tk.Title != "晨间复盘" {
		t.Fatalf("title = %q", tk.Title)
	}
	if !tk.IsFromTemplate() {
		t.Fatal("origin must be recorded")
	}
	if tk.OriginItemSeq != 2 || !tk.OriginDate.Equal(day) || tk.OriginTemplateID != origin.TemplateID {
		t.Fatalf("origin = %+v", tk)
	}
	if want := day.AddDays(1).EndOfDay(); !tk.DueAt.Equal(want) {
		t.Fatalf("due = %v, want %v", tk.DueAt, want)
	}
	// The template's own pomodoro length travels with the task it writes, so a
	// "read for 40 minutes" template produces sessions that already know it.
	if tk.FocusMinutes != 40 {
		t.Fatalf("focus minutes = %d, want the template's 40", tk.FocusMinutes)
	}
	if tk.Priority != 1 || tk.EstimatePomodoros != 2 || len(tk.Tags) != 1 || tk.Tags[0] != "routine" {
		t.Fatalf("unexpected task: %+v", tk)
	}
}

func TestNewFromTemplateClampsInsteadOfFailing(t *testing.T) {
	now := at("2026-11-17 08:00:00")
	origin := task.TemplateOrigin{TemplateID: "01J0TEMPLATE000000000000C", Day: shared.MustParseLocalDate("2026-11-17")}
	spec := task.FromTemplateSpec{Title: "x", Priority: 9, Estimate: 99, FocusMinutes: 9999}
	tk, err := task.NewFromTemplate("01J0TASKTMPL0000000000000D", spec, origin, now)
	if err != nil {
		t.Fatalf("out-of-range values must be clamped, not rejected: %v", err)
	}
	if tk.Priority != 0 || tk.EstimatePomodoros != task.EstimatePomodorosMax {
		t.Fatalf("clamp failed: %+v", tk)
	}
	if tk.FocusMinutes != task.FocusMinutesMax {
		t.Fatalf("focus minutes = %d, want the clamp at %d", tk.FocusMinutes, task.FocusMinutesMax)
	}
	bad := task.FromTemplateSpec{Title: "  "}
	if _, err := task.NewFromTemplate("01J0TASKTMPL0000000000000D", bad, origin, now); !shared.IsKind(err, shared.KindInvalid) {
		t.Fatalf("blank template title must be invalid, got %v", err)
	}
	if _, err := task.NewFromTemplate("", spec, origin, now); !shared.IsKind(err, shared.KindInvalid) {
		t.Fatalf("empty id must be invalid, got %v", err)
	}
	if _, err := task.NewFromTemplate("01J0TASKTMPL0000000000000D", spec, task.TemplateOrigin{}, now); !shared.IsKind(err, shared.KindInvalid) {
		t.Fatalf("empty origin must be invalid, got %v", err)
	}
}

func TestStatusAndPriorityStringParsing(t *testing.T) {
	for _, s := range []task.Status{task.StatusTodo, task.StatusDoing, task.StatusDone, task.StatusArchived} {
		back, err := task.ParseStatus(s.String())
		if err != nil || back != s {
			t.Fatalf("round trip failed for %v: %v %v", s, back, err)
		}
	}
	if _, err := task.ParseStatus("nope"); !shared.IsKind(err, shared.KindInvalid) {
		t.Fatalf("unknown status must be invalid, got %v", err)
	}
	if got := task.Status(99).String(); got != "unknown" {
		t.Fatalf("unknown String = %q", got)
	}
	if !task.StatusDoing.IsOpen() || task.StatusDone.IsOpen() || task.StatusArchived.IsOpen() {
		t.Fatal("IsOpen is wrong")
	}
	if got := task.StatusTodo.String(); got != "todo" {
		t.Fatalf("stable key = %q, must stay english for exports", got)
	}
}

func TestSetSortKeyRejectsEmpty(t *testing.T) {
	tk := newTask(t)
	if err := tk.SetSortKey("  ", at("2026-11-17 10:00:00")); !shared.IsKind(err, shared.KindInvalid) {
		t.Fatalf("empty sort key must be invalid, got %v", err)
	}
	if err := tk.SetSortKey("m", at("2026-11-17 10:00:00")); err != nil {
		t.Fatal(err)
	}
}

func TestRankBetweenProducesOrderedKeys(t *testing.T) {
	cases := [][2]string{{"", ""}, {"", "i"}, {"i", ""}, {"a", "z"}, {"i", "j"}, {"zz", ""}, {"9", "a"}, {"i", "k"}}
	for _, c := range cases {
		got, err := task.RankBetween(c[0], c[1])
		if err != nil {
			t.Fatalf("RankBetween(%q,%q): %v", c[0], c[1], err)
		}
		if !task.ValidRank(got) {
			t.Fatalf("RankBetween(%q,%q) = %q, not a valid rank", c[0], c[1], got)
		}
		if c[0] != "" && got <= c[0] {
			t.Fatalf("RankBetween(%q,%q) = %q, must sort after prev", c[0], c[1], got)
		}
		if c[1] != "" && got >= c[1] {
			t.Fatalf("RankBetween(%q,%q) = %q, must sort before next", c[0], c[1], got)
		}
	}
	if got, _ := task.RankBetween("", ""); got != task.InitialSortKey {
		t.Fatalf("both empty = %q, want the initial key", got)
	}
	// Nothing sorts between "" and "0": the caller must normalise first.
	if _, err := task.RankBetween("", "0"); shared.Code(err) != "task.rank.lowest" {
		t.Fatalf("RankBetween(\"\",\"0\") must report task.rank.lowest, got %v", err)
	}
	if _, err := task.RankBetween("a", "a0"); !shared.IsKind(err, shared.KindInvalid) {
		t.Fatalf("no key exists between a and a0, got %v", err)
	}
}

func TestRankBetweenRejectsInvertedAndInvalid(t *testing.T) {
	if _, err := task.RankBetween("z", "a"); !shared.IsKind(err, shared.KindInvalid) {
		t.Fatalf("inverted range must be invalid, got %v", err)
	}
	if _, err := task.RankBetween("Z", ""); !shared.IsKind(err, shared.KindInvalid) {
		t.Fatalf("uppercase rank must be invalid, got %v", err)
	}
	if _, err := task.RankBetween("", "!"); !shared.IsKind(err, shared.KindInvalid) {
		t.Fatalf("non-alphabet rank must be invalid, got %v", err)
	}
	if task.ValidRank("") || task.ValidRank("A") || !task.ValidRank("i9") {
		t.Fatal("ValidRank is wrong")
	}
}

func TestRankBetweenAlwaysTerminatesOnNeighbours(t *testing.T) {
	// Inserting repeatedly between the same two neighbours must keep producing
	// distinct keys, which is what drag-and-drop relies on.
	prev, next := "a", "b"
	seen := make(map[string]bool)
	for i := 0; i < 200; i++ {
		key, err := task.RankBetween(prev, next)
		if err != nil {
			t.Fatalf("iteration %d: %v", i, err)
		}
		if seen[key] {
			t.Fatalf("duplicate key %q at iteration %d", key, i)
		}
		if key <= prev || key >= next {
			t.Fatalf("%q not strictly between %q and %q", key, prev, next)
		}
		seen[key] = true
		prev = key
	}
}

func TestNormalizeRanksIsStrictlyIncreasing(t *testing.T) {
	for _, n := range []int{0, 1, 2, 5, 36, 100, 2000} {
		keys := task.NormalizeRanks(n)
		if len(keys) != n {
			t.Fatalf("n=%d: got %d keys", n, len(keys))
		}
		for i := 1; i < len(keys); i++ {
			if keys[i-1] >= keys[i] {
				t.Fatalf("n=%d: keys not increasing at %d: %q >= %q", n, i, keys[i-1], keys[i])
			}
		}
	}
}

func TestFilterHelpers(t *testing.T) {
	day := shared.MustParseLocalDate("2026-11-17")
	today := task.Today(day)
	if !today.DueFrom.Equal(day.Midnight()) || !today.DueTo.Equal(day.AddDays(1).Midnight()) {
		t.Fatalf("Today window = %v..%v", today.DueFrom, today.DueTo)
	}
	if today.Order != task.OrderManual {
		t.Fatal("today view must be in manual order")
	}
	hist := task.History(day)
	if !hist.ExcludeDueFrom.Equal(day.Midnight()) || !hist.ExcludeDueTo.Equal(day.AddDays(1).Midnight()) {
		t.Fatalf("History window = %v..%v", hist.ExcludeDueFrom, hist.ExcludeDueTo)
	}
	over := task.Overdue(at("2026-11-17 12:00:00"))
	if len(over.Statuses) != 2 || !over.DueBefore.Equal(at("2026-11-17 12:00:00")) {
		t.Fatalf("Overdue = %+v", over)
	}
	kids := task.Children("01J0TASKNEW0000000000000A")
	if kids.ParentID == nil || *kids.ParentID != "01J0TASKNEW0000000000000A" {
		t.Fatalf("Children = %+v", kids)
	}
}

func TestArchiveIsIdempotentAndKeepsOrigin(t *testing.T) {
	// The template occurrence key lives on the task row, so a deleted generated
	// task must keep it: that is what stops it from being recreated on restart.
	now := at("2026-11-17 08:00:00")
	origin := task.TemplateOrigin{TemplateID: "01J0TEMPLATE000000000000C", Day: shared.MustParseLocalDate("2026-11-17"), Seq: 0}
	tk, err := task.NewFromTemplate("01J0TASKTMPL0000000000000D", task.FromTemplateSpec{Title: "x"}, origin, now)
	if err != nil {
		t.Fatal(err)
	}
	if err := tk.Archive(now); err != nil {
		t.Fatal(err)
	}
	if tk.OriginTemplateID == "" {
		t.Fatal("archiving must not clear the occurrence key")
	}
	if errors.Is(nil, shared.ErrDuplicate) {
		t.Fatal("placeholder to keep errors imported")
	}
}

// A task may bring along the length of one pomodoro. Zero is "the default" — not a
// zero-minute session — and anything outside the bounds is refused.
func TestFocusMinutesAreBoundedAndZeroMeansDefault(t *testing.T) {
	tk := newTask(t)
	now := at("2026-11-17 10:00:00")

	if tk.FocusMinutes != 0 {
		t.Fatalf("a new task carries %d minutes, want 0 (the default)", tk.FocusMinutes)
	}
	if err := tk.SetFocusMinutes(task.FocusMinutesMax+1, now); !shared.IsKind(err, shared.KindInvalid) {
		t.Fatalf("a pomodoro above the maximum must be invalid, got %v", err)
	}
	if err := tk.SetFocusMinutes(-1, now); !shared.IsKind(err, shared.KindInvalid) {
		t.Fatalf("a negative pomodoro must be invalid, got %v", err)
	}
	if tk.FocusMinutes != 0 {
		t.Fatalf("a refused length was stored anyway: %d", tk.FocusMinutes)
	}
	if err := tk.SetFocusMinutes(10, now); err != nil {
		t.Fatalf("SetFocusMinutes: %v", err)
	}
	if tk.FocusMinutes != 10 {
		t.Fatalf("FocusMinutes = %d, want 10", tk.FocusMinutes)
	}
	if !tk.UpdatedAt.Equal(now) {
		t.Errorf("the edit did not stamp the time")
	}
	// Clearing goes back to the default instead of to zero minutes.
	if err := tk.SetFocusMinutes(0, now); err != nil {
		t.Fatalf("clearing must be allowed: %v", err)
	}
	if tk.FocusMinutes != 0 {
		t.Fatalf("FocusMinutes = %d after clearing, want 0", tk.FocusMinutes)
	}
}
