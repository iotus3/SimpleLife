package task

import (
	"time"

	"github.com/iotus3/simplelife/app/internal/domain/shared"
)

// Ordering selects the sort applied inside one sibling group / date group.
type Ordering uint8

const (
	// OrderManual is the user's drag-and-drop order (SortKey).
	OrderManual Ordering = iota + 1
	// OrderDue sorts by due date, earliest first; undated tasks go last.
	OrderDue
	// OrderPriority sorts by urgency, P0 first, unset last.
	OrderPriority
	// OrderCreated sorts newest first.
	OrderCreated
)

// Filter is the single query object the repository understands. One struct with
// zero-value-means-unset fields beats a dozen copy-pasted query methods: the
// SQLite adapter has exactly one WHERE builder to keep correct.
type Filter struct {
	// Statuses limits the result; empty means "every status that is not archived".
	Statuses []Status
	// IncludeArchived must be set explicitly to see deleted tasks.
	IncludeArchived bool

	// ParentID filters by parent: nil means any level, a pointer to the zero ID
	// means top-level only.
	ParentID *shared.ID

	// HasDue filters on the presence of a due date (nil = either).
	HasDue *bool
	// DueFrom/DueTo select tasks whose due date falls in [DueFrom, DueTo).
	DueFrom, DueTo time.Time
	// ExcludeDueFrom/ExcludeDueTo drop tasks whose due date falls in that range.
	// This is how the history view excludes "today" in one query instead of two.
	ExcludeDueFrom, ExcludeDueTo time.Time
	// DueBefore selects overdue tasks (due strictly before the instant).
	DueBefore time.Time

	// Search matches the title or the notes, case-insensitively.
	Search string
	// Tag requires the exact tag.
	Tag string

	Limit  int
	Offset int
	Order  Ordering
}

// Today returns the filter for the "today" view: everything due on day, in manual
// order, without archived tasks. Overdue tasks are deliberately excluded — the
// view promises "today only", and the overdue count is shown as a separate banner.
func Today(day shared.LocalDate) Filter {
	return Filter{
		DueFrom: day.Midnight(),
		DueTo:   day.AddDays(1).Midnight(),
		Order:   OrderManual,
	}
}

// History returns the filter for the "history" view: every task whose due date is
// not day (including tasks with no due date at all).
func History(day shared.LocalDate) Filter {
	return Filter{
		ExcludeDueFrom: day.Midnight(),
		ExcludeDueTo:   day.AddDays(1).Midnight(),
		Order:          OrderManual,
	}
}

// Overdue returns the filter for the overdue banner: open tasks whose deadline is
// already in the past.
func Overdue(now time.Time) Filter {
	open := []Status{StatusTodo, StatusDoing}
	return Filter{Statuses: open, DueBefore: now, Order: OrderDue}
}

// Children returns the filter for the subtasks of parent.
func Children(parent shared.ID) Filter {
	return Filter{ParentID: &parent, Order: OrderManual}
}

// FinishedOn returns the filter for what was already finished on day: done tasks
// due that day. The "today" view lists them under the open ones, and the history
// view keeps them, so ticking a task off does not make it disappear.
func FinishedOn(day shared.LocalDate) Filter {
	return Filter{
		Statuses: []Status{StatusDone},
		DueFrom:  day.Midnight(),
		DueTo:    day.AddDays(1).Midnight(),
		Order:    OrderManual,
	}
}

// Repository is the persistence port for tasks. The methods are synchronous and take no
// context: this is a local single-user store.
type Repository interface {
	Create(t *Task) error
	Update(t *Task) error
	// Delete removes the row physically. Callers must have checked
	// HasFocusHistory first: a task referenced by a focus session is archived
	// instead, so history never loses its link.
	Delete(id shared.ID) error
	Get(id shared.ID) (*Task, error)
	List(f Filter) ([]*Task, error)
	Count(f Filter) (int, error)
	// CountOpen counts todo + doing tasks, the dashboard's "open" figure.
	CountOpen() (int, error)
	// HasFocusHistory reports whether any focus session references the task.
	HasFocusHistory(id shared.ID) (bool, error)
	// ExistsOccurrence is the template expander's fast path. Correctness comes
	// from uq_task_template_occurrence, not from this check.
	ExistsOccurrence(templateID shared.ID, day shared.LocalDate, seq int) (bool, error)
}
