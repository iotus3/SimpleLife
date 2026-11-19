package presentation

import (
	"math"
	"strconv"

	"github.com/vivaui/viva/gfx"
	"github.com/vivaui/viva/ui"

	"github.com/iotus3/simplelife/app/internal/domain/focus"
	"github.com/iotus3/simplelife/app/internal/domain/shared"
	"github.com/iotus3/simplelife/app/internal/domain/task"
	"github.com/iotus3/simplelife/app/internal/presentation/format"
	"github.com/iotus3/simplelife/app/internal/presentation/i18n"
	"github.com/iotus3/simplelife/app/internal/presentation/icons"
	"github.com/iotus3/simplelife/app/internal/presentation/theme"
)

// homePage is the landing page: today's numbers, today's tasks and the timer.
func (s *State) homePage(c ui.C) {
	// The dashboard reads; it does not open editors. A draft opened here belongs to a page that does
	// not draw it, so the cards below lead to the pages that own the editing.
	s.pageHeader(c, icons.House, i18n.MsgHomeTitle, i18n.MsgNavHomeSub, func() {
		// The day itself, not "today": the numbers below only make sense once the
		// reader knows which day they count, and the week of the year is what a plan
		// that spans weeks is talked about in.
		ui.Text(c, format.DayBanner(s.lang, s.overview.day)).
			FontSize(12).Color(s.palette.Faint).MaxLines(1)
	})

	ui.Row(c).FillWidth().Gap(12).Children(func() {
		s.metric(c, icons.ListTodo, i18n.MsgHomeOpenTasks, strconv.Itoa(s.overview.open), s.palette.Accent)
		s.metric(c, icons.Check, i18n.MsgHomeDoneToday, strconv.Itoa(s.overview.doneToday), s.palette.Income)
		s.metric(c, icons.Calendar, i18n.MsgHomeOverdue, strconv.Itoa(s.overview.overdue), s.palette.Warning)
		s.metric(c, icons.Target, i18n.MsgHomePomodoros, strconv.Itoa(s.overview.completed), s.palette.Focus)
		s.metric(c, icons.Clock, i18n.MsgHomeFocusMinutes, strconv.Itoa(s.overview.minutes), s.palette.Muted)
	})

	ui.Row(c).FillWidth().Items(ui.Start).Gap(theme.Layout().Gap).Children(func() {
		ui.Column(c).FillWidth().Gap(theme.Layout().Gap).Children(func() {
			s.homeTasksCard(c)
			s.homeMoneyCard(c)
		})
		ui.Column(c).Width(340).Gap(theme.Layout().Gap).Children(func() {
			s.homeFocusCard(c)
		})
	})
}

// homeTasksCard is the short list of what is left today.
func (s *State) homeTasksCard(c ui.C) {
	s.cardTitled(c, icons.ListTodo, i18n.MsgHomeTasks, func() {
		if ui.Button(c, i18n.T(s.lang, i18n.MsgHomeSeeAll)).Icon(icons.ChevronRight).
			Ghost().Small().Clicked() {
			s.navigate(RouteTasks)
		}
	}, func() {
		if len(s.overview.tasks) == 0 {
			s.empty(c, i18n.MsgHomeEmptyTasks)
			return
		}
		limit := len(s.overview.tasks)
		if limit > 8 {
			limit = 8
		}
		for index, value := range s.overview.tasks[:limit] {
			s.taskRow(c, value, false, index == limit-1)
		}
	})
}

// homeMoneyCard shows what the day cost and earned, one line per currency.
func (s *State) homeMoneyCard(c ui.C) {
	s.cardTitled(c, icons.Wallet, i18n.MsgHomeMoney, func() {
		// Same reason: recording an entry is the ledger's job, so this card hands
		// over to it instead of opening a dialog the dashboard cannot draw.
		if ui.Button(c, i18n.T(s.lang, i18n.MsgHomeSeeEntries)).Icon(icons.ChevronRight).
			Ghost().Small().Clicked() {
			s.navigate(RouteLedger)
		}
	}, func() {
		ui.Row(c).FillWidth().Gap(12).Children(func() {
			s.moneyColumn(c, i18n.MsgLedgerSumIncome, s.overview.income, s.palette.Income, icons.ArrowUpRight)
			s.moneyColumn(c, i18n.MsgLedgerSumExpense, s.overview.expense, s.palette.Expense, icons.ArrowDownRight)
		})
	})
}

