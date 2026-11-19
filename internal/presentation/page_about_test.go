package presentation

import (
	"context"
	"testing"

	updateapp "github.com/iotus3/simplelife/app/internal/application/update"
	"github.com/iotus3/simplelife/app/internal/domain/update"
	"github.com/iotus3/simplelife/app/internal/presentation/i18n"
)

// stubChannel is a release channel for the interface tests: it answers without a
// server, so a page can be painted with a result already in hand.
type stubChannel struct{ release update.Release }

func (c stubChannel) Latest(context.Context) (update.Release, error) { return c.release, nil }

func TestAboutPageReportsTheBuild(t *testing.T) {
	fixture := newFixture(t)
	fixture.state.route = RouteAbout
	session := fixture.openWindow(t)
	fixture.state.markAll()
	settle(t, session)

	zh := func(id i18n.MessageID, args ...any) string { return i18n.T(i18n.ZhCN, id, args...) }
	for _, want := range []string{
		zh(i18n.MsgNavAbout),
		zh(i18n.MsgAboutTagline),
		zh(i18n.MsgAboutVersion),
		zh(i18n.MsgAboutPlatform),
		zh(i18n.MsgAboutBuild),
		zh(i18n.MsgAboutRuntime),
		"vtest",
		fixture.deps.DataDir,
		zh(i18n.MsgAboutColophon),
	} {
		if !session.HasText(want) {
			t.Errorf("the about page does not show %q; texts: %v", want, session.Texts())
		}
	}
	// The build date and the commit are what a bug report quotes; this build has
	// neither, and the page says so instead of showing an empty line.
	if !session.HasText(zh(i18n.MsgAboutNoDate)) {
		t.Errorf("a source build must say its date is unknown; texts: %v", session.Texts())
	}
}

func TestAboutPageShowsTheUpdateState(t *testing.T) {
	fixture := newFixture(t)
	fixture.deps.Update = updateapp.New(stubChannel{}, fixture.deps.Version)
	fixture.state = NewState(fixture.deps)
	fixture.state.route = RouteAbout
	session := fixture.openWindow(t)
	fixture.state.markAll()
	settle(t, session)

	zh := func(id i18n.MessageID, args ...any) string { return i18n.T(i18n.ZhCN, id, args...) }
	if !session.HasText(zh(i18n.MsgAboutCheckUpdate)) {
		t.Fatalf("a wired build must offer the check; texts: %v", session.Texts())
	}

	// A newer release: the page names it, quotes its notes and offers the download.
	fixture.state.setUpdateState(false, &update.Result{
		Current: "test", Latest: "0.2.0", Notes: "模板与导入", URL: "https://example.com/dl", Newer: true,
	}, nil)
	settle(t, session)
	if !session.HasText(zh(i18n.MsgAboutUpdateFound, "0.2.0")) {
		t.Errorf("the new version is not named; texts: %v", session.Texts())
	}
	if !session.HasText("模板与导入") || !session.HasText(zh(i18n.MsgAboutOpenDownload)) {
		t.Errorf("the release notes or the download button are missing; texts: %v", session.Texts())
	}

	// Up to date: the page says which version is current, and offers no download.
	fixture.state.setUpdateState(false, &update.Result{Current: "test", Latest: "test"}, nil)
	settle(t, session)
	if !session.HasText(zh(i18n.MsgAboutUpToDate, "test")) {
		t.Errorf("an up-to-date build must say so; texts: %v", session.Texts())
	}
	if session.HasText(zh(i18n.MsgAboutOpenDownload)) {
		t.Error("an up-to-date build must not offer a download")
	}

	// Back to no channel: the page explains why the button is unavailable.
	fixture.deps.Update = nil
	fixture.state = NewState(fixture.deps)
	fixture.state.route = RouteAbout
	session = fixture.openWindow(t)
	fixture.state.markAll()
	settle(t, session)
	if !session.HasText(zh(i18n.MsgAboutNoChannel)) {
		t.Errorf("a build without a channel must say so; texts: %v", session.Texts())
	}
}
