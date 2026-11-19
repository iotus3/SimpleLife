package presentation

import (
	"strconv"
	"time"

	"github.com/vivaui/viva/gfx"
	"github.com/vivaui/viva/ui"

	"github.com/iotus3/simplelife/app/internal/domain/shared"
	"github.com/iotus3/simplelife/app/internal/domain/template"
	"github.com/iotus3/simplelife/app/internal/presentation/format"
	"github.com/iotus3/simplelife/app/internal/presentation/i18n"
	"github.com/iotus3/simplelife/app/internal/presentation/icons"
	"github.com/iotus3/simplelife/app/internal/presentation/theme"
)

// templatesPage is M9: the templates that create tomorrow's tasks, their rules,
// and the log of what they produced.
func (s *State) templatesPage(c ui.C) {
	s.pageHeader(c, icons.LayoutTemplate, i18n.MsgTemplateTitle, i18n.MsgNavTemplateSub, func() {
		if ui.Button(c, i18n.T(s.lang, i18n.MsgTemplateExpand)).Icon(icons.Check).Secondary().Small().Clicked() {
			s.apply(c, RouteTemplates, func() error {
				result, err := s.deps.Templates.Expand(s.today())
				if err != nil {
					return err
				}
				s.toast(i18n.T(s.lang, i18n.MsgTemplateExpanded, result.Created, result.Skipped), ui.ToastSuccess)
				return nil
			})
		}
		// The preview is a look, not a place: the same button puts it away again, so
		// the page never keeps a card the user has finished reading.
		previewLabel := i18n.MsgTemplatePreview
		previewIcon := icons.ListTodo
		if s.templatePreviewOpen {
			previewLabel, previewIcon = i18n.MsgTemplatePreviewClose, icons.X
		}
		if ui.Button(c, i18n.T(s.lang, previewLabel)).Icon(previewIcon).Secondary().Small().Clicked() {
			if s.templatePreviewOpen {
				s.templatePreviewOpen = false
				s.templatePreview = nil
				s.repaint()
				return
			}
			s.templatePreviewOpen = true
			s.apply(c, RouteTemplates, func() error {
				items, err := s.deps.Templates.Preview(s.today())
				if err != nil {
					return err
				}
				s.templatePreview = items
				return nil
			})
		}
		if ui.Button(c, i18n.T(s.lang, i18n.MsgTemplateNew)).Icon(icons.Plus).Clicked() {
			s.resetTemplateDraft()
			s.templateDraft.open = true
		}
	})

	ui.Row(c).FillWidth().Items(ui.Start).Gap(theme.Layout().Gap).Children(func() {
		ui.Column(c).FillWidth().Gap(theme.Layout().Gap).Children(func() {
			if len(s.templates) == 0 {
				s.card(c, func() {
					s.empty(c, i18n.MsgTemplateEmpty)
				})
			}
			for _, value := range s.templates {
				s.templateCard(c, value)
			}
			if s.templatePreviewOpen {
				s.previewCard(c)
			}
		})
		ui.Column(c).Width(320).Gap(theme.Layout().Gap).Children(func() {
			s.logsCard(c)
		})
	})
	s.templateEditor(c)
}