// moneyColumn is one currency's total with its label, tinted by direction.
func (s *State) moneyColumn(c ui.C, label i18n.MessageID, totals []shared.MoneyTotal, tone ui.Color, icon gfx.Icon) {
	ui.Row(c).FillWidth().Items(ui.Center).Gap(10).Pad(12).
		Background(s.palette.SurfaceSoft).Radius(theme.Layout().RadiusControl).
		Children(func() {
			s.iconChip(c, icon, tone, s.palette.Surface, 30, 15)
			ui.Column(c).Gap(2).FillWidth().Children(func() {
				ui.Text(c, i18n.T(s.lang, label)).FontSize(11.5).Color(s.palette.Muted)
				ui.Text(c, format.TotalsIn(s.lang, totals, s.deps.Settings.DefaultCurrency())).FontSize(15).
					Weight(gfx.WeightSemibold).Color(tone).MaxLines(1).Ellipsis()
			})
		})
}

// homeFocusCard shows the running session or offers to start one.
func (s *State) homeFocusCard(c ui.C) {
	s.cardTitled(c, icons.Timer, i18n.MsgHomeFocus, nil, func() {
		if s.active == nil {
			ui.Column(c).FillWidth().Items(ui.Center).Gap(10).PadY(8).Children(func() {
				ui.Text(c, format.Clock(s.timerCfg.FocusDuration)).FontSize(38).
					Weight(gfx.WeightSemibold).Color(s.palette.Text).Mono()
				ui.Text(c, i18n.T(s.lang, i18n.MsgHomeActiveNone)).FontSize(12).
					Color(s.palette.Muted).TextCenter()
				if ui.Button(c, i18n.T(s.lang, i18n.MsgFocusActionStart)).Icon(icons.Play).
					Clicked() {
					s.startSession(c)
				}
			})
			return
		}
		ui.Column(c).FillWidth().Gap(6).Children(func() {
			ui.Text(c, clockOf(s.active, s.now())).FontSize(38).
				Weight(gfx.WeightSemibold).Color(s.palette.Accent).Mono()
			ui.Row(c).Items(ui.Center).Gap(6).Children(func() {
				s.tag(c, i18n.T(s.lang, s.kindMessage(s.active.Kind)), s.palette.Accent)
				s.tag(c, i18n.T(s.lang, s.focusStateMessage(s.active)), s.palette.Muted)
			})
			if s.active.TaskID != "" {
				ui.Text(c, s.taskTitle(s.active.TaskID)).FontSize(12).
					Color(s.palette.Muted).MaxLines(1).Ellipsis()
			}
			ui.Row(c).Gap(8).Children(func() {
				if ui.Button(c, i18n.T(s.lang, i18n.MsgFocusActionPause)).Icon(icons.Pause).
					Secondary().Small().Clicked() {
					s.pauseSession(c)
				}
				if ui.Button(c, i18n.T(s.lang, i18n.MsgFocusActionComplete)).Icon(icons.Check).
					Small().Clicked() {
					s.completeSession(c)
				}
			})
		})
	})
}

// taskTitle resolves a task id to its title for a one-line hint.
func (s *State) taskTitle(id shared.ID) string {
	for _, value := range s.taskToday {
		if value.ID == id {
			return value.Title
		}
	}
	for _, value := range s.overview.tasks {
		if value.ID == id {
			return value.Title
		}
	}
	return ""
}

// taskRow is the one row shape used on the dashboard and the task page: a
// checkbox, the title with its meta line, and — where the row allows it — the
// actions that belong to it.
func (s *State) taskRow(c ui.C, value *task.Task, withActions, last bool) {
	id := value.ID
	done := value.Status == task.StatusDone
	checked := done
	ui.Row(c).FillWidth().Items(ui.Center).Gap(10).PadY(6).Children(func() {
		// Changed() writes the new value into checked; reading it before that read
		// still sees the old one.
		box := ui.Checkbox(c, &checked, "")
		if box.Changed() && checked != done {
			if done {
				s.apply(c, s.route, func() error { _, err := s.deps.Tasks.Reopen(id); return err })
			} else {
				s.apply(c, s.route, func() error { _, err := s.deps.Tasks.Complete(id); return err })
			}
		}
		ui.Column(c).FillWidth().Gap(2).Children(func() {
			s.taskTitleLine(c, value.Title, done)
			if meta := s.taskMeta(value); meta != "" {
				ui.Text(c, meta).FontSize(12).Color(s.palette.Faint).MaxLines(1).Ellipsis()
			}
		})
		if value.HasDueDate() && value.DueDate().Before(s.today()) {
			s.tag(c, i18n.T(s.lang, i18n.MsgTaskDueOverdue), s.palette.Warning)
		}
		if !withActions {
			return
		}
		if session := s.sessionForTask(id); session != nil {
			s.sessionRowActions(c, session)
		} else if !done && s.iconAction(c, icons.Play, i18n.MsgTaskActionFocus) {
			// A finished task offers no start button: the domain refuses to focus a
			// done task (task.start.done), and a button that can only fail is not an
			// action. Clearing the tick reopens it, and the button comes back.
			s.startSessionFor(c, id)
		}
		if s.iconAction(c, icons.Pencil, i18n.MsgTaskActionEdit) {
			s.openTaskEditor(id)
		}
		if s.iconAction(c, icons.Trash2, i18n.MsgTaskActionDelete) {
			s.askDelete(confirmTask, id)
		}
	})
	if !last {
		ui.Divider(c)
	}
}

