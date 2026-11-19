package presentation

import (
	"math"
	"strconv"
	"time"

	"github.com/vivaui/viva/gfx"
	"github.com/vivaui/viva/ui"

	"github.com/iotus3/simplelife/app/internal/domain/focus"
	"github.com/iotus3/simplelife/app/internal/domain/shared"
	"github.com/iotus3/simplelife/app/internal/presentation/format"
	"github.com/iotus3/simplelife/app/internal/presentation/i18n"
	"github.com/iotus3/simplelife/app/internal/presentation/icons"
	"github.com/iotus3/simplelife/app/internal/presentation/theme"
)

// focusPage is M3: the ring, the controls, today's numbers and the recovery of a
// session a crash left behind.
func (s *State) focusPage(c ui.C) {
	s.pageHeader(c, icons.Timer, i18n.MsgNavFocus, i18n.MsgNavFocusSub, func() {
		if used := s.focusStats.completed; used > 0 {
			s.tag(c, i18n.T(s.lang, i18n.MsgFocusStatCompleted, used), s.palette.Accent)
		}
	})
	s.recoveryBanner(c)
	ui.Row(c).FillWidth().Items(ui.Start).Gap(theme.Layout().Gap).Children(func() {
		ui.Column(c).FillWidth().Gap(theme.Layout().Gap).Children(func() {
			s.focusCard(c)
		})
		ui.Column(c).Width(330).Gap(theme.Layout().Gap).Children(func() {
			s.focusStatsCard(c)
			s.focusRecentCard(c)
		})
	})
	s.sessionExpiry(c)
}

// recoveryBanner asks what to do with a session that never finished.
func (s *State) recoveryBanner(c ui.C) {
	if !s.recovering || s.suspicious == nil {
		return
	}
	elapsed := s.suspicious.Elapsed(s.now())
	ui.Row(c).FillWidth().Items(ui.Center).Gap(12).Pad(14).
		Background(s.palette.Surface).Radius(theme.Layout().RadiusCard).
		Border(1, s.palette.Warning).
		Children(func() {
			s.iconChip(c, icons.Clock, s.palette.Warning, s.palette.SurfaceSoft, 36, 18)
			ui.Column(c).FillWidth().Gap(2).Children(func() {
				ui.Text(c, i18n.T(s.lang, i18n.MsgFocusRecoverTitle)).FontSize(14).
					Weight(gfx.WeightSemibold).Color(s.palette.Text)
				ui.Text(c, i18n.T(s.lang, i18n.MsgFocusRecoverBody, format.Duration(s.lang, elapsed))).
					Color(s.palette.Muted).FontSize(12).MaxLines(2)
			})
			if ui.Button(c, i18n.T(s.lang, i18n.MsgFocusRecoverCredit)).Icon(icons.Check).Small().Clicked() {
				s.recoverSession(c, true)
			}
			if ui.Button(c, i18n.T(s.lang, i18n.MsgFocusRecoverDiscard)).Secondary().Small().Clicked() {
				s.recoverSession(c, false)
			}
		})
}

// focusCard is the timer: the kind, the ring with the clock inside it, the
// controls, and what the next session is counted against.
func (s *State) focusCard(c ui.C) {
	s.cardTitled(c, icons.Target, i18n.MsgHomeFocus, func() {
		s.tag(c, i18n.T(s.lang, s.focusStateMessage(s.active)), s.activeStateTone())
	}, func() {
		kinds := []focus.Kind{focus.KindFocus, focus.KindShortBreak, focus.KindLongBreak}
		tabs(c, &s.focusKind, kinds, func(kind focus.Kind) string {
			return i18n.T(s.lang, s.kindMessage(kind))
		})
		s.ring(c)
		s.focusControls(c)
		ui.Divider(c)
		s.focusBinding(c)
	})
}

