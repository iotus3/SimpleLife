package presentation

import (
	"context"
	"runtime"
	"strings"
	"time"

	"github.com/vivaui/viva/app"
	"github.com/vivaui/viva/gfx"
	"github.com/vivaui/viva/ui"

	"github.com/iotus3/simplelife/app/internal/domain/shared"
	"github.com/iotus3/simplelife/app/internal/domain/update"
	"github.com/iotus3/simplelife/app/internal/presentation/i18n"
	"github.com/iotus3/simplelife/app/internal/presentation/icons"
	"github.com/iotus3/simplelife/app/internal/presentation/theme"
)

// The project's addresses. They are constants, not settings: a build that pointed
// elsewhere would be a different application.
const (
	repositoryURL = "https://github.com/iotus3/simplelife"
	issuesURL     = "https://github.com/iotus3/simplelife/issues"
	docsURL       = "https://github.com/iotus3/simplelife#readme"
)

// aboutPage is where a user checks what they are running, whether a newer build exists, and
// where the project lives.
func (s *State) aboutPage(c ui.C) {
	s.pageHeader(c, icons.Info, i18n.MsgNavAbout, i18n.MsgNavAboutSub, func() {
		if ui.Button(c, i18n.T(s.lang, i18n.MsgAboutCopy)).Icon(icons.Copy).Clicked() {
			s.copyAbout(c)
		}
	})
	ui.Column(c).FillWidth().Gap(theme.Layout().Gap).Children(func() {
		s.aboutHero(c)
		s.aboutFacts(c)
		s.aboutUpdate(c)
		s.aboutLinks(c)
		ui.Text(c, i18n.T(s.lang, i18n.MsgAboutColophon)).FontSize(11.5).
			Color(s.palette.Faint).TextCenter().FillWidth().PadY(4)
	})
}

// aboutHero is the application's face: mark, name, tagline and version.
func (s *State) aboutHero(c ui.C) {
	s.card(c, func() {
		ui.Row(c).FillWidth().Items(ui.Center).Gap(16).Children(func() {
			ui.Image(c, s.brandIcon).Size(56, 56)
			ui.Column(c).FillWidth().Gap(4).Children(func() {
				ui.Text(c, i18n.T(s.lang, i18n.MsgAppTitle)).FontSize(21).
					Weight(gfx.WeightSemibold).Color(s.palette.Text).MaxLines(1)
				ui.Text(c, i18n.T(s.lang, i18n.MsgAboutTagline)).FontSize(12.5).
					Color(s.palette.Muted).MaxLines(2)
				ui.Row(c).Items(ui.Center).Gap(6).PadY(2).Children(func() {
					s.tag(c, "v"+s.version(), s.palette.Accent)
					s.tag(c, platformName(), s.palette.Muted)
					if s.deps.Commit != "" {
						s.tag(c, shortCommit(s.deps.Commit), s.palette.Muted)
					}
				})
			})
		})
	})
}

// aboutFacts is the four values a bug report asks for, in a two-column grid, plus
// the folder the data lives in.
func (s *State) aboutFacts(c ui.C) {
	s.cardTitled(c, icons.Info, i18n.MsgAboutInfo, nil, func() {
		ui.Row(c).FillWidth().Gap(12).Children(func() {
			fact(c, s, i18n.MsgAboutVersion, s.version())
			fact(c, s, i18n.MsgAboutPlatform, platformName())
		})
		ui.Row(c).FillWidth().Gap(12).Children(func() {
			fact(c, s, i18n.MsgAboutBuild, s.buildStamp())
			fact(c, s, i18n.MsgAboutRuntime, runtime.Version())
		})
		ui.Row(c).FillWidth().Items(ui.Center).Gap(10).Pad(10).
			Background(s.palette.SurfaceSoft).Radius(theme.Layout().RadiusControl).
			Children(func() {
				ui.Icon(c, icons.Database).Size(14, 14).Color(s.palette.Muted)
				ui.Text(c, i18n.T(s.lang, i18n.MsgAboutDataDir)).FontSize(12).
					Color(s.palette.Muted).MaxLines(1)
				ui.Text(c, s.deps.DataDir).FontSize(12).Mono().Color(s.palette.Text).
					FillWidth().MaxLines(1).Ellipsis()
			})
	})
}

// fact is one labelled value: a small caption above a monospaced value, so a
// version and a commit line up in a report.
func fact(c ui.C, s *State, label i18n.MessageID, value string) {
	ui.Column(c).FillWidth().Gap(3).Pad(10).
		Background(s.palette.SurfaceSoft).Radius(theme.Layout().RadiusControl).
		Children(func() {
			ui.Text(c, i18n.T(s.lang, label)).FontSize(11).Color(s.palette.Muted).MaxLines(1)
			ui.Text(c, value).FontSize(13).Mono().Color(s.palette.Text).
				MaxLines(1).Ellipsis()
		})
}

