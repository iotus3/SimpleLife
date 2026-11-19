package presentation

import (
	"time"

	"github.com/vivaui/viva/ui"

	focusapp "github.com/iotus3/simplelife/app/internal/application/focus"
	"github.com/iotus3/simplelife/app/internal/domain/focus"
	"github.com/iotus3/simplelife/app/internal/domain/shared"
	"github.com/iotus3/simplelife/app/internal/presentation/format"
	"github.com/iotus3/simplelife/app/internal/presentation/i18n"
)

// The actions of the interface live together, so a page only says what a button
// means and the transaction behind it is written once. Every one of them runs
// after the paint through State.apply: a page must not write while it declares.

// startSession starts a session of the kind chosen on the focus page, bound to the
// task chosen there.
func (s *State) startSession(c ui.C) {
	s.start(c, kindOf(s.focusKind), s.focusTask)
}

// startSessionFor starts a focus session on one task, from any page.
func (s *State) startSessionFor(c ui.C, id shared.ID) {
	s.start(c, focus.KindFocus, id)
}

func (s *State) start(c ui.C, kind focus.Kind, taskID shared.ID) {
	planned := s.timerCfg.DurationFor(kind)
	if kind == focus.KindFocus {
		planned = s.plannedFor(taskID)
	}
	s.apply(c, s.route, func() error {
		_, err := s.deps.Focus.Start(focusapp.StartInput{
			Kind:    kind,
			TaskID:  taskID,
			Planned: planned,
		})
		return err
	})
}

// plannedFor is how long the next focus session runs: the task's own length when it has one,
// otherwise the length chosen on the focus page.
func (s *State) plannedFor(taskID shared.ID) time.Duration {
	if !taskID.IsZero() {
		if value, err := s.deps.Tasks.Get(taskID); err == nil && value.FocusMinutes > 0 {
			return time.Duration(value.FocusMinutes) * time.Minute
		}
	}
	return s.focusPlanned()
}

func (s *State) pauseSession(c ui.C) {
	s.apply(c, s.route, func() error {
		_, err := s.deps.Focus.Pause()
		return err
	})
}

func (s *State) resumeSession(c ui.C) {
	s.apply(c, s.route, func() error {
		_, err := s.deps.Focus.Resume()
		return err
	})
}

// completeSession settles the running session: the application books the reward
// and counts the pomodoro on its task.
func (s *State) completeSession(c ui.C) {
	s.apply(c, s.route, func() error {
		session, outcome, err := s.deps.Focus.Complete()
		if err != nil {
			return err
		}
		s.settling = false
		if session != nil {
			s.active = nil
			s.running.Store(false)
		}
		if outcome.Applicable() {
			s.toast(i18n.T(s.lang, i18n.MsgFocusSessionDone, format.Money(outcome.Amount)), ui.ToastSuccess)
		}
		return nil
	})
}

// cancelSession gives a session up, which the domain records as an interruption
// and may fine.
func (s *State) cancelSession(c ui.C) {
	s.apply(c, s.route, func() error {
		_, _, err := s.deps.Focus.Cancel(true)
		if err != nil {
			return err
		}
		s.active = nil
		s.running.Store(false)
		s.toast(i18n.T(s.lang, i18n.MsgFocusSessionCancel), ui.ToastInfo)
		return nil
	})
}

// recoverSession settles the session a crash left behind.
func (s *State) recoverSession(c ui.C, credit bool) {
	s.apply(c, s.route, func() error {
		action := focusapp.RecoverDiscard
		if credit {
			action = focusapp.RecoverCredit
		}
		if _, _, err := s.deps.Focus.Recover(action); err != nil {
			return err
		}
		s.suspicious = nil
		s.recovering = false
		return nil
	})
}

// kindOf normalizes a stored kind.
func kindOf(kind focus.Kind) focus.Kind {
	switch kind {
	case focus.KindShortBreak, focus.KindLongBreak, focus.KindFocus:
		return kind
	}
	return focus.KindFocus
}
