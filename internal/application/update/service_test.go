package update_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"testing"

	app "github.com/iotus3/simplelife/app/internal/application/update"
	"github.com/iotus3/simplelife/app/internal/domain/shared"
	"github.com/iotus3/simplelife/app/internal/domain/update"
)

// stubChannel is a release feed that answers with a fixed document.
type stubChannel struct {
	release update.Release
	err     error
	calls   int
}

func (c *stubChannel) Latest(context.Context) (update.Release, error) {
	c.calls++
	return c.release, c.err
}

// stubInstaller records what it was asked to install.
type stubInstaller struct {
	calls    int
	installs []update.Release
	err      error
	progress int
}

func (i *stubInstaller) Install(_ context.Context, release update.Release, progress func(update.Progress)) error {
	i.calls++
	i.installs = append(i.installs, release)
	if progress != nil {
		i.progress++
		progress(update.Progress{Stage: update.StageDownload, Done: 1, Size: 2})
		progress(update.Progress{Stage: update.StageInstall, Done: 1, Size: 1})
	}
	return i.err
}

// packageAsset is an asset whose description passes every check.
func packageAsset() update.Asset {
	sum := sha256.Sum256([]byte("package"))
	return update.Asset{
		Name:   "simplelife-0.2.0-windows-amd64.zip",
		URL:    "https://example.com/simplelife-0.2.0-windows-amd64.zip",
		SHA256: hex.EncodeToString(sum[:]),
		Size:   7,
	}
}

func newerRelease() update.Release {
	return update.Release{
		Version: "0.2.0",
		Notes:   "templates and import",
		URL:     "https://example.com/releases/v0.2.0",
		Asset:   packageAsset(),
	}
}

func TestCheckReportsANewerRelease(t *testing.T) {
	channel := &stubChannel{release: newerRelease()}
	svc := app.New(channel, &stubInstaller{}, "0.1.0")
	if !svc.Available() || !svc.CanInstall() {
		t.Fatal("a wired feed with an installer must be available and installable")
	}
	result, err := svc.Check(context.Background())
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if !result.Newer || result.Latest != "0.2.0" || result.Current != "0.1.0" {
		t.Fatalf("result = %+v", result)
	}
	if !result.Installable {
		t.Fatalf("a release with a verified package must be installable: %+v", result)
	}
	if result.Notes != "templates and import" || result.URL != "https://example.com/releases/v0.2.0" {
		t.Fatalf("the feed's words must reach the user: %+v", result)
	}
}

func TestCheckSaysNothingWhenUpToDate(t *testing.T) {
	channel := &stubChannel{release: update.Release{Version: "0.1.0", URL: "https://example.com/download"}}
	result, err := app.New(channel, &stubInstaller{}, "0.1.0").Check(context.Background())
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if result.Newer || result.Installable {
		t.Fatal("the same version is not an update")

	}
	// An older release on the feed is a rollback, not an update.
	older := &stubChannel{release: update.Release{Version: "0.0.9", URL: "https://example.com/download"}}
	result, err = app.New(older, &stubInstaller{}, "0.1.0").Check(context.Background())
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if result.Newer {
		t.Fatal("an older release must not be offered")
	}
}

func TestCheckWithoutAFeedIsUnsupported(t *testing.T) {
	svc := app.New(nil, &stubInstaller{}, "0.1.0")
	if svc.Available() {
		t.Fatal("a nil feed must not be available")
	}
	if _, err := svc.Check(context.Background()); !shared.IsKind(err, shared.KindUnsupported) {
		t.Fatalf("want unsupported, got %v", err)
	}
	// A nil service is what an unwired build passes around; it must answer, not panic.
	var none *app.Service
	if none.Available() || none.CanInstall() || none.Current() != "" {
		t.Fatal("a nil service reports no feed and no version")
	}
	if _, err := none.Check(context.Background()); !shared.IsKind(err, shared.KindUnsupported) {
		t.Fatalf("want unsupported from a nil service, got %v", err)
	}
	if err := none.Install(context.Background(), nil); !shared.IsKind(err, shared.KindUnsupported) {
		t.Fatalf("a nil service cannot install, got %v", err)
	}
}

