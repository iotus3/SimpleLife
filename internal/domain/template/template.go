// Package template holds the todo templates: one named task plus a rule saying on which days
// to materialise it. It answers one question — "does this template fire on this day?" — and
// knows nothing about tasks.
package template

import (
	"strings"
	"time"
	"unicode/utf8"

	"github.com/iotus3/simplelife/app/internal/domain/shared"
)

const (
	// NameMaxRunes bounds the template name shown in lists.
	NameMaxRunes = 60
	// TitleMaxRunes matches the task title limit: a template's task title becomes
	// a task title verbatim.
	TitleMaxRunes = 200

	EveryNMin = 2
	EveryNMax = 365
)

// TriggerKind is how a rule decides its days.
type TriggerKind uint8

const (
	// TriggerDaily fires every day.
	TriggerDaily TriggerKind = iota + 1
	// TriggerWeekdays fires on selected weekdays.
	TriggerWeekdays
	// TriggerMonthly fires on one day of each month (clamped to the month's end).
	TriggerMonthly
	// TriggerSpecificDates fires on an explicit list of dates.
	TriggerSpecificDates
	// TriggerEveryNDays fires every N days counted from an anchor date.
	TriggerEveryNDays
)

// String returns the stable storage key. It is never translated: it is what the
// database and the CSV export contain.
func (k TriggerKind) String() string {
	switch k {
	case TriggerDaily:
		return "daily"
	case TriggerWeekdays:
		return "weekdays"
	case TriggerMonthly:
		return "monthly"
	case TriggerSpecificDates:
		return "dates"
	case TriggerEveryNDays:
		return "every_n_days"
	}
	return "unknown"
}

// ParseTriggerKind is the inverse of String.
func ParseTriggerKind(s string) (TriggerKind, error) {
	switch s {
	case "daily":
		return TriggerDaily, nil
	case "weekdays":
		return TriggerWeekdays, nil
	case "monthly":
		return TriggerMonthly, nil
	case "dates":
		return TriggerSpecificDates, nil
	case "every_n_days":
		return TriggerEveryNDays, nil
	}
	return 0, shared.Invalid("template.rule.kind", "kind", "unknown trigger kind: "+s)
}

// TriggerRule decides the days a template fires on. It is a pure value object:
// every branch is a function of the day alone, so it can be unit tested without
// a clock, a database or a UI.
type TriggerRule struct {
	Kind       TriggerKind
	Weekdays   []time.Weekday     // TriggerWeekdays
	DayOfMonth int                // TriggerMonthly, 1..31
	Dates      []shared.LocalDate // TriggerSpecificDates
	EveryNDays int                // TriggerEveryNDays, 2..365
	Anchor     shared.LocalDate   // TriggerEveryNDays reference day
}

// Matches reports whether the rule fires on day. An invalid or empty rule never fires, so a
// bad row cannot crash the expansion loop.
func (r TriggerRule) Matches(day shared.LocalDate) bool {
	if day.IsZero() {
		return false
	}
	switch r.Kind {
	case TriggerDaily:
		return true
	case TriggerWeekdays:
		wd := day.Weekday()
		for _, want := range r.Weekdays {
			if want == wd {
				return true
			}
		}
		return false
	case TriggerMonthly:
		if r.DayOfMonth < 1 {
			return false
		}
		want := r.DayOfMonth
		// A 31st-of-month rule still fires in February: it clamps to the last
		// day, otherwise the template would silently skip two months a year.
		if last := day.DaysInMonth(); want > last {
			want = last
		}
		return day.Day == want
	case TriggerSpecificDates:
		for _, d := range r.Dates {
			if d.Equal(day) {
				return true
			}
		}
		return false
	case TriggerEveryNDays:
		if r.EveryNDays < EveryNMin || r.Anchor.IsZero() {
			return false
		}
		diff := day.DaysSince(r.Anchor)
		// Days before the anchor never fire, and the anchor day always does.
		return diff >= 0 && diff%r.EveryNDays == 0
	}
	return false
}

// Validate checks the rule is self-consistent. The form calls it on save; the
// expander tolerates invalid rules by simply not firing them.
func (r TriggerRule) Validate() error {
	switch r.Kind {
	case TriggerDaily:
		return nil
	case TriggerWeekdays:
		if len(r.Weekdays) == 0 {
			return shared.Invalid("template.rule.weekdays", "weekdays", "pick at least one weekday")
		}
		for _, wd := range r.Weekdays {
			if wd < time.Sunday || wd > time.Saturday {
				return shared.Invalid("template.rule.weekdays", "weekdays", "weekday out of range")
			}
		}
		return nil
	case TriggerMonthly:
		if r.DayOfMonth < 1 || r.DayOfMonth > 31 {
			return shared.Invalid("template.rule.day_of_month", "day_of_month", "day of month must be 1..31")
		}
		return nil
	case TriggerSpecificDates:
		if len(r.Dates) == 0 {
			return shared.Invalid("template.rule.dates", "dates", "pick at least one date")
		}
		for _, d := range r.Dates {
			if d.IsZero() {
				return shared.Invalid("template.rule.dates", "dates", "date is empty")
			}
		}
		return nil
	case TriggerEveryNDays:
		if r.EveryNDays < EveryNMin || r.EveryNDays > EveryNMax {
			return shared.Invalid("template.rule.every_n_days", "every_n_days", "interval must be 2..365 days")
		}
		if r.Anchor.IsZero() {
			return shared.Invalid("template.rule.anchor", "anchor", "anchor date is required")
		}
		return nil
	}
	return shared.Invalid("template.rule.kind", "kind", "unknown trigger kind")
}