// activeStateTone colors the state of the timer: the accent while it runs, a
// warning while it waits, and quiet grey when nothing is running.
func (s *State) activeStateTone() ui.Color {
	if s.active == nil {
		return s.palette.Muted
	}
	if s.active.Phase == focus.PhasePaused {
		return s.palette.Warning
	}
	return s.palette.Accent
}

// ringState computes what the ring shows: how much of the planned time passed,
// and the time itself.
func (s *State) ringState() (float64, string) {
	now := s.now()
	if s.active == nil {
		planned := s.timerCfg.DurationFor(kindOf(s.focusKind))
		if s.focusKind == focus.KindFocus {
			// The bound task may carry its own length; the ring shows the length the
			// session will actually get, not the one the select happens to display.
			planned = s.plannedFor(s.focusTask)
		}
		return 0, format.Clock(planned)
	}
	return s.active.Progress(now), clockOf(s.active, now)
}

// ring draws the timer as a soft disc with sixty ticks around it, the ones the
// session has passed in the accent, and the clock in the middle. Viva paints no
// arcs, so the circle is made of small filled marks.
func (s *State) ring(c ui.C) {
	progress, label := s.ringState()
	if progress < 0 {
		progress = 0
	}
	if progress > 1 {
		progress = 1
	}
	accent := s.palette.Focus
	track := s.palette.Track
	ui.Column(c).FillWidth().Height(252).Items(ui.Center).Justify(ui.Center).Gap(6).
		Draw(func(d *ui.DrawContext) {
			const ticks = 60
			side := min(float32(d.Rect.Width), float32(d.Rect.Height)) - 8
			centerX := d.Rect.X + d.Rect.Width/2
			centerY := d.Rect.Y + d.Rect.Height/2
			// The disc: a quiet well the clock sits in, so the ring reads as a
			// dial rather than as loose dots on the card.
			d.Canvas.Rounded(ui.Rect{
				X:      centerX - side/2 + 14,
				Y:      centerY - side/2 + 14,
				Width:  side - 28,
				Height: side - 28,
			}, side/2-14, s.palette.SurfaceSoft)
			radius := side / 2
			for index := 0; index < ticks; index++ {
				angle := -math.Pi/2 + 2*math.Pi*float64(index)/ticks
				cos, sin := float32(math.Cos(angle)), float32(math.Sin(angle))
				color := track
				size := float32(5)
				if float64(index)/ticks < progress {
					color = accent
					size = 7
				}
				d.Canvas.Rounded(ui.Rect{
					X:      centerX + cos*radius - size/2,
					Y:      centerY + sin*radius - size/2,
					Width:  size,
					Height: size,
				}, size/2, color)
			}
		}).Children(func() {
		clock := label
		color := s.palette.Text
		if s.active != nil {
			color = s.palette.Accent
		}
		ui.Text(c, clock).FontSize(54).Weight(gfx.WeightSemibold).Mono().
			Color(color).TextCenter().MaxLines(1)
		// Idle, the strip above already says which kind is selected; running, this line
		// names the session and the task it is counted against.
		if s.active == nil {
			return
		}
		meta := i18n.T(s.lang, s.kindMessage(s.active.Kind))
		if name := s.taskTitle(s.active.TaskID); name != "" {
			meta = meta + " · " + name
		}
		ui.Text(c, meta).Color(s.palette.Muted).FontSize(12.5).TextCenter().
			MaxLines(1).Ellipsis()
	})
}

