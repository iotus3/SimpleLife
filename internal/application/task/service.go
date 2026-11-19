// Package task implements the todo use cases and owns every rule that spans more than
// one aggregate.
package task

import (
	"sort"
	"time"

	domain "github.com/iotus3/simplelife/app/internal/domain/task"

	"github.com/iotus3/simplelife/app/internal/domain/shared"
)

// Service is the todo application service.
type Service struct {
	repo  domain.Repository
	tx    shared.TxRunner
	clock shared.Clock
	ids   shared.IDGen
}

// New builds the service. All four collaborators are ports, so the tests can run
// it against an in-memory store with a frozen clock.
func New(repo domain.Repository, tx shared.TxRunner, clock shared.Clock, ids shared.IDGen) *Service {
	return &Service{repo: repo, tx: tx, clock: clock, ids: ids}
}

// CreateInput is the form data for a new task.
type CreateInput struct {
	Title    string
	Notes    string
	Priority int
	DueDate  shared.LocalDate
	Estimate int
	// FocusMinutes is how long one pomodoro runs for this task; 0 means "use the
	// length the focus page is set to".
	FocusMinutes int
	Tags         []string
	ParentID     shared.ID
}

// Create validates the input through the aggregate and stores the task.
func (s *Service) Create(in CreateInput) (*domain.Task, error) {
	now := s.clock.Now()
	t, err := domain.New(s.ids.New(), in.Title, now)
	if err != nil {
		return nil, err
	}
	if err := s.applyEdits(t, in.Notes, in.Priority, in.DueDate, in.Estimate, in.FocusMinutes, in.Tags, now); err != nil {
		return nil, err
	}
	if !in.ParentID.IsZero() {
		parent, err := s.repo.Get(in.ParentID)
		if err != nil {
			return nil, err
		}
		if err := t.AttachParent(parent, now); err != nil {
			return nil, err
		}
	}
	if err := s.tx.WithinTx(func() error { return s.repo.Create(t) }); err != nil {
		return nil, err
	}
	return t, nil
}

// UpdateInput is the form data for editing a task.
type UpdateInput struct {
	ID           shared.ID
	Title        string
	Notes        string
	Priority     int
	DueDate      shared.LocalDate
	Estimate     int
	FocusMinutes int
	Tags         []string
}

// Update applies every editable field in one transaction. Fields are applied to a
// copy first, so a validation failure halfway through cannot leave the stored
// aggregate half-edited.
func (s *Service) Update(in UpdateInput) (*domain.Task, error) {
	now := s.clock.Now()
	var updated *domain.Task
	err := s.tx.WithinTx(func() error {
		current, err := s.repo.Get(in.ID)
		if err != nil {
			return err
		}
		cp := *current
		cp.Tags = append([]string(nil), current.Tags...)
		if err := cp.SetTitle(in.Title, now); err != nil {
			return err
		}
		if err := s.applyEdits(&cp, in.Notes, in.Priority, in.DueDate, in.Estimate, in.FocusMinutes, in.Tags, now); err != nil {
			return err
		}
		if err := s.repo.Update(&cp); err != nil {
			return err
		}
		updated = &cp
		return nil
	})
	if err != nil {
		return nil, err
	}
	return updated, nil
}

func (s *Service) applyEdits(t *domain.Task, notes string, priority int, due shared.LocalDate, estimate, focusMinutes int, tags []string, now time.Time) error {
	if err := t.SetNotes(notes, now); err != nil {
		return err
	}
	if priority < 0 || priority > domain.PriorityMax {
		return shared.Invalid("task.priority.range", "priority", "priority must be 0..4")
	}
	if err := t.SetPriority(domain.Priority(priority), now); err != nil {
		return err
	}
	if err := t.SetDueDate(due, now); err != nil {
		return err
	}
	if err := t.SetEstimate(estimate, now); err != nil {
		return err
	}
	if err := t.SetFocusMinutes(focusMinutes, now); err != nil {
		return err
	}
	return t.SetTags(tags, now)
}

