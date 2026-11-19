package presentation

import (
	"math"
	"strconv"

	"github.com/vivaui/viva/gfx"
	"github.com/vivaui/viva/ui"

	"github.com/iotus3/simplelife/app/internal/presentation/format"
	"github.com/iotus3/simplelife/app/internal/presentation/i18n"
	"github.com/iotus3/simplelife/app/internal/presentation/icons"
	"github.com/iotus3/simplelife/app/internal/presentation/theme"
)

// statsPage is the pomodoros of a chosen span, the completion rate of the tasks, and where the
// money went.
func (s *State) statsPage(c ui.C) {
	s.pageHeader(c, icons.ChartColumn, i18n.MsgNavStats, i18n.MsgNavStatsSub, nil)
	pageTabs(c, &s.statsRange, statsRangeChoices, func(value int) string {
		switch value {
		case statsRangeWeek:
			return i18n.T(s.lang, i18n.MsgStatsRangeWeek)
		case statsRangeCustom:
			return i18n.T(s.lang, i18n.MsgStatsRangeCustom)
		}
		return i18n.T(s.lang, i18n.MsgStatsRangeDays, value)
	})
	if s.statsRange == statsRangeCustom {
		s.statsRangeField(c)
	}
	// A new span is new data: the days are read again when the window changes, and only then. The
	// window is read after the tabs, because a tab writes its choice while the page is declared.
	from, to, ok := s.statsWindow()
	s.statsValid = ok
	if !ok {
		s.card(c, func() {
			s.empty(c, i18n.MsgStatsRangeInvalid)
		})
		return
	}
	if from != s.statsWantFrom || to != s.statsWantTo {
		s.statsWantFrom, s.statsWantTo = from, to
		s.markAll()
	}
	if from != s.statsLoadedFrom || to != s.statsLoadedTo {
		s.card(c, func() {
			s.empty(c, i18n.MsgStatsPending)
		})
		return
	}
	if len(s.statsBars) == 0 {
		s.card(c, func() {
			s.empty(c, i18n.MsgStatsEmpty)
		})
		return
	}
	ui.Row(c).FillWidth().Items(ui.Start).Gap(theme.Layout().Gap).Children(func() {
		ui.Column(c).FillWidth().Gap(theme.Layout().Gap).Children(func() {
			s.weekCard(c)
			s.categoryCard(c)
		})
		ui.Column(c).Width(320).Gap(theme.Layout().Gap).Children(func() {
			s.statsNumbers(c)
		})
	})
}

// statsRangeField is the two dates of a custom span. They are the same date fields
// the editor uses, so a day is typed or picked from the calendar either way.
func (s *State) statsRangeField(c ui.C) {
	s.card(c, func() {
		ui.Row(c).FillWidth().Items(ui.End).Gap(10).Children(func() {
			ui.Column(c).FillWidth().Children(func() {
				s.dateField(c, dateFieldStatsFrom, &s.statsFromText, i18n.MsgStatsRangeFrom,
					format.Date(s.lang, s.today().AddDays(-(statsRangeWeek-1))))
			})
			ui.Column(c).FillWidth().Children(func() {
				s.dateField(c, dateFieldStatsTo, &s.statsToText, i18n.MsgStatsRangeTo,
					format.Date(s.lang, s.today()))
			})
		})
		s.hint(c, i18n.T(s.lang, i18n.MsgStatsRangeHint))
	})
}

// weekCard draws the span as columns.
func (s *State) weekCard(c ui.C) {
	s.cardTitled(c, icons.ChartColumn, i18n.MsgStatsFocusByDay, nil, func() {
		peak := 1
		for _, bar := range s.statsBars {
			if bar.pomodoros > peak {
				peak = bar.pomodoros
			}
		}
		s.columns(c, peak)
		s.hint(c, i18n.T(s.lang, i18n.MsgStatsFocusTotal, s.statsTotals.completed, s.statsTotals.minutes))
	})
}