// aboutUpdate checks for a newer release and holds the automatic check; nothing is downloaded.
func (s *State) aboutUpdate(c ui.C) {
	s.cardTitled(c, icons.Download, i18n.MsgAboutUpdate, nil, func() {
		s.hint(c, i18n.T(s.lang, i18n.MsgAboutUpdateHint))
		checking, result, failure := s.updateSnapshot()
		ui.Row(c).FillWidth().Items(ui.Center).Gap(12).Children(func() {
			auto := s.deps.Settings.AutoUpdate()
			box := ui.Switch(c, &auto, i18n.T(s.lang, i18n.MsgAboutAutoUpdate))
			if box.Changed() {
				chosen := auto
				s.apply(c, RouteAbout, func() error { return s.deps.Settings.SetAutoUpdate(chosen) })
			}
			ui.Spacer(c)
			label := i18n.T(s.lang, i18n.MsgAboutCheckUpdate)
			if checking {
				label = i18n.T(s.lang, i18n.MsgAboutChecking)
			}
			button := ui.Button(c, label).Icon(icons.RefreshCw)
			if !s.deps.Update.Available() || checking {
				button = button.Disabled(true)
			}
			if button.Clicked() {
				s.checkForUpdate(c)
			}
		})
		switch {
		case failure != nil:
			ui.Text(c, i18n.MessageFor(s.lang, failure)).FontSize(12.5).
				Color(s.palette.Expense).MaxLines(3)
		case result == nil:
			if !s.deps.Update.Available() {
				s.hint(c, i18n.T(s.lang, i18n.MsgAboutNoChannel))
			}
		case result.Newer:
			ui.Column(c).FillWidth().Gap(8).Pad(12).
				Background(s.palette.AccentSoft).Radius(theme.Layout().RadiusControl).
				Children(func() {
					ui.Text(c, i18n.T(s.lang, i18n.MsgAboutUpdateFound, result.Latest)).
						FontSize(13).Weight(gfx.WeightSemibold).Color(s.palette.Text)
					if result.Notes != "" {
						ui.Text(c, i18n.T(s.lang, i18n.MsgAboutUpdateNotes)).FontSize(11.5).
							Color(s.palette.Muted)
						ui.Text(c, result.Notes).FontSize(12.5).Color(s.palette.Text).MaxLines(4)
					}
					ui.Row(c).FillWidth().Children(func() {
						ui.Spacer(c)
						if ui.Button(c, i18n.T(s.lang, i18n.MsgAboutOpenDownload)).
							Icon(icons.ArrowUpRight).Clicked() {
							s.openDownload(c, result.URL)
						}
					})
				})
		default:
			ui.Text(c, i18n.T(s.lang, i18n.MsgAboutUpToDate, result.Latest)).FontSize(12.5).
				Color(s.palette.Income).MaxLines(2)
		}
	})
}

// aboutLinks points at the project: the source, the issue tracker and the readme.
func (s *State) aboutLinks(c ui.C) {
	s.cardTitled(c, icons.Globe, i18n.MsgAboutLinks, nil, func() {
		for _, link := range []struct {
			icon  gfx.Icon
			label i18n.MessageID
			url   string
		}{
			{icons.Code, i18n.MsgAboutRepository, repositoryURL},
			{icons.MessageCircle, i18n.MsgAboutIssues, issuesURL},
			{icons.BookOpen, i18n.MsgAboutDocs, docsURL},
		} {
			target := link.url
			row := ui.Row(c).FillWidth().Items(ui.Center).Gap(10).Height(38).
				PadSides(0, 10, 0, 10).Radius(theme.Layout().RadiusControl).
				Hit(ui.ID(c), i18n.T(s.lang, link.label)).Transition()
			row.Hover(ui.Look{}.Background(s.palette.SurfaceSoft))
			row.Children(func() {
				ui.Icon(c, link.icon).Size(15, 15).Color(s.palette.Muted)
				ui.Text(c, i18n.T(s.lang, link.label)).FontSize(13).
					Color(s.palette.Text).FillWidth().MaxLines(1)
				ui.Text(c, target).FontSize(11.5).Color(s.palette.Faint).
					MaxLines(1).Ellipsis()
				ui.Icon(c, icons.ArrowUpRight).Size(13, 13).Color(s.palette.Faint)
			})
			if row.Clicked() {
				s.openLink(c, target)
			}
		}
	})
}

