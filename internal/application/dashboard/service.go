// Package dashboard assembles the home screen from the services that own each number.
package dashboard

import (
	"time"

	focusapp "github.com/iotus3/simplelife/app/internal/application/focus"
	ledgerapp "github.com/iotus3/simplelife/app/internal/application/ledger"
	taskapp "github.com/iotus3/simplelife/app/internal/application/task"
	focus "github.com/iotus3/simplelife/app/internal/domain/focus"
	"github.com/iotus3/simplelife/app/internal/domain/shared"
	task "github.com/iotus3/simplelife/app/internal/domain/task"
)

// Service builds the overview.
type Service struct {
	tasks *taskapp.Service
	focus *focusapp.Service
	money *ledgerapp.Service
	clock shared.Clock
}

// New wires the dashboard to the three module services.
func New(tasks *taskapp.Service, focus *focusapp.Service, money *ledgerapp.Service, clock shared.Clock) *Service {
	return &Service{tasks: tasks, focus: focus, money: money, clock: clock}
}

// Overview is one snapshot of the home screen.
type Overview struct {
	Day shared.LocalDate

	// Tasks
	TodayTasks       []*task.Task
	OpenTasks        int
	DoneToday        int
	OverdueTasks     int
	PlannedPomodoros int

	// Focus
	ActiveSession  *focus.Session
	CompletedFocus int
	FocusMinutes   int

	// Money (today's movement, per currency; never a single number)
	Income  []shared.MoneyTotal
	Expense []shared.MoneyTotal
}

// Load builds the overview; a failing section degrades to an inline warning instead of
// blanking the screen.
func (s *Service) Load() (Overview, error) {
	now := s.clock.Now()
	day := shared.DateOf(now)
	out := Overview{Day: day}

	todayTasks, err := s.tasks.Today(day)
	if err != nil {
		return out, err
	}
	out.TodayTasks = todayTasks
	for _, t := range todayTasks {
		if t.Status.IsOpen() {
			out.OpenTasks++
			out.PlannedPomodoros += t.EstimatePomodoros
		}
	}
	if out.OverdueTasks, err = s.tasks.OverdueCount(); err != nil {
		return out, err
	}

	doneToday, err := s.doneToday(now)
	if err != nil {
		return out, err
	}
	out.DoneToday = doneToday

	stats, err := s.focus.TodayStats()
	if err != nil {
		return out, err
	}
	out.CompletedFocus = stats.CompletedFocus
	out.FocusMinutes = stats.FocusMinutes
	if out.ActiveSession, err = s.focus.Active(); err != nil {
		return out, err
	}

	summary, err := s.money.Summarise(day.AsMonth())
	if err != nil {
		return out, err
	}
	out.Income, out.Expense = summary.Income, summary.Expense
	return out, nil
}

// doneToday counts tasks completed on the current local day. It is a scan rather
// than a query because completion has no index of its own: the set of done tasks is
// small, and adding a second index for one counter would cost more than it saves.
func (s *Service) doneToday(now time.Time) (int, error) {
	day := shared.DateOf(now)
	done, err := s.tasks.List(statusFilter(task.StatusDone))
	if err != nil {
		return 0, err
	}
	n := 0
	for _, t := range done {
		if t.CompletedAt.IsZero() {
			continue
		}
		if shared.DateOf(t.CompletedAt.Local()).Equal(day) {
			n++
		}
	}
	return n, nil
}

func statusFilter(status task.Status) task.Filter {
	return task.Filter{Statuses: []task.Status{status}, Order: task.OrderCreated, Limit: 500}
}
