package presentation

import (
	"testing"

	"github.com/vivaui/viva/app"

	"github.com/iotus3/simplelife/app/internal/application/settings"
	"github.com/iotus3/simplelife/app/internal/presentation/i18n"
)

// The settings page decides what the window's close button does, and the choice
// survives a restart because it is stored with the other preferences.
func TestSettingsChooseWhatClosingDoes(t *testing.T) {
	fixture := newFixture(t)
	// The fixture has no desktop, so it has no status area either; a status item is
	// what makes the choice of what closing does worth offering.
	fixture.state.statusItem = &app.StatusItem{}
	session := fixture.openWindow(t)
	fixture.state.route = RouteSettings
	fixture.state.markAll()
	settle(t, session)

	zh := func(id i18n.MessageID, args ...any) string { return i18n.T(i18n.ZhCN, id, args...) }
	if got := fixture.deps.Settings.CloseAction(); got != settings.CloseExit {
		t.Fatalf("a fresh installation closes to %q, want %q", got, settings.CloseExit)
	}
	if !session.HasText(zh(i18n.MsgSettingsClose)) {
		t.Fatalf("the close behavior is not offered: %v", session.Texts())
	}
	if !session.HasText(zh(i18n.MsgSettingsCloseExit)) {
		t.Fatalf("the close behavior is missing its entries: %v", session.Texts())
	}
	// The hint wraps, so only a part of it is one painted line.
	if !session.HasText("系统托盘") {
		t.Fatalf("the close behavior explains nothing: %v", session.Texts())
	}

	if err := session.ClickLabel(zh(i18n.MsgSettingsCloseTray)); err != nil {
		t.Fatalf("choose the status area: %v", err)
	}
	settle(t, session)
	if got := fixture.deps.Settings.CloseAction(); got != settings.CloseTray {
		t.Fatalf("the choice stored %q, want %q", got, settings.CloseTray)
	}

	if err := session.ClickLabel(zh(i18n.MsgSettingsCloseExit)); err != nil {
		t.Fatalf("choose quitting: %v", err)
	}
	settle(t, session)
	if got := fixture.deps.Settings.CloseAction(); got != settings.CloseExit {
		t.Fatalf("the choice stored %q, want %q", got, settings.CloseExit)
	}
}

// A desktop without a status area cannot keep the program running in one, so the
// choice is not offered at all: a row that promises "keep it in the status area"
// where there is none would be a setting that cannot be honoured.
func TestSettingsHideTheCloseChoiceWithoutAStatusArea(t *testing.T) {
	fixture := newFixture(t)
	session := fixture.openWindow(t)
	fixture.state.route = RouteSettings
	fixture.state.markAll()
	settle(t, session)

	zh := func(id i18n.MessageID, args ...any) string { return i18n.T(i18n.ZhCN, id, args...) }
	if session.HasText(zh(i18n.MsgSettingsClose)) {
		t.Fatalf("the close behavior is offered without a status area: %v", session.Texts())
	}
	// The rows that do not depend on the desktop are still there.
	if !session.HasText(zh(i18n.MsgSettingsAppearance)) {
		t.Fatalf("the appearance row vanished with it: %v", session.Texts())
	}

	// A status item arriving later (the real order: the window opens, then the icon
	// is created) shows the row again without a restart.
	fixture.state.statusItem = &app.StatusItem{}
	fixture.state.markAll()
	settle(t, session)
	if !session.HasText(zh(i18n.MsgSettingsClose)) {
		t.Fatalf("the close behavior never came back: %v", session.Texts())
	}
}
