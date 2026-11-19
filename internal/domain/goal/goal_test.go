package goal_test

import (
	"testing"
	"time"

	"github.com/iotus3/simplelife/app/internal/domain/goal"
	"github.com/iotus3/simplelife/app/internal/domain/shared"
)

func day(s string) shared.LocalDate {
	return shared.MustParseLocalDate(s)
}

func at(s string) time.Time {
	t, err := time.ParseInLocation("2006-01-02 15:04", s, time.Local)
	if err != nil {
		panic(err)
	}
	return t
}

// The two numbers on a card add up to the span: that is what lets the bar between
// them mean something.
func TestProgressCountsBothEndsOfTheSpan(t *testing.T) {
	g, err := goal.New("g1", "Exam", "", day("2026-11-17"), day("2027-05-01"), at("2026-11-17 09:00"))
	if err != nil {
		t.Fatalf("new goal: %v", err)
	}
	// 2026-11-17 .. 2027-05-01 inclusive.
	if got, want := g.TotalDays(), 166; got != want {
		t.Fatalf("total days = %d, want %d", got, want)
	}
	cases := []struct {
		today           string
		elapsed, remain int
	}{
		{"2026-11-18", 1, 165}, // the day after it was set
		{"2026-12-31", 44, 122},
		{"2027-04-30", 164, 2},
		{"2027-05-01", 165, 1}, // the last day still has its own day left
		{"2027-05-02", 166, 0}, // past the end: all of it has passed
		{"2026-01-01", 0, 166}, // before the start: nothing has passed
	}
	for _, tc := range cases {
		got := g.ProgressOf(day(tc.today))
		if got.Elapsed != tc.elapsed || got.Remaining != tc.remain {
			t.Errorf("on %s: %+v, want elapsed %d and remaining %d", tc.today, got, tc.elapsed, tc.remain)
		}
		if got.Elapsed+got.Remaining != got.Total {
			t.Errorf("on %s: elapsed %d + remaining %d != total %d", tc.today, got.Elapsed, got.Remaining, got.Total)
		}
	}
}

// The end day is still a day to work on; the countdown ends the day after.
func TestFinishedOnlyAfterTheEndDay(t *testing.T) {
	g, err := goal.New("g1", "Exam", "", day("2026-11-17"), day("2027-05-01"), at("2026-11-17 09:00"))
	if err != nil {
		t.Fatalf("new goal: %v", err)
	}
	for _, tc := range []struct {
		today string
		want  bool
	}{
		{"2026-11-17", false},
		{"2027-05-01", false},
		{"2027-05-02", true},
	} {
		if got := g.Finished(day(tc.today)); got != tc.want {
			t.Errorf("finished on %s = %v, want %v", tc.today, got, tc.want)
		}
	}
}

// A goal set and due on the same day spans one day, not zero: "0 days" would be a
// card with nothing on it.
func TestOneDayGoalSpansOneDay(t *testing.T) {
	g, err := goal.New("g1", "Today's exam", "", day("2026-11-17"), day("2026-11-17"), at("2026-11-17 09:00"))
	if err != nil {
		t.Fatalf("new goal: %v", err)
	}
	if got := g.TotalDays(); got != 1 {
		t.Fatalf("total days = %d, want 1", got)
	}
	if got := g.ProgressOf(day("2026-11-17")); got.Elapsed != 0 || got.Remaining != 1 {
		t.Fatalf("progress on the only day = %+v", got)
	}
}

func TestValidationRefusesWhatCannotBeCounted(t *testing.T) {
	now := at("2026-11-17 09:00")
	cases := []struct {
		name        string
		goalName    string
		note        string
		start, end  shared.LocalDate
		wantRefused bool
	}{
		{name: "no name", goalName: "   ", start: day("2026-11-17"), end: day("2026-12-01"), wantRefused: true},
		{name: "name too long", goalName: longName(goal.NameMaxRunes + 1), start: day("2026-11-17"), end: day("2026-12-01"), wantRefused: true},
		{name: "note too long", goalName: "Exam", note: longName(goal.NoteMaxRunes + 1), start: day("2026-11-17"), end: day("2026-12-01"), wantRefused: true},
		{name: "no start", goalName: "Exam", end: day("2026-12-01"), wantRefused: true},
		{name: "no end", goalName: "Exam", start: day("2026-11-17"), wantRefused: true},
		{name: "end before start", goalName: "Exam", start: day("2026-12-01"), end: day("2026-11-17"), wantRefused: true},
		{name: "longer than ten years", goalName: "Exam", start: day("2026-11-17"), end: day("2037-01-01"), wantRefused: true},
		{name: "plain goal", goalName: "Exam", start: day("2026-11-17"), end: day("2026-12-01")},
	}
	for _, tc := range cases {
		_, err := goal.New("g1", tc.goalName, tc.note, tc.start, tc.end, now)
		if refused := err != nil; refused != tc.wantRefused {
			t.Errorf("%s: err = %v, want refused = %v", tc.name, err, tc.wantRefused)
		}
	}
}

// A refused edit leaves the stored goal untouched, note and dates included.
func TestRefusedEditKeepsTheGoal(t *testing.T) {
	g, err := goal.New("g1", "Exam", "keep me", day("2026-11-17"), day("2026-12-01"), at("2026-11-17 09:00"))
	if err != nil {
		t.Fatalf("new goal: %v", err)
	}
	before := *g
	if err := g.Edit("Exam", "keep me", day("2026-12-01"), day("2026-11-17"), at("2026-12-02 09:00")); err == nil {
		t.Fatal("a reversed span was accepted")
	}
	if *g != before {
		t.Fatalf("the goal changed after a refused edit: %+v", *g)
	}
	if err := g.Edit("Exam mk2", "new note", day("2026-11-20"), day("2027-01-10"), at("2026-12-02 09:00")); err != nil {
		t.Fatalf("valid edit: %v", err)
	}
	if g.Name != "Exam mk2" || g.Note != "new note" || !g.Start.Equal(day("2026-11-20")) {
		t.Fatalf("the edit did not land: %+v", *g)
	}
}

func longName(n int) string {
	out := make([]rune, n)
	for i := range out {
		out[i] = 'x'
	}
	return string(out)
}
