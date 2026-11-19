package sqlite_test

import (
	"testing"

	"github.com/iotus3/simplelife/app/internal/domain/goal"
	"github.com/iotus3/simplelife/app/internal/domain/shared"
)

func sampleGoal(t *testing.T) *goal.Goal {
	t.Helper()
	g, err := goal.New("0000000000GOAL0000000001", "软考高项", "高级项目经理",
		shared.MustParseLocalDate("2026-11-17"), shared.MustParseLocalDate("2027-05-01"), clock().Now())
	if err != nil {
		t.Fatalf("new goal: %v", err)
	}
	return g
}

func TestGoalRoundTrip(t *testing.T) {
	db := openFile(t)
	g := sampleGoal(t)
	if err := db.Goals().Save(g); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := db.Goals().Get(g.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Name != "软考高项" || got.Note != "高级项目经理" {
		t.Fatalf("goal = %+v", got)
	}
	if !got.Start.Equal(g.Start) || !got.End.Equal(g.End) {
		t.Fatalf("dates = %v .. %v, want %v .. %v", got.Start, got.End, g.Start, g.End)
	}
	if !got.CreatedAt.Equal(g.CreatedAt) {
		t.Fatalf("created_at = %v, want %v", got.CreatedAt, g.CreatedAt)
	}
	// The card's arithmetic must survive the round trip: DateOf/ParseLocalDate keep
	// the day, not an instant of it.
	if got.TotalDays() != g.TotalDays() {
		t.Fatalf("span = %d days, want %d", got.TotalDays(), g.TotalDays())
	}
}

// An edit writes the new fields and keeps the day the goal was set.
func TestGoalSaveUpdatesWithoutMovingCreatedAt(t *testing.T) {
	db := openFile(t)
	g := sampleGoal(t)
	if err := db.Goals().Save(g); err != nil {
		t.Fatalf("Save: %v", err)
	}
	before := g.CreatedAt
	if err := g.Edit("软考高架", "", g.Start, shared.MustParseLocalDate("2027-10-01"), clock().Now().Add(3600)); err != nil {
		t.Fatalf("edit: %v", err)
	}
	if err := db.Goals().Save(g); err != nil {
		t.Fatalf("Save after edit: %v", err)
	}
	got, err := db.Goals().Get(g.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Name != "软考高架" || got.Note != "" {
		t.Fatalf("goal after edit = %+v", got)
	}
	if !got.End.Equal(shared.MustParseLocalDate("2027-10-01")) {
		t.Fatalf("end = %v", got.End)
	}
	if !got.CreatedAt.Equal(before) {
		t.Fatalf("created_at moved on edit: %v -> %v", before, got.CreatedAt)
	}
}

// The list is ordered by the nearest deadline, so the page and the store agree on
// what "first" means.
func TestGoalListIsOrderedByDeadline(t *testing.T) {
	db := openFile(t)
	now := clock().Now()
	for _, spec := range []struct{ id, name, end string }{
		{"0000000000GOAL0000000003", "far", "2035-06-07"},
		{"0000000000GOAL0000000002", "near", "2027-05-01"},
		{"0000000000GOAL0000000004", "middle", "2027-12-10"},
	} {
		g, err := goal.New(shared.ID(spec.id), spec.name, "", shared.MustParseLocalDate("2026-11-17"),
			shared.MustParseLocalDate(spec.end), now)
		if err != nil {
			t.Fatalf("new %s: %v", spec.name, err)
		}
		if err := db.Goals().Save(g); err != nil {
			t.Fatalf("save %s: %v", spec.name, err)
		}
	}
	list, err := db.Goals().List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 3 {
		t.Fatalf("listed %d goals, want 3", len(list))
	}
	want := []string{"near", "middle", "far"}
	for i, name := range want {
		if list[i].Name != name {
			t.Fatalf("order = %v, want %v", []string{list[0].Name, list[1].Name, list[2].Name}, want)
		}
	}
}

func TestGoalDeleteAndMissing(t *testing.T) {
	db := openFile(t)
	g := sampleGoal(t)
	if err := db.Goals().Save(g); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if err := db.Goals().Delete(g.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := db.Goals().Get(g.ID); err == nil {
		t.Fatal("a deleted goal was still readable")
	}
	if err := db.Goals().Delete(g.ID); err == nil {
		t.Fatal("deleting a missing goal was accepted")
	}
}
