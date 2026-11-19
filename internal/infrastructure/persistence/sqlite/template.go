package sqlite

import (
	"database/sql"

	"github.com/iotus3/simplelife/app/internal/domain/shared"
	"github.com/iotus3/simplelife/app/internal/domain/template"
)

const templateColumns = `id, name, enabled, kind, weekdays, day_of_month, dates,
	every_n_days, anchor, created_at, updated_at,
	task_title, task_notes, task_priority, task_estimate, task_focus_minutes,
	task_due_offset_days, task_tags`

// Save writes the template. The task it writes lives on the template row: one
// template, one task, so there is no child table to keep in step.
func (r TemplateRepo) Save(t *template.Template) error {
	var exists int
	if err := r.queryRow(`SELECT COUNT(*) FROM template WHERE id = ?`, string(t.ID)).Scan(&exists); err != nil {
		return mapError("check template", err)
	}
	if exists == 0 {
		_, err := r.exec(`INSERT INTO template (`+templateColumns+`)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			string(t.ID), t.Name, boolToInt(t.Enabled), int(t.Rule.Kind),
			weekdays(t.Rule.Weekdays), t.Rule.DayOfMonth, dates(t.Rule.Dates),
			t.Rule.EveryNDays, date(t.Rule.Anchor), ms(t.CreatedAt), ms(t.UpdatedAt),
			t.Task.Title, t.Task.Notes, t.Task.Priority, t.Task.Estimate,
			t.Task.FocusMinutes, t.Task.DueOffsetDays, tags(t.Task.Tags))
		return err
	}
	_, err := r.exec(`UPDATE template SET name = ?, enabled = ?, kind = ?, weekdays = ?,
		day_of_month = ?, dates = ?, every_n_days = ?, anchor = ?, updated_at = ?,
		task_title = ?, task_notes = ?, task_priority = ?, task_estimate = ?,
		task_focus_minutes = ?, task_due_offset_days = ?, task_tags = ?
		WHERE id = ?`,
		t.Name, boolToInt(t.Enabled), int(t.Rule.Kind), weekdays(t.Rule.Weekdays),
		t.Rule.DayOfMonth, dates(t.Rule.Dates), t.Rule.EveryNDays, date(t.Rule.Anchor),
		ms(t.UpdatedAt), t.Task.Title, t.Task.Notes, t.Task.Priority, t.Task.Estimate,
		t.Task.FocusMinutes, t.Task.DueOffsetDays, tags(t.Task.Tags), string(t.ID))
	return err
}

// Delete removes a template. Its logs cascade.
func (r TemplateRepo) Delete(id shared.ID) error {
	res, err := r.exec(`DELETE FROM template WHERE id = ?`, string(id))
	if err != nil {
		return err
	}
	return requireAffected(res, "template.not_found", "template not found")
}

// Get returns one template.
func (r TemplateRepo) Get(id shared.ID) (*template.Template, error) {
	t, err := scanTemplate(r.queryRow(`SELECT `+templateColumns+` FROM template WHERE id = ?`, string(id)))
	if err != nil {
		if isNoRows(err) || err == shared.ErrNotFound {
			return nil, shared.NotFound("template.not_found", "template not found")
		}
		return nil, err
	}
	return t, nil
}

// List returns every template, oldest first.
func (r TemplateRepo) List() ([]*template.Template, error) {
	return r.list(`SELECT ` + templateColumns + ` FROM template ORDER BY created_at ASC, id ASC`)
}

// ListEnabled returns only the enabled templates: the expander must never be the
// component that remembers to filter.
func (r TemplateRepo) ListEnabled() ([]*template.Template, error) {
	return r.list(`SELECT ` + templateColumns + ` FROM template WHERE enabled = 1
		ORDER BY created_at ASC, id ASC`)
}

func (r TemplateRepo) list(query string, args ...any) ([]*template.Template, error) {
	rows, err := r.query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*template.Template
	for rows.Next() {
		t, err := scanTemplate(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// MarkExpanded writes a log row and stamps the template.
func (r TemplateRepo) MarkExpanded(id shared.ID, day shared.LocalDate, entry template.LogEntry) error {
	entry.TemplateID = id
	if entry.Day.IsZero() {
		entry.Day = day
	}
	// The log is append-only: two expansions of the same template (the midnight
	// timer and a manual run) each keep their own row, which is why the primary key
	// is an autoincrement counter rather than a timestamp.
	if entry.At.IsZero() {
		return shared.Invalid("template.log.at", "", "log timestamp is required")
	}
	// The template is stamped first: updating a row that is not there is how the
	// "no such template" error is produced, and it must be reported before the log
	// row is attempted (the foreign key would otherwise turn it into a conflict).
	res, err := r.exec(`UPDATE template SET updated_at = ? WHERE id = ?`, ms(entry.At), string(id))
	if err != nil {
		return err
	}
	if err := requireAffected(res, "template.not_found", "template not found"); err != nil {
		return err
	}
	_, err = r.exec(`INSERT INTO template_log
		(template_id, expand_date, created, skipped, failed, detail, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		string(id), entry.Day.String(), entry.Created, entry.Skipped, entry.Failed,
		"", ms(entry.At))
	return err
}

// RecentLogs returns the newest log rows.
func (r TemplateRepo) RecentLogs(limit int) ([]template.LogEntry, error) {
	rows, err := r.query(`SELECT template_id, expand_date, created, skipped, failed, detail, created_at
		FROM template_log ORDER BY created_at DESC, id DESC` + limitClause(limit, 0))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []template.LogEntry
	for rows.Next() {
		var (
			templateID, expandDate   string
			created, skipped, failed int
			at                       int64
		)
		var detail string
		if err := rows.Scan(&templateID, &expandDate, &created, &skipped, &failed, &detail, &at); err != nil {
			return nil, mapError("scan template log", err)
		}
		day, err := toDate(expandDate)
		if err != nil {
			return nil, err
		}
		out = append(out, template.LogEntry{
			TemplateID: shared.ID(templateID),
			Day:        day,
			Created:    created,
			Skipped:    skipped,
			Failed:     failed,
			At:         toTime(at),
		})
	}
	return out, rows.Err()
}

func scanTemplate(s scanner) (*template.Template, error) {
	var (
		id, name, weekdayJSON, dateJSON string
		anchor                          string
		enabled, kind, dayOfMonth       int
		everyN                          int
		createdAt, updatedAt            int64
		taskTitle, taskNotes, taskTags  string
		taskPriority, taskEstimate      int
		taskFocusMinutes, taskDueOffset int
	)
	if err := s.Scan(&id, &name, &enabled, &kind, &weekdayJSON, &dayOfMonth, &dateJSON,
		&everyN, &anchor, &createdAt, &updatedAt,
		&taskTitle, &taskNotes, &taskPriority, &taskEstimate, &taskFocusMinutes,
		&taskDueOffset, &taskTags); err != nil {
		if isNoRows(err) {
			return nil, err
		}
		return nil, mapError("scan template", err)
	}
	anchorDate, err := toDate(anchor)
	if err != nil {
		return nil, err
	}
	return &template.Template{
		ID:      shared.ID(id),
		Name:    name,
		Enabled: intToBool(enabled),
		Rule: template.TriggerRule{
			Kind:       template.TriggerKind(kind),
			Weekdays:   toWeekdays(weekdayJSON),
			DayOfMonth: dayOfMonth,
			Dates:      toDates(dateJSON),
			EveryNDays: everyN,
			Anchor:     anchorDate,
		},
		Task: template.Task{
			Title:         taskTitle,
			Notes:         taskNotes,
			Priority:      taskPriority,
			Estimate:      taskEstimate,
			FocusMinutes:  taskFocusMinutes,
			DueOffsetDays: taskDueOffset,
			Tags:          toTags(taskTags),
		},
		CreatedAt: toTime(createdAt),
		UpdatedAt: toTime(updatedAt),
	}, nil
}

// rowsScanner is what *sql.Rows provides; naming it keeps the collect helpers from
// importing database/sql just for a type name.
type rowsScanner interface {
	Next() bool
	Scan(dest ...any) error
	Close() error
	Err() error
}

var _ rowsScanner = (*sql.Rows)(nil)
