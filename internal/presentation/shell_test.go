package presentation

import (
	"fmt"
	"testing"
	"time"

	"github.com/vivaui/viva/app"
	"github.com/vivaui/viva/gfx"

	focusapp "github.com/iotus3/simplelife/app/internal/application/focus"
	"github.com/iotus3/simplelife/app/internal/domain/focus"
	"github.com/iotus3/simplelife/app/internal/domain/ledger"
)

// TestBrandBitmapPremultiplies checks the conversion the sidebar depends on: the
// platform hands over straight RGBA, the canvas wants premultiplied BGRA, and a
// wrong order turns the mark's warm orange into blue.
func TestBrandBitmapPremultiplies(t *testing.T) {
	icon := app.WindowIcon{
		Size: 2,
		RGBA: []byte{
			255, 0, 0, 255, // opaque red
			0, 0, 255, 255, // opaque blue
			10, 20, 30, 0, // fully transparent
			100, 150, 200, 128, // half transparent
		},
	}
	bitmap := brandBitmap(icon)
	if bitmap.Width != 2 || bitmap.Height != 2 || bitmap.Stride != 8 {
		t.Fatalf("bitmap %dx%d stride %d, want 2x2 stride 8", bitmap.Width, bitmap.Height, bitmap.Stride)
	}
	if len(bitmap.Pix) != 16 {
		t.Fatalf("pixels %d bytes, want 16", len(bitmap.Pix))
	}
	// Opaque red stays red, and it is stored blue-last.
	if got := bitmap.Pix[0:4]; got[0] != 0 || got[1] != 0 || got[2] != 255 || got[3] != 255 {
		t.Errorf("opaque red became %v, want BGRA 0 0 255 255", got)
	}
	// Opaque blue is blue in the first byte.
	if got := bitmap.Pix[4:8]; got[0] != 255 || got[1] != 0 || got[2] != 0 || got[3] != 255 {
		t.Errorf("opaque blue became %v, want BGRA 255 0 0 255", got)
	}
	// A transparent pixel carries no color, whatever it was given.
	if got := bitmap.Pix[8:12]; got[0] != 0 || got[1] != 0 || got[2] != 0 || got[3] != 0 {
		t.Errorf("transparent pixel became %v, want zero", got)
	}
	// Half transparency halves the color and keeps the alpha.
	got := bitmap.Pix[12:16]
	if got[3] != 128 || got[0] != uint8(200*128/255) || got[1] != uint8(150*128/255) || got[2] != uint8(100*128/255) {
		t.Errorf("half transparent pixel became %v", got)
	}
}

// TestBrandBitmapSurvivesAShortIcon keeps the conversion from reading past a
// bitmap the platform truncated.
func TestBrandBitmapSurvivesAShortIcon(t *testing.T) {
	short := app.WindowIcon{Size: 4, RGBA: []byte{255, 0, 0, 255}}
	bitmap := brandBitmap(short)
	if len(bitmap.Pix) != 4*4*4 {
		t.Fatalf("pixels %d bytes, want a full 4x4 bitmap", len(bitmap.Pix))
	}
	if got := bitmap.Pix[0:4]; got[2] != 255 {
		t.Errorf("the pixel the platform did send was dropped: %v", got)
	}
}

// TestEveryRouteHasItsOwnIcon guards the rail: two pages sharing a mark would be
// indistinguishable at a glance.
func TestEveryRouteHasItsOwnIcon(t *testing.T) {
	seen := map[string]Route{}
	for _, route := range routes {
		icon := routeIcon(route)
		if icon.Commands == nil && icon.Layers == nil {
			t.Errorf("route %v has an empty icon", route)
			continue
		}
		key := iconKey(icon)
		if other, ok := seen[key]; ok {
			t.Errorf("routes %v and %v draw the same icon", other, route)
		}
		seen[key] = route
	}
	fallback := iconKey(routeIcon(Route(99)))
	if fallback != iconKey(routeIcon(RouteHome)) {
		t.Error("an unknown route should fall back to the home mark")
	}
}

// iconKey renders an icon to a comparable string: its geometry decides what is
// drawn, so two icons with the same commands are the same mark.
func iconKey(icon gfx.Icon) string {
	key := fmt.Sprintf("%.2f/%.2f/%.2f/%v/%v", icon.ViewBox, icon.ViewHeight, icon.Stroke, icon.Cap, icon.Join)
	for _, command := range icon.Commands {
		key += fmt.Sprintf("|%d %.3f %.3f %.3f %.3f %.3f %.3f", command.Kind,
			command.Point.X, command.Point.Y,
			command.Control1.X, command.Control1.Y,
			command.Control2.X, command.Control2.Y)
	}
	for _, layer := range icon.Layers {
		key += fmt.Sprintf("#%v", layer)
	}
	return key
}

// TestWindowColorKeepsThePalette checks the window clear color is the workspace,
// not a stock white that would flash before the first paint.
func TestWindowColorKeepsThePalette(t *testing.T) {
	fixture := newFixture(t)
	got := windowColor(fixture.state.palette.Workspace)
	want := app.Color{
		Red:   float64(fixture.state.palette.Workspace.Red),
		Green: float64(fixture.state.palette.Workspace.Green),
		Blue:  float64(fixture.state.palette.Workspace.Blue),
		Alpha: float64(fixture.state.palette.Workspace.Alpha),
	}
	if got != want {
		t.Errorf("windowColor = %+v, want %+v", got, want)
	}
}

// TestRowMarksAreStable pins what each kind of row draws, so a mark keeps meaning
// the same thing on every page.
func TestRowMarksAreStable(t *testing.T) {
	fixture := newFixture(t)
	state := fixture.state

	focusMark := iconKey(state.kindIcon(focus.KindFocus))
	breakMark := iconKey(state.kindIcon(focus.KindShortBreak))
	if focusMark == breakMark {
		t.Error("a focus session and a break draw the same mark")
	}
	if iconKey(state.kindIcon(focus.KindLongBreak)) != breakMark {
		t.Error("both breaks should carry the same mark")
	}

	if iconKey(state.entryIcon(&ledger.Entry{Kind: ledger.EntryExpense})) ==
		iconKey(state.entryIcon(&ledger.Entry{Kind: ledger.EntryIncome})) {
		t.Error("money in and money out draw the same mark")
	}
}

// TestActiveStateToneFollowsTheTimer checks the state tag of the focus page: the
// accent while it runs, a warning while it waits, and quiet grey when idle.
func TestActiveStateToneFollowsTheTimer(t *testing.T) {
	fixture := newFixture(t)
	state := fixture.state
	if state.activeStateTone() != state.palette.Muted {
		t.Fatal("an idle timer should be quiet")
	}
	if _, err := fixture.deps.Focus.Start(focusapp.StartInput{
		Kind:    focus.KindFocus,
		Planned: 25 * time.Minute,
	}); err != nil {
		t.Fatalf("start: %v", err)
	}
	active, err := fixture.deps.Focus.Active()
	if err != nil {
		t.Fatalf("active: %v", err)
	}
	state.active = active
	if state.activeStateTone() != state.palette.Accent {
		t.Error("a running timer should take the accent")
	}
	if _, err := fixture.deps.Focus.Pause(); err != nil {
		t.Fatalf("pause: %v", err)
	}
	active, err = fixture.deps.Focus.Active()
	if err != nil {
		t.Fatalf("active after pause: %v", err)
	}
	state.active = active
	if state.activeStateTone() != state.palette.Warning {
		t.Error("a paused timer should warn that it is not counting")
	}
}
