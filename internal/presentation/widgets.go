package presentation

import (
	"time"

	"github.com/vivaui/viva/app"
	"github.com/vivaui/viva/gfx"
	"github.com/vivaui/viva/ui"

	"github.com/iotus3/simplelife/app/internal/domain/focus"
	"github.com/iotus3/simplelife/app/internal/domain/ledger"
	"github.com/iotus3/simplelife/app/internal/domain/shared"
	"github.com/iotus3/simplelife/app/internal/presentation/format"
	"github.com/iotus3/simplelife/app/internal/presentation/i18n"
	"github.com/iotus3/simplelife/app/internal/presentation/icons"
	"github.com/iotus3/simplelife/app/internal/presentation/theme"
)

// The page is built from a few pieces: a header, cards, and rows inside them.
// Every one of them asks the palette for its colors, so an appearance change
// reaches them all.

// pageHeader is the top of a page: the page's icon and title, the sentence that
// says what the page is for, and its actions, right-aligned.
func (s *State) pageHeader(c ui.C, icon gfx.Icon, title, subtitle i18n.MessageID, actions func()) {
	ui.Row(c).FillWidth().Items(ui.Center).Gap(14).Children(func() {
		s.iconChip(c, icon, s.palette.Accent, s.palette.AccentSoft, 40, 20)
		ui.Column(c).Gap(3).FillWidth().Children(func() {
			ui.Text(c, i18n.T(s.lang, title)).FontSize(22).Weight(gfx.WeightSemibold).
				Color(s.palette.Text).MaxLines(1)
			ui.Text(c, i18n.T(s.lang, subtitle)).FontSize(12.5).
				Color(s.palette.Muted).MaxLines(1).Ellipsis()
		})
		if actions != nil {
			ui.Row(c).Items(ui.Center).Gap(8).Children(actions)
		}
	})
}

// heading is a bare page title, for a page that has no subtitle of its own.
func (s *State) heading(c ui.C, id i18n.MessageID) {
	ui.Text(c, i18n.T(s.lang, id)).FontSize(22).Weight(gfx.WeightSemibold).Color(s.palette.Text)
}

// sectionTitle is the title of a card: a small icon and a word.
func (s *State) sectionTitle(c ui.C, icon gfx.Icon, text string) {
	ui.Row(c).FillWidth().Items(ui.Center).Gap(8).Children(func() {
		if icon.Commands != nil || icon.Layers != nil {
			ui.Icon(c, icon).Size(15, 15).Color(s.palette.Muted)
		}
		ui.Text(c, text).FontSize(14).Weight(gfx.WeightSemibold).Color(s.palette.Text)
	})
}

// subtitle is a section title inside a page, without an icon.
func (s *State) subtitle(c ui.C, text string) {
	s.sectionTitle(c, gfx.Icon{}, text)
}

// tabs is a tab strip at its natural width: a strip of two tabs belongs at the
// left of the card, not stretched across it.
func tabs[T comparable](c ui.C, value *T, values []T, label func(T) string) ui.Box {
	var box ui.Box
	ui.Row(c).Children(func() { box = ui.Tabs(c, value, values, label) })
	return box
}

// pageTabs switches what a whole page shows, in the transparent variant with the
// underline under the active tab.
func pageTabs[T comparable](c ui.C, value *T, values []T, label func(T) string) {
	ui.Row(c).Children(func() { ui.Tabs(c, value, values, label).Line() })
}

// label is the caption above a control that is not a text field, and the words it returns are
// the control's accessible name: text beside a field is only ink.
func (s *State) label(c ui.C, id i18n.MessageID) string {
	text := i18n.T(s.lang, id)
	ui.Text(c, text).FontSize(12).Color(s.palette.Muted).MaxLines(1)
	return text
}

// hint is secondary text under a control.
func (s *State) hint(c ui.C, text string) {
	ui.Text(c, text).Color(s.palette.Faint).FontSize(12).MaxLines(3)
}

// empty says a list has nothing in it yet: an icon, the sentence, and room to
// breathe, so an empty page reads as ready rather than broken.
func (s *State) empty(c ui.C, id i18n.MessageID) {
	ui.Column(c).FillWidth().Items(ui.Center).Justify(ui.Center).Gap(8).
		PadY(26).Children(func() {
		ui.Icon(c, icons.Inbox).Size(22, 22).Color(s.palette.Faint)
		ui.Text(c, i18n.T(s.lang, id)).Color(s.palette.Muted).FontSize(13).
			TextCenter().MaxLines(3)
	})
}

// card is a raised container: the surface every group of controls sits on.
func (s *State) card(c ui.C, fn func()) ui.Box {
	m := theme.Layout()
	box := ui.Column(c).FillWidth().Pad(m.CardPad).Gap(12).
		Background(s.palette.Surface).
		Border(1, s.palette.Line).
		Radius(m.RadiusCard)
	box.Children(fn)
	return box
}