func TestCheckSurfacesAFeedFailure(t *testing.T) {
	boom := errors.New("dns: no such host")
	if _, err := app.New(&stubChannel{err: boom}, &stubInstaller{}, "0.1.0").Check(context.Background()); !errors.Is(err, boom) {
		t.Fatalf("the feed's error must be returned as it is, got %v", err)
	}
}

func TestCheckRefusesAReleaseWithNowhereToGo(t *testing.T) {
	channel := &stubChannel{release: update.Release{Version: "9.9.9"}}
	if _, err := app.New(channel, &stubInstaller{}, "0.1.0").Check(context.Background()); !shared.IsKind(err, shared.KindInvalid) {
		t.Fatalf("a newer release without a URL must be invalid, got %v", err)
	}
}

func TestCheckToleratesANilContext(t *testing.T) {
	channel := &stubChannel{release: update.Release{Version: "0.1.0"}}
	if _, err := app.New(channel, nil, "0.1.0").Check(nil); err != nil {
		t.Fatalf("a nil context must not fail the check: %v", err)
	}
}

func TestInstallAppliesTheReleaseTheCheckFound(t *testing.T) {
	channel := &stubChannel{release: newerRelease()}
	installer := &stubInstaller{}
	svc := app.New(channel, installer, "0.1.0")
	if _, err := svc.Check(context.Background()); err != nil {
		t.Fatalf("Check: %v", err)
	}
	if err := svc.Install(context.Background(), func(update.Progress) {}); err != nil {
		t.Fatalf("Install: %v", err)
	}
	if installer.calls != 1 {
		t.Fatalf("installer calls = %d, want 1", installer.calls)
	}
	if got := installer.installs[0]; got.Version != "0.2.0" || got.Asset.Name != packageAsset().Name {
		t.Fatalf("the installer was handed %+v", got)
	}
	// The release is spent: a second click cannot install the same package again.
	if err := svc.Install(context.Background(), nil); !shared.IsKind(err, shared.KindInvalid) {
		t.Fatalf("second install must be refused, got %v", err)
	}
}

func TestInstallRequiresACheckFirst(t *testing.T) {
	svc := app.New(&stubChannel{release: newerRelease()}, &stubInstaller{}, "0.1.0")
	if err := svc.Install(context.Background(), nil); !shared.IsKind(err, shared.KindInvalid) {
		t.Fatalf("want invalid before a check, got %v", err)
	}
}

func TestCheckWithoutAnInstallerOffersThePage(t *testing.T) {
	channel := &stubChannel{release: newerRelease()}
	svc := app.New(channel, nil, "0.1.0")
	if svc.CanInstall() {
		t.Fatal("a build without an installer cannot install")
	}
	result, err := svc.Check(context.Background())
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if !result.Newer || result.Installable {
		t.Fatalf("the release is newer but not installable here: %+v", result)
	}
	if err := svc.Install(context.Background(), nil); !shared.IsKind(err, shared.KindUnsupported) {
		t.Fatalf("want unsupported, got %v", err)
	}
}

func TestInstallReportsTheInstallersFailure(t *testing.T) {
	boom := errors.New("disk full")
	svc := app.New(&stubChannel{release: newerRelease()}, &stubInstaller{err: boom}, "0.1.0")
	if _, err := svc.Check(context.Background()); err != nil {
		t.Fatalf("Check: %v", err)
	}
	if err := svc.Install(context.Background(), nil); !errors.Is(err, boom) {
		t.Fatalf("want the installer's error, got %v", err)
	}
	// A failed installation keeps the release, so the user can try again.
	if _, ok := svc.Pending(); !ok {
		t.Fatal("a failed install must keep the release for another attempt")
	}
}
