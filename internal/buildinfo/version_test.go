package buildinfo

import (
	"os"
	"regexp"
	"runtime/debug"
	"testing"
)

// viva.toml decides the version and version.go is generated from it, so this test is what
// notices a generated file left behind.
func TestVersionMatchesTheManifest(t *testing.T) {
	data, err := os.ReadFile("../../viva.toml")
	if os.IsNotExist(err) {
		t.Skip("viva.toml is not next to this test (built elsewhere)")
	}
	if err != nil {
		t.Fatal(err)
	}
	match := regexp.MustCompile(`(?m)^version\s*=\s*"([^"]+)"`).FindSubmatch(data)
	if match == nil {
		t.Fatalf("viva.toml has no [app] version:\n%s", data)
	}
	if got := string(match[1]); got != Version {
		t.Fatalf("viva.toml version %q, buildinfo.Version %q: run scripts/version.sh (or just update-version %s)", got, Version, got)
	}
}

func TestVersionIsADottedNumber(t *testing.T) {
	if !regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+$`).MatchString(Version) {
		t.Fatalf("Version = %q, want a dotted number such as 0.1.3", Version)
	}
}

// The commit and the date are read from the build's own VCS stamp, which is what a
// binary built by the framework CLI carries; nothing is passed at link time.
func TestIdentityReadsTheBuildStamp(t *testing.T) {
	info := &debug.BuildInfo{Settings: []debug.BuildSetting{
		{Key: "GOARCH", Value: "amd64"},
		{Key: "vcs.revision", Value: "0123456789abcdef"},
		{Key: "vcs.time", Value: "2026-11-18T08:30:00Z"},
	}}
	commit, date := IdentityFrom(info)
	if commit != "0123456789abcdef" {
		t.Fatalf("commit = %q", commit)
	}
	if date != "2026-11-18T08:30:00Z" {
		t.Fatalf("date = %q", date)
	}
}

// A build from a source archive has no stamp, and the about page is told so rather
// than shown an invented commit.
func TestIdentityIsEmptyWithoutAStamp(t *testing.T) {
	if commit, date := IdentityFrom(nil); commit != "" || date != "" {
		t.Fatalf("nil build info gave %q, %q", commit, date)
	}
	commit, date := IdentityFrom(&debug.BuildInfo{Settings: []debug.BuildSetting{{Key: "GOARCH", Value: "arm64"}}})
	if commit != "" || date != "" {
		t.Fatalf("an unstamped build gave %q, %q", commit, date)
	}
}
