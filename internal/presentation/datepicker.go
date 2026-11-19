package presentation

import (
	"strconv"
	"time"

	"github.com/vivaui/viva/components"
	"github.com/vivaui/viva/ui"

	"github.com/iotus3/simplelife/app/internal/domain/shared"
	"github.com/iotus3/simplelife/app/internal/presentation/format"
	"github.com/iotus3/simplelife/app/internal/presentation/i18n"
	"github.com/iotus3/simplelife/app/internal/presentation/icons"
	"github.com/iotus3/simplelife/app/internal/presentation/theme"
)

// The fields that take a day. Naming the field rather than keeping a pointer to the
// value keeps the calendar a piece of the page's state: a draft that is reset closes
// its calendar with it, instead of leaving a calendar editing a value nobody shows.
const (
	dateFieldTaskDue    = "task.due"
	dateFieldEntry      = "entry.date"
	dateFieldExportFrom = "export.from"
	dateFieldExportTo   = "export.to"
	dateFieldStatsFrom  = "stats.from"
	dateFieldGoalStart  = "goal.start"
	dateFieldGoalEnd    = "goal.end"
	dateFieldStatsTo    = "stats.to"
)

// datePick is the calendar one date field has open, and the month it shows. One at a
// time: opening another field's calendar closes it.
type datePick struct {
	field string
	month shared.Month
}

// open reports whether field's calendar is the one showing.
func (p datePick) open(field string) bool { return p.field == field }

// dateField is a day input: the field takes a typed date and the button beside it opens a month
// calendar over it, because a form that grows by a grid pushes its own buttons off a small
// window.
func (s *State) dateField(c ui.C, field string, value *string, label i18n.MessageID, placeholder string) {
	ui.Column(c).FillWidth().Gap(4).Children(func() {
		name := s.label(c, label)
		ui.Row(c).FillWidth().Items(ui.Center).Gap(6).Children(func() {
			ui.TextField(c, value, name, placeholder).FillWidth()
			button := ui.Button(c, i18n.T(s.lang, i18n.MsgDatePick, name)).
				Icon(icons.Calendar).IconOnly().Ghost().Height(iconSize)
			if button.Clicked() {
				s.toggleDatePick(field, *value)
			}
			open := s.datePick.open(field)
			dismissed := ui.PopoverWith(c, button, ui.PopoverSpec{
				Align: ui.Start, Width: datePickerWidth(),
			}, &open, func() {
				s.dateCalendar(c, field, value)
			})
			if dismissed && s.datePick.open(field) {
				// A press outside, Escape or the button again: the panel asked to
				// close, and the state it was declared from follows it.
				s.datePick = datePick{}
			}
		})
	})
}

// toggleDatePick opens a field's calendar, or closes it when it is the one showing.
// It opens on the month of the value, so changing a date a few days back is one click.
func (s *State) toggleDatePick(field, text string) {
	if s.datePick.open(field) {
		s.datePick = datePick{}
		return
	}
	day, err := format.ParseDate(s.lang, text)
	if err != nil || day.IsZero() {
		day = s.today()
	}
	s.datePick = datePick{field: field, month: day.AsMonth()}
}

// pickDate writes the chosen day into the field and closes the calendar. The empty
// day is "no date": a task without a due date, or an open range bound.
func (s *State) pickDate(field string, value *string, day shared.LocalDate) {
	*value = ""
	if !day.IsZero() {
		*value = format.Date(s.lang, day)
	}
	s.datePick = datePick{}
	s.repaint()
}

