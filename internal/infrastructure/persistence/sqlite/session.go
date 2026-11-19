package sqlite

import (
	"database/sql"
	"time"

	focus "github.com/iotus3/simplelife/app/internal/domain/focus"
	"github.com/iotus3/simplelife/app/internal/domain/shared"
)

const sessionColumns = `id, task_id, predecessor_id, kind, phase, planned_ms, started_at,
	ended_at, paused_total_ms, paused_at, interruptions, note`

// Create inserts a session. A second running or paused row is rejected by the
// partial unique index, so "one active session" holds even if the use case is
// called twice at once.
func (r SessionRepo) Create(s *focus.Session) error {
	_, err := r.exec(`INSERT INTO focus_session (`+sessionColumns+`)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		string(s.ID), nullable(s.TaskID), nullable(s.PredecessorID), int(s.Kind), int(s.Phase),
		durMs(s.Planned), ms(s.StartedAt), ms(s.EndedAt), durMs(s.PausedTotal),
		ms(s.PausedAt), s.Interruptions, s.Note)
	return err
}

// Update rewrites a session.
func (r SessionRepo) Update(s *focus.Session) error {
	res, err := r.exec(`UPDATE focus_session SET
		task_id = ?, predecessor_id = ?, kind = ?, phase = ?, planned_ms = ?, started_at = ?,
		ended_at = ?, paused_total_ms = ?, paused_at = ?, interruptions = ?, note = ?
		WHERE id = ?`,
		nullable(s.TaskID), nullable(s.PredecessorID), int(s.Kind), int(s.Phase),
		durMs(s.Planned), ms(s.StartedAt), ms(s.EndedAt), durMs(s.PausedTotal),
		ms(s.PausedAt), s.Interruptions, s.Note, string(s.ID))
	if err != nil {
		return err
	}
	return requireAffected(res, "focus.not_found", "session not found")
}

// Get returns one session.
func (r SessionRepo) Get(id shared.ID) (*focus.Session, error) {
	s, err := scanSession(r.queryRow(`SELECT `+sessionColumns+` FROM focus_session WHERE id = ?`, string(id)))
	if err != nil {
		if isNoRows(err) || err == shared.ErrNotFound {
			return nil, shared.NotFound("focus.not_found", "session not found")
		}
		return nil, err
	}
	return s, nil
}

// Active returns the running or paused session, or nil when the timer is idle.
func (r SessionRepo) Active() (*focus.Session, error) {
	rows, err := r.query(`SELECT `+sessionColumns+` FROM focus_session
		WHERE phase IN (?, ?) LIMIT 1`, int(focus.PhaseRunning), int(focus.PhasePaused))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	if !rows.Next() {
		return nil, rows.Err()
	}
	s, err := scanSession(rows)
	if err != nil {
		return nil, err
	}
	return s, rows.Err()
}

// ListByRange returns sessions started in [from, to), newest first.
func (r SessionRepo) ListByRange(from, to time.Time) ([]*focus.Session, error) {
	rows, err := r.query(`SELECT `+sessionColumns+` FROM focus_session
		WHERE started_at >= ? AND started_at < ? ORDER BY started_at DESC, id DESC`,
		ms(from), ms(to))
	if err != nil {
		return nil, err
	}
	return collectSessions(rows)
}

// ListRecent returns the newest sessions.
func (r SessionRepo) ListRecent(limit int) ([]*focus.Session, error) {
	rows, err := r.query(`SELECT ` + sessionColumns + ` FROM focus_session
		ORDER BY started_at DESC, id DESC` + limitClause(limit, 0))
	if err != nil {
		return nil, err
	}
	return collectSessions(rows)
}

// FocusCountBetween counts completed focus sessions started in [from, to).
func (r SessionRepo) FocusCountBetween(from, to time.Time) (int, error) {
	var n int
	err := r.queryRow(`SELECT COUNT(*) FROM focus_session
		WHERE kind = ? AND phase = ? AND started_at >= ? AND started_at < ?`,
		int(focus.KindFocus), int(focus.PhaseDone), ms(from), ms(to)).Scan(&n)
	if err != nil {
		return 0, mapError("count focus sessions", err)
	}
	return n, nil
}

// FocusMinutesBetween sums credited focus minutes in [from, to). The minutes are computed in Go:
// the rule depends on how a session ended, and a second copy in SQL would drift.
func (r SessionRepo) FocusMinutesBetween(from, to time.Time) (int, error) {
	rows, err := r.query(`SELECT `+sessionColumns+` FROM focus_session
		WHERE kind = ? AND phase = ? AND started_at >= ? AND started_at < ?`,
		int(focus.KindFocus), int(focus.PhaseDone), ms(from), ms(to))
	if err != nil {
		return 0, err
	}
	return sumFocusMinutes(rows)
}

// TaskFocusMinutes sums credited minutes for one task.
func (r SessionRepo) TaskFocusMinutes(taskID shared.ID) (int, error) {
	rows, err := r.query(`SELECT `+sessionColumns+` FROM focus_session WHERE task_id = ?`,
		string(taskID))
	if err != nil {
		return 0, err
	}
	return sumFocusMinutes(rows)
}

func sumFocusMinutes(rows *sql.Rows) (int, error) {
	sessions, err := collectSessions(rows)
	if err != nil {
		return 0, err
	}
	total := 0
	for _, s := range sessions {
		total += s.CompletedFocusMinutes()
	}
	return total, nil
}

func collectSessions(rows *sql.Rows) ([]*focus.Session, error) {
	defer rows.Close()
	var out []*focus.Session
	for rows.Next() {
		s, err := scanSession(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

func scanSession(s scanner) (*focus.Session, error) {
	var (
		id, note                   string
		taskID, predecessor        sql.NullString
		kind, phase, interruptions int
		plannedMs, startedAt       int64
		endedAt, pausedTotalMs     int64
		pausedAt                   int64
	)
	if err := s.Scan(&id, &taskID, &predecessor, &kind, &phase, &plannedMs, &startedAt,
		&endedAt, &pausedTotalMs, &pausedAt, &interruptions, &note); err != nil {
		if isNoRows(err) {
			return nil, err
		}
		return nil, mapError("scan session", err)
	}
	return &focus.Session{
		ID:            shared.ID(id),
		TaskID:        idOrEmpty(taskID),
		PredecessorID: idOrEmpty(predecessor),
		Kind:          focus.Kind(kind),
		Phase:         focus.Phase(phase),
		Planned:       toDuration(plannedMs),
		StartedAt:     toTime(startedAt),
		EndedAt:       toTime(endedAt),
		PausedTotal:   toDuration(pausedTotalMs),
		PausedAt:      toTime(pausedAt),
		Interruptions: interruptions,
		Note:          note,
	}, nil
}
