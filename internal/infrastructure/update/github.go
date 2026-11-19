// Package update reads the project's GitHub releases and installs one of them.
//
// A release is the only thing the feed has to describe: its tag names the version, its
// body is the notes, and its assets name a package for this platform. Every download is
// held to the SHA-256 the release published, so a truncated or altered file never
// replaces a working program.
package update

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/iotus3/simplelife/app/internal/domain/shared"
	"github.com/iotus3/simplelife/app/internal/domain/update"
)

// Feed is the release the project publishes its builds to: GitHub's "latest release",
// which already excludes drafts and pre-releases.
const Feed = "https://api.github.com/repos/iotus3/SimpleLife/releases/latest"

// FeedEnv overrides Feed. It exists for development and for the tests, which point it at
// a local server; a shipped build never sets it.
const FeedEnv = "SIMPLELIFE_UPDATE_FEED"

// checksumAsset is the file the release publishes beside the packages: one
// "<sha256>  <name>" line each, the format sha256sum writes and reads.
const checksumAsset = "checksums.txt"

// GitHub reads the feed. The zero value is the feed the project publishes.
type GitHub struct {
	// URL overrides Feed, which is what the tests use.
	URL string
	// Client is the HTTP client; nil means a client with a short timeout.
	Client *http.Client
	// OS and Arch select the package; empty means this build's own platform, which is
	// the only thing a real run ever wants.
	OS, Arch string
}

// Latest implements the application's feed port.
func (g GitHub) Latest(ctx context.Context) (update.Release, error) {
	address := strings.TrimSpace(g.URL)
	if address == "" {
		address = strings.TrimSpace(os.Getenv(FeedEnv))
	}
	if address == "" {
		address = Feed
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, address, nil)
	if err != nil {
		return update.Release{}, shared.Invalid("update.feed", "url", "unusable release feed address").Wrap(err)
	}
	request.Header.Set("Accept", "application/vnd.github+json")
	body, err := g.read(request, 256<<10)
	if err != nil {
		return update.Release{}, err
	}
	document, err := decodeFeed(body)
	if err != nil {
		return update.Release{}, err
	}
	release := document.release(g.platform())
	if release.Asset.Name != "" && release.Asset.SHA256 == "" && document.checksumsURL != "" {
		// The feed reported no digest for the package. The release's own checksum file is
		// the second source; when neither says, the package is offered as a download for
		// the user to install by hand instead of being installed unverified.
		release.Asset.SHA256, _ = g.checksum(ctx, document.checksumsURL, release.Asset.Name)
	}
	return release, nil
}

// read fetches one feed document and holds the answer to the statuses a feed can give.
func (g GitHub) read(request *http.Request, limit int64) ([]byte, error) {
	response, err := g.client().Do(request)
	if err != nil {
		return nil, shared.Invalid("update.unreachable", "", "could not reach the release feed").Wrap(err)
	}
	defer func() { _ = response.Body.Close() }()
	switch response.StatusCode {
	case http.StatusOK:
	case http.StatusNotFound:
		return nil, shared.Invalid("update.none", "", "the release feed has no published release")
	default:
		return nil, shared.Invalid("update.status", "", "the release feed answered "+response.Status)
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, limit))
	if err != nil {
		return nil, shared.Invalid("update.read", "", "could not read the release feed").Wrap(err)
	}
	return body, nil
}

// checksum reads the release's checksum file and returns the hash for one package.
func (g GitHub) checksum(ctx context.Context, address, name string) (string, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, address, nil)
	if err != nil {
		return "", shared.Invalid("update.feed", "url", "unusable checksum address").Wrap(err)
	}
	body, err := g.read(request, 64<<10)
	if err != nil {
		return "", err
	}
	return parseChecksums(string(body), name)
}

func (g GitHub) client() *http.Client {
	if g.Client != nil {
		return g.Client
	}
	return &http.Client{Timeout: 15 * time.Second}
}

// platform is the pair of names the package file carries.
func (g GitHub) platform() string {
	osName, arch := g.OS, g.Arch
	if osName == "" {
		osName = runtime.GOOS
	}
	if arch == "" {
		arch = runtime.GOARCH
	}
	return osName + "-" + arch
}

