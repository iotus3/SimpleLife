// Package goal implements the countdown goal use cases; the arithmetic stays in the
// domain.
package goal

import (
	domain "github.com/iotus3/simplelife/app/internal/domain/goal"
	"github.com/iotus3/simplelife/app/internal/domain/shared"
)

// Service is the goal application service.
type Service struct {
	repo  domain.Repository
	tx    shared.TxRunner
	clock shared.Clock
	ids   shared.IDGen
}

// New builds the service. Every collaborator is a port, so a test runs it against
// the in-memory store with a frozen clock.
func New(repo domain.Repository, tx shared.TxRunner, clock shared.Clock, ids shared.IDGen) *Service {
	return &Service{repo: repo, tx: tx, clock: clock, ids: ids}
}

// Input is the form data of a goal. The two dates arrive as calendar days, never
// as instants: the window asks about days, and a time of day would make the
// countdown depend on the hour the form was filled in.
type Input struct {
	Name  string
	Note  string
	Start shared.LocalDate
	End   shared.LocalDate
}

// Create validates the input through the aggregate and stores the goal.
func (s *Service) Create(in Input) (*domain.Goal, error) {
	now := s.clock.Now()
	g, err := domain.New(s.ids.New(), in.Name, in.Note, in.Start, in.End, now)
	if err != nil {
		return nil, err
	}
	if err := s.tx.WithinTx(func() error { return s.repo.Save(g) }); err != nil {
		return nil, err
	}
	return g, nil
}

// Update edits a stored goal. The read, the edit and the write share one
// transaction, so a goal deleted in between is reported instead of resurrected.
func (s *Service) Update(id shared.ID, in Input) (*domain.Goal, error) {
	now := s.clock.Now()
	var updated *domain.Goal
	err := s.tx.WithinTx(func() error {
		current, err := s.repo.Get(id)
		if err != nil {
			return err
		}
		cp := *current
		if err := cp.Edit(in.Name, in.Note, in.Start, in.End, now); err != nil {
			return err
		}
		if err := s.repo.Save(&cp); err != nil {
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

// Delete removes one goal.
func (s *Service) Delete(id shared.ID) error {
	if id.IsZero() {
		return shared.Invalid("goal.id.empty", "", "goal id is required")
	}
	return s.tx.WithinTx(func() error { return s.repo.Delete(id) })
}

// List returns every goal, the nearest deadline first.
func (s *Service) List() ([]*domain.Goal, error) {
	return s.repo.List()
}

// Today is the day the countdown is counted on, read from the service's own clock
// so a page never mixes two "now"s in one paint.
func (s *Service) Today() shared.LocalDate { return shared.Today(s.clock) }
