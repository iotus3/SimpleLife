// Package update reads the release manifest this build was pointed at:
//
//	{"version": "0.2.0", "notes": "…", "url": "https://…/simplelife.exe"}
//
// A source build has no address and the about page says so. Nothing is downloaded or installed:
// the application only tells the user a newer build exists.
package update

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/iotus3/simplelife/app/internal/domain/shared"
	"github.com/iotus3/simplelife/app/internal/domain/update"
)

// ManifestURL is the release channel this build checks. The build script sets it
// with -ldflags "-X …/internal/infrastructure/update.ManifestURL=…"; an empty
// value means "this build cannot check".
var ManifestURL = ""

// Fetcher reads the manifest over HTTP.
type Fetcher struct {
	// URL overrides ManifestURL, which is what the tests use.
	URL string
	// Client is the HTTP client; nil means a client with a short timeout.
	Client *http.Client
}

// Latest implements the application's channel port.
func (f Fetcher) Latest(ctx context.Context) (update.Release, error) {
	address := strings.TrimSpace(f.URL)
	if address == "" {
		address = strings.TrimSpace(ManifestURL)
	}
	if address == "" {
		return update.Release{}, shared.Unsupported("update.not_configured",
			"this build has no release channel address")
	}
	if !strings.HasPrefix(address, "https://") && !strings.HasPrefix(address, "http://") {
		return update.Release{}, shared.Invalid("update.url", "url", "the release channel must be an http URL")
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, address, nil)
	if err != nil {
		return update.Release{}, shared.Invalid("update.url", "url", "unusable release channel address").Wrap(err)
	}
	request.Header.Set("Accept", "application/json")
	response, err := f.client().Do(request)
	if err != nil {
		return update.Release{}, shared.Invalid("update.unreachable", "", "could not reach the release channel").Wrap(err)
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK {
		return update.Release{}, shared.Invalid("update.status", "", "the release channel answered "+response.Status)
	}
	// One manifest is a few hundred bytes; refuse anything that is clearly not one
	// rather than reading a misconfigured address into memory.
	body, err := io.ReadAll(io.LimitReader(response.Body, 64<<10))
	if err != nil {
		return update.Release{}, shared.Invalid("update.read", "", "could not read the release channel").Wrap(err)
	}
	release, err := Decode(body)
	if err != nil {
		return update.Release{}, err
	}
	return release, nil
}

func (f Fetcher) client() *http.Client {
	if f.Client != nil {
		return f.Client
	}
	return &http.Client{Timeout: 8 * time.Second}
}

// manifest is the document on the channel. The field names are the contract with
// whatever publishes a release; they are not translated.
type manifest struct {
	Version string `json:"version"`
	Notes   string `json:"notes"`
	URL     string `json:"url"`
}

// Decode reads a manifest. It is exported so a test can pin the contract without a
// server, and so a future local-file channel reuses the same parsing.
func Decode(body []byte) (update.Release, error) {
	var doc manifest
	if err := json.Unmarshal(body, &doc); err != nil {
		return update.Release{}, shared.Invalid("update.manifest", "", "the release manifest is not JSON").Wrap(err)
	}
	version := strings.TrimSpace(doc.Version)
	if version == "" {
		return update.Release{}, shared.Invalid("update.manifest.version", "version", "the release manifest names no version")
	}
	return update.Release{
		Version: version,
		Notes:   strings.TrimSpace(doc.Notes),
		URL:     strings.TrimSpace(doc.URL),
	}, nil
}
