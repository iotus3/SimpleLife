// Package task holds the todo aggregate: the only place a task's invariants are
// enforced. Every field change goes through a method, so no code path — form,
// template expansion, focus completion — can produce an inconsistent task.
package task

import (
	"strings"
	"time"
	"unicode/utf8"

	"github.com/iotus3/simplelife/app/internal/domain/shared"
)

// Status is where a task sits in its lifecycle. Archived doubles as the soft-delete state, so
// a deleted template-generated task keeps its occurrence key.
type Status uint8

const (
	StatusTodo Status = iota + 1
	StatusDoing
	StatusDone
	StatusArchived
)

// String returns the stable storage key, also used by the CSV/XLSX export. It is
// never translated.
func (s Status) String() string {
	switch s {
	case StatusTodo:
		return "todo"
	case StatusDoing:
		return "doing"
	case StatusDone:
		return "done"
	case StatusArchived:
		return "archived"
	}
	return "unknown"
}

// ParseStatus is the inverse of String.
func ParseStatus(s string) (Status, error) {
	switch s {
	case "todo":
		return StatusTodo, nil
	case "doing":
		return StatusDoing, nil
	case "done":
		return StatusDone, nil
	case "archived":
		return StatusArchived, nil
	}
	return 0, shared.Invalid("task.status.parse", "status", "unknown task status: "+s)
}

// IsOpen reports whether the task is still actionable (todo or doing).
func (s Status) IsOpen() bool { return s == StatusTodo || s == StatusDoing }

// Priority is 0 (unset) or 1..4 where 1 is the most urgent. The UI labels 1..4 as
// P0..P3; the domain keeps plain numbers so sorting never depends on labels.
type Priority uint8

// PriorityMax is the lowest urgency level; priority 0 means "not set".
const PriorityMax = 4

// Label returns "P0".."P3" for set priorities and "" for unset.
func (p Priority) Label() string {
	if p < 1 || p > PriorityMax {
		return ""
	}
	return "P" + string(rune('0'+p-1))
}

const (
	// TitleMaxRunes bounds a title so a list row stays readable and the SQLite
	// index on titles stays small.
	TitleMaxRunes = 200
	// NotesMaxRunes bounds the free-form note.
	NotesMaxRunes = 5000
	// EstimatePomodorosMax caps the estimate at one very long day.
	EstimatePomodorosMax = 20
	// FocusMinutesMin and FocusMinutesMax bound a task's own pomodoro length. The
	// numbers themselves live in domain/shared: a template carries the same length
	// for the task it writes, and the two must never disagree.
	FocusMinutesMin = shared.FocusMinutesMin
	FocusMinutesMax = shared.FocusMinutesMax
	// MaxTags bounds tag count per task.
	MaxTags = 10
	// TagMaxRunes bounds one tag.
	TagMaxRunes = 24
)

// Task is the aggregate root.
type Task struct {
	ID                shared.ID
	Title             string
	Notes             string
	Status            Status
	Priority          Priority
	ParentID          shared.ID // empty = top-level
	Tags              []string
	DueAt             time.Time // zero = no due date; otherwise the local day's 23:59:59.999
	EstimatePomodoros int
	DonePomodoros     int
	// FocusMinutes is how long one pomodoro runs for this task; 0 means "the length the focus
	// page is set to".
	FocusMinutes int
	SortKey      string // lexicographic rank inside its sibling group
	CreatedAt    time.Time
	UpdatedAt    time.Time
	CompletedAt  time.Time // zero = never completed

	// Provenance of a template-generated task. Together the three fields form the
	// idempotency key enforced by uq_task_template_occurrence, so a repeated
	// expansion cannot create a duplicate even if the use case skips its check.
	OriginTemplateID shared.ID
	OriginDate       shared.LocalDate
	OriginItemSeq    int
}

