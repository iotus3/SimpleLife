package presentation

import (
	"github.com/vivaui/viva/app"
	"github.com/vivaui/viva/gfx"
	"github.com/vivaui/viva/ui"

	"github.com/iotus3/simplelife/app/internal/application/settings"
	"github.com/iotus3/simplelife/app/internal/domain/focus"
	"github.com/iotus3/simplelife/app/internal/presentation/i18n"
	"github.com/iotus3/simplelife/app/internal/presentation/icons"
	"github.com/iotus3/simplelife/app/internal/presentation/theme"
)

// Window sizes: the minimum is the size at which the English interface still
// fits, which is wider than the Chinese one.
const (
	windowWidth     = 1240
	windowHeight    = 820
	windowMinWidth  = 1000
	windowMinHeight = 680
)

// Option configures the window beyond its dependencies.
type Option func(*options)

type options struct {
	onLanguage func(i18n.Lang)
}

// WithLanguageObserver is told the language the window draws in, now and after
// every switch, so adapters outside the window (a notification, a status item)
// speak the same language.
func WithLanguageObserver(observer func(i18n.Lang)) Option {
	return func(options *options) { options.onLanguage = observer }
}

// Run opens the window and blocks until it closes.
func Run(deps Deps, opts ...Option) error {
	configured := options{}
	for _, option := range opts {
		option(&configured)
	}
	invalidator := &app.Invalidator{}
	state := NewState(deps)
	state.onLanguage = configured.onLanguage
	if state.onLanguage != nil {
		state.onLanguage(state.lang)
	}
	state.Attach(invalidator)

	page := ui.NewPage(ui.PageConfig{
		View:        state.View,
		Theme:       state.themeSlot,
		Invalidator: invalidator,
	})
	var canvas gfx.Canvas
	var window *app.Window
	config := app.Config{
		Title:       i18n.T(state.lang, i18n.MsgAppTitle),
		Icon:        deps.Icon,
		Width:       windowWidth,
		Height:      windowHeight,
		MinWidth:    windowMinWidth,
		MinHeight:   windowMinHeight,
		Invalidator: invalidator,
		// The window is drawn to its edges: the shell is the only chrome.
		ClearColor: windowColor(state.palette.Workspace),
		// Closing hides the window when the user asked it to keep running, so a
		// pomodoro is not lost to tidying the desktop; the status item brings it back.
		OnCloseRequest: func(allow func()) {
			if state.shouldHideOnClose() && window != nil {
				window.Hide()
				state.noteHiddenToTray()
				return
			}
			allow()
		},
	}
	state.startBackground()
	// A build with a release channel checks once at start, and only when the user
	// asked it to; the result is a toast, never an installation.
	if deps.Settings.AutoUpdate() {
		state.autoCheckForUpdate()
	}
	// A "follow the system" window is repainted when the desktop switches its own
	// look. The callback only notes the change: reading the appearance while
	// painting is what the framework asks for, and it keeps the answer fresh.
	if state.themePref == settings.ThemeAuto {
		stop := app.WatchSystemAppearance(func() {
			state.appearanceDirty.Store(true)
			state.repaint()
		})
		defer stop()
	}
	return app.RunApplication(func(application *app.Application) error {
		var err error
		// The application object is the only thing that can end the process; an update
		// that has replaced the program uses it to stop this build.
		state.quit = application.Quit
		window, err = application.OpenWindow(page.Config(config), &canvas)
		if err != nil {
			return err
		}
		// The status item is how a hidden window comes back and how the application is quit once its
		// window is out of the way; a platform without one simply has none.
		state.attachStatusItem(application, window)
		if window != nil {
			state.watchSecondLaunch(window.Show, deps.SecondLaunch)
		}
		return nil
	})
}

// watchSecondLaunch brings the window forward every time a later start is handed to this copy.
//
// The waking call is this copy's own window.Show(): the framework posts it to the event loop
// and keeps its idea of "hidden by the application" in step with the real window.
func (s *State) watchSecondLaunch(show func(), launches <-chan struct{}) {
	if launches == nil || show == nil {
		return
	}
	go func() {
		for range launches {
			show()
			s.noteSecondLaunch()
		}
	}()
}

// noteSecondLaunch records that another start was handed to this copy: the next
// paint turns it into a toast. It may be called from the watcher goroutine, so the
// flag is atomic and the repaint is asked for, never performed.
func (s *State) noteSecondLaunch() {
	s.secondLaunch.Store(true)
	s.repaint()
}

// noteHiddenToTray says where the window went, once in the life of the installation: a window
// that vanishes into the status area looks like one that ended the program. The flag is written
// before the notification is attempted, so a desktop that cannot show one still keeps the
// promise.
func (s *State) noteHiddenToTray() {
	if s.deps.Settings.TrayHintShown() {
		return
	}
	_ = s.deps.Settings.SetTrayHintShown()
	if s.deps.Notify == nil {
		return
	}
	s.deps.Notify(i18n.T(s.lang, i18n.MsgAppTitle), i18n.T(s.lang, i18n.MsgHiddenToTray))
}