// Task is the one todo a template produces. One template, one todo: a user who wants three
// writes three templates.
type Task struct {
	Title         string
	Notes         string
	Priority      int // 0 = unset, 1..4
	Estimate      int // estimated pomodoros
	FocusMinutes  int // length of one pomodoro for the written task; 0 = the focus page's
	DueOffsetDays int // days after the expansion day; 0 = same day
	Tags          []string
}

// Validate checks one task definition.
func (t Task) Validate() error {
	title := strings.TrimSpace(t.Title)
	if n := utf8.RuneCountInString(title); n == 0 || n > TitleMaxRunes {
		return shared.Invalid("template.task.title", "title", "task title must be 1..200 runes")
	}
	if t.Priority < 0 || t.Priority > 4 {
		return shared.Invalid("template.task.priority", "priority", "priority must be 0..4")
	}
	if t.Estimate < 0 {
		return shared.Invalid("template.task.estimate", "estimate", "estimate must not be negative")
	}
	// Zero means "the length the focus page is set to", so only a real but out-of-range number
	// is refused; the bounds are the task's own.
	if t.FocusMinutes != 0 && (t.FocusMinutes < shared.FocusMinutesMin || t.FocusMinutes > shared.FocusMinutesMax) {
		return shared.Invalid("template.task.focus_minutes", "focus_minutes", "pomodoro length must be 1..180 minutes")
	}
	if t.DueOffsetDays < 0 || t.DueOffsetDays > 365 {
		return shared.Invalid("template.task.due_offset", "due_offset", "due offset must be 0..365 days")
	}
	return nil
}

// Template is the aggregate root: a name, a rule and the one task it writes.
type Template struct {
	ID        shared.ID
	Name      string
	Enabled   bool
	Rule      TriggerRule
	Task      Task
	CreatedAt time.Time
	UpdatedAt time.Time
}

// New builds a disabled-by-default template with no task yet: a half-finished template never
// creates tasks, and SetTask is what completes it.
func New(id shared.ID, name string, now time.Time) (*Template, error) {
	t := &Template{ID: id, Name: strings.TrimSpace(name), Rule: TriggerRule{Kind: TriggerDaily}, CreatedAt: now, UpdatedAt: now}
	if err := t.validateHeader(); err != nil {
		return nil, err
	}
	return t, nil
}

// Validate checks the name, the rule and the task.
func (t *Template) Validate() error {
	if err := t.validateHeader(); err != nil {
		return err
	}
	return t.Task.Validate()
}

// validateHeader checks everything but the task, so a draft can exist before its
// task does. It is the half of Validate that New can insist on.
func (t *Template) validateHeader() error {
	if t.ID.IsZero() {
		return shared.Invalid("template.id.empty", "", "template id is required")
	}
	name := strings.TrimSpace(t.Name)
	if n := utf8.RuneCountInString(name); n == 0 || n > NameMaxRunes {
		return shared.Invalid("template.name.length", "name", "name must be 1..60 runes")
	}
	return t.Rule.Validate()
}

// SetTask replaces the task the template writes.
func (t *Template) SetTask(task Task, now time.Time) error {
	t.Task = task
	t.UpdatedAt = now
	return t.Validate()
}

// Disable stops future expansion without touching already created tasks.
func (t *Template) Disable(now time.Time) {
	t.Enabled = false
	t.UpdatedAt = now
}

// LogEntry records one expansion run, for the "last generated" line in the UI.
// It is written by the repository's MarkExpanded and is never authoritative.
type LogEntry struct {
	TemplateID shared.ID
	Day        shared.LocalDate
	Created    int
	Skipped    int
	Failed     int
	At         time.Time
}

// Repository is the persistence port for templates.
type Repository interface {
	// ListEnabled returns only enabled templates: the expander never has to
	// filter, so a disabled template cannot fire by omission.
	ListEnabled() ([]*Template, error)
	// List returns every template, enabled or not, for the management page.
	List() ([]*Template, error)
	Get(id shared.ID) (*Template, error)
	// Save creates when the ID is unknown and updates otherwise, keeping one
	// method for both so callers cannot pick wrong.
	Save(t *Template) error
	Delete(id shared.ID) error
	// MarkExpanded writes a log row and the template's updated_at; it never
	// edits the template body, so expansion cannot corrupt user input.
	MarkExpanded(id shared.ID, day shared.LocalDate, res LogEntry) error
	RecentLogs(limit int) ([]LogEntry, error)
}