// Complete marks a task done. Idempotent: completing twice is not an error.
func (s *Service) Complete(id shared.ID) (*domain.Task, error) {
	return s.mutate(id, func(t *domain.Task, now time.Time) error { return t.Complete(now) })
}

// Reopen moves a done task back to the open list.
func (s *Service) Reopen(id shared.ID) (*domain.Task, error) {
	return s.mutate(id, func(t *domain.Task, now time.Time) error { return t.Reopen(now) })
}

// StartDoing marks a task as in progress.
func (s *Service) StartDoing(id shared.ID) (*domain.Task, error) {
	return s.mutate(id, func(t *domain.Task, now time.Time) error { return t.StartDoing(now) })
}

// Restore brings back an archived task.
func (s *Service) Restore(id shared.ID) (*domain.Task, error) {
	return s.mutate(id, func(t *domain.Task, now time.Time) error { return t.Restore(now) })
}

func (s *Service) mutate(id shared.ID, fn func(*domain.Task, time.Time) error) (*domain.Task, error) {
	now := s.clock.Now()
	var out *domain.Task
	err := s.tx.WithinTx(func() error {
		current, err := s.repo.Get(id)
		if err != nil {
			return err
		}
		if err := fn(current, now); err != nil {
			return err
		}
		if err := s.repo.Update(current); err != nil {
			return err
		}
		out = current
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// Delete removes a task, except one a focus session referenced or one a template
// generated: those are archived, so history keeps its link and the occurrence key stays
// occupied.
func (s *Service) Delete(id shared.ID) error {
	now := s.clock.Now()
	return s.tx.WithinTx(func() error {
		current, err := s.repo.Get(id)
		if err != nil {
			return err
		}
		if current.IsFromTemplate() {
			return s.repo.Update(archiveOf(current, now))
		}
		hasHistory, err := s.repo.HasFocusHistory(id)
		if err != nil {
			return err
		}
		if hasHistory {
			return s.repo.Update(archiveOf(current, now))
		}
		return s.repo.Delete(id)
	})
}

func archiveOf(t *domain.Task, now time.Time) *domain.Task {
	cp := *t
	cp.Tags = append([]string(nil), t.Tags...)
	// Archive cannot fail on a live task, so ignoring the error here keeps the
	// call site readable; a failure would mean the task was already archived,
	// which is what we want anyway.
	_ = cp.Archive(now)
	return &cp
}

// MoveInput describes a drag-and-drop move: place ID between PrevID and NextID
// (either may be empty for "first" and "last").
type MoveInput struct {
	ID     shared.ID
	PrevID shared.ID
	NextID shared.ID
}

// Move rewrites one sort key. The two neighbour keys are read inside the same
// transaction as the write, so a concurrent insert cannot produce a duplicate key.
func (s *Service) Move(in MoveInput) error {
	now := s.clock.Now()
	return s.tx.WithinTx(func() error {
		current, err := s.repo.Get(in.ID)
		if err != nil {
			return err
		}
		prevKey, nextKey := "", ""
		if !in.PrevID.IsZero() {
			prev, err := s.repo.Get(in.PrevID)
			if err != nil {
				return err
			}
			prevKey = prev.SortKey
		}
		if !in.NextID.IsZero() {
			next, err := s.repo.Get(in.NextID)
			if err != nil {
				return err
			}
			nextKey = next.SortKey
		}
		key, err := domain.RankBetween(prevKey, nextKey)
		if err != nil {
			// The list is too dense to split: renumber the sibling group and retry
			// once. This is the only recovery path, and it is deterministic.
			if shared.Code(err) != "task.rank.lowest" {
				return err
			}
			if err := s.renumberSiblings(current, now); err != nil {
				return err
			}
			if !in.PrevID.IsZero() {
				prev, gerr := s.repo.Get(in.PrevID)
				if gerr != nil {
					return gerr
				}
				prevKey = prev.SortKey
			}
			if !in.NextID.IsZero() {
				next, gerr := s.repo.Get(in.NextID)
				if gerr != nil {
					return gerr
				}
				nextKey = next.SortKey
			}
			key, err = domain.RankBetween(prevKey, nextKey)
			if err != nil {
				return err
			}
		}
		if err := current.SetSortKey(key, now); err != nil {
			return err
		}
		return s.repo.Update(current)
	})
}

func (s *Service) renumberSiblings(moving *domain.Task, now time.Time) error {
	var parentID *shared.ID
	if moving.ParentID != "" {
		p := moving.ParentID
		parentID = &p
	} else {
		zero := shared.ID("")
		parentID = &zero
	}
	siblings, err := s.repo.List(domain.Filter{ParentID: parentID, Order: domain.OrderManual})
	if err != nil {
		return err
	}
	keys := domain.NormalizeRanks(len(siblings))
	for i, sib := range siblings {
		if sib.SortKey == keys[i] {
			continue
		}
		if err := sib.SetSortKey(keys[i], now); err != nil {
			return err
		}
		if err := s.repo.Update(sib); err != nil {
			return err
		}
	}
	return nil
}

// Today returns the tasks due on the given day, in manual order.
func (s *Service) Today(day shared.LocalDate) ([]*domain.Task, error) {
	return s.repo.List(domain.Today(day))
}

// History returns every task that is not due today, for the date-grouped view.
func (s *Service) History(day shared.LocalDate) ([]*domain.Task, error) {
	return s.repo.List(domain.History(day))
}

// Finished returns what was already finished on day. The today view lists it under
// the open tasks, and the history view keeps it: a task that is ticked off must not
// vanish from both views at once.
func (s *Service) Finished(day shared.LocalDate) ([]*domain.Task, error) {
	return s.repo.List(domain.FinishedOn(day))
}

// OverdueCount counts open tasks whose deadline has passed. The today view shows
// it as a banner: overdue work is excluded from "today" by design, but silently
// hiding it would let it rot.
func (s *Service) OverdueCount() (int, error) {
	return s.repo.Count(domain.Overdue(s.clock.Now()))
}

// List runs a raw filter. It exists for callers that own a view this service does
// not model — the dashboard's "completed today" counter, for instance — without
// making them depend on the repository port.
func (s *Service) List(f domain.Filter) ([]*domain.Task, error) { return s.repo.List(f) }

// OpenCount counts todo + doing tasks, for the dashboard.
func (s *Service) OpenCount() (int, error) { return s.repo.CountOpen() }

// Get returns one task.
func (s *Service) Get(id shared.ID) (*domain.Task, error) { return s.repo.Get(id) }

// Children returns the subtasks of a task.
func (s *Service) Children(id shared.ID) ([]*domain.Task, error) {
	return s.repo.List(domain.Children(id))
}

// Group is one date bucket of the history view.
type Group struct {
	Day   shared.LocalDate
	Tasks []*domain.Task
}

// HistoryGroups buckets the history by due date, newest first, undated last; what was
// finished on day belongs there too.
func (s *Service) HistoryGroups(day shared.LocalDate) ([]Group, error) {
	tasks, err := s.History(day)
	if err != nil {
		return nil, err
	}
	finished, err := s.Finished(day)
	if err != nil {
		return nil, err
	}
	tasks = append(tasks, finished...)
	byDay := make(map[shared.LocalDate]int)
	var groups []Group
	undated := -1
	for _, t := range tasks {
		d := t.DueDate()
		if d.IsZero() {
			if undated < 0 {
				undated = len(groups)
				groups = append(groups, Group{})
			}
			groups[undated].Tasks = append(groups[undated].Tasks, t)
			continue
		}
		idx, ok := byDay[d]
		if !ok {
			idx = len(groups)
			byDay[d] = idx
			groups = append(groups, Group{Day: d})
		}
		groups[idx].Tasks = append(groups[idx].Tasks, t)
	}
	sort.SliceStable(groups, func(i, j int) bool { return lessGroup(groups[i], groups[j]) })
	return groups, nil
}

// lessGroup orders dated groups newest first and keeps the undated group last, at
// the bottom of the history view.
func lessGroup(a, b Group) bool {
	if a.Day.IsZero() {
		return false
	}
	if b.Day.IsZero() {
		return true
	}
	return a.Day.After(b.Day)
}
