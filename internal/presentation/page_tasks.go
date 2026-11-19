package presentation

import (
	"strconv"

	"github.com/vivaui/viva/ui"

	tapp "github.com/iotus3/simplelife/app/internal/application/task"
	"github.com/iotus3/simplelife/app/internal/domain/shared"
	"github.com/iotus3/simplelife/app/internal/domain/task"
	"github.com/iotus3/simplelife/app/internal/presentation/format"
	"github.com/iotus3/simplelife/app/internal/presentation/i18n"
	"github.com/iotus3/simplelife/app/internal/presentation/icons"
)

// tasksPage is M2: today's tasks in one tab, the days before in another. Only
// today's list is "live"; everything else is history grouped by day.
func (s *State) tasksPage(c ui.C) {
	s.pageHeader(c, icons.ListTodo, i18n.MsgNavTasks, i18n.MsgNavTasksSub, func() {
		if s.taskOverdue > 0 {
			ui.Text(c, i18n.T(s.lang, s.lang.Plural(s.taskOverdue, i18n.MsgTaskOverdueOne, i18n.MsgTaskOverdueCount), s.taskOverdue)).
				Color(s.palette.Warning).FontSize(12).PadSides(4, 4, 4, 4)
		}
		if ui.Button(c, i18n.T(s.lang, i18n.MsgTaskNew)).Icon(icons.Plus).Clicked() {
			s.openTaskEditor("")
		}
	})
	tabs := []int{0, 1}
	pageTabs(c, &s.taskTab, tabs, func(index int) string {
		if index == 0 {
			return i18n.T(s.lang, i18n.MsgTaskTabToday)
		}
		return i18n.T(s.lang, i18n.MsgTaskTabHistory)
	})
	if s.taskTab == 1 {
		s.taskHistory(c)
	} else {
		s.taskTodayList(c)
	}
	s.taskEditor(c)
	// A session started from this list settles here too: the countdown reaching zero
	// is the same event on every page, and waiting for a visit to the focus page to
	// book the pomodoro would make the row lie.
	s.sessionExpiry(c)
}

// taskTodayList is the live list: a quick add field, then the open tasks with
// their subtasks, then what was finished today.
func (s *State) taskTodayList(c ui.C) {
	s.card(c, func() {
		ui.Row(c).FillWidth().Items(ui.Center).Gap(8).Children(func() {
			// The caption names the quick-add field, so it is not confused with the
			// title field of the editor behind it.
			s.field(c, &s.taskQuickAdd, i18n.MsgTaskQuickAdd, i18n.T(s.lang, i18n.MsgTaskPlaceholder))
			if ui.Button(c, i18n.T(s.lang, i18n.MsgAdd)).Clicked() && s.taskQuickAdd != "" {
				title := s.taskQuickAdd
				s.taskQuickAdd = ""
				s.apply(c, s.route, func() error {
					_, err := s.deps.Tasks.Create(tapp.CreateInput{
						Title:   title,
						DueDate: s.today(),
					})
					return err
				})
			}
		})
		if len(s.taskToday) == 0 {
			s.empty(c, i18n.MsgTaskEmptyToday)
			return
		}
		rows := make([]*task.Task, 0, len(s.taskToday))
		finished := make([]*task.Task, 0, len(s.taskToday))
		for _, value := range s.taskToday {
			if !value.ParentID.IsZero() {
				continue
			}
			// A ticked-off task stays where it was: a row that disappears the moment
			// it is checked cannot be unchecked, and the day's list would silently
			// shrink instead of showing what got done.
			if value.Status.IsOpen() {
				rows = append(rows, value)
				continue
			}
			finished = append(finished, value)
		}
		// The children of the last parent are the last row, so the divider is
		// left off there: a rule under the final row closes a list that has ended.
		for index, value := range rows {
			closeRow := index == len(rows)-1 && len(finished) == 0
			children := s.taskChildren[value.ID]
			s.taskRow(c, value, true, closeRow && len(children) == 0)
			for childIndex, child := range children {
				ui.Row(c).PadLeft(30).Children(func() {
					s.taskRow(c, child, true, closeRow && childIndex == len(children)-1)
				})
			}
		}
		if len(finished) > 0 {
			if len(rows) > 0 {
				ui.Divider(c)
			}
			s.subtitle(c, i18n.T(s.lang, i18n.MsgTaskDoneSection, len(finished)))
			for index, value := range finished {
				s.taskRow(c, value, true, index == len(finished)-1)
			}
		}
		if len(rows) == 0 && len(finished) == 0 {
			s.empty(c, i18n.MsgTaskEmptyToday)
		}
	})
}