// attachStatusItem puts the application in the desktop's status area: a press opens
// the window, its menu offers the same and quitting. The labels are read on every
// menu build, so they follow a language change made while the app is running.
func (s *State) attachStatusItem(application *app.Application, window *app.Window) {
	item, err := application.AddStatusItem(app.StatusItemConfig{
		Tooltip: i18n.T(s.lang, i18n.MsgAppTitle),
		Icon:    app.StatusIcon{Size: s.deps.Icon.Size, RGBA: s.deps.Icon.RGBA},
		OnClick: func(app.StatusClick) { window.Show() },
		Menu: func() []app.MenuItem {
			return []app.MenuItem{
				{Title: i18n.T(s.lang, i18n.MsgTrayShow), OnSelect: func() { window.Show() }},
				{Separator: true},
				{Title: i18n.T(s.lang, i18n.MsgTrayQuit), OnSelect: func() { application.Quit() }},
			}
		},
	})
	if err != nil {
		// No status area is a normal desktop, not a failure: the window keeps working, and closing it
		// quits because shouldHideOnClose checks this reference.
		return
	}
	s.statusItem = item
	// The settings page offers the close-button behaviour only where the status area
	// exists, and it is painted before this call: ask for one more frame, so the row
	// appears (or stays away) on the strength of what is really there.
	s.repaint()
}

// closeActionVisible reports whether closing the window can leave the program in the
// status area, which is the only case where the choice of what closing does is worth
// offering. A desktop without one gets no row that promises something it cannot do.
func (s *State) closeActionVisible() bool {
	return s.statusItem != nil
}

// shouldHideOnClose reports whether the close button leaves the program running in the status
// area. Both halves matter: the behaviour chosen, and a status item that actually exists.
func (s *State) shouldHideOnClose() bool {
	return s.deps.Settings.CloseAction() == settings.CloseTray && s.statusItem != nil
}

// windowColor converts a palette color into the window's own color type, which
// the native clear and the border of a custom titlebar take.
func windowColor(c ui.Color) app.Color {
	return app.Color{
		Red:   float64(c.Red),
		Green: float64(c.Green),
		Blue:  float64(c.Blue),
		Alpha: float64(c.Alpha),
	}
}

// View declares the window: the rail, the page beside it, and the toast layer.
func (s *State) View(c ui.C) {
	s.rollDay()
	if s.appearanceDirty.Swap(false) {
		s.applyAppearance()
	}
	s.pump(c)
	ui.Row(c).Fill().Children(func() {
		s.sidebar(c)
		// A one pixel separator instead of a border on the rail: the rail has no
		// border of its own, so the window's edge stays quiet.
		ui.Column(c).Width(1).FillHeight().Background(s.palette.Line)
		s.body(c)
	})
	if s.secondLaunch.Swap(false) {
		s.toast(i18n.T(s.lang, i18n.MsgSecondLaunch), ui.ToastInfo)
	}
	if s.updateNotice.Swap(false) {
		if _, result, _ := s.updateSnapshot(); result != nil && result.Newer {
			s.toast(i18n.T(s.lang, i18n.MsgAboutUpdateFound, result.Latest), ui.ToastInfo)
		}
	}
	s.confirmDialog(c)
	ui.Toasts(c, &s.toaster)
}

// body is the workspace: one scroll area every page draws in, so a page that
// grows past the window can be read to its end, and the offset goes back to the
// top when the page changes.
func (s *State) body(c ui.C) {
	m := theme.Layout()
	area := ui.Scroll(c).Fill().Scrollbars()
	area.Pad(m.PagePad).Gap(m.Gap).
		Background(s.palette.Workspace)
	if s.scrolledRoute != s.route {
		area.ScrollState().ScrollTo(0)
		s.scrolledRoute = s.route
	}
	area.Children(func() {
		s.page(c)
	})
}

// sidebar is the navigation rail: the brand, the eight pages, the running
// pomodoro and the version, in that order from the top.
func (s *State) sidebar(c ui.C) {
	m := theme.Layout()
	ui.Column(c).Width(m.SidebarWidth).FillHeight().
		Background(s.palette.Shell).
		Children(func() {
			s.brandHeader(c)
			ui.Column(c).FillWidth().PadSides(0, 10, 0, 10).Gap(2).Children(func() {
				ui.Column(c).FillWidth().PadSides(0, 0, 0, 0).Gap(2).Children(func() {
					for _, route := range routes {
						s.navItem(c, route)
					}
				})
			})
			ui.Spacer(c)
			if s.active != nil {
				s.sidebarSession(c)
			}
			ui.Text(c, i18n.T(s.lang, i18n.MsgAppVersion, s.deps.Version)).
				FontSize(11).Color(s.palette.Faint).
				PadSides(0, 18, 14, 18).MaxLines(1)
		})
}

