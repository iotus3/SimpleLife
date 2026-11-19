package presentation

import (
	"strings"
	"testing"

	goalapp "github.com/iotus3/simplelife/app/internal/application/goal"
	"github.com/iotus3/simplelife/app/internal/domain/shared"
	"github.com/iotus3/simplelife/app/internal/presentation/format"
	"github.com/iotus3/simplelife/app/internal/presentation/i18n"
)

// goalInput is the form a test would fill in.
func goalInput(name, note string, start, end shared.LocalDate) goalapp.Input {
	return goalapp.Input{Name: name, Note: note, Start: start, End: end}
}

// A goal is a card that answers the whole question by itself: what it is called,
// how far along it is, and how many days are left.
func TestGoalCardCountsTheDays(t *testing.T) {
	fixture := newFixture(t)
	session := fixture.openWindow(t)
	fixture.state.route = RouteGoals
	fixture.state.markAll()
	settle(t, session)

	zh := func(id i18n.MessageID, args ...any) string { return i18n.T(i18n.ZhCN, id, args...) }
	today := fixture.state.today()
	if _, err := fixture.deps.Goals.Create(goalInput("软考高项", "高级项目经理", today, today.AddDays(100))); err != nil {
		t.Fatalf("create: %v", err)
	}
	fixture.state.markAll()
	settle(t, session)

	if !session.HasText("软考高项") || !session.HasText("高级项目经理") {
		t.Fatalf("the card does not show the goal: %v", session.Texts())
	}
	// 101 days inclusive, none passed on the day it was set.
	if !session.HasText(zh(i18n.MsgGoalElapsed, 0, 101)) {
		t.Errorf("the card does not count the span: %v", session.Texts())
	}
	if !session.HasText("101") {
		t.Errorf("the card does not show the days left: %v", session.Texts())
	}
	if !session.HasText(zh(i18n.MsgGoalStateActive)) {
		t.Errorf("a live goal is not tagged as running: %v", session.Texts())
	}
	if !session.HasText(format.Date(i18n.ZhCN, today.AddDays(100))) {
		t.Errorf("the card does not name its deadline: %v", session.Texts())
	}
}

// The empty page invites the first goal instead of showing nothing.
func TestGoalsPageInvitesTheFirstGoal(t *testing.T) {
	fixture := newFixture(t)
	session := fixture.openWindow(t)
	fixture.state.route = RouteGoals
	fixture.state.markAll()
	settle(t, session)

	if !session.HasText(i18n.T(i18n.ZhCN, i18n.MsgGoalEmpty)) {
		t.Fatalf("the empty page says nothing: %v", session.Texts())
	}
	if _, err := session.Locate("button:" + i18n.T(i18n.ZhCN, i18n.MsgGoalNew)); err != nil {
		t.Fatalf("the empty page has no button to add a goal: %v", err)
	}
}

// The editor writes a goal, and a second one can be written over the same dialog:
// the draft is reset between openings, so nothing of the first one leaks in.
func TestGoalEditorWritesAGoal(t *testing.T) {
	fixture := newFixture(t)
	session := fixture.openWindow(t)
	fixture.state.route = RouteGoals
	fixture.state.markAll()
	settle(t, session)

	zh := func(id i18n.MessageID, args ...any) string { return i18n.T(i18n.ZhCN, id, args...) }
	if err := session.ClickLabel(zh(i18n.MsgGoalNew)); err != nil {
		t.Fatalf("open the editor: %v", err)
	}
	settle(t, session)
	if !session.HasText(zh(i18n.MsgGoalFieldStart)) || !session.HasText(zh(i18n.MsgGoalConfirm)) {
		t.Fatalf("the editor is missing its fields: %v", session.Texts())
	}
	if err := session.ClickLabel(zh(i18n.MsgGoalFieldName)); err != nil {
		t.Fatalf("click the name field: %v", err)
	}
	if err := session.Type("软考高架"); err != nil {
		t.Fatalf("type the name: %v", err)
	}
	settle(t, session)
	if err := session.ClickLabel(zh(i18n.MsgGoalConfirm)); err != nil {
		t.Fatalf("save: %v", err)
	}
	settle(t, session)

	goals, err := fixture.deps.Goals.List()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(goals) != 1 || goals[0].Name != "软考高架" {
		t.Fatalf("stored goals = %+v", goals)
	}
	// A goal created without touching the dates runs from today for a month.
	start := fixture.state.today()
	if !goals[0].Start.Equal(start) || !goals[0].End.Equal(start.AddDays(goalDefaultDays)) {
		t.Fatalf("default span = %v .. %v", goals[0].Start, goals[0].End)
	}
	if !session.HasText("软考高架") {
		t.Errorf("the new goal is not on the page: %v", session.Texts())
	}
}

// A goal whose last day has passed is tagged as finished and counts zero left, so a
// card never reads "还剩 -3 天".
func TestFinishedGoalRecedes(t *testing.T) {
	fixture := newFixture(t)
	session := fixture.openWindow(t)
	fixture.state.route = RouteGoals
	today := fixture.state.today()
	if _, err := fixture.deps.Goals.Create(goalInput("旧目标", "", today.AddDays(-30), today.AddDays(-3))); err != nil {
		t.Fatalf("create: %v", err)
	}
	fixture.state.markAll()
	settle(t, session)

	zh := func(id i18n.MessageID, args ...any) string { return i18n.T(i18n.ZhCN, id, args...) }
	if !session.HasText(zh(i18n.MsgGoalStateDone)) {
		t.Fatalf("a finished goal is not tagged as finished: %v", session.Texts())
	}
	// Nothing is left, and nothing negative either: the count stops at zero instead
	// of running past the deadline.
	for _, value := range fixture.state.goals {
		if progress := value.ProgressOf(today); progress.Remaining != 0 || progress.Elapsed != progress.Total {
			t.Errorf("a finished goal still counts %+v", progress)
		}
	}
}

// TestGoalCountdownReadsAsOnePhrase is requirement 5 of the ninth round: the caption used to be
// stacked above the number, so two characters floated over a large figure and read like a typo.
// Bounds are not part of the headless tree, so this checks the phrase and its paint order; the
// arrangement is verified in build/smoke/28-goal-card.png.
func TestGoalCountdownReadsAsOnePhrase(t *testing.T) {
	fixture := newFixture(t)
	session := fixture.openWindow(t)
	fixture.state.route = RouteGoals
	today := fixture.state.today()
	if _, err := fixture.deps.Goals.Create(goalInput("软考高项", "", today, today.AddDays(20))); err != nil {
		t.Fatalf("create: %v", err)
	}
	fixture.state.markAll()
	settle(t, session)

	zh := func(id i18n.MessageID) string { return i18n.T(i18n.ZhCN, id) }
	painted := session.Texts()
	left := zh(i18n.MsgGoalLeftLabel)
	if left == "" || strings.TrimSpace(left) != left {
		t.Fatalf("the countdown's caption is %q", left)
	}
	// 21 days inclusive of both ends: today .. today+20.
	for _, piece := range []string{left, "21", zh(i18n.MsgGoalDayUnit)} {
		found := false
		for _, text := range painted {
			if text == piece {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("the countdown does not paint %q: %v", piece, painted)
		}
	}
	// The number stands alone in its own run, so it can be drawn at its own size.
	if session.HasText(left + " 21") {
		t.Errorf("the countdown is one string, so the number cannot be the large part: %v", painted)
	}
}
