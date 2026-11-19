package update_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	domain "github.com/iotus3/simplelife/app/internal/domain/shared"
	infra "github.com/iotus3/simplelife/app/internal/infrastructure/update"
)

// hashABC is the digest the test feed reports for its packages.
const hashABC = "abababababababababababababababababababababababababababababababab"

// releaseDocument is a GitHub release for the tests: the project's package for Windows,
// one for another platform, and the checksum file.
func releaseDocument(tag, body string, withDigest bool) string {
	version := strings.TrimPrefix(tag, "v")
	digest := ""
	if withDigest {
		digest = `,"digest":"sha256:` + hashABC + `"`
	}
	return fmt.Sprintf(`{
		"tag_name": %q,
		"body": "  %s  ",
		"html_url": "https://github.com/iotus3/SimpleLife/releases/tag/%s",
		"assets": [
			{"name":"checksums.txt","browser_download_url":"https://example.com/checksums.txt","size":120},
			{"name":"simplelife-%s-linux-amd64.zip","browser_download_url":"https://example.com/linux.zip","size":10,"digest":"sha256:%s"},
			{"name":"simplelife-%s-windows-amd64.zip","browser_download_url":"https://example.com/windows.zip","size":2048%s}
		]
	}`, tag, body, tag, version, hashABC, version, digest)
}

// feedServer serves one release document and records the headers it was asked with.
func feedServer(t *testing.T, document string) (*httptest.Server, *http.Header) {
	t.Helper()
	seen := &http.Header{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if seen.Get("Accept") == "" {
			*seen = r.Header.Clone()
		}
		_, _ = w.Write([]byte(document))
	}))
	t.Cleanup(server.Close)
	return server, seen
}

func TestLatestReadsTheReleaseForThisPlatform(t *testing.T) {
	server, headers := feedServer(t, releaseDocument("v0.2.0", "templates and import", true))
	release, err := infra.GitHub{
		URL: server.URL, Client: server.Client(), OS: "windows", Arch: "amd64",
	}.Latest(context.Background())
	if err != nil {
		t.Fatalf("Latest: %v", err)
	}
	if headers.Get("Accept") != "application/vnd.github+json" {
		t.Fatalf("the feed was asked for %q", headers.Get("Accept"))
	}
	if release.Version != "0.2.0" || release.Notes != "templates and import" {
		t.Fatalf("release = %+v", release)
	}
	if !strings.HasSuffix(release.URL, "/v0.2.0") {
		t.Fatalf("the release page is %q", release.URL)
	}
	if release.Asset.Name != "simplelife-0.2.0-windows-amd64.zip" || release.Asset.Size != 2048 {
		t.Fatalf("asset = %+v", release.Asset)
	}
	if !release.Asset.Usable() {
		t.Fatalf("the asset must be usable: %+v", release.Asset)
	}
	if release.Asset.SHA256 != hashABC {
		t.Fatalf("the digest was not read: %q", release.Asset.SHA256)
	}
}

func TestLatestFallsBackToTheChecksumFile(t *testing.T) {
	sum := sha256.Sum256([]byte("package"))
	hash := hex.EncodeToString(sum[:])
	document := releaseDocument("v0.2.0", "notes", false)
	server := httptest.NewUnstartedServer(nil)
	server.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/checksums.txt") {
			fmt.Fprintf(w, "%s  simplelife-0.2.0-windows-amd64.zip\n%s  other.zip\n", hash, strings.Repeat("cd", 32))
			return
		}
		// The document names example.com; point it at this server instead.
		_, _ = w.Write([]byte(strings.ReplaceAll(document, "https://example.com/checksums.txt", server.URL+"/checksums.txt")))
	})
	server.Start()
	defer server.Close()

	release, err := infra.GitHub{URL: server.URL, Client: server.Client(), OS: "windows", Arch: "amd64"}.
		Latest(context.Background())
	if err != nil {
		t.Fatalf("Latest: %v", err)
	}
	if release.Asset.SHA256 != hash {
		t.Fatalf("checksum = %q, want %q", release.Asset.SHA256, hash)
	}
	if !release.Asset.Usable() {
		t.Fatalf("a package with a published checksum is installable: %+v", release.Asset)
	}
}