// focusControls is the one row of buttons whose meaning follows the phase.
func (s *State) focusControls(c ui.C) {
	ui.Row(c).FillWidth().Items(ui.Center).Justify(ui.Center).Gap(10).PadY(4).
		Children(func() {
			if s.active == nil {
				if ui.Button(c, i18n.T(s.lang, i18n.MsgFocusActionStart)).Icon(icons.Play).
					Clicked() {
					s.startSession(c)
				}
				return
			}
			if s.active.Phase == focus.PhasePaused {
				if ui.Button(c, i18n.T(s.lang, i18n.MsgFocusActionResume)).Icon(icons.Play).Clicked() {
					s.resumeSession(c)
				}
			} else if ui.Button(c, i18n.T(s.lang, i18n.MsgFocusActionPause)).Icon(icons.Pause).
				Secondary().Clicked() {
				s.pauseSession(c)
			}
			if ui.Button(c, i18n.T(s.lang, i18n.MsgFocusActionComplete)).Icon(icons.Check).
				Clicked() {
				s.completeSession(c)
			}
			if ui.Button(c, i18n.T(s.lang, i18n.MsgFocusActionCancel)).Destructive().Clicked() {
				s.cancelSession(c)
			}
		})
}

// focusBinding picks the task the next session is counted against, and the
// length of a focus session when the user wants one that is not the default.
func (s *State) focusBinding(c ui.C) {
	if s.active != nil {
		s.hint(c, i18n.T(s.lang, i18n.MsgFocusBindLocked))
		return
	}
	choices := make([]shared.ID, 0, len(s.taskToday)+1)
	choices = append(choices, "")
	for _, value := range s.taskToday {
		if value.Status.IsOpen() {
			choices = append(choices, value.ID)
		}
	}
	minutes := focusLengthChoices
	ui.Row(c).FillWidth().Gap(24).Children(func() {
		ui.Column(c).FillWidth().Gap(6).Children(func() {
			name := i18n.T(s.lang, i18n.MsgFocusBindTask)
			ui.Text(c, name).Color(s.palette.Muted).FontSize(12)
			ui.Select(c, &s.focusTask, choices, func(id shared.ID) string {
				if id == "" {
					return i18n.T(s.lang, i18n.MsgFocusBindNone)
				}
				return s.taskTitle(id)
			}).FillWidth().Label(name)
		})
		ui.Column(c).Width(220).Gap(6).Children(func() {
			lengthName := i18n.T(s.lang, i18n.MsgFocusPickDuration)
			ui.Text(c, lengthName).Color(s.palette.Muted).FontSize(12)
			ui.Select(c, &s.focusMin, minutes, func(value int) string {
				if value == focusDurationCustom {
					return i18n.T(s.lang, i18n.MsgFocusDurationCustom)
				}
				return i18n.T(s.lang, i18n.MsgFocusDurationValue, value)
			}).FillWidth().Label(lengthName)
			if minutes, ok := s.taskPomodoro(s.focusTask); ok {
				// Said out loud, because otherwise the select above looks broken: the
				// user picks 35 and the session runs for the task's own ten minutes.
				s.hint(c, i18n.T(s.lang, i18n.MsgFocusTaskLength, minutes))
				return
			}
			if s.focusMin != focusDurationCustom {
				return
			}
			// A typed length is only read when the custom choice is the one selected:
			// a field beside the other choices would be a second, silent answer to the
			// same question.
			s.field(c, &s.focusCustom, i18n.MsgFocusDurationMinutes, "15")
		})
	})
}

// taskPomodoro reports the length a task brings along, if it has one.
func (s *State) taskPomodoro(id shared.ID) (int, bool) {
	if id.IsZero() {
		return 0, false
	}
	value, err := s.deps.Tasks.Get(id)
	if err != nil || value.FocusMinutes <= 0 {
		return 0, false
	}
	return value.FocusMinutes, true
}

