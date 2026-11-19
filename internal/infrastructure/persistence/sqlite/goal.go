package sqlite

import (
	"github.com/iotus3/simplelife/app/internal/domain/goal"
	"github.com/iotus3/simplelife/app/internal/domain/shared"
)

const goalColumns = `id, name, note, start_date, end_date, created_at, updated_at`

// Save writes the goal, creating the row when it is new.
func (r GoalRepo) Save(g *goal.Goal) error {
	var exists int
	if err := r.queryRow(`SELECT COUNT(*) FROM goal WHERE id = ?`, string(g.ID)).Scan(&exists); err != nil {
		return mapError("check goal", err)
	}
	if exists == 0 {
		_, err := r.exec(`INSERT INTO goal (`+goalColumns+`) VALUES (?, ?, ?, ?, ?, ?, ?)`,
			string(g.ID), g.Name, g.Note, date(g.Start), date(g.End), ms(g.CreatedAt), ms(g.UpdatedAt))
		return err
	}
	// created_at is not written on update: the day a goal was set is part of what the
	// card reports, and an edit must not move it.
	_, err := r.exec(`UPDATE goal SET name = ?, note = ?, start_date = ?, end_date = ?, updated_at = ?
		WHERE id = ?`,
		g.Name, g.Note, date(g.Start), date(g.End), ms(g.UpdatedAt), string(g.ID))
	return err
}

// Delete removes a goal. Unlike a task there is no history to keep: nothing else
// points at a goal, so it simply goes.
func (r GoalRepo) Delete(id shared.ID) error {
	res, err := r.exec(`DELETE FROM goal WHERE id = ?`, string(id))
	if err != nil {
		return err
	}
	return requireAffected(res, "goal.not_found", "goal not found")
}

// Get returns one goal.
func (r GoalRepo) Get(id shared.ID) (*goal.Goal, error) {
	g, err := scanGoal(r.queryRow(`SELECT `+goalColumns+` FROM goal WHERE id = ?`, string(id)))
	if err != nil {
		if isNoRows(err) || err == shared.ErrNotFound {
			return nil, shared.NotFound("goal.not_found", "goal not found")
		}
		return nil, err
	}
	return g, nil
}

// List returns every goal, the nearest deadline first: a countdown page is read
// from the soonest down. The id breaks ties so two goals ending on one day keep a
// stable order between reads.
func (r GoalRepo) List() ([]*goal.Goal, error) {
	rows, err := r.query(`SELECT ` + goalColumns + ` FROM goal ORDER BY end_date ASC, created_at ASC, id ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*goal.Goal
	for rows.Next() {
		g, err := scanGoal(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, g)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func scanGoal(s scanner) (*goal.Goal, error) {
	var (
		id, name, note, start, end string
		createdAt, updatedAt       int64
	)
	if err := s.Scan(&id, &name, &note, &start, &end, &createdAt, &updatedAt); err != nil {
		if isNoRows(err) {
			return nil, err
		}
		return nil, mapError("scan goal", err)
	}
	startDate, err := toDate(start)
	if err != nil {
		return nil, err
	}
	endDate, err := toDate(end)
	if err != nil {
		return nil, err
	}
	return &goal.Goal{
		ID:        shared.ID(id),
		Name:      name,
		Note:      note,
		Start:     startDate,
		End:       endDate,
		CreatedAt: toTime(createdAt),
		UpdatedAt: toTime(updatedAt),
	}, nil
}