// templateCard is one template: what it is called, when it fires, and the one todo
// it writes.
func (s *State) templateCard(c ui.C, value *template.Template) {
	id := value.ID
	s.card(c, func() {
		ui.Row(c).FillWidth().Items(ui.Center).Gap(12).Children(func() {
			tint := s.palette.Income
			if !value.Enabled {
				tint = s.palette.Faint
			}
			s.iconChip(c, icons.LayoutTemplate, tint, s.palette.SurfaceSoft, 36, 18)
			ui.Column(c).FillWidth().Gap(3).Children(func() {
				ui.Row(c).Items(ui.Center).Gap(8).Children(func() {
					ui.Text(c, value.Name).FontSize(14).Weight(gfx.WeightSemibold).
						Color(s.palette.Text).MaxLines(1).Ellipsis()
					if !value.Enabled {
						s.tag(c, i18n.T(s.lang, i18n.MsgTemplateDisabled), s.palette.Muted)
					}
				})
				ui.Text(c, s.ruleText(value.Rule)).Color(s.palette.Muted).FontSize(12).
					MaxLines(1).Ellipsis()
				next := templateNextRun(value, s.today())
				if !next.IsZero() {
					ui.Text(c, i18n.T(s.lang, i18n.MsgTemplateNextRun, format.Date(s.lang, next))).
						Color(s.palette.Faint).FontSize(12).MaxLines(1)
				}
			})
			if s.iconAction(c, icons.Pencil, i18n.MsgTemplateEdit) {
				s.openTemplateEditor(id)
			}
			if s.iconAction(c, icons.Trash2, i18n.MsgDelete) {
				s.askDelete(confirmTemplate, id)
			}
		})
		// The todo the template writes, on a quiet strip of its own: it is the part
		// the user came to read, so it is not one more tag in a row.
		ui.Row(c).FillWidth().Items(ui.Center).Gap(10).Pad(10).
			Background(s.palette.SurfaceSoft).Radius(theme.Layout().RadiusControl).
			Children(func() {
				ui.Icon(c, icons.ListTodo).Size(14, 14).Color(s.palette.Muted)
				ui.Text(c, value.Task.Title).FontSize(13.5).Color(s.palette.Text).
					FillWidth().MaxLines(1).Ellipsis()
				if value.Task.Estimate > 0 {
					s.tag(c, i18n.T(s.lang, i18n.MsgTaskEstimateCount, value.Task.Estimate),
						s.palette.Muted)
				}
				// What the generated tasks will get as a pomodoro length, so a template
				// that sets one is visible without opening the editor.
				if value.Task.FocusMinutes > 0 {
					s.tag(c, i18n.T(s.lang, i18n.MsgTemplateFocusLength, value.Task.FocusMinutes),
						s.palette.Muted)
				}
				if value.Task.DueOffsetDays > 0 {
					s.tag(c, i18n.T(s.lang, i18n.MsgTemplateFieldOffset)+" +"+
						strconv.Itoa(value.Task.DueOffsetDays), s.palette.Muted)
				}
			})
	})
}

// ruleText describes a trigger rule in the language of the interface.
func (s *State) ruleText(rule template.TriggerRule) string {
	switch rule.Kind {
	case template.TriggerWeekdays:
		return i18n.T(s.lang, i18n.MsgTemplateRuleWeekly) + " · " + format.Weekdays(s.lang, rule.Weekdays)
	case template.TriggerMonthly:
		return i18n.T(s.lang, i18n.MsgTemplateRuleDayOfMonth, rule.DayOfMonth)
	case template.TriggerSpecificDates:
		parts := ""
		for index, day := range rule.Dates {
			if index > 0 {
				parts += ", "
			}
			parts += day.String()
		}
		return i18n.T(s.lang, i18n.MsgTemplateRuleDates) + " · " + parts
	case template.TriggerEveryNDays:
		return i18n.T(s.lang, i18n.MsgTemplateRuleEveryNText, rule.EveryNDays)
	}
	return i18n.T(s.lang, i18n.MsgTemplateRuleDaily)
}

// templateNextRun finds the next day the rule fires, scanning a year at most.
func templateNextRun(value *template.Template, from shared.LocalDate) shared.LocalDate {
	if !value.Enabled {
		return shared.LocalDate{}
	}
	for offset := 1; offset <= 366; offset++ {
		day := from.AddDays(offset)
		if value.Rule.Matches(day) {
			return day
		}
	}
	return shared.LocalDate{}
}

