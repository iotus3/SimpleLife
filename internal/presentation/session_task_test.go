package presentation

import (
	"testing"
	"time"

	focusapp "github.com/iotus3/simplelife/app/internal/application/focus"
	taskapp "github.com/iotus3/simplelife/app/internal/application/task"
	"github.com/iotus3/simplelife/app/internal/domain/focus"
	"github.com/iotus3/simplelife/app/internal/domain/task"
	"github.com/iotus3/simplelife/app/internal/presentation/i18n"
)

// TestCompletingASessionCompletesItsTask is requirement 1 of the ninth round: a
// finished pomodoro is the user saying "this is done", so the task follows the
// session instead of waiting for a second tick in the list.
func TestCompletingASessionCompletesItsTask(t *testing.T) {
	fixture := newFixture(t)
	session := fixture.openWindow(t)
	fixture.state.route = RouteTasks
	fixture.state.markAll()
	settle(t, session)

	zh := func(id i18n.MessageID) string { return i18n.T(i18n.ZhCN, id) }
	created, err := fixture.deps.Tasks.Create(taskapp.CreateInput{Title: "写周报", DueDate: fixture.state.today()})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	fixture.state.markAll()
	settle(t, session)

	// Started from the row, so this is the path a user actually takes.
	if err := session.ClickLabel("button:" + zh(i18n.MsgTaskActionFocus)); err != nil {
		t.Fatalf("start from the row: %v", err)
	}
	settle(t, session)
	if fixture.state.active == nil {
		t.Fatal("the row's start button started nothing")
	}

	// A session with no elapsed time cannot be completed, the same as the real
	// window: the clock has to move before the button means anything.
	fixture.clock.Advance(25 * time.Minute)
	if err := session.ClickLabel("button:" + zh(i18n.MsgFocusActionComplete)); err != nil {
		t.Fatalf("complete the session: %v", err)
	}
	settle(t, session)

	stored, err := fixture.deps.Tasks.Get(created.ID)
	if err != nil {
		t.Fatalf("reload the task: %v", err)
	}
	if stored.Status != task.StatusDone {
		t.Fatalf("a finished session left its task %v, want done", stored.Status)
	}
	if stored.DonePomodoros != 1 {
		t.Errorf("the task counts %d pomodoros, want 1", stored.DonePomodoros)
	}
	if !stored.CompletedAt.Equal(fixture.clock.Now()) {
		t.Errorf("completed at %v, want the moment the session settled (%v)",
			stored.CompletedAt, fixture.clock.Now())
	}
	// The row keeps its place and shows the tick, which is how the same task is
	// reopened if the completion was a mistake.
	if !session.HasText(i18n.T(i18n.ZhCN, i18n.MsgTaskDoneSection, 1)) {
		t.Errorf("the finished task is not in the done section: %v", session.Texts())
	}
}

// TestSessionOnAFinishedTaskStillSettles covers the other order: the user ticks the
// task off while the timer runs. Settling must not fail over a task that is already
// done — the pomodoro happened either way.
func TestSessionOnAFinishedTaskStillSettles(t *testing.T) {
	fixture := newFixture(t)
	created, err := fixture.deps.Tasks.Create(taskapp.CreateInput{Title: "写周报", DueDate: fixture.state.today()})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := fixture.deps.Focus.Start(focusapp.StartInput{
		Kind: focus.KindFocus, TaskID: created.ID, Planned: 25 * time.Minute,
	}); err != nil {
		t.Fatalf("start: %v", err)
	}
	if _, err := fixture.deps.Tasks.Complete(created.ID); err != nil {
		t.Fatalf("complete the task while the timer runs: %v", err)
	}
	fixture.clock.Advance(25 * time.Minute)

	if _, _, err := fixture.deps.Focus.Complete(); err != nil {
		t.Fatalf("settling a session on a finished task failed: %v", err)
	}
	stored, err := fixture.deps.Tasks.Get(created.ID)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if stored.Status != task.StatusDone {
		t.Fatalf("status = %v, want done", stored.Status)
	}
	if stored.DonePomodoros != 0 {
		// The task was already done when the session settled, so it is not credited
		// again: a second tick would inflate the statistics.
		t.Errorf("DonePomodoros = %d, want 0 for a task that was already done", stored.DonePomodoros)
	}
}

// TestTaskCarriesItsOwnPomodoroLength is requirement 2: the editor gained a length
// beside the estimate, and a session started on that task runs for it.
func TestTaskCarriesItsOwnPomodoroLength(t *testing.T) {
	fixture := newFixture(t)
	session := fixture.openWindow(t)
	fixture.state.route = RouteTasks
	fixture.state.markAll()
	settle(t, session)

	zh := func(id i18n.MessageID) string { return i18n.T(i18n.ZhCN, id) }
	if err := session.ClickLabel(zh(i18n.MsgTaskNew)); err != nil {
		t.Fatalf("open the task editor: %v", err)
	}
	settle(t, session)
	draft := fixture.state.taskDraft
	if draft == nil || !draft.open {
		t.Fatal("the task editor did not open")
	}
	draft.title = "写周报"
	draft.pomodoro = "10"
	if err := session.ClickLabel(zh(i18n.MsgSave)); err != nil {
		t.Fatalf("save: %v", err)
	}
	settle(t, session)

	list, err := fixture.deps.Tasks.Today(fixture.state.today())
	if err != nil {
		t.Fatalf("today: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("today has %d tasks, want 1: %v", len(list), titles(list))
	}
	if list[0].FocusMinutes != 10 {
		t.Fatalf("FocusMinutes = %d, want 10", list[0].FocusMinutes)
	}

	// The length is the task's, wherever the session is started from.
	if err := session.ClickLabel("button:" + zh(i18n.MsgTaskActionFocus)); err != nil {
		t.Fatalf("start from the row: %v", err)
	}
	settle(t, session)
	if fixture.state.active == nil {
		t.Fatal("the row's start button started nothing")
	}
	if got := fixture.state.active.Planned; got != 10*time.Minute {
		t.Fatalf("the session runs for %v, want the task's 10m", got)
	}
	if !session.HasText("10:00") {
		t.Errorf("the row does not show the task's countdown: %v", session.Texts())
	}
}

// TestTaskWithoutALengthUsesThePagesLength is the other half of requirement 2: an
// empty field means "the length the focus page is set to", not zero minutes.
func TestTaskWithoutALengthUsesThePagesLength(t *testing.T) {
	fixture := newFixture(t)
	created, err := fixture.deps.Tasks.Create(taskapp.CreateInput{Title: "写周报", DueDate: fixture.state.today()})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if created.FocusMinutes != 0 {
		t.Fatalf("a task with no length carries %d", created.FocusMinutes)
	}
	fixture.state.focusMin = 35
	session := fixture.openWindow(t)
	fixture.state.route = RouteTasks
	fixture.state.markAll()
	settle(t, session)

	if err := session.ClickLabel("button:" + i18n.T(i18n.ZhCN, i18n.MsgTaskActionFocus)); err != nil {
		t.Fatalf("start from the row: %v", err)
	}
	settle(t, session)
	if got := fixture.state.active.Planned; got != 35*time.Minute {
		t.Fatalf("the session runs for %v, want the page's 35m", got)
	}
}