// openDownload opens the release page, refusing an address that is not a web page
// rather than handing the shell something else.
func (s *State) openDownload(c ui.C, target string) {
	s.openLink(c, target)
}

// openLink hands an http(s) address to the desktop's browser. A rejected address is
// reported on the page instead of being opened blindly.
func (s *State) openLink(c ui.C, target string) {
	lower := strings.ToLower(target)
	if !strings.HasPrefix(lower, "https://") && !strings.HasPrefix(lower, "http://") {
		s.toastError(shared.Invalid("ui.link", "url", "not a web address: "+target))
		return
	}
	c.After(func() {
		if err := app.OpenURL(target); err != nil {
			s.toastError(err)
		}
		s.repaint()
	})
}

// copyAbout puts the build details on the clipboard: the first thing a bug report
// needs, and the reason the button sits in the header.
func (s *State) copyAbout(c ui.C) {
	text := strings.Join([]string{
		i18n.T(s.lang, i18n.MsgAppTitle) + " " + s.version(),
		i18n.T(s.lang, i18n.MsgAboutPlatform) + ": " + platformName(),
		i18n.T(s.lang, i18n.MsgAboutBuild) + ": " + s.buildStamp(),
		i18n.T(s.lang, i18n.MsgAboutRuntime) + ": " + runtime.Version(),
		i18n.T(s.lang, i18n.MsgAboutDataDir) + ": " + s.deps.DataDir,
	}, "\n")
	c.After(func() {
		if err := app.SetClipboardText(text); err != nil {
			s.toastError(err)
			s.repaint()
			return
		}
		s.toast(i18n.T(s.lang, i18n.MsgAboutCopied), ui.ToastSuccess)
		s.repaint()
	})
}

// version is the running build's version, or a placeholder for a build that was
// never stamped.
func (s *State) version() string {
	if strings.TrimSpace(s.deps.Version) == "" {
		return "0.0.0"
	}
	return strings.TrimSpace(s.deps.Version)
}

// buildStamp is the build date, or a word for a build that has none.
func (s *State) buildStamp() string {
	stamp := strings.TrimSpace(s.deps.BuildDate)
	if stamp == "" {
		return i18n.T(s.lang, i18n.MsgAboutNoDate)
	}
	if at, err := time.Parse(time.RFC3339, stamp); err == nil {
		return at.Local().Format("2006-01-02 15:04")
	}
	return stamp
}

// shortCommit is the first seven characters of a commit, which is what a person
// reads and pastes.
func shortCommit(commit string) string {
	commit = strings.TrimSpace(commit)
	if len(commit) > 7 {
		return commit[:7]
	}
	return commit
}

// platformName spells the platform the way a person says it, instead of the way Go
// names it.
func platformName() string {
	name := runtime.GOOS
	switch runtime.GOOS {
	case "windows":
		name = "Windows"
	case "darwin":
		name = "macOS"
	case "linux":
		name = "Linux"
	}
	arch := runtime.GOARCH
	switch runtime.GOARCH {
	case "amd64":
		arch = "x86-64"
	case "arm64":
		arch = "arm64"
	case "386":
		arch = "x86"
	}
	return name + " · " + arch
}

// checkForUpdate asks the release channel, off the paint, and repaints with the
// answer. The request has a deadline so a silent server cannot leave the button
// spinning for the rest of the session.
func (s *State) checkForUpdate(c ui.C) {
	if !s.deps.Update.Available() {
		return
	}
	c.After(func() {
		s.setUpdateState(true, nil, nil)
		s.repaint()
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()
			result, err := s.deps.Update.Check(ctx)
			s.finishUpdateCheck(result, err, false)
		}()
	})
}

// autoCheckForUpdate is the startup check the "check on start" switch turns on.
// It only ever sets a notice: the user is told, never installed to.
func (s *State) autoCheckForUpdate() {
	if !s.deps.Settings.AutoUpdate() || !s.deps.Update.Available() {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		result, err := s.deps.Update.Check(ctx)
		s.finishUpdateCheck(result, err, true)
	}()
}

// finishUpdateCheck stores the answer and asks for a repaint. A background failure
// is silent: an unreachable server on start must not interrupt the user, while the
// same failure after a click is shown, because then it was asked for.
func (s *State) finishUpdateCheck(result update.Result, err error, background bool) {
	if background && err != nil {
		return
	}
	s.setUpdateState(false, &result, err)
	if background && result.Newer {
		s.updateNotice.Store(true)
	}
	s.repaint()
}