func TestLatestWithoutAnyChecksumIsNotInstallable(t *testing.T) {
	// Neither the feed nor the release names a digest: the package stays a download for
	// the user to install by hand, because nothing could check it.
	server, _ := feedServer(t, releaseDocument("v0.2.0", "notes", false))
	release, err := infra.GitHub{URL: server.URL, Client: server.Client(), OS: "windows", Arch: "amd64"}.
		Latest(context.Background())
	if err != nil {
		t.Fatalf("Latest: %v", err)
	}
	if release.Asset.Usable() {
		t.Fatalf("a package without a checksum is not installable: %+v", release.Asset)
	}
}

func TestLatestReadsAChecksumFileWrittenBySha256sum(t *testing.T) {
	// sha256sum writes the listed name three ways: plain, with the "*" binary marker it
	// uses on Windows, and with the "./" a glob leaves in front. All three name the same
	// file, and the release is only installable when the hash is found.
	sum := sha256.Sum256([]byte("package"))
	hash := hex.EncodeToString(sum[:])
	for _, listed := range []string{
		"simplelife-0.2.0-windows-amd64.zip",
		"*simplelife-0.2.0-windows-amd64.zip",
		"./simplelife-0.2.0-windows-amd64.zip",
		"*./simplelife-0.2.0-windows-amd64.zip",
	} {
		t.Run(listed, func(t *testing.T) {
			document := releaseDocument("v0.2.0", "notes", false)
			server := httptest.NewUnstartedServer(nil)
			server.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if strings.HasSuffix(r.URL.Path, "/checksums.txt") {
					fmt.Fprintf(w, "%s  %s\n", hash, listed)
					return
				}
				_, _ = w.Write([]byte(strings.ReplaceAll(document, "https://example.com/checksums.txt", server.URL+"/checksums.txt")))
			})
			server.Start()
			defer server.Close()

			release, err := infra.GitHub{URL: server.URL, Client: server.Client(), OS: "windows", Arch: "amd64"}.
				Latest(context.Background())
			if err != nil {
				t.Fatalf("Latest: %v", err)
			}
			if release.Asset.SHA256 != hash || !release.Asset.Usable() {
				t.Fatalf("asset = %+v, want the hash from the checksum file", release.Asset)
			}
		})
	}
}

func TestLatestOffersAReleaseWithoutAPackageForThisPlatform(t *testing.T) {
	document := strings.ReplaceAll(releaseDocument("v0.2.0", "notes", true), "-windows-amd64.zip", "-plan9-amd64.zip")
	server, _ := feedServer(t, document)
	release, err := infra.GitHub{URL: server.URL, Client: server.Client(), OS: "windows", Arch: "amd64"}.
		Latest(context.Background())
	if err != nil {
		t.Fatalf("Latest: %v", err)
	}
	if release.Version != "0.2.0" {
		t.Fatalf("the release itself must still be reported: %+v", release)
	}
	if release.Asset.Usable() {
		t.Fatalf("a release without this platform's package is not installable: %+v", release.Asset)
	}
}

func TestLatestReportsAFeedWithoutARelease(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()
	_, err := infra.GitHub{URL: server.URL, Client: server.Client()}.Latest(context.Background())
	if !domain.IsKind(err, domain.KindInvalid) {
		t.Fatalf("an empty feed is invalid, got %v", err)
	}
}

func TestLatestReportsABrokenFeed(t *testing.T) {
	cases := []struct {
		name   string
		status int
		body   string
	}{
		{"html", http.StatusOK, "<!doctype html>"},
		{"no-tag", http.StatusOK, `{"body":"notes"}`},
		{"server-error", http.StatusInternalServerError, "boom"},
	}
	for _, c := range cases {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(c.status)
			_, _ = w.Write([]byte(c.body))
		}))
		_, err := infra.GitHub{URL: server.URL, Client: server.Client()}.Latest(context.Background())
		server.Close()
		if !domain.IsKind(err, domain.KindInvalid) {
			t.Fatalf("%s: want invalid, got %v", c.name, err)
		}
	}
}

func TestLatestReportsAnUnreachableFeed(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	address := server.URL
	server.Close()
	if _, err := (infra.GitHub{URL: address}).Latest(context.Background()); err == nil {
		t.Fatal("a closed server must be an error")
	}
}