// brand is the application's mark and name at the top of the rail.
func (s *State) brandHeader(c ui.C) {
	ui.Row(c).FillWidth().Items(ui.Center).Gap(10).
		PadSides(16, 16, 18, 16).Children(func() {
		ui.Image(c, s.brandIcon).Size(28, 28)
		ui.Column(c).Gap(1).FillWidth().Children(func() {
			ui.Text(c, i18n.T(s.lang, i18n.MsgAppTitle)).FontSize(15).
				Weight(gfx.WeightSemibold).Color(s.palette.Text).MaxLines(1)
			ui.Text(c, i18n.T(s.lang, i18n.MsgSidebarTag)).FontSize(11).
				Color(s.palette.Faint).MaxLines(1).Ellipsis()
		})
	})
}

// navItem is one page in the rail: an icon and its name, the current page filled
// with the accent's tint and its icon in the accent.
func (s *State) navItem(c ui.C, route Route) {
	m := theme.Layout()
	active := route == s.route
	label := i18n.T(s.lang, route.message())
	row := ui.Row(c).FillWidth().Height(38).Items(ui.Center).Gap(10).
		PadSides(0, 10, 0, 10).Radius(m.RadiusControl)
	row.Hit(ui.ID(c), label).Transition()
	switch {
	case active:
		row.Background(s.palette.AccentSoft)
	case true:
		row.Hover(ui.Look{}.Background(s.palette.SurfaceSoft))
	}
	textTint, iconTint := s.palette.Muted, s.palette.Muted
	if active {
		textTint, iconTint = s.palette.Text, s.palette.Accent
	}
	row.Children(func() {
		ui.Icon(c, routeIcon(route)).Size(18, 18).Color(iconTint)
		ui.Text(c, label).FontSize(14).Color(textTint).MaxLines(1).Ellipsis()
	})
	if row.Clicked() {
		s.navigate(route)
	}
}

// routeIcon is the mark of a page.
func routeIcon(route Route) gfx.Icon {
	switch route {
	case RouteGoals:
		return icons.Flag
	case RouteTasks:
		return icons.ListTodo
	case RouteFocus:
		return icons.Timer
	case RouteLedger:
		return icons.Wallet
	case RouteStats:
		return icons.ChartColumn
	case RouteTemplates:
		return icons.LayoutTemplate
	case RouteSettings:
		return icons.Settings
	case RouteData:
		return icons.Database
	case RouteAbout:
		return icons.Info
	}
	return icons.House
}

// sidebarSession is the miniature timer that stays visible on every page: the
// one thing a pomodoro application must never hide.
func (s *State) sidebarSession(c ui.C) {
	session := s.active
	running := session.Phase != focus.PhasePaused
	ui.Column(c).FillWidth().PadSides(0, 12, 12, 12).Children(func() {
		ui.Column(c).FillWidth().Pad(12).Gap(6).
			Background(s.palette.Surface).Radius(theme.Layout().RadiusCard-4).
			Border(1, s.palette.Line).Children(func() {
			ui.Row(c).FillWidth().Items(ui.Center).Gap(6).Children(func() {
				dot := s.palette.Accent
				if !running {
					dot = s.palette.Warning
				}
				ui.Text(c, "●").FontSize(8).Color(dot)
				ui.Text(c, i18n.T(s.lang, s.kindMessage(session.Kind))).
					FontSize(11.5).Color(s.palette.Muted).FillWidth().MaxLines(1)
				ui.Text(c, i18n.T(s.lang, s.focusStateMessage(session))).
					FontSize(11).Color(s.palette.Faint).MaxLines(1)
			})
			ui.Text(c, clockOf(session, s.now())).FontSize(20).
				Weight(gfx.WeightSemibold).Color(s.palette.Text).Mono()
		})
	})
}

func (s *State) navigate(route Route) {
	if s.route == route {
		return
	}
	s.route = route
	s.markAll()
}

// page dispatches to the current page.
func (s *State) page(c ui.C) {
	switch s.route {
	case RouteGoals:
		s.goalsPage(c)
	case RouteTasks:
		s.tasksPage(c)
	case RouteFocus:
		s.focusPage(c)
	case RouteLedger:
		s.ledgerPage(c)
	case RouteStats:
		s.statsPage(c)
	case RouteTemplates:
		s.templatesPage(c)
	case RouteSettings:
		s.settingsPage(c)
	case RouteData:
		s.dataPage(c)
	case RouteAbout:
		s.aboutPage(c)
	default:
		s.homePage(c)
	}
}

// Reload marks every page stale and repaints: the entry point the composition
// root uses when something outside the window changed the data.
func (s *State) Reload() {
	s.markAll()
	s.repaint()
}

// Language is the language the interface is drawn in.
func (s *State) Language() i18n.Lang { return s.lang }
