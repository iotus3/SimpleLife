package goal_test

import (
	"testing"

	app "github.com/iotus3/simplelife/app/internal/application/goal"
	"github.com/iotus3/simplelife/app/internal/apptest/harness"
	"github.com/iotus3/simplelife/app/internal/apptest/store"
	"github.com/iotus3/simplelife/app/internal/domain/shared"
)

func newService(t *testing.T) (*app.Service, *harness.Clock) {
	t.Helper()
	memory := store.New()
	clock := harness.NewClock(harness.At("2026-11-17 09:00:00"))
	return app.New(memory.Goals(), memory, clock, harness.NewIDs()), clock
}

func day(s string) shared.LocalDate { return shared.MustParseLocalDate(s) }

func TestCreateListEditDelete(t *testing.T) {
	service, _ := newService(t)

	exam, err := service.Create(app.Input{
		Name: "软考高项", Note: "高级项目经理",
		Start: day("2026-11-17"), End: day("2027-05-01"),
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if exam.Name != "软考高项" || exam.Note != "高级项目经理" {
		t.Fatalf("created goal = %+v", exam)
	}
	if got := service.Today(); !got.Equal(day("2026-11-17")) {
		t.Fatalf("today = %v, want the frozen clock's day", got)
	}

	if _, err := service.Create(app.Input{
		Name: "考研", Start: day("2026-11-17"), End: day("2027-12-10"),
	}); err != nil {
		t.Fatalf("create second: %v", err)
	}
	list, err := service.List()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("listed %d goals, want 2", len(list))
	}
	// The nearest deadline comes first, so the page reads from the soonest down.
	if list[0].Name != "软考高项" || list[1].Name != "考研" {
		t.Fatalf("order = %q, %q", list[0].Name, list[1].Name)
	}

	updated, err := service.Update(exam.ID, app.Input{
		Name: "软考高项 · 已过", Note: "改过备注",
		Start: day("2026-11-17"), End: day("2027-06-01"),
	})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if updated.Name != "软考高项 · 已过" || !updated.End.Equal(day("2027-06-01")) {
		t.Fatalf("updated goal = %+v", updated)
	}
	if !updated.CreatedAt.Equal(exam.CreatedAt) {
		t.Fatalf("an edit moved the day the goal was set: %v -> %v", exam.CreatedAt, updated.CreatedAt)
	}

	if err := service.Delete(exam.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if err := service.Delete(exam.ID); err == nil {
		t.Fatal("deleting a goal twice was accepted")
	}
	left, err := service.List()
	if err != nil {
		t.Fatalf("list after delete: %v", err)
	}
	if len(left) != 1 || left[0].Name != "考研" {
		t.Fatalf("after delete: %+v", left)
	}
}

// The service refuses a span it cannot count, and nothing is stored: a bad date is
// a form error, not a row.
func TestCreateRefusesWhatTheDomainRefuses(t *testing.T) {
	service, _ := newService(t)
	if _, err := service.Create(app.Input{
		Name: "倒着来", Start: day("2026-12-01"), End: day("2026-11-17"),
	}); err == nil {
		t.Fatal("an end before the start was accepted")
	}
	list, err := service.List()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 0 {
		t.Fatalf("a refused goal was stored: %+v", list)
	}
}

// Updating a goal that is gone reports it instead of quietly creating a new one.
func TestUpdateReportsAGoalThatIsGone(t *testing.T) {
	service, _ := newService(t)
	if _, err := service.Update("missing", app.Input{
		Name: "x", Start: day("2026-11-17"), End: day("2026-12-01"),
	}); err == nil {
		t.Fatal("updating a missing goal was accepted")
	}
}
