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
	// URL is where the user downloads it. An empty URL means the release exists
	// but there is nowhere to send them, which the interface reports rather than
	// opening a blank page.
	URL string
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
