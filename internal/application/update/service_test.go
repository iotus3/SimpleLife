package update_test

import (
	"context"
	"errors"
	"testing"

	app "github.com/iotus3/simplelife/app/internal/application/update"
	"github.com/iotus3/simplelife/app/internal/domain/shared"
	"github.com/iotus3/simplelife/app/internal/domain/update"
)

// stubChannel is a release channel that answers with a fixed document.
type stubChannel struct {
	release update.Release
	err     error
	calls   int
}

func (c *stubChannel) Latest(context.Context) (update.Release, error) {
	c.calls++
	return c.release, c.err
}

func TestCheckReportsANewerRelease(t *testing.T) {
	channel := &stubChannel{release: update.Release{
		Version: "0.2.0", Notes: "模板与导入", URL: "https://example.com/download",
	}}
	svc := app.New(channel, "0.1.0")
	if !svc.Available() {
		t.Fatal("a wired channel must be available")
	}
	result, err := svc.Check(context.Background())
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if !result.Newer || result.Latest != "0.2.0" || result.Current != "0.1.0" {
		t.Fatalf("result = %+v", result)
	}
	if result.Notes != "模板与导入" || result.URL != "https://example.com/download" {
		t.Fatalf("the channel's words must reach the user: %+v", result)
	}
}

func TestCheckSaysNothingWhenUpToDate(t *testing.T) {
	channel := &stubChannel{release: update.Release{Version: "0.1.0", URL: "https://example.com/download"}}
	result, err := app.New(channel, "0.1.0").Check(context.Background())
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if result.Newer {
		t.Fatal("the same version is not an update")
	}

	// An older release on the channel is a rollback, not an update.
	older := &stubChannel{release: update.Release{Version: "0.0.9", URL: "https://example.com/download"}}
	result, err = app.New(older, "0.1.0").Check(context.Background())
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if result.Newer {
		t.Fatal("an older release must not be offered")
	}
}

func TestCheckWithoutAChannelIsUnsupported(t *testing.T) {
	svc := app.New(nil, "0.1.0")
	if svc.Available() {
		t.Fatal("a nil channel must not be available")
	}
	if _, err := svc.Check(context.Background()); !shared.IsKind(err, shared.KindUnsupported) {
		t.Fatalf("want unsupported, got %v", err)
	}
	// A nil service is what an unwired build passes around; it must answer, not
	// panic.
	var none *app.Service
	if none.Available() || none.Current() != "" {
		t.Fatal("a nil service reports no channel and no version")
	}
	if _, err := none.Check(context.Background()); !shared.IsKind(err, shared.KindUnsupported) {
		t.Fatalf("want unsupported from a nil service, got %v", err)
	}
}

func TestCheckSurfacesAChannelFailure(t *testing.T) {
	boom := errors.New("dns: no such host")
	if _, err := app.New(&stubChannel{err: boom}, "0.1.0").Check(context.Background()); !errors.Is(err, boom) {
		t.Fatalf("the channel's error must be returned as it is, got %v", err)
	}
}

func TestCheckRefusesAReleaseWithNowhereToGo(t *testing.T) {
	channel := &stubChannel{release: update.Release{Version: "9.9.9"}}
	if _, err := app.New(channel, "0.1.0").Check(context.Background()); !shared.IsKind(err, shared.KindInvalid) {
		t.Fatalf("a newer release without a URL must be invalid, got %v", err)
	}
}

func TestCheckToleratesANilContext(t *testing.T) {
	channel := &stubChannel{release: update.Release{Version: "0.1.0"}}
	if _, err := app.New(channel, "0.1.0").Check(nil); err != nil {
		t.Fatalf("a nil context must not fail the check: %v", err)
	}
}