// New is the only constructor for a hand-made task.
func New(id shared.ID, title string, now time.Time) (*Task, error) {
	if id.IsZero() {
		return nil, shared.Invalid("task.id.empty", "", "task id is required")
	}
	t := &Task{
		ID:        id,
		Title:     title,
		Status:    StatusTodo,
		SortKey:   InitialSortKey,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := t.SetTitle(title, now); err != nil {
		return nil, err
	}
	return t, nil
}

// FromTemplateSpec is the slice of a template item the task aggregate needs. It
// exists so domain/task does not import domain/template: the two aggregates stay
// independent and the application layer maps between them.
type FromTemplateSpec struct {
	Title         string
	Notes         string
	Priority      int
	Estimate      int
	FocusMinutes  int // 0 = the length the focus page is set to
	DueOffsetDays int
	Tags          []string
}

// TemplateOrigin identifies which occurrence a task was materialised from.
type TemplateOrigin struct {
	TemplateID shared.ID
	Day        shared.LocalDate
	Seq        int
}

// NewFromTemplate materialises one template item into a task, writing the idempotency key
// into it: the unique index refuses a second write.
func NewFromTemplate(id shared.ID, spec FromTemplateSpec, origin TemplateOrigin, now time.Time) (*Task, error) {
	if id.IsZero() {
		return nil, shared.Invalid("task.id.empty", "", "task id is required")
	}
	if origin.TemplateID.IsZero() || origin.Day.IsZero() {
		return nil, shared.Invalid("task.origin.empty", "", "template origin is required")
	}
	t := &Task{
		ID:               id,
		Notes:            spec.Notes,
		Status:           StatusTodo,
		SortKey:          InitialSortKey,
		CreatedAt:        now,
		UpdatedAt:        now,
		OriginTemplateID: origin.TemplateID,
		OriginDate:       origin.Day,
		OriginItemSeq:    origin.Seq,
	}
	if err := t.SetTitle(spec.Title, now); err != nil {
		return nil, err
	}
	// Priority and estimate are clamped rather than rejected: a template saved
	// under an older limit must still expand, not block the whole run.
	if spec.Priority < 0 || spec.Priority > PriorityMax {
		t.Priority = 0
	} else {
		t.Priority = Priority(spec.Priority)
	}
	if spec.Estimate < 0 {
		t.EstimatePomodoros = 0
	} else if spec.Estimate > EstimatePomodorosMax {
		t.EstimatePomodoros = EstimatePomodorosMax
	} else {
		t.EstimatePomodoros = spec.Estimate
	}
	if err := t.SetTags(spec.Tags, now); err != nil {
		return nil, err
	}
	// The length is clamped for the same reason as the estimate: a template written
	// under an older bound must still expand rather than block the whole run.
	switch {
	case spec.FocusMinutes <= 0:
		t.FocusMinutes = 0
	case spec.FocusMinutes > FocusMinutesMax:
		t.FocusMinutes = FocusMinutesMax
	default:
		t.FocusMinutes = spec.FocusMinutes
	}
	t.DueAt = origin.Day.AddDays(spec.DueOffsetDays).EndOfDay()
	return t, nil
}

// IsFromTemplate reports whether this task was generated by a template.
func (t *Task) IsFromTemplate() bool { return !t.OriginTemplateID.IsZero() }

// HasDueDate reports whether the task carries a due date.
func (t *Task) HasDueDate() bool { return !t.DueAt.IsZero() }

// DueDate returns the calendar day the task is due on, or the zero date.
func (t *Task) DueDate() shared.LocalDate {
	if t.DueAt.IsZero() {
		return shared.LocalDate{}
	}
	return shared.DateOf(t.DueAt.Local())
}

// IsOverdue reports whether the task is still open and its deadline has passed.
func (t *Task) IsOverdue(now time.Time) bool {
	return t.Status.IsOpen() && !t.DueAt.IsZero() && now.After(t.DueAt)
}

// ── editing ────────────────────────────────────────────────────────────

// SetTitle validates and stores a trimmed title.
func (t *Task) SetTitle(title string, now time.Time) error {
	title = strings.TrimSpace(title)
	if n := utf8.RuneCountInString(title); n == 0 || n > TitleMaxRunes {
		return shared.Invalid("task.title.length", "title", "title must be 1..200 runes")
	}
	t.Title = title
	t.UpdatedAt = now
	return nil
}

// SetNotes validates and stores the free-form note.
func (t *Task) SetNotes(notes string, now time.Time) error {
	notes = strings.TrimSpace(notes)
	if n := utf8.RuneCountInString(notes); n > NotesMaxRunes {
		return shared.Invalid("task.notes.length", "notes", "notes must be at most 5000 runes")
	}
	t.Notes = notes
	t.UpdatedAt = now
	return nil
}

// SetPriority stores 0..4.
func (t *Task) SetPriority(p Priority, now time.Time) error {
	if p > PriorityMax {
		return shared.Invalid("task.priority.range", "priority", "priority must be 0..4")
	}
	t.Priority = p
	t.UpdatedAt = now
	return nil
}

// SetDueDate stores the due day as its local end-of-day instant. Passing the zero
// date clears the due date, which is how "no deadline" is represented.
func (t *Task) SetDueDate(day shared.LocalDate, now time.Time) error {
	if day.IsZero() {
		t.DueAt = time.Time{}
		t.UpdatedAt = now
		return nil
	}
	if day.Year < 1970 || day.Year > 9999 {
		return shared.Invalid("task.due.range", "due_at", "due date is out of range")
	}
	t.DueAt = day.EndOfDay()
	t.UpdatedAt = now
	return nil
}

// SetEstimate stores the estimated pomodoro count, 0..20.
func (t *Task) SetEstimate(n int, now time.Time) error {
	if n < 0 || n > EstimatePomodorosMax {
		return shared.Invalid("task.estimate.range", "estimate", "estimate must be 0..20")
	}
	t.EstimatePomodoros = n
	t.UpdatedAt = now
	return nil
}

// SetFocusMinutes stores how long one pomodoro runs for this task. Zero clears it,
// which means "whatever the focus page is set to"; anything else must be a length a
// session can actually be started with.
func (t *Task) SetFocusMinutes(n int, now time.Time) error {
	if n == 0 {
		t.FocusMinutes = 0
		t.UpdatedAt = now
		return nil
	}
	if n < FocusMinutesMin || n > FocusMinutesMax {
		return shared.Invalid("task.focus_minutes.range", "focus_minutes",
			"a pomodoro must be 1..180 minutes")
	}
	t.FocusMinutes = n
	t.UpdatedAt = now
	return nil
}

// SetTags normalises tags: trimmed, lower-cased for stable grouping, empties and
// duplicates dropped, at most MaxTags of at most TagMaxRunes.
func (t *Task) SetTags(tags []string, now time.Time) error {
	seen := make(map[string]bool, len(tags))
	out := make([]string, 0, len(tags))
	for _, raw := range tags {
		tag := strings.ToLower(strings.TrimSpace(raw))
		if tag == "" || seen[tag] {
			continue
		}
		if n := utf8.RuneCountInString(tag); n > TagMaxRunes {
			return shared.Invalid("task.tag.length", "tags", "tag must be at most 24 runes")
		}
		seen[tag] = true
		out = append(out, tag)
	}
	if len(out) > MaxTags {
		return shared.Invalid("task.tags.count", "tags", "at most 10 tags per task")
	}
	t.Tags = out
	t.UpdatedAt = now
	return nil
}

// ClearDue drops the due date.
func (t *Task) ClearDue(now time.Time) {
	t.DueAt = time.Time{}
	t.UpdatedAt = now
}

// ── state machine ──────────────────────────────────────────────────────

// Complete marks the task done. It is idempotent on purpose: a double click or a
// replayed event must not surface an error dialog.
func (t *Task) Complete(now time.Time) error {
	if t.Status == StatusArchived {
		return shared.Conflict("task.archived", "an archived task cannot be completed")
	}
	if t.Status == StatusDone {
		return nil
	}
	t.Status = StatusDone
	t.CompletedAt = now
	t.UpdatedAt = now
	return nil
}

// Reopen moves a done task back to todo and clears the completion stamp.
func (t *Task) Reopen(now time.Time) error {
	if t.Status == StatusArchived {
		return shared.Conflict("task.archived", "an archived task cannot be reopened")
	}
	if t.Status != StatusDone {
		return shared.Conflict("task.reopen.not_done", "only a done task can be reopened")
	}
	t.Status = StatusTodo
	t.CompletedAt = time.Time{}
	t.UpdatedAt = now
	return nil
}

// StartDoing marks the task as in progress. Idempotent.
func (t *Task) StartDoing(now time.Time) error {
	if t.Status == StatusArchived {
		return shared.Conflict("task.archived", "an archived task cannot be started")
	}
	if t.Status == StatusDone {
		return shared.Conflict("task.start.done", "a done task cannot be started; reopen it first")
	}
	if t.Status == StatusDoing {
		return nil
	}
	t.Status = StatusDoing
	t.UpdatedAt = now
	return nil
}

// Archive soft-deletes the task. It is idempotent so "delete twice" is harmless.
// The row survives, which keeps the template occurrence key occupied — that is
// what makes a deleted generated task stay deleted after a restart.
func (t *Task) Archive(now time.Time) error {
	if t.Status == StatusArchived {
		return nil
	}
	t.Status = StatusArchived
	t.UpdatedAt = now
	return nil
}

// Restore un-archives a task (undo delete).
func (t *Task) Restore(now time.Time) error {
	if t.Status != StatusArchived {
		return shared.Conflict("task.restore.not_archived", "only an archived task can be restored")
	}
	t.Status = StatusTodo
	t.CompletedAt = time.Time{}
	t.UpdatedAt = now
	return nil
}

// RecordPomodoro adds one completed pomodoro to the task. Only open tasks accept
// it: a pomodoro finished after the task was closed must not silently reopen the
// accounting. now comes from the injected clock — the domain never calls time.Now.
func (t *Task) RecordPomodoro(now time.Time) error {
	if !t.Status.IsOpen() {
		return shared.Conflict("task.record.not_open", "only an open task accumulates pomodoros")
	}
	t.DonePomodoros++
	t.UpdatedAt = now
	return nil
}

// SetSortKey stores a new lexicographic rank (see RankBetween).
func (t *Task) SetSortKey(key string, now time.Time) error {
	key = strings.TrimSpace(key)
	if key == "" {
		return shared.Invalid("task.sortkey.empty", "", "sort key is required")
	}
	t.SortKey = key
	t.UpdatedAt = now
	return nil
}

// AttachParent makes the task a child of parent. Two levels only: a subtask
// cannot itself have children, so the list rendering stays predictable.
func (t *Task) AttachParent(parent *Task, now time.Time) error {
	if parent == nil {
		t.ParentID = ""
		t.UpdatedAt = now
		return nil
	}
	if parent.ID == t.ID {
		return shared.Invalid("task.parent.self", "parent_id", "a task cannot be its own parent")
	}
	if parent.ParentID != "" {
		return shared.Invalid("task.parent.depth", "parent_id", "subtasks cannot have subtasks")
	}
	if parent.Status == StatusArchived {
		return shared.Invalid("task.parent.archived", "parent_id", "parent task is deleted")
	}
	t.ParentID = parent.ID
	t.UpdatedAt = now
	return nil
}

// DetachParent promotes the task back to top level.
func (t *Task) DetachParent(now time.Time) {
	t.ParentID = ""
	t.UpdatedAt = now
}
