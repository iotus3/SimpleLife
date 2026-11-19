// Package update answers "is there a newer SimpleLife?" and, when the user asks for it,
// replaces this copy with that release. Where the answer comes from and how a program is
// replaced are both ports.
package update

import (
	"context"
	"sync"

	"github.com/iotus3/simplelife/app/internal/domain/shared"
	"github.com/iotus3/simplelife/app/internal/domain/update"
)

// Fetcher is the release feed: it returns what the feed says is the newest build.
type Fetcher interface {
	Latest(ctx context.Context) (update.Release, error)
}

// Installer replaces the running program with a release it downloads and verifies.
type Installer interface {
	Install(ctx context.Context, release update.Release, progress func(update.Progress)) error
}

// Service checks one release feed against the running build and installs what it finds.
type Service struct {
	fetcher   Fetcher
	installer Installer
	current   string

	// latest is the release the last check found newer, kept so the install button
	// does not have to fetch the feed again. Guarded: the check runs off the paint.
	mu     sync.Mutex
	latest *update.Release
}

// New wires the service with the version of the running build. A nil fetcher means this
// build has no feed; a nil installer means it can report a release but not apply one.
func New(fetcher Fetcher, installer Installer, current string) *Service {
	return &Service{fetcher: fetcher, installer: installer, current: current}
}

// Available reports whether this build can check at all. The about page asks before it
// offers the button, so a source build says "not configured" instead of offering a check
// that can only fail.
func (s *Service) Available() bool { return s != nil && s.fetcher != nil }

// CanInstall reports whether this build can replace itself. Windows portable copies can;
// a build without a feed, or a platform whose package is not published, cannot.
func (s *Service) CanInstall() bool { return s != nil && s.installer != nil }

// Current is the version of the running build.
func (s *Service) Current() string {
	if s == nil {
		return ""
	}
	return s.current
}

// Check compares the feed's answer with the running build. An unreachable feed is an
// error; an older or equal version is the ordinary answer.
func (s *Service) Check(ctx context.Context) (update.Result, error) {
	if !s.Available() {
		return update.Result{}, shared.Unsupported("update.not_configured",
			"this build has no release feed")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	release, err := s.fetcher.Latest(ctx)
	if err != nil {
		return update.Result{}, err
	}
	newer := update.Newer(s.current, release.Version)
	installable := newer && release.Asset.Usable() && s.CanInstall()
	if newer && release.URL == "" && !installable {
		// A newer release with nowhere to go is a broken feed, not an update the
		// user can act on.
		return update.Result{}, shared.Invalid("update.no_url", "", "the release has no download address")
	}
	s.remember(release, newer)
	return update.Result{
		Current:     s.current,
		Latest:      release.Version,
		Notes:       release.Notes,
		URL:         release.URL,
		Newer:       newer,
		Installable: installable,
	}, nil
}

// Install downloads the release the last check found and replaces this copy with it. The
// program does not restart itself: that is the composition root's job, because the
// single-instance claim has to be released first.
func (s *Service) Install(ctx context.Context, progress func(update.Progress)) error {
	if !s.CanInstall() {
		return shared.Unsupported("update.install_unsupported",
			"this build cannot replace itself")
	}
	release, ok := s.Pending()
	if !ok {
		return shared.Invalid("update.nothing_to_install", "",
			"no newer release has been found; check for updates first")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := s.installer.Install(ctx, release, progress); err != nil {
		return err
	}
	s.forget()
	return nil
}

// remember keeps the release for a later install, or drops the previous one when the feed
// says there is nothing to do.
func (s *Service) remember(release update.Release, newer bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !newer || !release.Asset.Usable() {
		s.latest = nil
		return
	}
	copied := release
	s.latest = &copied
}

// Pending is the release an install would apply, if the last check found one it can use.
func (s *Service) Pending() (update.Release, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.latest == nil {
		return update.Release{}, false
	}
	return *s.latest, true
}

// forget drops the release after it has been applied, so a second click cannot install
// the same package twice.
func (s *Service) forget() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.latest = nil
}
