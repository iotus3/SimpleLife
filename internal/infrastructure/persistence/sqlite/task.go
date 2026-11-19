package sqlite

import (
	"database/sql"
	"strconv"
	"strings"

	"github.com/iotus3/simplelife/app/internal/domain/shared"
	task "github.com/iotus3/simplelife/app/internal/domain/task"
)

// taskColumns is the projection every task read uses. Spelling it once keeps the
// scan order and the INSERT column order from drifting apart.
const taskColumns = `id, title, notes, status, priority, parent_id, tags, due_at,
	estimate_pomodoros, focus_minutes, done_pomodoros, sort_key, created_at, updated_at,
	completed_at, origin_template_id, origin_date, origin_item_seq`

// Create inserts a task. The partial unique index on the occurrence key is what
// actually stops a duplicate expansion; the error is translated to ErrDuplicate so
// the expander can treat it as "already done".
func (r TaskRepo) Create(t *task.Task) error {
	_, err := r.exec(`INSERT INTO task (`+taskColumns+`)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		string(t.ID), t.Title, t.Notes, int(t.Status), int(t.Priority), nullable(t.ParentID),
		tags(t.Tags), ms(t.DueAt), t.EstimatePomodoros, t.FocusMinutes, t.DonePomodoros,
		t.SortKey, ms(t.CreatedAt), ms(t.UpdatedAt), ms(t.CompletedAt),
		string(t.OriginTemplateID), date(t.OriginDate), originSeq(t))
	return err
}

// originSeq stores -1 for "not from a template", matching the DDL default, so the
// unique index never treats two ordinary tasks as the same occurrence.
func originSeq(t *task.Task) int {
	if t.OriginTemplateID == "" {
		return -1
	}
	return t.OriginItemSeq
}

// Update writes every mutable column.
func (r TaskRepo) Update(t *task.Task) error {
	res, err := r.exec(`UPDATE task SET
		title = ?, notes = ?, status = ?, priority = ?, parent_id = ?, tags = ?, due_at = ?,
		estimate_pomodoros = ?, focus_minutes = ?, done_pomodoros = ?, sort_key = ?,
		updated_at = ?, completed_at = ?, origin_template_id = ?, origin_date = ?,
		origin_item_seq = ?
		WHERE id = ?`,
		t.Title, t.Notes, int(t.Status), int(t.Priority), nullable(t.ParentID), tags(t.Tags),
		ms(t.DueAt), t.EstimatePomodoros, t.FocusMinutes, t.DonePomodoros, t.SortKey,
		ms(t.UpdatedAt), ms(t.CompletedAt), string(t.OriginTemplateID), date(t.OriginDate),
		originSeq(t), string(t.ID))
	if err != nil {
		return err
	}
	return requireAffected(res, "task.not_found", "task not found")
}

// Delete removes the row. Subtasks cascade, which is why the use case archives
// instead when a task has children or focus history.
func (r TaskRepo) Delete(id shared.ID) error {
	res, err := r.exec(`DELETE FROM task WHERE id = ?`, string(id))
	if err != nil {
		return err
	}
	return requireAffected(res, "task.not_found", "task not found")
}

// Get returns one task.
func (r TaskRepo) Get(id shared.ID) (*task.Task, error) {
	row := r.queryRow(`SELECT `+taskColumns+` FROM task WHERE id = ?`, string(id))
	t, err := scanTask(row)
	if err != nil {
		if err == shared.ErrNotFound || isNoRows(err) {
			return nil, shared.NotFound("task.not_found", "task not found")
		}
		return nil, err
	}
	return t, nil
}

// List runs the filter. The WHERE builder is shared with Count so the two can
// never disagree about what they are counting.
func (r TaskRepo) List(f task.Filter) ([]*task.Task, error) {
	where, args := taskWhere(f)
	query := `SELECT ` + taskColumns + ` FROM task` + where + taskOrder(f.Order) + limitClause(f.Limit, f.Offset)
	rows, err := r.query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*task.Task
	for rows.Next() {
		t, err := scanTask(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// Count counts the matching tasks.
func (r TaskRepo) Count(f task.Filter) (int, error) {
	where, args := taskWhere(f)
	var n int
	if err := r.queryRow(`SELECT COUNT(*) FROM task`+where, args...).Scan(&n); err != nil {
		return 0, mapError("count tasks", err)
	}
	return n, nil
}

// CountOpen counts todo and doing tasks.
func (r TaskRepo) CountOpen() (int, error) {
	var n int
	if err := r.queryRow(`SELECT COUNT(*) FROM task WHERE status IN (?, ?)`,
		int(task.StatusTodo), int(task.StatusDoing)).Scan(&n); err != nil {
		return 0, mapError("count open tasks", err)
	}
	return n, nil
}

// HasFocusHistory reports whether a session references the task.
func (r TaskRepo) HasFocusHistory(id shared.ID) (bool, error) {
	var n int
	if err := r.queryRow(`SELECT COUNT(*) FROM focus_session WHERE task_id = ?`,
		string(id)).Scan(&n); err != nil {
		return false, mapError("count task sessions", err)
	}
	return n > 0, nil
}

// ExistsOccurrence reports whether the template already produced this item for
// this day. It is the expander's fast path; the unique index is the guarantee.
func (r TaskRepo) ExistsOccurrence(templateID shared.ID, day shared.LocalDate, seq int) (bool, error) {
	var n int
	err := r.queryRow(`SELECT COUNT(*) FROM task
		WHERE origin_template_id = ? AND origin_date = ? AND origin_item_seq = ?`,
		string(templateID), date(day), seq).Scan(&n)
	if err != nil {
		return false, mapError("count task occurrence", err)
	}
	return n > 0, nil
}

// taskWhere builds the WHERE clause for a filter. Archived tasks are excluded unless asked for,
// and that rule comes before any other.
func taskWhere(f task.Filter) (string, []any) {
	var clauses []string
	var args []any
	switch {
	case len(f.Statuses) > 0:
		// An explicit status list is taken literally: asking for done tasks must not
		// silently add "and not archived", and asking for archived must work.
		holders := make([]string, len(f.Statuses))
		for i, s := range f.Statuses {
			holders[i] = "?"
			args = append(args, int(s))
		}
		clauses = append(clauses, "status IN ("+strings.Join(holders, ", ")+")")
	case !f.IncludeArchived:
		clauses = append(clauses, "status <> ?")
		args = append(args, int(task.StatusArchived))
	}
	if f.ParentID != nil {
		if f.ParentID.IsZero() {
			clauses = append(clauses, "parent_id IS NULL")
		} else {
			clauses = append(clauses, "parent_id = ?")
			args = append(args, string(*f.ParentID))
		}
	}
	if f.HasDue != nil {
		if *f.HasDue {
			clauses = append(clauses, "due_at <> 0")
		} else {
			clauses = append(clauses, "due_at = 0")
		}
	}
	if !f.DueFrom.IsZero() {
		clauses = append(clauses, "due_at <> 0 AND due_at >= ?")
		args = append(args, ms(f.DueFrom))
	}
	if !f.DueTo.IsZero() {
		clauses = append(clauses, "due_at <> 0 AND due_at < ?")
		args = append(args, ms(f.DueTo))
	}
	if !f.ExcludeDueFrom.IsZero() || !f.ExcludeDueTo.IsZero() {
		// The history view: "not in this range", and undated tasks always qualify.
		clauses = append(clauses, "(due_at = 0 OR NOT (due_at >= ? AND due_at < ?))")
		args = append(args, ms(f.ExcludeDueFrom), ms(f.ExcludeDueTo))
	}
	if !f.DueBefore.IsZero() {
		clauses = append(clauses, "due_at <> 0 AND due_at < ?")
		args = append(args, ms(f.DueBefore))
	}
	if f.Search != "" {
		// LIKE with an escaped pattern: a user searching for "100%" must not turn
		// into a wildcard.
		needle := "%" + escapeLike(strings.ToLower(f.Search)) + "%"
		clauses = append(clauses, `(LOWER(title) LIKE ? ESCAPE '\' OR LOWER(notes) LIKE ? ESCAPE '\')`)
		args = append(args, needle, needle)
	}
	if f.Tag != "" {
		clauses = append(clauses, "EXISTS (SELECT 1 FROM json_each(task.tags) WHERE json_each.value = ?)")
		args = append(args, f.Tag)
	}
	if len(clauses) == 0 {
		return "", nil
	}
	return " WHERE " + strings.Join(clauses, " AND "), args
}

// taskOrder renders the ORDER BY: priorities sort P1 first with unset last, created_at sorts
// newest first with the sort key as a stable tie-break.
func taskOrder(o task.Ordering) string {
	switch o {
	case task.OrderDue:
		return " ORDER BY (due_at = 0) ASC, due_at ASC, sort_key ASC, id ASC"
	case task.OrderPriority:
		return " ORDER BY CASE WHEN priority = 0 THEN 99 ELSE priority END ASC, sort_key ASC, id ASC"
	case task.OrderCreated:
		return " ORDER BY created_at DESC, sort_key ASC, id ASC"
	default:
		return " ORDER BY sort_key ASC, id ASC"
	}
}

// scanTask reads one row in taskColumns order.
func scanTask(s scanner) (*task.Task, error) {
	var (
		id, title, notes, sortKey string
		status, priority          int
		parent                    sql.NullString
		tagJSON                   string
		dueAt, createdAt          int64
		updatedAt, completedAt    int64
		estimate, focusMinutes    int
		done                      int
		originTemplate, originDay string
		originSeqValue            int
	)
	err := s.Scan(&id, &title, &notes, &status, &priority, &parent, &tagJSON, &dueAt,
		&estimate, &focusMinutes, &done, &sortKey, &createdAt, &updatedAt, &completedAt,
		&originTemplate, &originDay, &originSeqValue)
	if err != nil {
		if isNoRows(err) {
			return nil, shared.ErrNotFound
		}
		return nil, mapError("scan task", err)
	}
	originDate, err := toDate(originDay)
	if err != nil {
		return nil, err
	}
	t := &task.Task{
		ID:                shared.ID(id),
		Title:             title,
		Notes:             notes,
		Status:            task.Status(status),
		Priority:          task.Priority(priority),
		ParentID:          idOrEmpty(parent),
		Tags:              toTags(tagJSON),
		DueAt:             toTime(dueAt),
		EstimatePomodoros: estimate,
		FocusMinutes:      focusMinutes,
		DonePomodoros:     done,
		SortKey:           sortKey,
		CreatedAt:         toTime(createdAt),
		UpdatedAt:         toTime(updatedAt),
		CompletedAt:       toTime(completedAt),
		OriginTemplateID:  shared.ID(originTemplate),
		OriginDate:        originDate,
		OriginItemSeq:     originSeqValue,
	}
	return t, nil
}

// ── shared SQL helpers ─────────────────────────────────────────────────

// scanner is what both *sql.Row and *sql.Rows provide.
type scanner interface {
	Scan(dest ...any) error
}

// isNoRows reports the driver's "no rows" error.
func isNoRows(err error) bool { return err == sql.ErrNoRows }

// requireAffected turns "0 rows changed" into the domain's not-found error, so an
// update of a deleted row fails loudly instead of silently doing nothing.
func requireAffected(res sql.Result, code, msg string) error {
	n, err := res.RowsAffected()
	if err != nil {
		// The driver cannot always report it; the write itself did not fail.
		return nil
	}
	if n == 0 {
		return shared.NotFound(code, msg)
	}
	return nil
}

// limitClause renders LIMIT/OFFSET. A negative offset is clamped to zero because
// SQLite rejects it and the value comes from a page state that may underflow.
func limitClause(limit, offset int) string {
	if offset < 0 {
		offset = 0
	}
	switch {
	case limit > 0 && offset > 0:
		return " LIMIT " + strconv.Itoa(limit) + " OFFSET " + strconv.Itoa(offset)
	case limit > 0:
		return " LIMIT " + strconv.Itoa(limit)
	case offset > 0:
		return " LIMIT -1 OFFSET " + strconv.Itoa(offset)
	}
	return ""
}

// escapeLike escapes the LIKE wildcards so a search for "50%" stays literal.
func escapeLike(s string) string {
	var b strings.Builder
	b.Grow(len(s) + 8)
	for _, r := range s {
		switch r {
		case '%', '_', '\\':
			b.WriteByte('\\')
		}
		b.WriteRune(r)
	}
	return b.String()
}