func (s *State) previewCard(c ui.C) {
	s.cardTitled(c, icons.ListTodo, i18n.MsgTemplatePreview, nil, func() {
		if len(s.templatePreview) == 0 {
			s.empty(c, i18n.MsgTemplatePreviewEmpty)
			return
		}
		for _, item := range s.templatePreview {
			value := item
			ui.Row(c).FillWidth().Items(ui.Center).Gap(10).PadY(3).Children(func() {
				tone := s.palette.Income
				status := i18n.T(s.lang, i18n.MsgTemplatePreviewNew)
				if value.Exists {
					tone = s.palette.Faint
					status = i18n.T(s.lang, i18n.MsgTemplatePreviewExists)
				}
				ui.Text(c, value.Title).FontSize(13.5).Color(s.palette.Text).FillWidth().
					MaxLines(1).Ellipsis()
				s.tag(c, status, tone)
			})
		}
	})
}

func (s *State) logsCard(c ui.C) {
	s.cardTitled(c, icons.Clock, i18n.MsgTemplateLogs, nil, func() {
		if len(s.templateLogs) == 0 {
			s.empty(c, i18n.MsgTemplateLogsEmpty)
			return
		}
		for _, entry := range s.templateLogs {
			log := entry
			ui.Row(c).FillWidth().Items(ui.Center).Gap(10).PadY(4).Children(func() {
				s.iconChip(c, icons.Check, s.palette.Income, s.palette.SurfaceSoft, 26, 13)
				ui.Column(c).FillWidth().Gap(1).Children(func() {
					ui.Text(c, format.Date(s.lang, log.Day)).FontSize(13).
						Color(s.palette.Text).MaxLines(1)
					ui.Text(c, i18n.T(s.lang, i18n.MsgTemplateExpanded, log.Created, log.Skipped)).
						Color(s.palette.Faint).FontSize(11.5).MaxLines(1).Ellipsis()
				})
			})
		}
	})
}

