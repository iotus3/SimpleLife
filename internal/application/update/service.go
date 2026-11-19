// Package update answers "is there a newer SimpleLife?"; where the answer comes from is a
// port.
package update

import (
	"context"

	"github.com/iotus3/simplelife/app/internal/domain/shared"
	"github.com/iotus3/simplelife/app/internal/domain/update"
)

// Fetcher is the release channel: it returns what the channel says is the newest
// build. A nil Fetcher means this build has no channel configured.
type Fetcher interface {
	Latest(ctx context.Context) (update.Release, error)
}

// Service checks one release channel against the running build.
type Service struct {
	fetcher Fetcher
	current string
}

// New wires the service with the version of the running build.
func New(fetcher Fetcher, current string) *Service {
	return &Service{fetcher: fetcher, current: current}
}

// Available reports whether this build can check at all. The about page asks
// before it offers the button, so a source build says "not configured" instead of
// offering a check that can only fail.
func (s *Service) Available() bool { return s != nil && s.fetcher != nil }

// Current is the version of the running build.
func (s *Service) Current() string {
	if s == nil {
		return ""
	}
	return s.current
}

// Check compares the channel's answer with the running build. An unreachable channel is
// an error; an older or equal version is the ordinary answer.
func (s *Service) Check(ctx context.Context) (update.Result, error) {
	if !s.Available() {
		return update.Result{}, shared.Unsupported("update.not_configured",
			"this build has no release channel")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	release, err := s.fetcher.Latest(ctx)
	if err != nil {
		return update.Result{}, err
	}
	if release.URL == "" && update.Newer(s.current, release.Version) {
		// A newer release with nowhere to download it is a broken channel, not an
		// update the user can act on.
		return update.Result{}, shared.Invalid("update.no_url", "", "the release has no download address")
	}
	return update.Result{
		Current: s.current,
		Latest:  release.Version,
		Notes:   release.Notes,
		URL:     release.URL,
		Newer:   update.Newer(s.current, release.Version),
	}, nil
}
