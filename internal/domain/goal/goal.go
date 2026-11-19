// Package goal holds the countdown goals: a name and the two days that bound it. It is a
// value with a clock, not a scheduler.
package goal

import (
	"strings"
	"time"
	"unicode/utf8"

	"github.com/iotus3/simplelife/app/internal/domain/shared"
)

const (
	// NameMaxRunes bounds the name on a card.
	NameMaxRunes = 60
	// NoteMaxRunes bounds the note: it is a line or two under the name, not a
	// document.
	NoteMaxRunes = 300
	// SpanMaxDays caps a goal at ten years, so a typed year that meant 2026 and came
	// out as 2206 is refused instead of counting down for a century.
	SpanMaxDays = 3650
)

// Goal is one countdown: what it is called, and the two days that bound it. Both days are
// dates, so "days left" does not depend on the hour the card is painted.
type Goal struct {
	ID   shared.ID
	Name string
	// Note is what the user wants to remember about the goal. It may be empty.
	Note string
	// Start is the day counting begins (usually the day the goal was set), End the
	// day it should be reached. Both are inclusive: a goal set and due on the same
	// day spans one day, not zero.
	Start     shared.LocalDate
	End       shared.LocalDate
	CreatedAt time.Time
	UpdatedAt time.Time
}

// New builds a goal and validates it. The caller stores it afterwards.
func New(id shared.ID, name, note string, start, end shared.LocalDate, now time.Time) (*Goal, error) {
	g := &Goal{
		ID:        id,
		Name:      strings.TrimSpace(name),
		Note:      strings.TrimSpace(note),
		Start:     start,
		End:       end,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := g.Validate(); err != nil {
		return nil, err
	}
	return g, nil
}

// Edit replaces every field the user can change, on a copy: an edit that fails
// validation leaves the goal exactly as it was, so a half-typed date cannot reach
// the card behind the dialog.
func (g *Goal) Edit(name, note string, start, end shared.LocalDate, now time.Time) error {
	cp := *g
	cp.Name = strings.TrimSpace(name)
	cp.Note = strings.TrimSpace(note)
	cp.Start = start
	cp.End = end
	if err := cp.Validate(); err != nil {
		return err
	}
	cp.UpdatedAt = now
	*g = cp
	return nil
}

// Validate checks the goal can be stored and counted.
func (g *Goal) Validate() error {
	if g.ID.IsZero() {
		return shared.Invalid("goal.id.empty", "", "goal id is required")
	}
	if n := utf8.RuneCountInString(g.Name); n == 0 || n > NameMaxRunes {
		return shared.Invalid("goal.name.length", "name", "name must be 1..60 runes")
	}
	if n := utf8.RuneCountInString(g.Note); n > NoteMaxRunes {
		return shared.Invalid("goal.note.length", "note", "note must be at most 300 runes")
	}
	if g.Start.IsZero() || g.End.IsZero() {
		return shared.Invalid("goal.date.empty", "date", "both days are required")
	}
	if g.End.Before(g.Start) {
		return shared.Invalid("goal.date.order", "date", "the end must not be before the start")
	}
	if g.TotalDays() > SpanMaxDays {
		return shared.Invalid("goal.date.span", "date", "a goal may span ten years at most")
	}
	return nil
}

// TotalDays is how many days the goal spans, counting both ends: a goal set and due
// on one day spans one day. A goal with no days answers 0, which is not a span any
// card is drawn from — validation rejects those first.
func (g Goal) TotalDays() int {
	if g.Start.IsZero() || g.End.IsZero() || g.End.Before(g.Start) {
		return 0
	}
	return g.End.DaysSince(g.Start) + 1
}

// Progress is what a card shows: how much of the span has passed and how much is
// left. Elapsed plus Remaining is always Total, so the two numbers on a card can
// never disagree with the bar drawn between them.
type Progress struct {
	Elapsed   int
	Total     int
	Remaining int
}

// ProgressOf counts the goal on day: no days passed before the start, all of them after the
// end, and exactly one day left on the end day itself.
func (g Goal) ProgressOf(day shared.LocalDate) Progress {
	total := g.TotalDays()
	elapsed := day.DaysSince(g.Start)
	if elapsed < 0 {
		elapsed = 0
	}
	if elapsed > total {
		elapsed = total
	}
	return Progress{Elapsed: elapsed, Total: total, Remaining: total - elapsed}
}

// Finished reports whether the goal's last day has passed. The end day itself is
// still a day to work on, so it is not finished until the next day.
func (g Goal) Finished(day shared.LocalDate) bool {
	return !g.End.IsZero() && day.After(g.End)
}

// Repository is the persistence port for goals.
type Repository interface {
	// List returns every goal, the one that ends soonest first: a countdown page is
	// read from the nearest deadline down.
	List() ([]*Goal, error)
	Get(id shared.ID) (*Goal, error)
	// Save creates when the id is unknown and updates otherwise, so callers cannot
	// pick the wrong one of two methods.
	Save(g *Goal) error
	Delete(id shared.ID) error
}
