// Package focus implements the pomodoro use cases and the money booking that hangs off
// a settled session.
package focus

import (
	"time"

	focus "github.com/iotus3/simplelife/app/internal/domain/focus"
	ledger "github.com/iotus3/simplelife/app/internal/domain/ledger"
	"github.com/iotus3/simplelife/app/internal/domain/shared"
	task "github.com/iotus3/simplelife/app/internal/domain/task"

	"github.com/iotus3/simplelife/app/internal/contracts"
)

// Settings is the narrow view of the settings service the timer needs. Declaring
// it here (instead of importing the settings service) keeps the dependency
// pointing inward and lets tests pass a literal.
type Settings interface {
	Timer() focus.TimerSettings
	Economy() focus.EconomySettings
}

// Service is the pomodoro application service.
type Service struct {
	sessions focus.Repository
	tasks    task.Repository
	entries  ledger.EntryRepository
	booker   contracts.Booker

	tx    shared.TxRunner
	clock shared.Clock
	ids   shared.IDGen
	set   Settings

	notifier shared.Notifier
	sleep    shared.SleepGuard

	// releaseSleep is the function returned by the last Hold, kept so the lock is
	// dropped exactly once when the session settles.
	releaseSleep func()
}

// New wires the service. notifier and sleep may be no-op implementations; a
// missing notification centre must never fail a pomodoro.
func New(
	sessions focus.Repository,
	tasks task.Repository,
	entries ledger.EntryRepository,
	booker contracts.Booker,
	tx shared.TxRunner,
	clock shared.Clock,
	ids shared.IDGen,
	set Settings,
	notifier shared.Notifier,
	sleep shared.SleepGuard,
) *Service {
	if notifier == nil {
		notifier = shared.NoopNotifier{}
	}
	if sleep == nil {
		sleep = shared.NoopSleepGuard{}
	}
	return &Service{
		sessions: sessions, tasks: tasks, entries: entries, booker: booker,
		tx: tx, clock: clock, ids: ids, set: set,
		notifier: notifier, sleep: sleep,
	}
}

// StartInput is what the "start" button knows.
type StartInput struct {
	Kind focus.Kind
	// TaskID may be empty for free focus.
	TaskID shared.ID
	// Planned overrides the configured duration; zero means "use the setting".
	// A configured duration of zero means count-up.
	Planned time.Duration
	Note    string
}

// Start opens a session. There is at most one active session: starting a second
// one is a conflict, not a silent replacement, because silently discarding a
// running pomodoro loses the user's time.
func (s *Service) Start(in StartInput) (*focus.Session, error) {
	now := s.clock.Now()
	active, err := s.sessions.Active()
	if err != nil {
		return nil, err
	}
	if active != nil {
		return nil, shared.Conflict("focus.start.already_active", "a session is already running")
	}
	planned := in.Planned
	if planned == 0 && in.Kind != focus.KindFocus {
		planned = s.set.Timer().DurationFor(in.Kind)
	} else if planned == 0 && in.Kind == focus.KindFocus {
		planned = s.set.Timer().FocusDuration
	}
	session, err := focus.Start(s.ids.New(), in.TaskID, in.Kind, planned, now)
	if err != nil {
		return nil, err
	}
	session.SetNote(in.Note)
	if err := s.tx.WithinTx(func() error {
		if err := s.sessions.Create(session); err != nil {
			return err
		}
		if in.TaskID.IsZero() {
			return nil
		}
		t, err := s.tasks.Get(in.TaskID)
		if err != nil {
			return err
		}
		if err := t.StartDoing(now); err != nil {
			return err
		}
		return s.tasks.Update(t)
	}); err != nil {
		return nil, err
	}
	// Holding the sleep lock is best effort: an unsupported platform must not stop
	// the timer, so the error is deliberately dropped (the UI shows the capability).
	if release, herr := s.sleep.Hold("SimpleLife focus session"); herr == nil {
		s.releaseSleep = release
	}
	return session, nil
}

// Active returns the running or paused session, or nil.
func (s *Service) Active() (*focus.Session, error) { return s.sessions.Active() }

// Pause pauses the active session.
func (s *Service) Pause() (*focus.Session, error) {
	return s.settleActive(func(session *focus.Session, now time.Time) error { return session.Pause(now) })
}

// Resume resumes the paused session.
func (s *Service) Resume() (*focus.Session, error) {
	// Resuming means the machine should stay awake again: re-acquire the lock.
	if release, err := s.sleep.Hold("SimpleLife focus session"); err == nil {
		s.releaseSleep = release
	}
	return s.settleActive(func(session *focus.Session, now time.Time) error { return session.Resume(now) })
}

