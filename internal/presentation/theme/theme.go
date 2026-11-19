// Package theme is the application's look: one palette per appearance, built from a small set of
// surfaces and one accent, so a page never picks a color of its own. Semantics stay semantic:
// income is green and expense is red in both appearances.
package theme

import (
	"github.com/vivaui/viva/gfx"
	"github.com/vivaui/viva/ui"
)

// Palette is one appearance.
type Palette struct {
	// Theme is what the framework's controls paint with.
	Theme *ui.Theme
	// Dark reports which appearance this is.
	Dark bool

	// Surfaces, from the furthest back to the closest.
	Shell       ui.Color // the navigation rail
	Workspace   ui.Color // the page behind the cards
	Surface     ui.Color // cards, inputs, dialogs
	SurfaceSoft ui.Color // a quiet fill: tags, header strips
	SurfaceHot  ui.Color // a selected row, a hovered one

	// Line separates surfaces; LineStrong outlines a control, where a hairline
	// would disappear. Text, Muted and Faint rank what is written.
	Line       ui.Color
	LineStrong ui.Color
	Text       ui.Color
	Muted      ui.Color
	Faint      ui.Color

	// Accent is the one warm color: primary buttons, the active page, and a
	// running pomodoro.
	Accent     ui.Color
	AccentSoft ui.Color // a tint of it, for icon chips
	OnAccent   ui.Color

	// Focus is the pomodoro ring and the running timer.
	Focus ui.Color
	// Income and Expense color ledger amounts.
	Income  ui.Color
	Expense ui.Color
	// Warning marks an overdue task and an exhausted budget.
	Warning ui.Color
	// Track is the unfilled part of a ring or a bar.
	Track ui.Color
}

// Metrics are the sizes the interface is laid out with. They are the framework's
// density plus the few the pages draw themselves, so one change reaches both.
type Metrics struct {
	// RadiusCard is the corner of a card, a dialog and a row.
	RadiusCard float32
	// RadiusControl is the corner of a button, a field and a tag.
	RadiusControl float32
	// PagePad is the gap between the window edge and the page.
	PagePad float32
	// CardPad is the inside of a card.
	CardPad float32
	// Gap is the space between cards, and between a header and a card.
	Gap float32
	// RowHeight is the height of a list row.
	RowHeight float32
	// SidebarWidth is the width of the navigation rail. It is fixed, so a label
	// that grows in English does not move the page.
	SidebarWidth float32
}

// Layout is the metrics shared by both appearances.
func Layout() Metrics {
	return Metrics{
		RadiusCard:    14,
		RadiusControl: 8,
		PagePad:       26,
		CardPad:       18,
		Gap:           16,
		RowHeight:     44,
		SidebarWidth:  236,
	}
}

// For returns the palette of an appearance.
func For(dark bool) *Palette {
	palette := lightPalette()
	if dark {
		palette = darkPalette()
	}
	palette.Dark = dark
	base := ui.LightTheme()
	if dark {
		base = ui.DarkTheme()
	}
	palette.Theme = themed(&base, palette)
	return palette
}

// themed paints the framework's controls with the palette.
func themed(base *ui.Theme, p *Palette) *ui.Theme {
	base.Background = p.Workspace
	base.Foreground = p.Text
	base.Card = p.Surface
	base.CardForeground = p.Text
	base.CardElevated = p.SurfaceSoft
	base.Popover = p.Surface
	base.PopoverForeground = p.Text
	base.Muted = p.SurfaceSoft
	base.MutedForeground = p.Muted
	base.Border = p.Line
	base.Hover = p.SurfaceSoft
	base.Input = p.LineStrong
	base.Primary = p.Accent
	base.PrimaryForeground = p.OnAccent
	base.Selection = p.AccentSoft
	base.SelectionForeground = p.Text
	base.Secondary = p.SurfaceSoft
	base.SecondaryForeground = p.Text
	base.Accent = p.AccentSoft
	base.AccentForeground = p.Text
	base.Ring = p.Accent
	base.Success = p.Income
	base.Danger = p.Expense
	base.DangerForeground = p.OnAccent
	base.Chart = [5]ui.Color{p.Accent, p.Income, p.Expense, p.Warning, p.Muted}
	base.Metrics = gfx.Metrics{
		ControlHeight: 34,
		RadiusControl: Layout().RadiusControl,
		RadiusSurface: Layout().RadiusCard,
		FontSizeS:     12,
		FontSizeM:     14,
		FontSizeL:     22,
		PadControlX:   12,
		PadSurface:    Layout().CardPad,
		GapItem:       8,
	}
	return base
}

// lightPalette is daylight, kept off pure white on the chrome so a large window
// does not glare, with the cards one step brighter than the page.
func lightPalette() *Palette {
	return &Palette{
		Shell:       ui.RGB(238, 240, 244),
		Workspace:   ui.RGB(245, 246, 249),
		Surface:     ui.RGB(255, 255, 255),
		SurfaceSoft: ui.RGB(241, 243, 247),
		SurfaceHot:  ui.RGB(229, 232, 238),
		Line:        ui.RGB(222, 226, 232),
		LineStrong:  ui.RGB(203, 209, 219),
		Text:        ui.RGB(29, 33, 41),
		Muted:       ui.RGB(95, 102, 114),
		Faint:       ui.RGB(140, 147, 159),
		Accent:      ui.RGB(226, 84, 43),
		AccentSoft:  ui.RGB(253, 237, 231),
		OnAccent:    ui.RGB(255, 255, 255),
		Focus:       ui.RGB(226, 84, 43),
		Income:      ui.RGB(17, 133, 90),
		Expense:     ui.RGB(194, 53, 47),
		Warning:     ui.RGB(169, 106, 0),
		Track:       ui.RGB(231, 234, 239),
	}
}

// darkPalette is graphite with the same accent: cool neutrals keep the window
// crisp without going black, and the cards sit above the workspace so the page
// reads as layered paper rather than one flat sheet.
func darkPalette() *Palette {
	return &Palette{
		Shell:       ui.RGB(22, 26, 32),
		Workspace:   ui.RGB(15, 18, 22),
		Surface:     ui.RGB(27, 32, 39),
		SurfaceSoft: ui.RGB(35, 41, 50),
		SurfaceHot:  ui.RGB(44, 51, 62),
		Line:        ui.RGB(42, 49, 59),
		LineStrong:  ui.RGB(64, 73, 86),
		Text:        ui.RGB(230, 234, 240),
		Muted:       ui.RGB(152, 161, 174),
		Faint:       ui.RGB(107, 116, 128),
		Accent:      ui.RGB(255, 122, 69),
		AccentSoft:  ui.RGB(58, 40, 33),
		OnAccent:    ui.RGB(26, 18, 12),
		Focus:       ui.RGB(255, 122, 69),
		Income:      ui.RGB(82, 208, 139),
		Expense:     ui.RGB(242, 109, 109),
		Warning:     ui.RGB(232, 192, 72),
		Track:       ui.RGB(38, 44, 53),
	}
}
