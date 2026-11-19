package presentation

import (
	"strconv"

	"github.com/vivaui/viva/gfx"
	"github.com/vivaui/viva/ui"

	"github.com/iotus3/simplelife/app/internal/domain/goal"
	"github.com/iotus3/simplelife/app/internal/presentation/format"
	"github.com/iotus3/simplelife/app/internal/presentation/i18n"
	"github.com/iotus3/simplelife/app/internal/presentation/icons"
	"github.com/iotus3/simplelife/app/internal/presentation/theme"
)

// goalSoonDays is the point at which a countdown stops being a plan and starts
// being a deadline: a week left is worth a warmer color than a year.
const goalSoonDays = 7

// goalsPage is the countdown board: one banner per goal, newest deadline first, and
// the button that adds another. Each banner answers the whole question by itself —
// what it is called, how far along it is, and how many days are left.
func (s *State) goalsPage(c ui.C) {
	s.pageHeader(c, icons.Flag, i18n.MsgNavGoals, i18n.MsgNavGoalsSub, func() {
		if ui.Button(c, i18n.T(s.lang, i18n.MsgGoalNew)).Icon(icons.Plus).Clicked() {
			s.openGoalEditor("")
		}
	})
	if len(s.goals) == 0 {
		s.card(c, func() {
			s.empty(c, i18n.MsgGoalEmpty)
		})
	}
	ui.Column(c).FillWidth().Gap(theme.Layout().Gap).Children(func() {
		for _, value := range s.goals {
			s.goalCard(c, value)
		}
	})
	s.goalEditor(c)
}

// goalCard is one countdown as a banner: name and note on the left, the days left as the one
// large number, the span behind a bar, and a quiet fill once the day has passed.
func (s *State) goalCard(c ui.C, value *goal.Goal) {
	id := value.ID
	today := s.today()
	progress := value.ProgressOf(today)
	done := value.Finished(today)
	fill, ink := s.palette.AccentSoft, s.palette.Accent
	if done {
		fill, ink = s.palette.SurfaceSoft, s.palette.Faint
	} else if progress.Remaining <= goalSoonDays {
		ink = s.palette.Warning
	}
	ui.Column(c).FillWidth().Pad(theme.Layout().CardPad).Gap(12).
		Background(fill).Border(1, s.palette.Line).Radius(theme.Layout().RadiusCard).
		Children(func() {
			ui.Row(c).FillWidth().Items(ui.Start).Gap(14).Children(func() {
				s.iconChip(c, icons.Flag, ink, s.palette.Surface, 40, 20)
				ui.Column(c).FillWidth().Gap(4).Children(func() {
					ui.Row(c).FillWidth().Items(ui.Center).Gap(8).Children(func() {
						ui.Text(c, value.Name).FontSize(16).Weight(gfx.WeightSemibold).
							Color(s.palette.Text).MaxLines(1).Ellipsis()
						s.tag(c, i18n.T(s.lang, s.goalStateMessage(value, progress)), ink)
					})
					if value.Note != "" {
						ui.Text(c, value.Note).FontSize(12.5).Color(s.palette.Muted).
							MaxLines(2).Ellipsis()
					}
				})
				if s.iconAction(c, icons.Pencil, i18n.MsgGoalEditAction) {
					s.openGoalEditor(id)
				}
				if s.iconAction(c, icons.Trash2, i18n.MsgGoalDeleteAction) {
					s.askDelete(confirmGoal, id)
				}
			})
			ui.Row(c).FillWidth().Items(ui.Center).Gap(8).Children(func() {
				ui.Column(c).FillWidth().Gap(6).Children(func() {
					ui.Text(c, i18n.T(s.lang, i18n.MsgGoalElapsed, progress.Elapsed, progress.Total)).
						FontSize(12.5).Color(s.palette.Muted).MaxLines(1)
					s.bar(c, ratio(progress.Elapsed, progress.Total), ink)
					ui.Text(c, i18n.T(s.lang, i18n.MsgGoalDates,
						format.Date(s.lang, value.Start), format.Date(s.lang, value.End))).
						FontSize(11.5).Color(s.palette.Faint).MaxLines(1).Ellipsis()
				})
				// The countdown sits on one line: the words around the number are small, the number is the one
				// thing the card is for. Stacking the caption above it read as a typo.
				ui.Row(c).Items(ui.Center).Justify(ui.End).Gap(6).Children(func() {
					ui.Text(c, i18n.T(s.lang, i18n.MsgGoalLeftLabel)).FontSize(12.5).
						Color(s.palette.Muted).MaxLines(1)
					ui.Text(c, strconv.Itoa(progress.Remaining)).FontSize(32).
						Weight(gfx.WeightSemibold).Color(ink).MaxLines(1)
					ui.Text(c, i18n.T(s.lang, i18n.MsgGoalDayUnit)).FontSize(12.5).
						Color(s.palette.Muted).MaxLines(1)
				})
			})
		})
}

// goalStateMessage is the tag on a banner: running, nearly due, or finished.
func (s *State) goalStateMessage(value *goal.Goal, progress goal.Progress) i18n.MessageID {
	switch {
	case value.Finished(s.today()):
		return i18n.MsgGoalStateDone
	case progress.Remaining <= goalSoonDays:
		return i18n.MsgGoalStateSoon
	}
	return i18n.MsgGoalStateActive
}

// goalEditor is the dialog behind "new goal" and the pencil. The footer holds only the two
// buttons that close it, never one that belongs to the form.
func (s *State) goalEditor(c ui.C) {
	draft := s.goalDraft
	if draft == nil || !draft.open {
		return
	}
	title := i18n.T(s.lang, i18n.MsgGoalNew)
	confirm := i18n.MsgGoalConfirm
	if !draft.id.IsZero() {
		title = i18n.T(s.lang, i18n.MsgGoalEditTitle)
		confirm = i18n.MsgSave
	}
	ui.ModalWith(c, &draft.open, title, ui.ModalSpec{Width: 520}, func() {
		ui.Column(c).FillWidth().Gap(10).Children(func() {
			s.field(c, &draft.name, i18n.MsgGoalFieldName, i18n.T(s.lang, i18n.MsgGoalNameHolder))
			s.field(c, &draft.note, i18n.MsgGoalFieldNote, i18n.T(s.lang, i18n.MsgGoalNoteHolder))
			ui.Row(c).FillWidth().Gap(12).Children(func() {
				ui.Column(c).FillWidth().Children(func() {
					s.dateField(c, dateFieldGoalStart, &draft.start, i18n.MsgGoalFieldStart,
						format.Date(s.lang, s.today()))
				})
				ui.Column(c).FillWidth().Children(func() {
					s.dateField(c, dateFieldGoalEnd, &draft.end, i18n.MsgGoalFieldEnd,
						format.Date(s.lang, s.today().AddDays(goalDefaultDays)))
				})
			})
			s.hint(c, i18n.T(s.lang, i18n.MsgGoalHint))
		})
	}, func() {
		ui.Row(c).Items(ui.Center).Gap(8).Children(func() {
			ui.Spacer(c)
			if ui.Button(c, i18n.T(s.lang, i18n.MsgCancel)).Secondary().Clicked() {
				draft.open = false
			}
			if ui.Button(c, i18n.T(s.lang, confirm)).Icon(icons.Check).Clicked() {
				s.apply(c, RouteGoals, func() error {
					if err := s.saveGoal(); err != nil {
						return err
					}
					draft.open = false
					return nil
				})
			}
		})
	})
}