// cardTitled is a card whose first row is its title, with the actions of the
// section on the right.
func (s *State) cardTitled(c ui.C, icon gfx.Icon, title i18n.MessageID, actions func(), fn func()) {
	s.card(c, func() {
		ui.Row(c).FillWidth().Items(ui.Center).Gap(8).Children(func() {
			if icon.Commands != nil || icon.Layers != nil {
				ui.Icon(c, icon).Size(15, 15).Color(s.palette.Muted)
			}
			ui.Text(c, i18n.T(s.lang, title)).FontSize(14).Weight(gfx.WeightSemibold).
				Color(s.palette.Text).FillWidth()
			if actions != nil {
				ui.Row(c).Items(ui.Center).Gap(6).Children(actions)
			}
		})
		fn()
	})
}

// iconChip is a rounded square holding one icon: the mark of a page, of a metric
// or of a row.
func (s *State) iconChip(c ui.C, icon gfx.Icon, tint, fill ui.Color, size, iconSize float32) {
	ui.Row(c).Size(size, size).Items(ui.Center).Justify(ui.Center).
		Background(fill).Radius(theme.Layout().RadiusControl + 2).
		Children(func() {
			ui.Icon(c, icon).Size(iconSize, iconSize).Color(tint)
		})
}

// metric is one number with its label and icon, for the dashboard.
func (s *State) metric(c ui.C, icon gfx.Icon, label i18n.MessageID, value string, tone ui.Color) {
	ui.Row(c).FillWidth().Items(ui.Center).Gap(12).Pad(14).
		Background(s.palette.Surface).Border(1, s.palette.Line).
		Radius(theme.Layout().RadiusCard).Children(func() {
		s.iconChip(c, icon, tone, s.palette.SurfaceSoft, 36, 18)
		ui.Column(c).Gap(2).FillWidth().Children(func() {
			ui.Text(c, value).FontSize(19).Weight(gfx.WeightSemibold).
				Color(s.palette.Text).MaxLines(1).Ellipsis()
			ui.Text(c, i18n.T(s.lang, label)).FontSize(11.5).
				Color(s.palette.Muted).MaxLines(1).Ellipsis()
		})
	})
}

// tag is a small rounded label: a project tag, a count, a state.
func (s *State) tag(c ui.C, text string, tone ui.Color) {
	ui.Text(c, text).FontSize(11.5).Color(tone).PadSides(3, 8, 3, 8).
		Background(s.palette.SurfaceSoft).Radius(theme.Layout().RadiusControl - 2)
}

// field is a labelled input: the caption above says what it is, and the
// placeholder shows the shape of a value. The label is also the control's
// accessible name, so the caption is not read twice.
func (s *State) field(c ui.C, value *string, label i18n.MessageID, placeholder string) {
	ui.Text(c, i18n.T(s.lang, label)).FontSize(12).Color(s.palette.Muted)
	ui.TextField(c, value, i18n.T(s.lang, label), placeholder).FillWidth()
}

// rowHint is a muted line that fills the space beside row actions.
func (s *State) rowHint(c ui.C, text string) {
	ui.Text(c, text).Color(s.palette.Muted).FontSize(12).MaxLines(1).Ellipsis().FillWidth()
}

// sectionRow is a horizontal band: a title on the left, actions on the right.
func (s *State) sectionRow(c ui.C, title string, actions func()) {
	ui.Row(c).FillWidth().Items(ui.Center).Gap(8).Children(func() {
		ui.Text(c, title).FontSize(14).Weight(gfx.WeightSemibold).
			Color(s.palette.Text).FillWidth()
		actions()
	})
}

// iconSize is the side of a small icon button. Small() would bring the label back,
// so an icon-only button is square and sized by its height instead.
const iconSize = 28

// iconAction is a small icon-only button, named for assistive technology by the
// label it is given.
func (s *State) iconAction(c ui.C, icon gfx.Icon, label i18n.MessageID) bool {
	return s.iconActionNamed(c, icon, i18n.T(s.lang, label))
}

// iconActionNamed is iconAction with the name spelled out, for a button whose name
// has to say which of several look-alikes it is: the calendar of one date field, not
// of the other one beside it.
func (s *State) iconActionNamed(c ui.C, icon gfx.Icon, name string) bool {
	return ui.Button(c, name).Icon(icon).IconOnly().Ghost().Height(iconSize).Clicked()
}

