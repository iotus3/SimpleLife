package theme

import (
	"testing"
)

// TestControlBorderStandsOutOfTheSurface guards the one mistake the palette can
// make silently: an input border that is the surface's own color, which hides a
// checkbox against a card.
func TestControlBorderStandsOutOfTheSurface(t *testing.T) {
	for _, dark := range []bool{false, true} {
		palette := For(dark)
		if palette.Theme.Input == palette.Surface {
			t.Errorf("dark=%v: the control border equals the surface, so a checkbox on a card draws no edge",
				dark)
		}
		if palette.Theme.Input == palette.Workspace {
			t.Errorf("dark=%v: the control border equals the workspace", dark)
		}
		if palette.LineStrong == palette.Line {
			t.Errorf("dark=%v: the strong line is the hairline, so a control outline is not stronger than a separator",
				dark)
		}
	}
}

// TestSurfacesAreOrdered checks the layering the pages rely on: the rail is
// behind the workspace, the workspace behind the cards, and a hover is a step
// above a resting fill.
func TestSurfacesAreOrdered(t *testing.T) {
	for _, dark := range []bool{false, true} {
		palette := For(dark)
		luma := func(c [3]float32) float32 { return 0.299*c[0] + 0.587*c[1] + 0.114*c[2] }
		shell := [3]float32{palette.Shell.Red, palette.Shell.Green, palette.Shell.Blue}
		workspace := [3]float32{palette.Workspace.Red, palette.Workspace.Green, palette.Workspace.Blue}
		surface := [3]float32{palette.Surface.Red, palette.Surface.Green, palette.Surface.Blue}
		soft := [3]float32{palette.SurfaceSoft.Red, palette.SurfaceSoft.Green, palette.SurfaceSoft.Blue}
		hot := [3]float32{palette.SurfaceHot.Red, palette.SurfaceHot.Green, palette.SurfaceHot.Blue}
		if !dark {
			if !(luma(surface) > luma(workspace) && luma(workspace) >= luma(shell)) {
				t.Errorf("light: cards should sit above the page, and the rail below it")
			}
			if !(luma(hot) < luma(soft)) {
				t.Errorf("light: a hovered fill should be deeper than a resting one")
			}
		} else {
			if !(luma(surface) > luma(workspace)) {
				t.Errorf("dark: cards should sit above the page")
			}
			if !(luma(hot) > luma(soft)) {
				t.Errorf("dark: a hovered fill should be lighter than a resting one")
			}
		}
	}
}

// TestSemanticColorsStaySemantic keeps income green and expense red in both
// appearances: a page asks for a meaning, never for a hue.
func TestSemanticColorsStaySemantic(t *testing.T) {
	for _, dark := range []bool{false, true} {
		palette := For(dark)
		if !(palette.Income.Green > palette.Income.Red && palette.Income.Green > palette.Income.Blue) {
			t.Errorf("dark=%v: income is not green", dark)
		}
		if !(palette.Expense.Red > palette.Expense.Green) {
			t.Errorf("dark=%v: expense is not red", dark)
		}
		if palette.OnAccent == palette.Accent {
			t.Errorf("dark=%v: text on the accent is the accent", dark)
		}
	}
}

// TestThemeIsWiredToThePalette checks the framework's controls take their colors
// from the palette rather than from the stock theme.
func TestThemeIsWiredToThePalette(t *testing.T) {
	palette := For(false)
	if palette.Theme.Primary != palette.Accent {
		t.Error("the primary of the controls is not the accent")
	}
	if palette.Theme.Card != palette.Surface {
		t.Error("a card of the controls is not the palette's surface")
	}
	if palette.Theme.Background != palette.Workspace {
		t.Error("the background of the controls is not the workspace")
	}
	if palette.Theme.Metrics.ControlHeight <= 0 {
		t.Error("the control height is not set, so every control falls back to the stock one")
	}
}

// TestLayoutIsPositive guards the metrics the pages divide by.
func TestLayoutIsPositive(t *testing.T) {
	m := Layout()
	values := map[string]float32{
		"RadiusCard":    m.RadiusCard,
		"RadiusControl": m.RadiusControl,
		"PagePad":       m.PagePad,
		"CardPad":       m.CardPad,
		"Gap":           m.Gap,
		"RowHeight":     m.RowHeight,
		"SidebarWidth":  m.SidebarWidth,
	}
	for name, value := range values {
		if value <= 0 {
			t.Errorf("%s = %v, want a positive size", name, value)
		}
	}
}