// feed is the part of GitHub's release document this application reads. The field names
// are the feed's contract; they are not translated.
type feed struct {
	TagName string      `json:"tag_name"`
	Body    string      `json:"body"`
	HTMLURL string      `json:"html_url"`
	Assets  []feedAsset `json:"assets"`
}

type feedAsset struct {
	Name   string `json:"name"`
	URL    string `json:"browser_download_url"`
	Size   int64  `json:"size"`
	Digest string `json:"digest"`
}

// releaseDocument is a parsed feed with the address of its checksum file, which the
// caller may still need to ask for.
type releaseDocument struct {
	feed
	checksumsURL string
}

func decodeFeed(body []byte) (releaseDocument, error) {
	var document feed
	if err := json.Unmarshal(body, &document); err != nil {
		return releaseDocument{}, shared.Invalid("update.feed", "", "the release feed is not JSON").Wrap(err)
	}
	if strings.TrimSpace(document.TagName) == "" {
		return releaseDocument{}, shared.Invalid("update.feed.version", "version", "the release names no version")
	}
	parsed := releaseDocument{feed: document}
	for _, asset := range document.Assets {
		if asset.Name == checksumAsset {
			parsed.checksumsURL = strings.TrimSpace(asset.URL)
		}
	}
	return parsed, nil
}

// release turns the document into a release for one platform.
func (d releaseDocument) release(platform string) update.Release {
	version := versionOf(d.TagName)
	release := update.Release{
		Version: version,
		Notes:   strings.TrimSpace(d.Body),
		URL:     strings.TrimSpace(d.HTMLURL),
	}
	asset, ok := findAsset(d.Assets, fmt.Sprintf("simplelife-%s-%s.zip", version, platform), platform)
	if !ok {
		// A release without a package for this platform is still a release: the user can
		// read the notes and open the page, they just cannot install in place.
		return release
	}
	release.Asset = update.Asset{
		Name:   asset.Name,
		URL:    strings.TrimSpace(asset.URL),
		Size:   asset.Size,
		SHA256: digestOf(asset),
	}
	return release
}

// versionOf is the version a tag names: "v0.2.0" and "0.2.0" are the same release.
func versionOf(tag string) string {
	return strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(tag), "v"))
}

// findAsset picks the package for one platform: the name the packaging step gives it, or
// any zip for the same platform when the name carries something else.
func findAsset(assets []feedAsset, wanted, platform string) (feedAsset, bool) {
	suffix := "-" + platform + ".zip"
	for _, asset := range assets {
		if asset.Name == wanted {
			return asset, true
		}
	}
	for _, asset := range assets {
		if strings.HasPrefix(asset.Name, "simplelife-") && strings.HasSuffix(asset.Name, suffix) {
			return asset, true
		}
	}
	return feedAsset{}, false
}

// digestOf reads the hash the feed published for an asset. GitHub reports it as
// "sha256:<hex>" once the upload has finished; an older asset has none, and the release's
// checksum file answers instead.
func digestOf(asset feedAsset) string {
	digest := strings.ToLower(strings.TrimSpace(asset.Digest))
	if rest, ok := strings.CutPrefix(digest, "sha256:"); ok {
		return strings.TrimSpace(rest)
	}
	return ""
}

// parseChecksums reads a sha256sum file and returns the hash of one file name.
func parseChecksums(text, name string) (string, error) {
	for _, line := range strings.Split(text, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		// sha256sum writes "hash  name". The name arrives in three shapes: plain, with the
		// "*" binary marker, and with the "./" a glob puts in front of every match.
		listed := strings.TrimPrefix(fields[len(fields)-1], "*")
		if strings.TrimPrefix(listed, "./") != name {
			continue
		}
		hash := strings.ToLower(fields[0])
		if len(hash) != 64 {
			continue
		}
		return hash, nil
	}
	return "", shared.Invalid("update.checksum_missing", "", "the release names no checksum for "+name)
}