// focusStatsCard is today's summary and the reward the next pomodoro earns.
func (s *State) focusStatsCard(c ui.C) {
	s.cardTitled(c, icons.ChartColumn, i18n.MsgFocusTodayStats, nil, func() {
		s.statRow(c, icons.Target, i18n.T(s.lang, i18n.MsgHomePomodoros),
			strconv.Itoa(s.focusStats.completed), s.palette.Accent)
		s.statRow(c, icons.Clock, i18n.T(s.lang, i18n.MsgHomeFocusMinutes),
			strconv.Itoa(s.focusStats.minutes), s.palette.Muted)
		s.statRow(c, icons.Coffee, i18n.T(s.lang, i18n.MsgStatsInterruption),
			strconv.Itoa(s.focusStats.breaks), s.palette.Warning)
		ui.Divider(c)
		if !s.economy.Enabled {
			s.hint(c, i18n.T(s.lang, i18n.MsgFocusEconomyOff))
			return
		}
		sample := focus.Session{
			Kind:        focus.KindFocus,
			Phase:       focus.PhaseDone,
			Planned:     s.timerCfg.FocusDuration,
			PausedTotal: 0,
		}
		sample.StartedAt = s.now().Add(-sample.Planned)
		outcome, err := s.economy.Reward(&sample, shared.ZeroMoney(s.economy.Rate.Currency()))
		if err == nil && outcome.Applicable() {
			ui.Row(c).FillWidth().Items(ui.Center).Gap(6).Children(func() {
				ui.Icon(c, icons.Coins).Size(14, 14).Color(s.palette.Income)
				ui.Text(c, i18n.T(s.lang, i18n.MsgFocusEconomyReward, format.Money(outcome.Amount))).
					Color(s.palette.Income).FontSize(12).FillWidth().MaxLines(2)
			})
		}
		suggestion := s.deps.Focus.NextBreakSuggestion()
		message := i18n.MsgFocusNextShort
		if suggestion == focus.KindLongBreak {
			message = i18n.MsgFocusNextLong
		}
		s.hint(c, i18n.T(s.lang, message))
	})
}

// statRow is one number inside a card: the icon, what it counts, and the count.
func (s *State) statRow(c ui.C, icon gfx.Icon, label, value string, tone ui.Color) {
	ui.Row(c).FillWidth().Items(ui.Center).Gap(10).PadY(2).Children(func() {
		s.iconChip(c, icon, tone, s.palette.SurfaceSoft, 28, 14)
		ui.Text(c, label).FontSize(12.5).Color(s.palette.Muted).FillWidth().
			MaxLines(1).Ellipsis()
		ui.Text(c, value).FontSize(15).Weight(gfx.WeightSemibold).Color(s.palette.Text).
			MaxLines(1)
	})
}

// focusRecentCard lists the last sessions.
func (s *State) focusRecentCard(c ui.C) {
	s.cardTitled(c, icons.Clock, i18n.MsgFocusRecent, nil, func() {
		if len(s.focusRecent) == 0 {
			s.empty(c, i18n.MsgFocusRecentEmpty)
			return
		}
		for _, session := range s.focusRecent {
			value := session
			ui.Row(c).FillWidth().Items(ui.Center).Gap(10).PadY(3).Children(func() {
				s.iconChip(c, s.kindIcon(value.Kind), s.palette.Muted, s.palette.SurfaceSoft, 26, 13)
				ui.Text(c, i18n.T(s.lang, s.kindMessage(value.Kind))).FontSize(13).
					Color(s.palette.Text).FillWidth().MaxLines(1).Ellipsis()
				ui.Text(c, format.Duration(s.lang, time.Duration(value.CompletedFocusMinutes())*time.Minute)).
					Color(s.palette.Muted).FontSize(12).MaxLines(1)
				ui.Text(c, value.StartedAt.Format("15:04")).Color(s.palette.Faint).
					FontSize(12).MaxLines(1)
			})
		}
	})
}

// kindIcon is the mark of a session kind: a tomato for focus, a cup for a break.
func (s *State) kindIcon(kind focus.Kind) gfx.Icon {
	if kind == focus.KindFocus {
		return icons.Target
	}
	return icons.Coffee
}

// sessionExpiry settles a session whose planned time ran out while the window was
// open: the ring reaches zero, and the pomodoro is completed once.
func (s *State) sessionExpiry(c ui.C) {
	if s.active == nil || s.settling {
		return
	}
	if !s.active.Expired(s.now()) {
		return
	}
	s.settling = true
	s.completeSession(c)
}