// taskHistory is the past, one section per day, newest first.
func (s *State) taskHistory(c ui.C) {
	if len(s.taskGroups) == 0 {
		s.empty(c, i18n.MsgTaskEmptyHistory)
		return
	}
	ui.Column(c).FillWidth().Gap(12).Children(func() {
		for _, group := range s.taskGroups {
			day := group.Day
			s.card(c, func() {
				s.subtitle(c, s.historyDayLabel(day))
				for index, value := range group.Tasks {
					s.taskRow(c, value, false, index == len(group.Tasks)-1)
				}
			})
		}
	})
}

// historyDayLabel names a day section the way an archive reads: its date, or the word for "no
// date". Never "today": a section that says "today" tomorrow is one nobody can point at.
func (s *State) historyDayLabel(day shared.LocalDate) string {
	if day.IsZero() {
		return i18n.T(s.lang, i18n.MsgTaskHistoryUndated)
	}
	return format.Date(s.lang, day)
}

// taskEditor is the modal behind "new" and "edit". It holds the whole task: a
// title, notes, a priority, a due date, an estimate and tags.
func (s *State) taskEditor(c ui.C) {
	draft := s.taskDraft
	if draft == nil || !draft.open {
		return
	}
	title := i18n.T(s.lang, i18n.MsgTaskNewTitle)
	if !draft.id.IsZero() {
		title = i18n.T(s.lang, i18n.MsgTaskEditTitle)
	}
	ui.Modal(c, &draft.open, title, func() {
		ui.Column(c).FillWidth().Gap(10).Children(func() {
			s.field(c, &draft.title, i18n.MsgTaskFieldTitle, i18n.T(s.lang, i18n.MsgTaskPlaceholder))
			s.dateField(c, dateFieldTaskDue, &draft.due, i18n.MsgTaskFieldDue,
				i18n.T(s.lang, i18n.MsgTaskDueNone))
			priorityName := i18n.T(s.lang, i18n.MsgTaskFieldPriority)
			ui.Text(c, priorityName).Muted().FontSize(12)
			priorities := []int{0, 1, 2, 3, 4}
			ui.Select(c, &draft.priority, priorities, func(value int) string {
				return format.Priority(s.lang, value)
			}).Label(priorityName)
			ui.Row(c).Gap(8).Children(func() {
				ui.Column(c).FillWidth().Gap(4).Children(func() {
					s.field(c, &draft.estimate, i18n.MsgTaskFieldEstimate, "1")
				})
				// How long one pomodoro runs for this task; it sits beside the estimate because the two answer
				// one question.
				ui.Column(c).FillWidth().Gap(4).Children(func() {
					s.field(c, &draft.pomodoro, i18n.MsgTaskFieldPomodoro,
						strconv.Itoa(defaultFocusMinutes))
				})
			})
			s.field(c, &draft.tags, i18n.MsgTaskFieldTags, "work, home")
			ui.Text(c, i18n.T(s.lang, i18n.MsgTaskFieldNotes)).Muted().FontSize(12)
			ui.TextArea(c, &draft.notes, "", "").FillWidth().Height(96)
			if !draft.id.IsZero() {
				if ui.Button(c, i18n.T(s.lang, i18n.MsgTaskActionSubtasks)).Ghost().Small().Clicked() {
					parent := draft.id
					draft.open = false
					subtask := s.newTaskDraft(parent)
					subtask.open = true
					s.taskDraft = subtask
				}
			}
		})
	}, func() {
		ui.Row(c).Gap(8).Children(func() {
			if ui.Button(c, i18n.T(s.lang, i18n.MsgCancel)).Secondary().Clicked() {
				draft.open = false
			}
			if ui.Button(c, i18n.T(s.lang, i18n.MsgSave)).Clicked() {
				// A rejected title leaves the dialog open with what was typed.
				s.apply(c, s.route, func() error {
					if err := s.saveTask(); err != nil {
						return err
					}
					draft.open = false
					return nil
				})
			}
		})
	})
}