// dateCalendar is the panel: the month grid, its arrows, and the two shortcuts that
// keep a value out of the grid (today, nothing).
func (s *State) dateCalendar(c ui.C, field string, value *string) {
	month := s.datePick.month
	if month.IsZero() {
		month = s.today().AsMonth()
	}
	selected, _ := format.ParseDate(s.lang, *value)
	cells, days := s.monthCells(c, month, selected)
	previous, next := ui.ID(c), ui.ID(c)
	// The scope the panel's controls are declared in, asked from this page so the
	// grid can claim the pointer while it is open (see the Draw below).
	panelScope := ui.ID(c).Scope().At(0)
	ui.Column(c).FillWidth().Gap(8).Children(func() {
		ui.Row(c).FillWidth().Items(ui.Center).Children(func() {
			ui.Row(c).Width(calendarWidth()).Height(calendarHeight()).
				Draw(func(d *ui.DrawContext) {
					// A date field can sit in a dialog, which traps the pointer to its own scope. Viva paints this
					// panel in a scope of its own, so it claims one while it is open, or the days would be drawn
					// over the form yet answer no press.
					if !d.Canvas.TrappedFocusScope().IsZero() {
						d.Canvas.TrapFocus(panelScope)
					}
					calendar := components.BeginCalendar(d.Canvas, *d.Theme, d.Rect, components.CalendarOptions{
						MonthTitle:    format.MonthLabel(s.lang, month),
						Weekdays:      s.weekdayHeader(),
						Days:          cells,
						PreviousID:    previous,
						NextID:        next,
						PreviousLabel: i18n.T(s.lang, i18n.MsgDateMonthPrev),
						NextLabel:     i18n.T(s.lang, i18n.MsgDateMonthNext),
					})
					calendar.Paint()
					// The arrows and the days are the calendar's own controls, so a
					// click on them is read from its paint.
					if d.Canvas.Clicked(previous) {
						s.datePick.month = month.AddMonths(-1)
						s.repaint()
					} else if d.Canvas.Clicked(next) {
						s.datePick.month = month.AddMonths(1)
						s.repaint()
					} else {
						for index, day := range days {
							if day.IsZero() {
								continue
							}
							if d.Canvas.Clicked(cells[index].ID) {
								s.pickDate(field, value, day)
								break
							}
						}
					}
				})
		})
		s.datePick.month = month
		ui.Row(c).FillWidth().Items(ui.Center).Gap(6).Children(func() {
			if ui.Button(c, i18n.T(s.lang, i18n.MsgToday)).Ghost().Small().Clicked() {
				s.pickDate(field, value, s.today())
			}
			ui.Spacer(c)
			clear := ui.Button(c, i18n.T(s.lang, i18n.MsgDateClear)).Ghost().Small()
			if valueEmpty(value) {
				clear = clear.Disabled(true)
			}
			if clear.Clicked() {
				s.pickDate(field, value, shared.LocalDate{})
			}
		})
	})
}

// monthCells builds one month of cells, in display order: empty leading cells up to
// the first weekday, then the days, then empty trailing cells to the end of the week.
// days is parallel to cells so a click on a cell finds its date.
func (s *State) monthCells(c ui.C, month shared.Month, selected shared.LocalDate) ([]components.CalendarDay, []shared.LocalDate) {
	first := shared.DateOf(month.Start())
	// Weeks start on Monday: the week a person plans, and the order the template
	// rule's weekday picker uses.
	offset := (int(first.Weekday()) + 6) % 7
	count := first.DaysInMonth()
	total := ((offset + count + 6) / 7) * 7
	today := s.today()
	cells := make([]components.CalendarDay, total)
	days := make([]shared.LocalDate, total)
	for index := 0; index < total; index++ {
		dayNumber := index - offset + 1
		if dayNumber < 1 || dayNumber > count {
			continue
		}
		day := shared.NewLocalDate(month.Year, month.Month, dayNumber)
		days[index] = day
		cells[index] = components.CalendarDay{
			ID:                 ui.ID(c),
			Label:              strconv.Itoa(dayNumber),
			AccessibilityLabel: format.Date(s.lang, day),
			Value:              day.String(),
			Selected:           day.Equal(selected),
			Today:              day.Equal(today),
		}
	}
	return cells, days
}

// weekdayHeader is the seven column captions, Monday first.
func (s *State) weekdayHeader() [7]string {
	var header [7]string
	for index := range header {
		// Index 0 is Monday, index 6 is Sunday.
		header[index] = format.Weekday(s.lang, time.Weekday((index+1)%7))
	}
	return header
}

// calendarCellSide is the side of one day cell. Viva's calendar takes it from the
// control height of the theme it is given, so the width this page reserves for the
// grid has to be measured from the same number.
func calendarCellSide() float32 {
	// Both appearances share the density, so either palette answers the same.
	return theme.For(false).Theme.Metrics.ControlHeight - 4
}

// calendarWidth is seven cells and the grid's own padding.
func calendarWidth() float32 { return 7*calendarCellSide() + 16 }

// calendarHeight is the room the grid takes: a caption, the weekday row, and six
// weeks, which is the most a month can need.
func calendarHeight() float32 { return 6*(calendarCellSide()+8) + 64 }

// datePickerWidth is the panel's width: the grid plus the popover's own padding.
func datePickerWidth() float32 { return calendarWidth() + 64 }

// valueEmpty reports an unset text value, for the controls that clear one.
func valueEmpty(value *string) bool { return value == nil || *value == "" }
