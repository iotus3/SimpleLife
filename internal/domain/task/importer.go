package task

import (
	"strings"
	"time"
	"unicode/utf8"

	"github.com/iotus3/simplelife/app/internal/domain/shared"
)

// ImportSpec is one task as an export wrote it, counters and timestamps included: replaying
// the editing operations would stamp today's clock over the original dates.
type ImportSpec struct {
	ID       shared.ID
	Title    string
	Notes    string
	Status   Status
	Priority Priority
	Tags     []string
	DueDate  shared.LocalDate // zero = no due date
	Estimate int
	Done     int
	// FocusMinutes is the task's own pomodoro length; 0 means "the default".
	FocusMinutes int
	SortKey      string
	CreatedAt    time.Time
	UpdatedAt    time.Time
	Completed    time.Time // zero = never completed
}

// importSortKey is the rank a restored task gets when the export carries none.
const importSortKey = InitialSortKey

// Imported rebuilds a task from an export, validating every field with the same
// rules the live aggregate uses. now is the fallback for a timestamp the export
// did not record.
func Imported(spec ImportSpec, now time.Time) (*Task, error) {
	if spec.ID.IsZero() {
		return nil, shared.Invalid("task.id.empty", "", "task id is required")
	}
	title := strings.TrimSpace(spec.Title)
	if n := utf8.RuneCountInString(title); n == 0 || n > TitleMaxRunes {
		return nil, shared.Invalid("task.title.length", "title", "title must be 1..120 runes")
	}
	if spec.Notes != "" {
		if n := utf8.RuneCountInString(strings.TrimSpace(spec.Notes)); n > NotesMaxRunes {
			return nil, shared.Invalid("task.notes.length", "notes", "notes are too long")
		}
	}
	if spec.Priority > PriorityMax {
		return nil, shared.Invalid("task.priority.range", "priority", "priority must be 0..4")
	}
	if spec.Estimate < 0 || spec.Estimate > EstimatePomodorosMax {
		return nil, shared.Invalid("task.estimate.range", "estimate", "estimate is out of range")
	}
	if spec.FocusMinutes < 0 || spec.FocusMinutes > FocusMinutesMax {
		return nil, shared.Invalid("task.focus_minutes.range", "focus_minutes", "a pomodoro is out of range")
	}
	if spec.Done < 0 || spec.Done > EstimatePomodorosMax {
		return nil, shared.Invalid("task.done.range", "done", "completed pomodoros are out of range")
	}
	if spec.Status < StatusTodo || spec.Status > StatusArchived {
		return nil, shared.Invalid("task.status.parse", "status", "unknown task status")
	}
	tags, err := normalizeTags(spec.Tags)
	if err != nil {
		return nil, err
	}

	created := spec.CreatedAt
	if created.IsZero() {
		created = now
	}
	updated := spec.UpdatedAt
	if updated.IsZero() {
		updated = created
	}
	t := &Task{
		ID:                spec.ID,
		Title:             title,
		Notes:             strings.TrimSpace(spec.Notes),
		Status:            spec.Status,
		Priority:          spec.Priority,
		Tags:              tags,
		EstimatePomodoros: spec.Estimate,
		FocusMinutes:      spec.FocusMinutes,
		DonePomodoros:     spec.Done,
		SortKey:           spec.SortKey,
		CreatedAt:         created,
		UpdatedAt:         updated,
		CompletedAt:       spec.Completed,
	}
	if t.SortKey == "" {
		t.SortKey = importSortKey
	}
	if !spec.DueDate.IsZero() {
		t.DueAt = spec.DueDate.EndOfDay()
	}
	if t.Status == StatusDone && t.CompletedAt.IsZero() {
		// A done task with no completion date is what an incomplete export looks
		// like; keeping "done" is what matters, so the date falls back to updated.
		t.CompletedAt = updated
	}
	return t, nil
}

// normalizeTags folds tags the way the live aggregate does — trimmed, lower-cased
// and de-duplicated — and refuses the same two things it refuses: a tag longer
// than a tag may be, and more tags than a task may carry.
func normalizeTags(tags []string) ([]string, error) {
	seen := make(map[string]bool, len(tags))
	out := make([]string, 0, len(tags))
	for _, raw := range tags {
		tag := strings.ToLower(strings.TrimSpace(raw))
		if tag == "" || seen[tag] {
			continue
		}
		if n := utf8.RuneCountInString(tag); n > TagMaxRunes {
			return nil, shared.Invalid("task.tag.length", "tags", "tag must be at most 24 runes")
		}
		seen[tag] = true
		out = append(out, tag)
	}
	if len(out) > MaxTags {
		return nil, shared.Invalid("task.tags.count", "tags", "at most 10 tags per task")
	}
	return out, nil
}
