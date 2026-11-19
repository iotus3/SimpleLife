package presentation

import (
	"testing"

	"github.com/iotus3/simplelife/app/internal/application/settings"
	"github.com/iotus3/simplelife/app/internal/presentation/i18n"
)

// TestAppearanceDefaultsToTheSystem checks that a fresh install follows the
// desktop: the first frame must already be dark on a dark desktop, not light with
// a switch after the user finds the setting.
func TestAppearanceDefaultsToTheSystem(t *testing.T) {
	memory := newFixture(t)
	if memory.state.themePref != settings.ThemeAuto || memory.state.dark {
		t.Fatalf("default appearance = %q dark=%v, want auto and light", memory.state.themePref, memory.state.dark)
	}

	memory.system.Dark = true
	dark := NewState(memory.deps)
	if !dark.dark || !dark.palette.Dark {
		t.Fatal("with the desktop dark, an auto preference must paint dark")
	}
	// An unanswered question from the platform reads as light rather than as a
	// guess in either direction.
	memory.system.Unknown = true
	unknown := NewState(memory.deps)
	if unknown.dark {
		t.Fatal("an unknown system appearance must paint light")
	}
}

// TestAppearanceChoiceSurvivesTheWindow checks that picking light or dark wins over
// the desktop and is stored, so the next window opens in it.
func TestAppearanceChoiceSurvivesTheWindow(t *testing.T) {
	memory := newFixture(t)
	memory.system.Dark = true
	session := memory.openWindow(t)
	memory.state.route = RouteSettings
	memory.state.markAll()
	settle(t, session)

	zh := func(id i18n.MessageID) string { return i18n.T(i18n.ZhCN, id) }
	if !session.HasText(zh(i18n.MsgSettingsAppearance)) {
		t.Fatalf("the settings page does not offer an appearance; texts: %v", session.Texts())
	}
	if err := session.ClickLabel(zh(i18n.MsgSettingsThemeLight)); err != nil {
		t.Fatalf("choose light: %v", err)
	}
	settle(t, session)
	if memory.state.dark || memory.state.palette.Dark {
		t.Fatal("choosing light on a dark desktop must paint light")
	}
	if got := memory.deps.Settings.Theme(); got != settings.ThemeLight {
		t.Fatalf("stored theme = %q, want light", got)
	}

	if err := session.ClickLabel(zh(i18n.MsgSettingsThemeDark)); err != nil {
		t.Fatalf("choose dark: %v", err)
	}
	settle(t, session)
	if !memory.state.dark {
		t.Fatal("choosing dark must paint dark")
	}
	// The page holds themeSlot for the life of the window, so the palette must be
	// written through it; a replaced pointer would leave the window light forever.
	if *memory.state.themeSlot != *memory.state.palette.Theme {
		t.Fatal("the palette the page paints with must be the one the state resolved")
	}

	// A new window over the same store opens in the stored appearance.
	reopened := NewState(memory.deps)
	if reopened.themePref != settings.ThemeDark || !reopened.dark {
		t.Fatalf("reopened appearance = %q dark=%v", reopened.themePref, reopened.dark)
	}

	// Back to the system: the desktop is dark, so the window stays dark but the
	// stored preference follows the desktop again.
	if err := session.ClickLabel(zh(i18n.MsgSettingsThemeSystem)); err != nil {
		t.Fatalf("choose follow-system: %v", err)
	}
	settle(t, session)
	if got := memory.deps.Settings.Theme(); got != settings.ThemeAuto {
		t.Fatalf("stored theme = %q, want auto", got)
	}
	if !memory.state.dark {
		t.Fatal("auto on a dark desktop must paint dark")
	}
}

// TestSystemSwitchRepaintsAnAutoWindow checks the path the framework's appearance
// watcher takes: it notes the change and repaints, and the paint resolves it.
func TestSystemSwitchRepaintsAnAutoWindow(t *testing.T) {
	memory := newFixture(t)
	session := memory.openWindow(t)
	settle(t, session)
	if memory.state.dark {
		t.Fatal("the window started dark")
	}

	memory.system.Dark = true
	memory.state.appearanceDirty.Store(true)
	settle(t, session)
	if !memory.state.dark || !memory.state.palette.Dark {
		t.Fatal("a system switch must repaint an auto window in the new look")
	}
	if *memory.state.themeSlot != *memory.state.palette.Theme {
		t.Fatal("the page keeps the theme slot, so it must hold the new palette")
	}

	// A pinned appearance ignores the desktop.
	if err := memory.deps.Settings.SetTheme(settings.ThemeLight); err != nil {
		t.Fatal(err)
	}
	memory.state.themePref = settings.ThemeLight
	memory.state.appearanceDirty.Store(true)
	settle(t, session)
	if memory.state.dark {
		t.Fatal("a pinned light window must ignore the desktop")
	}
}
