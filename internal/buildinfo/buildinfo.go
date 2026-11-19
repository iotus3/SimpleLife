// Package buildinfo names the build the program is: a generated version constant and the
// VCS stamp go writes into every release.
package buildinfo

import "runtime/debug"

// Identity reports the commit and the date this binary was built from, as the VCS stamp
// records them; a build from a source archive has neither.
func Identity() (commit, date string) {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "", ""
	}
	return IdentityFrom(info)
}

// IdentityFrom reads the two settings go writes into a stamped build. It is separate
// from Identity so a test can hand it a build info of its own.
func IdentityFrom(info *debug.BuildInfo) (commit, date string) {
	if info == nil {
		return "", ""
	}
	for _, setting := range info.Settings {
		switch setting.Key {
		case "vcs.revision":
			commit = setting.Value
		case "vcs.time":
			date = setting.Value
		}
	}
	return commit, date
}
