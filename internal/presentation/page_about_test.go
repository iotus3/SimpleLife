package presentation

import (
	"context"
	"strings"
	"testing"
	"time"

	updateapp "github.com/iotus3/simplelife/app/internal/application/update"
	"github.com/iotus3/simplelife/app/internal/domain/shared"
	"github.com/iotus3/simplelife/app/internal/domain/update"
	"github.com/iotus3/simplelife/app/internal/presentation/i18n"
)

// stubChannel is a release feed for the interface tests: it answers without a
// server, so a page can be painted with a result already in hand.
type stubChannel struct{ release update.Release }

func (c stubChannel) Latest(context.Context) (update.Release, error) { return c.release, nil }

// stubInstaller stands in for the download and the file swap, which need a release and a
// writable program folder. It reports once it has been asked.
type stubInstaller struct {
	calls int
	done  chan struct{}
}

func (i *stubInstaller) Install(context.Context, update.Release, func(update.Progress)) error {
	i.calls++
	if i.done != nil {
		close(i.done)
	}
	return nil
}

// installableRelease is a release the page can offer to install: newer, with a package
// this platform can verify.
func installableRelease() update.Release {
	return update.Release{
		Version: "0.2.0",
		Notes:   "templates and import",
		URL:     "https://example.com/releases/v0.2.0",
		Asset: update.Asset{
			Name:   "simplelife-0.2.0-windows-amd64.zip",
			URL:    "https://example.com/simplelife-0.2.0-windows-amd64.zip",
			SHA256: strings.Repeat("ab", 32),
			Size:   2048,
		},
	}
}

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
	fixture.deps.Update = updateapp.New(stubChannel{}, nil, fixture.deps.Version)
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

// TestAboutPageInstallsANewerRelease is the auto-update path: a release with a verified
// package is offered as an installation, and taking it hands the work to the installer,
// starts the new build and ends this one.
func TestAboutPageInstallsANewerRelease(t *testing.T) {
	fixture := newFixture(t)
	installer := &stubInstaller{done: make(chan struct{})}
	restarted := make(chan struct{}, 1)
	// The feed is read for real, so the service holds the release it is asked to install.
	// The running version is numeric: "test" is not a version and nothing is newer than it.
	service := updateapp.New(stubChannel{release: installableRelease()}, installer, "0.1.0")
	fixture.deps.Update = service
	fixture.deps.Restart = func() error {
		restarted <- struct{}{}
		return nil
	}
	quit := make(chan struct{}, 1)
	fixture.state = NewState(fixture.deps)
	fixture.state.quit = func() { quit <- struct{}{} }
	fixture.state.route = RouteAbout
	// The about page stacks the hero, the facts, the update card and the links; the
	// install button lives below the default window, so the test asks for a taller one.
	session := fixture.openWindowSize(t, appConfigSize(windowWidth, 2000))
	zh := func(id i18n.MessageID, args ...any) string { return i18n.T(i18n.ZhCN, id, args...) }

	result, err := service.Check(context.Background())
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	fixture.state.setUpdateState(false, &result, nil)
	fixture.state.markAll()
	settle(t, session)
	if !session.HasText(zh(i18n.MsgAboutInstall)) {
		t.Fatalf("an installable release must offer the install; texts: %v", session.Texts())
	}

	if err := session.ClickLabel(zh(i18n.MsgAboutInstall)); err != nil {
		t.Fatalf("start the installation: %v", err)
	}
	settle(t, session)
	waitFor(t, installer.done, "the installer was never asked")
	waitFor(t, restarted, "the new build was never started")
	waitFor(t, quit, "the old build did not end")
	if installer.calls != 1 {
		t.Fatalf("installer calls = %d, want 1", installer.calls)
	}
}

// TestAboutPageWithoutAPackageOffersThePage keeps the honest case: a release this build
// cannot verify is a download to open, not an installation to run.
func TestAboutPageWithoutAPackageOffersThePage(t *testing.T) {
	fixture := newFixture(t)
	installer := &stubInstaller{}
	release := installableRelease()
	release.Asset = update.Asset{}
	service := updateapp.New(stubChannel{release: release}, installer, "0.1.0")
	fixture.deps.Update = service
	fixture.state = NewState(fixture.deps)
	fixture.state.route = RouteAbout
	session := fixture.openWindowSize(t, appConfigSize(windowWidth, 2000))
	zh := func(id i18n.MessageID, args ...any) string { return i18n.T(i18n.ZhCN, id, args...) }

	result, err := service.Check(context.Background())
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	fixture.state.setUpdateState(false, &result, nil)
	fixture.state.markAll()
	settle(t, session)
	if session.HasText(zh(i18n.MsgAboutInstall)) {
		t.Fatal("a release without a package must not offer to install")
	}
	if !session.HasText(zh(i18n.MsgAboutNoPackage)) || !session.HasText(zh(i18n.MsgAboutOpenDownload)) {
		t.Fatalf("the page must say why and offer the download; texts: %v", session.Texts())
	}
}

// TestAboutPageReportsAnInstallFailure keeps a failed update visible: the user has to know
// that nothing was replaced.
func TestAboutPageReportsAnInstallFailure(t *testing.T) {
	fixture := newFixture(t)
	fixture.deps.Update = updateapp.New(stubChannel{}, &stubInstaller{}, fixture.deps.Version)
	fixture.state = NewState(fixture.deps)
	fixture.state.route = RouteAbout
	session := fixture.openWindow(t)
	zh := func(id i18n.MessageID, args ...any) string { return i18n.T(i18n.ZhCN, id, args...) }

	fixture.state.setInstalling(false, update.Progress{}, shared.Invalid("update.checksum", "", "mismatch"))
	fixture.state.markAll()
	settle(t, session)
	if !session.HasText(zh(i18n.MsgErrUpdateChecksum)) {
		t.Fatalf("a checksum mismatch must be reported; texts: %v", session.Texts())
	}
}

// waitFor blocks until the signal arrives or the test gives up.
func waitFor(t *testing.T, signal <-chan struct{}, message string) {
	t.Helper()
	select {
	case <-signal:
	case <-time.After(5 * time.Second):
		t.Fatal(message)
	}
}