func (s *Service) settleActive(fn func(*focus.Session, time.Time) error) (*focus.Session, error) {
	now := s.clock.Now()
	var out *focus.Session
	err := s.tx.WithinTx(func() error {
		active, err := s.sessions.Active()
		if err != nil {
			return err
		}
		if active == nil {
			return shared.Conflict("focus.no_active_session", "there is no active session")
		}
		if err := fn(active, now); err != nil {
			return err
		}
		if err := s.sessions.Update(active); err != nil {
			return err
		}
		out = active
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// Complete settles the active session as done, credits the bound task and books the
// reward; a failed booking is a warning, never a failed completion.
func (s *Service) Complete() (*focus.Session, focus.Outcome, error) {
	now := s.clock.Now()
	var (
		settled *focus.Session
		outcome focus.Outcome
	)
	err := s.tx.WithinTx(func() error {
		active, err := s.sessions.Active()
		if err != nil {
			return err
		}
		if active == nil {
			return shared.Conflict("focus.no_active_session", "there is no active session")
		}
		if err := active.Complete(now); err != nil {
			return err
		}
		if err := s.sessions.Update(active); err != nil {
			return err
		}
		if active.Kind == focus.KindFocus && !active.TaskID.IsZero() {
			if err := s.creditTask(active.TaskID, now); err != nil {
				return err
			}
		}
		settled = active
		return nil
	})
	if err != nil {
		return nil, focus.Outcome{}, err
	}
	s.releaseSleepLock()

	outcome = s.book(settled, true)
	s.notify(settled)
	return settled, outcome, nil
}

// creditTask counts one more pomodoro on the task and marks it done. An archived task,
// or one the user already ticked, is left alone rather than failing the settle.
func (s *Service) creditTask(id shared.ID, now time.Time) error {
	t, err := s.tasks.Get(id)
	if err != nil {
		return err
	}
	if t.Status == task.StatusArchived {
		return nil
	}
	if t.Status.IsOpen() {
		if err := t.RecordPomodoro(now); err != nil {
			return err
		}
	}
	// Idempotent by contract: a task that is already done stays done.
	if err := t.Complete(now); err != nil {
		return err
	}
	return s.tasks.Update(t)
}

// Cancel settles the active session as abandoned and books the penalty.
func (s *Service) Cancel(interrupted bool) (*focus.Session, focus.Outcome, error) {
	now := s.clock.Now()
	var settled *focus.Session
	err := s.tx.WithinTx(func() error {
		active, err := s.sessions.Active()
		if err != nil {
			return err
		}
		if active == nil {
			return shared.Conflict("focus.no_active_session", "there is no active session")
		}
		if err := active.Cancel(now, interrupted); err != nil {
			return err
		}
		if err := s.sessions.Update(active); err != nil {
			return err
		}
		settled = active
		return nil
	})
	if err != nil {
		return nil, focus.Outcome{}, err
	}
	s.releaseSleepLock()
	outcome := s.book(settled, false)
	return settled, outcome, nil
}

func (s *Service) releaseSleepLock() {
	if s.releaseSleep != nil {
		s.releaseSleep()
		s.releaseSleep = nil
	}
}

// book writes the money for a settled session. It never returns an error to the
// caller: the caller has already committed the session, and losing a booking is a
// warning plus a retry entry, not a reason to tell the user their pomodoro failed.
func (s *Service) book(session *focus.Session, reward bool) focus.Outcome {
	if session == nil || session.Kind != focus.KindFocus {
		return focus.Outcome{Reason: focus.ReasonNotFocus}
	}
	econ := s.set.Economy()
	if !econ.Enabled {
		return focus.Outcome{Reason: focus.ReasonDisabled}
	}
	now := s.clock.Now()
	day := shared.DateOf(now)
	from, to := day.Midnight(), day.AddDays(1).Midnight()
	currency := econ.Rate.Currency()

	kind := ledger.EntryExpense
	already := shared.ZeroMoney(currency)
	var err error
	if reward {
		kind = ledger.EntryIncome
	}
	already, err = s.entries.SumByOrigin(ledger.OriginFocusReward, kind, from, to, currency)
	if err != nil {
		return focus.Outcome{Reason: "focus.economy.booking_failed"}
	}

	var outcome focus.Outcome
	if reward {
		outcome, err = econ.Reward(session, already)
	} else {
		outcome, err = econ.Penalty(session, already)
	}
	if err != nil || !outcome.Applicable() {
		return outcome
	}

	// Idempotency: if this session already produced an entry, do nothing. The
	// unique index would refuse the write anyway; checking first turns a database
	// error into a calm no-op.
	exists, eerr := s.entries.ExistsFocusSession(session.ID)
	if eerr != nil {
		return focus.Outcome{Reason: "focus.economy.booking_failed"}
	}
	if exists {
		return focus.Outcome{Reason: "focus.economy.already_booked"}
	}

	direction := contracts.BookingIncome
	categoryID := econ.RewardCategoryID
	if !reward {
		direction = contracts.BookingExpense
		categoryID = econ.PenaltyCategoryID
	}
	if berr := s.booker.Book(contracts.BookingRequest{
		Direction:  direction,
		Amount:     outcome.Amount,
		AccountID:  econ.RewardAccountID,
		CategoryID: categoryID,
		SessionID:  session.ID,
		OccurredAt: now,
	}); berr != nil {
		// The booking failed but the session stands. The reason tells the UI to
		// show "money not booked" instead of claiming success.
		return focus.Outcome{Amount: shared.ZeroMoney(outcome.Amount.Currency()), Reason: "focus.economy.booking_failed", CappedByDaily: outcome.CappedByDaily}
	}
	return outcome
}

func (s *Service) notify(session *focus.Session) {
	title := "focus.done.title"
	body := "focus.done.body"
	if session.Kind != focus.KindFocus {
		title = "focus.break.done.title"
		body = "focus.break.done.body"
	}
	// The notifier takes plain strings; the adapter translates the codes. A failed
	// notification is advisory only.
	_ = s.notifier.Notify(shared.Notification{Title: title, Body: body, Tag: session.ID.String()})
}

// RecoverAction is what the user chose for a session that outlived the sanity
// threshold (app closed, machine asleep).
type RecoverAction uint8

const (
	// RecoverCredit settles the session as completed at its planned duration.
	RecoverCredit RecoverAction = iota + 1
	// RecoverDiscard cancels the session without counting an interruption.
	RecoverDiscard
	// RecoverKeep resumes the session as if nothing happened.
	RecoverKeep
)

// Suspicious returns the active session when it has been running suspiciously long.
func (s *Service) Suspicious() (*focus.Session, error) {
	active, err := s.sessions.Active()
	if err != nil || active == nil {
		return nil, err
	}
	if !active.Suspicious(s.clock.Now()) {
		return nil, nil
	}
	return active, nil
}

// Recover applies the user's choice for a suspicious session.
func (s *Service) Recover(action RecoverAction) (*focus.Session, focus.Outcome, error) {
	switch action {
	case RecoverCredit:
		return s.Complete()
	case RecoverDiscard:
		return s.Cancel(false)
	case RecoverKeep:
		session, err := s.Active()
		return session, focus.Outcome{}, err
	default:
		return nil, focus.Outcome{}, shared.Invalid("focus.recover.action", "", "unknown recovery action")
	}
}

// DayStats is the focus summary of one day.
type DayStats struct {
	Day            shared.LocalDate
	CompletedFocus int
	FocusMinutes   int
	Interruptions  int
	PlannedMinutes int
}

// TodayStats summarises today's sessions for the dashboard.
func (s *Service) TodayStats() (DayStats, error) {
	return s.DayStats(shared.Today(s.clock))
}

// DayStats summarises one day's sessions. The statistics page needs a whole week
// of them, so the day is a parameter rather than always "today".
func (s *Service) DayStats(day shared.LocalDate) (DayStats, error) {
	stats := DayStats{Day: day}
	sessions, err := s.sessions.ListByRange(day.Midnight(), day.AddDays(1).Midnight())
	if err != nil {
		return stats, err
	}
	for _, session := range sessions {
		if session.Kind != focus.KindFocus {
			continue
		}
		stats.Interruptions += session.Interruptions
		stats.PlannedMinutes += int(session.Planned / time.Minute)
		if session.Phase == focus.PhaseDone {
			stats.CompletedFocus++
			stats.FocusMinutes += session.CompletedFocusMinutes()
		}
	}
	return stats, nil
}

// NextBreakSuggestion tells the UI which break to offer after the focus session
// that just ended. The count comes from the store, so restarting the app in the
// middle of a cycle does not reset it.
func (s *Service) NextBreakSuggestion() focus.Kind {
	now := s.clock.Now()
	day := shared.DateOf(now)
	count, err := s.sessions.FocusCountBetween(day.Midnight(), day.AddDays(1).Midnight())
	if err != nil {
		return focus.KindShortBreak
	}
	return s.set.Timer().NextBreakKind(count)
}

// ListRecent returns recent sessions for the history panel.
func (s *Service) ListRecent(limit int) ([]*focus.Session, error) {
	if limit <= 0 {
		limit = 20
	}
	return s.sessions.ListRecent(limit)
}