// templateEditor edits a template: its name, the one todo it writes, and the rule that decides
// when it writes it. The footer holds only the two buttons that close the dialog.
func (s *State) templateEditor(c ui.C) {
	draft := s.templateDraft
	if draft == nil || !draft.open {
		return
	}
	title := i18n.T(s.lang, i18n.MsgTemplateEdit)
	if draft.id.IsZero() {
		title = i18n.T(s.lang, i18n.MsgTemplateNew)
	}
	ui.ModalWith(c, &draft.open, title, ui.ModalSpec{Width: 560}, func() {
		ui.Column(c).FillWidth().Gap(14).Children(func() {
			s.templateSection(c, i18n.MsgTemplateSectionBlog, func() {
				s.field(c, &draft.name, i18n.MsgTemplateFieldName,
					i18n.T(s.lang, i18n.MsgTemplateNamePlaceholder))
				ui.Switch(c, &draft.enabled, i18n.T(s.lang, i18n.MsgTemplateFieldEnabled))
			})
			s.templateSection(c, i18n.MsgTemplateSectionTask, func() {
				s.field(c, &draft.taskTitle, i18n.MsgTemplateFieldTitle,
					i18n.T(s.lang, i18n.MsgTemplateTitlePlaceholder))
				s.field(c, &draft.taskNotes, i18n.MsgTemplateFieldNotes,
					i18n.T(s.lang, i18n.MsgOptional))
				// Two rows of two: the estimate and the length answer one question, the offset and the priority
				// are independent. Three columns squeeze the English captions past each other.
				ui.Row(c).FillWidth().Gap(12).Children(func() {
					ui.Column(c).FillWidth().Gap(4).Children(func() {
						s.field(c, &draft.taskEstimate, i18n.MsgTemplateFieldEstimate, "1")
					})
					ui.Column(c).FillWidth().Gap(4).Children(func() {
						s.field(c, &draft.taskFocus, i18n.MsgTaskFieldPomodoro,
							strconv.Itoa(defaultFocusMinutes))
					})
				})
				ui.Row(c).FillWidth().Gap(12).Children(func() {
					ui.Column(c).FillWidth().Gap(4).Children(func() {
						s.field(c, &draft.taskOffset, i18n.MsgTemplateFieldOffset, "0")
					})
					ui.Column(c).FillWidth().Gap(4).Children(func() {
						priorityName := s.label(c, i18n.MsgPriority)
						priority := draft.taskPriority
						values := []int{0, 1, 2, 3, 4}
						ui.Select(c, &priority, values, func(value int) string {
							return i18n.T(s.lang, taskPriorityMessage(value))
						}).FillWidth().Label(priorityName)
						if priority != draft.taskPriority {
							draft.taskPriority = priority
						}
					})
				})
			})
			s.templateSection(c, i18n.MsgTemplateSectionRule, func() {
				kinds := []template.TriggerKind{
					template.TriggerDaily, template.TriggerWeekdays, template.TriggerMonthly,
					template.TriggerSpecificDates, template.TriggerEveryNDays,
				}
				tabs(c, &draft.kind, kinds, func(kind template.TriggerKind) string {
					return i18n.T(s.lang, triggerMessage(kind))
				})
				switch draft.kind {
				case template.TriggerWeekdays:
					ui.Flow(c).FillWidth().Gap(14).Children(func() {
						for index := 0; index < 7; index++ {
							day := time.Weekday(index)
							on := draft.weekdays[index]
							box := ui.Checkbox(c, &on, format.Weekday(s.lang, day))
							if box.Changed() {
								draft.weekdays[index] = on
							}
						}
					})
				case template.TriggerMonthly:
					s.field(c, &draft.dayOfMonth, i18n.MsgTemplateRuleDayOfMonth, "1")
				case template.TriggerEveryNDays:
					s.field(c, &draft.everyN, i18n.MsgTemplateRuleEveryN, "2")
				case template.TriggerSpecificDates:
					s.field(c, &draft.dates, i18n.MsgTemplateRuleDates,
						"2026-11-17, 2026-12-01")
				}
			})
		})
	}, func() {
		ui.Row(c).Items(ui.Center).Gap(8).Children(func() {
			ui.Spacer(c)
			if ui.Button(c, i18n.T(s.lang, i18n.MsgCancel)).Secondary().Clicked() {
				draft.open = false
			}
			if ui.Button(c, i18n.T(s.lang, i18n.MsgSave)).Icon(icons.Check).Clicked() {
				s.apply(c, RouteTemplates, func() error {
					if err := s.saveTemplate(); err != nil {
						return err
					}
					draft.open = false
					return nil
				})
			}
		})
	})
}

// templateSection is one labelled block of the editor: a quiet title, then the
// controls that belong to it.
func (s *State) templateSection(c ui.C, title i18n.MessageID, children func()) {
	ui.Column(c).FillWidth().Gap(8).Pad(12).
		Background(s.palette.SurfaceSoft).Radius(theme.Layout().RadiusControl).
		Children(func() {
			ui.Text(c, i18n.T(s.lang, title)).FontSize(12).Weight(gfx.WeightSemibold).
				Color(s.palette.Muted).MaxLines(1)
			children()
		})
}

// taskPriorityMessage is the label of a task priority.
func taskPriorityMessage(priority int) i18n.MessageID {
	switch priority {
	case 1:
		return i18n.MsgTaskPriorityLow
	case 2:
		return i18n.MsgTaskPriorityMedium
	case 3:
		return i18n.MsgTaskPriorityHigh
	case 4:
		return i18n.MsgTaskPriorityUrgent
	}
	return i18n.MsgTaskPriorityUnset
}

func triggerMessage(kind template.TriggerKind) i18n.MessageID {
	switch kind {
	case template.TriggerWeekdays:
		return i18n.MsgTemplateRuleWeekly
	case template.TriggerMonthly:
		return i18n.MsgTemplateRuleMonthly
	case template.TriggerSpecificDates:
		return i18n.MsgTemplateRuleDates
	case template.TriggerEveryNDays:
		return i18n.MsgTemplateRuleEveryN
	}
	return i18n.MsgTemplateRuleDaily
}
