package presentation

import (
	"testing"
	"time"

	focusapp "github.com/iotus3/simplelife/app/internal/application/focus"
	taskapp "github.com/iotus3/simplelife/app/internal/application/task"
	"github.com/iotus3/simplelife/app/internal/domain/focus"
	"github.com/iotus3/simplelife/app/internal/presentation/format"
	"github.com/iotus3/simplelife/app/internal/presentation/i18n"
)

// A task that is ticked off keeps its row: it moves under the open ones with a line
// through it, instead of vanishing from the only list that was showing it.
func TestFinishedTaskKeepsItsRow(t *testing.T) {
	fixture := newFixture(t)
	session := fixture.openWindow(t)
	fixture.state.route = RouteTasks
	fixture.state.markAll()
	settle(t, session)

	zh := func(id i18n.MessageID, args ...any) string { return i18n.T(i18n.ZhCN, id, args...) }
	title := "写周报"
	created, err := fixture.deps.Tasks.Create(taskapp.CreateInput{Title: title, DueDate: fixture.state.today()})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	fixture.state.markAll()
	settle(t, session)
	if !session.HasText(title) {
		t.Fatalf("the new task is not on the page: %v", session.Texts())
	}

	if _, err := fixture.deps.Tasks.Complete(created.ID); err != nil {
		t.Fatalf("complete: %v", err)
	}
	fixture.state.markAll()
	settle(t, session)

	if !session.HasText(title) {
		t.Fatalf("a finished task left the day's list: %v", session.Texts())
	}
	if !session.HasText(zh(i18n.MsgTaskDoneSection, 1)) {
		t.Errorf("the finished tasks have no heading: %v", session.Texts())
	}
	// The strike is drawn over the words, not a framework style, and the row keeps the
	// checkbox that puts the task back: a finished task is still a control.
	if _, err := session.Locate("checkbox:"); err != nil {
		t.Fatalf("the finished row has no checkbox to reopen it: %v", err)
	}
}

// The history keeps what was finished today too: ticking a task off must not hide it
// from both views at once, which is what made finished work look lost.
func TestHistoryKeepsWhatWasFinishedToday(t *testing.T) {
	fixture := newFixture(t)
	session := fixture.openWindow(t)
	fixture.state.route = RouteTasks
	fixture.state.taskTab = 1
	fixture.state.markAll()
	settle(t, session)

	today := fixture.state.today()
	created, err := fixture.deps.Tasks.Create(taskapp.CreateInput{Title: "整理账单", DueDate: today})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := fixture.deps.Tasks.Complete(created.ID); err != nil {
		t.Fatalf("complete: %v", err)
	}
	fixture.state.markAll()
	settle(t, session)

	found := false
	for _, group := range fixture.state.taskGroups {
		for _, value := range group.Tasks {
			if value.ID == created.ID {
				found = true
			}
		}
	}
	if !found {
		t.Fatalf("the task finished today is not in the history groups: %+v", fixture.state.taskGroups)
	}
	if !session.HasText("整理账单") {
		t.Errorf("the history does not show it: %v", session.Texts())
	}
}

// A task a session is running on takes over its own row: the countdown and the
// three controls of the focus page, in the list where the session was started.
func TestRunningSessionTakesOverItsTaskRow(t *testing.T) {
	fixture := newFixture(t)
	session := fixture.openWindow(t)
	fixture.state.route = RouteTasks
	fixture.state.markAll()
	settle(t, session)

	zh := func(id i18n.MessageID, args ...any) string { return i18n.T(i18n.ZhCN, id, args...) }
	today := fixture.state.today()
	created, err := fixture.deps.Tasks.Create(taskapp.CreateInput{Title: "写周报", DueDate: today})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	fixture.state.markAll()
	settle(t, session)
	if _, err := session.Locate("button:" + zh(i18n.MsgTaskActionFocus)); err != nil {
		t.Fatalf("an idle row has no start button: %v", err)
	}

	if _, err := fixture.deps.Focus.Start(focusapp.StartInput{
		Kind: focus.KindFocus, TaskID: created.ID, Planned: 25 * time.Minute,
	}); err != nil {
		t.Fatalf("start: %v", err)
	}
	fixture.state.markAll()
	settle(t, session)

	if !session.HasText("25:00") {
		t.Errorf("the row shows no countdown: %v", session.Texts())
	}
	for _, id := range []i18n.MessageID{
		i18n.MsgFocusActionPause, i18n.MsgFocusActionComplete, i18n.MsgFocusActionCancel,
	} {
		if _, err := session.Locate("button:" + zh(id)); err != nil {
			t.Errorf("the running row has no %q button: %v", zh(id), err)
		}
	}
	// The row must not offer a second start for a session that is already running.
	if _, err := session.Locate("button:" + zh(i18n.MsgTaskActionFocus)); err == nil {
		t.Errorf("the running row still offers a start button: %v", session.Texts())
	}

	// Paused, the same row offers resume instead of pause.
	if _, err := fixture.deps.Focus.Pause(); err != nil {
		t.Fatalf("pause: %v", err)
	}
	fixture.state.markAll()
	settle(t, session)
	if _, err := session.Locate("button:" + zh(i18n.MsgFocusActionResume)); err != nil {
		t.Errorf("a paused session cannot be resumed from its row: %v", err)
	}
}

// The history is a calendar: its sections carry dates, never "today", so a section
// still means the same day a week later.
func TestHistorySectionsCarryDates(t *testing.T) {
	fixture := newFixture(t)
	session := fixture.openWindow(t)
	fixture.state.route = RouteTasks
	fixture.state.taskTab = 1
	fixture.state.markAll()
	settle(t, session)

	today := fixture.state.today()
	created, err := fixture.deps.Tasks.Create(taskapp.CreateInput{Title: "整理账单", DueDate: today})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := fixture.deps.Tasks.Complete(created.ID); err != nil {
		t.Fatalf("complete: %v", err)
	}
	fixture.state.markAll()
	settle(t, session)

	if !session.HasText(format.Date(i18n.ZhCN, today)) {
		t.Errorf("the history section is not named by its date: %v", session.Texts())
	}
	// "Today" stays where it belongs — the name of the tab — and names no section:
	// one occurrence means the tab alone.
	todayWord := i18n.T(i18n.ZhCN, i18n.MsgToday)
	count := 0
	for _, text := range session.Texts() {
		if text == todayWord {
			count++
		}
	}
	if count != 1 {
		t.Errorf("%q appears %d times, want only the tab: %v", todayWord, count, session.Texts())
	}
}