// columns paints the bars and the labels under them. A long span gets a label every
// few columns instead of all of them: thirty overlapping dates say nothing.
func (s *State) columns(c ui.C, peak int) {
	accent := s.palette.Focus
	track := s.palette.Track
	heights := make([]float64, len(s.statsBars))
	for index, bar := range s.statsBars {
		heights[index] = float64(bar.pomodoros) / float64(peak)
	}
	ui.Row(c).FillWidth().Height(150).Draw(func(d *ui.DrawContext) {
		count := len(heights)
		if count == 0 {
			return
		}
		slot := d.Rect.Width / float32(count)
		width := slot * 0.46
		for index := 0; index < count; index++ {
			x := d.Rect.X + slot*float32(index) + (slot-width)/2
			d.Canvas.Rounded(ui.Rect{X: x, Y: d.Rect.Y, Width: width, Height: d.Rect.Height},
				width/2, track)
			height := float32(math.Round(heights[index] * float64(d.Rect.Height)))
			if height <= 0 {
				continue
			}
			d.Canvas.Rounded(ui.Rect{
				X:      x,
				Y:      d.Rect.Y + d.Rect.Height - height,
				Width:  width,
				Height: height,
			}, width/2, accent)
		}
	})
	step := 1
	if len(s.statsBars) > 7 {
		step = (len(s.statsBars) + 6) / 7
	}
	ui.Row(c).FillWidth().Children(func() {
		for index, bar := range s.statsBars {
			ui.Column(c).FillWidth().Items(ui.Center).Children(func() {
				label := ""
				switch {
				case len(s.statsBars) <= 7:
					label = format.Weekday(s.lang, bar.day.Weekday())
				case index%step == 0:
					label = bar.day.Midnight().Format("01-02")
				}
				ui.Text(c, label).Color(s.palette.Muted).FontSize(11.5).MaxLines(1)
			})
		}
	})
}

// categoryCard lists what the month's spending went to.
func (s *State) categoryCard(c ui.C) {
	s.cardTitled(c, icons.Tag, i18n.MsgStatsByCategory, nil, func() {
		if len(s.spending) == 0 {
			s.empty(c, i18n.MsgStatsEmptyMoney)
			return
		}
		largest := int64(0)
		for _, spend := range s.spending {
			if minor := spend.Spent.Abs().Minor(); minor > largest {
				largest = minor
			}
		}
		for _, spend := range s.spending {
			fraction := 0.0
			if largest > 0 {
				fraction = float64(spend.Spent.Abs().Minor()) / float64(largest)
			}
			ui.Column(c).FillWidth().Gap(5).PadY(4).Children(func() {
				ui.Row(c).FillWidth().Items(ui.Center).Gap(10).Children(func() {
					ui.Text(c, i18n.DisplayName(s.lang, spend.Category.Name, spend.Category.BuiltinKey)).
						FontSize(13.5).Color(s.palette.Text).FillWidth().MaxLines(1).Ellipsis()
					ui.Text(c, format.Money(spend.Spent)).FontSize(13.5).
						Weight(gfx.WeightSemibold).Color(s.palette.Text).MaxLines(1)
				})
				s.bar(c, fraction, s.palette.Expense)
			})
		}
	})
}

// statsNumbers is the tall column of single numbers, summed over the same span as
// the chart above it.
func (s *State) statsNumbers(c ui.C) {
	s.cardTitled(c, icons.Target, i18n.MsgStatsCompletion, nil, func() {
		done, total := s.completion()
		ui.Column(c).FillWidth().Items(ui.Center).Gap(4).PadY(6).Children(func() {
			ui.Text(c, format.Percent(ratio(done, total))).FontSize(34).
				Weight(gfx.WeightSemibold).Color(s.palette.Text)
			ui.Text(c, i18n.T(s.lang, s.lang.Plural(done, i18n.MsgTaskCountOne, i18n.MsgTaskCount), done)).
				Color(s.palette.Muted).FontSize(12).TextCenter()
		})
		ui.Divider(c)
		s.statRow(c, icons.Target, i18n.T(s.lang, i18n.MsgHomePomodoros),
			strconv.Itoa(s.statsTotals.completed), s.palette.Accent)
		s.statRow(c, icons.Clock, i18n.T(s.lang, i18n.MsgHomeFocusMinutes),
			strconv.Itoa(s.statsTotals.minutes), s.palette.Muted)
		s.statRow(c, icons.Coffee, i18n.T(s.lang, i18n.MsgStatsInterruption),
			strconv.Itoa(s.statsTotals.breaks), s.palette.Warning)
	})
}

// completion counts what was finished today against what was planned for today.
func (s *State) completion() (int, int) {
	done := 0
	total := 0
	for _, value := range s.overview.tasks {
		total++
		if !value.Status.IsOpen() {
			done++
		}
	}
	for _, group := range s.taskGroups {
		for _, value := range group.Tasks {
			total++
			if !value.Status.IsOpen() {
				done++
			}
		}
	}
	return done, total
}

func ratio(part, whole int) float64 {
	if whole <= 0 {
		return 0
	}
	return float64(part) / float64(whole)
}