// sessionForTask returns the running or paused session that is counted against
// this task, or nil. It is what turns a row's "start" button into the controls of
// the session already going on it.
func (s *State) sessionForTask(id shared.ID) *focus.Session {
	if s.active == nil || s.active.TaskID != id {
		return nil
	}
	return s.active
}

// sessionRowActions is the live part of a task row: the countdown and the same three buttons as
// the focus page, so a running session is never offered a second start.
func (s *State) sessionRowActions(c ui.C, session *focus.Session) {
	tone := s.palette.Accent
	if session.Phase == focus.PhasePaused {
		tone = s.palette.Warning
	}
	ui.Text(c, clockOf(session, s.now())).FontSize(14).Weight(gfx.WeightSemibold).
		Mono().Color(tone).MaxLines(1).PadSides(6, 6, 6, 6)
	if session.Phase == focus.PhasePaused {
		if s.iconAction(c, icons.Play, i18n.MsgFocusActionResume) {
			s.resumeSession(c)
		}
	} else if s.iconAction(c, icons.Pause, i18n.MsgFocusActionPause) {
		s.pauseSession(c)
	}
	if s.iconAction(c, icons.Check, i18n.MsgFocusActionComplete) {
		s.completeSession(c)
	}
	if s.iconAction(c, icons.X, i18n.MsgFocusActionCancel) {
		s.cancelSession(c)
	}
}

// taskTitleLine is the title of a task, struck through once it is finished.
func (s *State) taskTitleLine(c ui.C, title string, done bool) {
	if !done {
		ui.Text(c, title).FontSize(titleSize).Color(s.palette.Text).MaxLines(1).Ellipsis()
		return
	}
	// Viva has no struck text style, so the words and the rule through them are painted by one
	// callback. The rule uses the softer ink, which reads as a line at two pixels.
	ink := s.palette.Muted
	ui.Row(c).FillWidth().Height(titleSize + 6).Draw(func(d *ui.DrawContext) {
		d.Canvas.Label(title, d.Rect, gfx.TextStyle{Size: titleSize, Color: s.palette.Faint})
		// A box that reports no width bounds nothing: a column that has not settled a
		// width is not a box of no width, and min is not used here because it would
		// carry a NaN into the fill and drop the whole rule.
		width := d.Canvas.TextWidth(title, titleSize)
		if box := d.Rect.Width; box > 0 && width > box {
			width = box
		}
		if !(width > 0) {
			return
		}
		d.Canvas.Rounded(gfx.Rect{
			X:      d.Rect.X,
			Y:      float32(math.Round(float64(d.Rect.Y + d.Rect.Height/2))),
			Width:  width,
			Height: 2,
		}, 1, ink)
	})
}

// titleSize is the size of a task title, shared by its text and the rule through
// it: the rule is measured in the face the title is drawn with.
const titleSize float32 = 14

// taskMeta is the second line of a task row: the day, the estimate and the tags
// it carries, whichever of them it has.
func (s *State) taskMeta(value *task.Task) string {
	parts := make([]string, 0, 3)
	if value.HasDueDate() {
		parts = append(parts, s.dueLabel(value.DueDate()))
	}
	if value.EstimatePomodoros > 0 {
		parts = append(parts, i18n.T(s.lang, i18n.MsgTaskEstimateCount, value.EstimatePomodoros))
	}
	if tags := format.Tags(value.Tags); tags != "" {
		parts = append(parts, tags)
	}
	meta := ""
	for index, part := range parts {
		if index > 0 {
			meta += " · "
		}
		meta += part
	}
	return meta
}