// confirmDialog is the one delete confirmation of the window.
func (s *State) confirmDialog(c ui.C) {
	if !s.confirm.open {
		return
	}
	message := i18n.MsgTaskDeleteConfirm
	switch s.confirm.kind {
	case confirmEntry:
		message = i18n.MsgLedgerDeleteConfirm
	case confirmTemplate:
		message = i18n.MsgTemplateDeleteConfirm
	case confirmCategory:
		message = i18n.MsgLedgerCategoryDeleteAsk
	case confirmGoal:
		message = i18n.MsgGoalDeleteAsk
	case confirmAccount:
		message = i18n.MsgLedgerAccountDeleteAsk
	case confirmBudget:
		message = i18n.MsgLedgerBudgetDeleteAsk
	}
	kind, id := s.confirm.kind, s.confirm.id
	ui.Modal(c, &s.confirm.open, i18n.T(s.lang, message), func() {
		ui.Text(c, i18n.T(s.lang, message)).Color(s.palette.Muted)
		if kind == confirmCategory {
			// Said before the press, not after it: the delete is refused for a
			// category anything still points at, and a refusal after the fact reads
			// like a bug.
			s.hint(c, i18n.T(s.lang, i18n.MsgLedgerCategoryDeleteBlock))
		}
	}, func() {
		ui.Row(c).Gap(8).Items(ui.Center).Children(func() {
			if ui.Button(c, i18n.T(s.lang, i18n.MsgCancel)).Secondary().Clicked() {
				s.confirm.open = false
			}
			if ui.Button(c, i18n.T(s.lang, i18n.MsgDelete)).Destructive().Clicked() {
				s.confirm.open = false
				s.apply(c, s.route, func() error {
					switch kind {
					case confirmTask:
						return s.deps.Tasks.Delete(id)
					case confirmEntry:
						return s.deps.Ledger.DeleteEntry(id)
					case confirmTemplate:
						return s.deps.Templates.Delete(id)
					case confirmCategory:
						// The service refuses a category that entries or children still
						// point at, and the refusal is the answer: it is reported as it is.
						return s.deps.Ledger.DeleteCategory(id)
					case confirmGoal:
						return s.deps.Goals.Delete(id)
					case confirmAccount:
						// Refused while an entry still points at it, and for a seeded
						// account; the refusal is reported as it is.
						return s.deps.Ledger.DeleteAccount(id)
					case confirmBudget:
						return s.deps.Ledger.DeleteBudget(id)
					}
					return nil
				})
			}
		})
	})
}

// brandBitmap converts the window icon into the bitmap the sidebar paints: the
// platform gives straight RGBA and the canvas wants premultiplied BGRA.
func brandBitmap(icon app.WindowIcon) gfx.Bitmap {
	size := icon.Size
	pixels := make([]byte, size*size*4)
	for i := 0; i+3 < len(icon.RGBA); i += 4 {
		r, g, b, a := icon.RGBA[i], icon.RGBA[i+1], icon.RGBA[i+2], icon.RGBA[i+3]
		scale := uint32(a)
		pixels[i+0] = uint8(uint32(b) * scale / 255)
		pixels[i+1] = uint8(uint32(g) * scale / 255)
		pixels[i+2] = uint8(uint32(r) * scale / 255)
		pixels[i+3] = a
	}
	return gfx.Bitmap{Pix: pixels, Width: size, Height: size, Stride: size * 4, ID: 1, Version: 1}
}

// kindMessage is the label of a session kind.
func (s *State) kindMessage(kind focus.Kind) i18n.MessageID {
	switch kind {
	case focus.KindShortBreak:
		return i18n.MsgFocusKindShort
	case focus.KindLongBreak:
		return i18n.MsgFocusKindLong
	}
	return i18n.MsgFocusKindFocus
}

// focusStateMessage is the label of a session phase.
func (s *State) focusStateMessage(session *focus.Session) i18n.MessageID {
	if session == nil {
		return i18n.MsgFocusStateIdle
	}
	if session.Phase == focus.PhasePaused {
		return i18n.MsgFocusStatePaused
	}
	return i18n.MsgFocusStateRunning
}

// accountKindMessage is the label of an account kind.
func (s *State) accountKindMessage(kind ledger.AccountKind) i18n.MessageID {
	switch kind {
	case ledger.AccountDebitCard:
		return i18n.MsgLedgerAccountKindDebit
	case ledger.AccountWallet:
		return i18n.MsgLedgerAccountKindWallet
	case ledger.AccountSavings:
		return i18n.MsgLedgerAccountKindSaving
	case ledger.AccountCredit:
		return i18n.MsgLedgerAccountKindCredit
	}
	return i18n.MsgLedgerAccountKindCash
}

// entryKindMessage is the label of an entry kind.
func (s *State) entryKindMessage(kind ledger.EntryKind) i18n.MessageID {
	switch kind {
	case ledger.EntryIncome:
		return i18n.MsgLedgerKindIncome
	case ledger.EntryTransfer:
		return i18n.MsgLedgerKindTransfer
	}
	return i18n.MsgLedgerKindExpense
}

// clockOf renders what the timer shows for a session: what is left when it counts
// down, what has passed when it counts up.
func clockOf(session *focus.Session, now time.Time) string {
	if session == nil {
		return format.Clock(0)
	}
	if session.IsCountUp() {
		return format.Clock(session.Elapsed(now))
	}
	return format.Clock(session.Remaining(now))
}

// dueLabel describes a task's due date relative to today.
func (s *State) dueLabel(day shared.LocalDate) string {
	if day.IsZero() {
		return i18n.T(s.lang, i18n.MsgTaskDueNone)
	}
	today := s.today()
	switch {
	case day.Before(today):
		return i18n.T(s.lang, i18n.MsgTaskDueOverdue) + " · " + format.Date(s.lang, day)
	case day.Equal(today):
		return i18n.T(s.lang, i18n.MsgTaskDueToday)
	}
	return format.Date(s.lang, day)
}
