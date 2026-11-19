// Package update decides one thing: is the release the server names newer than the running
// build? Nothing here knows about HTTP or about installing anything.
package update

import (
	"strconv"
	"strings"
)

// Release is what a release channel says is the newest build.
type Release struct {
	// Version is the released version, with or without a leading "v".
	Version string
	// Notes is the one-paragraph description shown to the user.
	Notes string
	// URL is the release page. An empty URL means the release exists but there is
	// nowhere to send the user, which the interface reports rather than opening a
	// blank page.
	URL string
	// Asset is the package this platform installs from, when the release names one.
	Asset Asset
}

// Asset is one downloadable package.
type Asset struct {
	// Name is the file name, without a directory.
	Name string
	// URL is the https address the bytes come from.
	URL string
	// SHA256 is the lower-case hex digest the bytes must have.
	SHA256 string
	// Size is the exact length in bytes.
	Size int64
}

// Usable reports whether the asset can be downloaded and checked. An incomplete
// description is refused rather than downloaded and hoped for.
func (a Asset) Usable() bool {
	if a.Name == "" || strings.ContainsAny(a.Name, `/\`) || a.Size <= 0 {
		return false
	}
	if !strings.HasPrefix(a.URL, "https://") && !strings.HasPrefix(a.URL, "http://") {
		return false
	}
	if len(a.SHA256) != 64 {
		return false
	}
	for _, r := range a.SHA256 {
		if (r < '0' || r > '9') && (r < 'a' || r > 'f') {
			return false
		}
	}
	return true
}

// Result is the answer to one check, in the terms the interface shows.
type Result struct {
	Current string
	Latest  string
	Notes   string
	URL     string
	// Newer reports whether Latest is newer than Current. Both must parse: an
	// unreadable version means "do not tell the user anything", never "probably
	// newer", because a false prompt is worse than a missed one.
	Newer bool
	// Installable reports whether this release can replace the running program:
	// newer, with a package this platform can verify and unpack.
	Installable bool
}

// Stage is where an installation has got to.
type Stage string

const (
	// StageDownload is fetching the package.
	StageDownload Stage = "download"
	// StageInstall is unpacking and replacing the program.
	StageInstall Stage = "install"
	// StageRestart is starting the new build.
	StageRestart Stage = "restart"
)

// Progress is how far an installation has got. Size is 0 while the length is unknown.
type Progress struct {
	Stage Stage
	Done  int64
	Size  int64
}

// Fraction is how much of the download has arrived, between 0 and 1. An unknown
// total reads as 0, which the interface shows as an indefinite wait.
func (p Progress) Fraction() float64 {
	if p.Size <= 0 {
		return 0
	}
	share := float64(p.Done) / float64(p.Size)
	if share > 1 {
		return 1
	}
	return share
}

// Compare orders two versions by their dot-separated numeric parts, so 0.10 is newer than
// 0.9. A leading "v" and any pre-release suffix are ignored.
func Compare(a, b string) int {
	left, leftOK := parse(a)
	right, rightOK := parse(b)
	if !leftOK || !rightOK {
		return 0
	}
	for i := range left {
		switch {
		case left[i] < right[i]:
			return -1
		case left[i] > right[i]:
			return 1
		}
	}
	return 0
}

// Newer reports whether latest is a strictly newer release than current.
func Newer(current, latest string) bool {
	if _, ok := parse(current); !ok {
		return false
	}
	if _, ok := parse(latest); !ok {
		return false
	}
	return Compare(latest, current) > 0
}

// parse splits a version into its numeric parts. Missing parts count as zero, so
// "1.2" and "1.2.0" are the same release.
func parse(version string) ([3]int, bool) {
	var out [3]int
	text := strings.TrimSpace(version)
	text = strings.TrimPrefix(strings.TrimPrefix(text, "v"), "V")
	if cut := strings.IndexAny(text, "-+"); cut >= 0 {
		text = text[:cut]
	}
	if text == "" {
		return out, false
	}
	parts := strings.Split(text, ".")
	if len(parts) > len(out) {
		parts = parts[:len(out)]
	}
	for i, part := range parts {
		if part == "" {
			return out, false
		}
		n, err := strconv.Atoi(part)
		if err != nil || n < 0 {
			return out, false
		}
		out[i] = n
	}
	return out, true
}
