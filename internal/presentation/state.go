package presentation

import (
	"context"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/vivaui/viva/app"
	"github.com/vivaui/viva/gfx"
	"github.com/vivaui/viva/ui"

	lapp "github.com/iotus3/simplelife/app/internal/application/ledger"
	"github.com/iotus3/simplelife/app/internal/application/settings"
	tapp "github.com/iotus3/simplelife/app/internal/application/task"
	tplapp "github.com/iotus3/simplelife/app/internal/application/template"
	"github.com/iotus3/simplelife/app/internal/domain/focus"
	"github.com/iotus3/simplelife/app/internal/domain/goal"
	"github.com/iotus3/simplelife/app/internal/domain/ledger"
	"github.com/iotus3/simplelife/app/internal/domain/shared"
	"github.com/iotus3/simplelife/app/internal/domain/task"
	"github.com/iotus3/simplelife/app/internal/domain/template"
	"github.com/iotus3/simplelife/app/internal/domain/update"
	"github.com/iotus3/simplelife/app/internal/presentation/format"
	"github.com/iotus3/simplelife/app/internal/presentation/i18n"
	"github.com/iotus3/simplelife/app/internal/presentation/theme"
)

// Route is a page of the window.
type Route int

const (
	RouteHome Route = iota
	RouteGoals
	RouteTasks
	RouteFocus
	RouteLedger
	RouteStats
	RouteTemplates
	RouteSettings
	RouteData
	RouteAbout
)

// routes is the sidebar, in order. The goals sit right after the dashboard: a
// countdown is what the day is aimed at, so it is read before the work that fills
// it. The timer follows, then the day's tasks.
var routes = []Route{
	RouteHome, RouteGoals, RouteFocus, RouteTasks, RouteLedger,
	RouteStats, RouteTemplates, RouteSettings, RouteData, RouteAbout,
}

func (r Route) message() i18n.MessageID {
	switch r {
	case RouteGoals:
		return i18n.MsgNavGoals
	case RouteTasks:
		return i18n.MsgNavTasks
	case RouteFocus:
		return i18n.MsgNavFocus
	case RouteLedger:
		return i18n.MsgNavLedger
	case RouteStats:
		return i18n.MsgNavStats
	case RouteTemplates:
		return i18n.MsgNavTemplate
	case RouteSettings:
		return i18n.MsgNavSettings
	case RouteData:
		return i18n.MsgNavData
	case RouteAbout:
		return i18n.MsgNavAbout
	}
	return i18n.MsgNavHome
}

// State is what the interface keeps between paints. Everything in it is owned by
// the window thread: a page reads and writes it while it declares, and the only
// cross-thread flags are the atomic ones below.
type State struct {
	deps    Deps
	inv     *app.Invalidator
	palette *theme.Palette
	// themeSlot is the theme the page was built with. The page keeps this pointer
	// for the life of the window and reads it on every paint, so switching the
	// appearance writes through it instead of replacing it.
	themeSlot *ui.Theme
	// themePref is the stored appearance preference, and dark what it resolved to.
	themePref string
	dark      bool
	// appearanceDirty is set when the desktop changed its own look, which the next
	// paint turns into a palette.
	appearanceDirty atomic.Bool

	lang     i18n.Lang
	langPref string
	// onLanguage tells the world outside the window which language it draws in.
	onLanguage func(i18n.Lang)
	route      Route
	toaster    ui.Toaster

	// brandIcon is the application's icon as the sidebar paints it.
	brandIcon gfx.Bitmap
	// statusItem is the desktop's status-area icon, when the platform has one. The
	// reference is held on purpose: the icon lives as long as the value does.
	statusItem *app.StatusItem

	// scrolledRoute is the page the scroll area is showing. The area keeps its
	// offset under its own name, so the next page would start where the last one
	// was left; a page change puts it back at the top.
	scrolledRoute Route

	// generation counts data changes. The page's loader effect is keyed by it, so
	// a change starts a load and the paint after it draws the new data.
	generation uint64
	// loader runs the read of a route off the paint; its result only says "read
	// this route again", because the read itself is a local query that must not
	// touch the model from another goroutine.
	loader *ui.EffectState[loadKey, loadKey]

	// Dashboard.
	overview dashboardView

	// Tasks.
	taskTab        int
	taskHistoryDay shared.LocalDate
	taskToday      []*task.Task
	taskOverdue    int
	taskGroups     []tapp.Group
	taskChildren   map[shared.ID][]*task.Task
	taskSearch     string
	taskQuickAdd   string
	taskDraft      *taskDraft
	confirm        confirmState

	// Focus.
	active      *focus.Session
	focusStats  focusStats
	focusRecent []*focus.Session
	focusKind   focus.Kind
	// focusMin is the length the next focus session gets: 0 for the length from the
	// settings, a positive number of minutes for one of the usual lengths, and
	// focusDurationCustom for focusCustom's typed number.
	focusMin    int
	focusCustom string
	focusTask   shared.ID
	settling    bool
	recovering  bool
	suspicious  *focus.Session
	economy     focus.EconomySettings
	timerCfg    focus.TimerSettings

	// Ledger.
	ledgerTab    int
	ledgerMonth  shared.Month
	entries      []*ledger.Entry
	entrySummary lapp.Summary
	balances     []lapp.AccountBalance
	accounts     []*ledger.Account
	categories   []*ledger.Category
	budgets      []*ledger.Budget
	spending     []lapp.CategorySpend
	// budgetRows is the budget tab: the ceilings of the month first, then the
	// categories that spent money without one. Built from the budgets, not from the
	// spending, so a ceiling shows up the moment it is set.
	budgetRows []lapp.BudgetRow
	statsBars  []monthBar
	// statsRange is the window the statistics page counts: a number of days back
	// from today, or statsRangeCustom for the two dates the user typed.
	statsRange    int
	statsFromText string
	statsToText   string
	// statsTotals is the sum over the window, and statsValid says whether the window
	// is one that can be counted at all.
	statsTotals focusStats
	statsValid  bool
	// statsWantFrom/To is the window the page asked for, statsLoadedFrom/To is the window the bars
	// describe. They part while a read is in flight, so the page can say it is counting.
	statsWantFrom, statsWantTo     shared.LocalDate
	statsLoadedFrom, statsLoadedTo shared.LocalDate
	entryDraft                     *entryDraft
	accountDraft                   *accountDraft
	budgetDraft                    *budgetDraft
	// categoryDraft is the category editor. Categories live on their own tab now,
	// where creating, renaming and deleting one all belong.
	categoryDraft *categoryDraft

	// Goals.
	goals     []*goal.Goal
	goalDraft *goalDraft

	// Templates.
	templates       []*template.Template
	templateDraft   *templateDraft
	templatePreview []tplapp.PreviewItem
	// templatePreviewOpen is whether "preview today" is showing; the button that
	// asks for it takes it away again.
	templatePreviewOpen bool
	templateLogs        []template.LogEntry

	// Settings.
	timerForm   timerForm
	economyForm economyForm

	// Data export.
	exportDraft exportDraft

	// The calendar a date field has open, if any.
	datePick datePick

	// Data import. The draft is written from the file dialog's callback, so it is
	// guarded; importState copies it for a paint.
	importMu    sync.Mutex
	importDraft importDraft

	// Update check. The result arrives from a goroutine, so it is guarded; the
	// notice is a flag the paint turns into a toast, because a toast must be shown
	// while declaring.
	updateMu       sync.Mutex
	updateChecking bool
	updateResult   *update.Result
	updateErr      error
	updateNotice   atomic.Bool

	// secondLaunch is set when another start was handed to this copy. The paint
	// turns it into a toast, because a window that comes forward looks exactly like
	// one that has just been started.
	secondLaunch atomic.Bool

	// running mirrors "a focus session is active" for the ticker goroutine.
	running atomic.Bool
	// dayRolled is set by the ticker when midnight passed.
	dayRolled atomic.Bool

	tickMu  sync.Mutex
	tickDay shared.LocalDate
}

// What a confirmation dialog is about. One dialog serves every delete, so the
// question looks the same wherever it is asked.
const (
	confirmNone = iota
	confirmTask
	confirmEntry
	confirmTemplate
	confirmCategory
	confirmGoal
	confirmAccount
	confirmBudget
)

type confirmState struct {
	open bool
	kind int
	id   shared.ID
}

func (s *State) askDelete(kind int, id shared.ID) {
	s.confirm = confirmState{open: true, kind: kind, id: id}
}

type dashboardView struct {
	day       shared.LocalDate
	tasks     []*task.Task
	open      int
	doneToday int
	overdue   int
	planned   int
	completed int
	minutes   int
	income    []shared.MoneyTotal
	expense   []shared.MoneyTotal
}

type focusStats struct {
	completed int
	minutes   int
	breaks    int
	planned   int
}

// monthBar is one day of the statistics chart.
type monthBar struct {
	day       shared.LocalDate
	pomodoros int
}

// NewState builds the interface state. It reads the settings once so the first
// frame is already in the user's language and currency.
func NewState(deps Deps) *State {
	state := &State{
		deps:         deps,
		themeSlot:    &ui.Theme{},
		themePref:    deps.Settings.Theme(),
		taskChildren: make(map[shared.ID][]*task.Task),
		focusKind:    focus.KindFocus,
		focusMin:     defaultFocusMinutes,
		generation:   1,
	}
	state.applyAppearance()
	state.loader = ui.NewEffectKeyed(func(_ context.Context, key loadKey) (loadKey, error) {
		return key, nil
	})
	state.brandIcon = brandBitmap(deps.Icon)
	state.langPref = deps.Settings.Language()
	state.lang = i18n.Resolve(state.langPref, deps.SystemLanguages)
	state.ledgerMonth = shared.ThisMonth(deps.Clock)
	state.taskHistoryDay = shared.Today(deps.Clock)
	state.suspicious, _ = deps.Focus.Suspicious()
	state.recovering = state.suspicious != nil
	state.loadSchedules()
	state.statsRange = statsRangeWeek
	state.resetStatsDates()
	state.resetTimerForm()
	state.resetEconomyForm()
	state.resetEntryDraft()
	state.resetCategoryDraft()
	state.resetGoalDraft()
	state.resetExportDraft()
	return state
}

// applyAppearance resolves the appearance preference into a palette and installs it through
// themeSlot, because the page holds that pointer for the life of the window.
func (s *State) applyAppearance() {
	dark := s.systemDark()
	switch s.themePref {
	case settings.ThemeLight:
		dark = false
	case settings.ThemeDark:
		dark = true
	}
	palette := theme.For(dark)
	*s.themeSlot = *palette.Theme
	s.palette = palette
	s.dark = dark
	// Only the parts the system draws (window buttons, native menus, native text
	// editing) are pinned; an "auto" preference hands them back to the desktop.
	switch s.themePref {
	case settings.ThemeLight:
		app.SetNativeAppearance(app.AppearanceLight)
	case settings.ThemeDark:
		app.SetNativeAppearance(app.AppearanceDark)
	default:
		app.SetNativeAppearance(app.AppearanceUnknown)
	}
}

// systemDark asks the desktop which look it uses. An unanswered question — a test,
// a platform without the setting — means light.
func (s *State) systemDark() bool {
	if s.deps.SystemAppearance == nil {
		return false
	}
	return s.deps.SystemAppearance() == app.AppearanceDark
}

// setThemePref stores the appearance preference and repaints in it.
func (s *State) setThemePref(pref string) {
	if !settings.ValidTheme(pref) || pref == s.themePref {
		return
	}
	s.themePref = pref
	s.applyAppearance()
	s.repaint()
}

// setUpdateState stores the outcome of one check.
func (s *State) setUpdateState(checking bool, result *update.Result, err error) {
	s.updateMu.Lock()
	defer s.updateMu.Unlock()
	s.updateChecking = checking
	s.updateResult = result
	s.updateErr = err
}

// updateSnapshot reads the last check for the about page.
func (s *State) updateSnapshot() (bool, *update.Result, error) {
	s.updateMu.Lock()
	defer s.updateMu.Unlock()
	return s.updateChecking, s.updateResult, s.updateErr
}

// Attach gives the state the window's invalidator, which background work uses to
// ask for a repaint.
func (s *State) Attach(invalidator *app.Invalidator) { s.inv = invalidator }

// repaint asks the window to paint again; a state used without a window (tests)
// simply skips it.
func (s *State) repaint() {
	if s.inv != nil {
		s.inv.Invalidate()
	}
}

// loadKey names one read: which page, and which generation of the data.
type loadKey struct {
	route      Route
	generation uint64
}

// pump schedules the read of the current page and applies it. Viva runs a page's After only in
// the event pass of a click, so the read is declared as an Effect and the result is applied
// here, before the page is declared.
func (s *State) pump(c ui.C) {
	ui.Effect(c, s.loader, loadKey{route: s.route, generation: s.generation})
	if key, _, ok := s.loader.Take(); ok {
		s.loadRoute(key.route)
	}
}

// resetStatsDates points the custom range at the last week, so opening it is one
// press away from a range that works.
func (s *State) resetStatsDates() {
	today := s.today()
	s.statsFromText = format.Date(s.lang, today.AddDays(-(statsRangeWeek - 1)))
	s.statsToText = format.Date(s.lang, today)
}

// markAll makes the data stale, after a write that can touch several pages.
func (s *State) markAll() {
	s.generation++
}

// apply runs a mutation after the current paint, reports its failure as a toast
// and makes the data stale so the next paint reads it again.
func (s *State) apply(c ui.C, route Route, fn func() error) {
	_ = route
	c.After(func() {
		if err := fn(); err != nil {
			s.toastError(err)
			s.repaint()
			return
		}
		s.markAll()
		s.repaint()
	})
}

func (s *State) toastError(err error) {
	s.toaster.Show(i18n.T(s.lang, i18n.MsgErrorGeneric), i18n.MessageFor(s.lang, err), ui.ToastError)
}

func (s *State) toast(message string, kind ui.ToastKind) {
	s.toaster.Show(message, "", kind)
}

func (s *State) now() time.Time { return s.deps.Clock.Now() }

func (s *State) today() shared.LocalDate { return shared.Today(s.deps.Clock) }

// focusDurationCustom is the length choice that opens a field for any number of
// minutes. The other choices are a number of minutes.
const focusDurationCustom = -1

// focusLengthChoices are the lengths the focus page offers: the three usual pomodoros and the
// custom one, 25 first because a session starts there when the user has not said otherwise.
var focusLengthChoices = []int{25, 35, 55, focusDurationCustom}

// defaultFocusMinutes is the length of a session started without a choice.
const defaultFocusMinutes = 25

// The windows the statistics page counts in. statsRangeCustom is the entry that
// shows the two date fields; statsRangeMaxDays caps a custom span at a year, so a
// typo cannot ask for millions of days.
const (
	statsRangeWeek      = 7
	statsRangeFortnight = 14
	statsRangeMonth     = 30
	statsRangeCustom    = -1
	statsRangeMaxDays   = 366
)

// focusMinutes is the length the next focus session gets. A typed length out of
// bounds falls back to the default rather than refusing to start.
func (s *State) focusMinutes() int {
	if s.focusMin == focusDurationCustom {
		typed, err := strconv.Atoi(strings.TrimSpace(s.focusCustom))
		if err != nil || typed < int(focus.FocusMin/time.Minute) || typed > int(focus.FocusMax/time.Minute) {
			return defaultFocusMinutes
		}
		return typed
	}
	if s.focusMin > 0 {
		return s.focusMin
	}
	return defaultFocusMinutes
}

// focusPlanned is the length as a duration, for the session and the ring.
func (s *State) focusPlanned() time.Duration {
	return time.Duration(s.focusMinutes()) * time.Minute
}

// startBackground runs the two timers the window needs: a one-second beat that
// repaints a running session, and the check that notices midnight passing.
func (s *State) startBackground() {
	s.tickMu.Lock()
	s.tickDay = s.today()
	s.tickMu.Unlock()
	go func() {
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for range ticker.C {
			today := s.today()
			s.tickMu.Lock()
			rolled := today != s.tickDay
			if rolled {
				s.tickDay = today
			}
			s.tickMu.Unlock()
			if rolled {
				s.dayRolled.Store(true)
				s.repaint()
				continue
			}
			if s.running.Load() {
				s.repaint()
			}
		}
	}()
}

// rollDay runs what the new day needs: the templates expand for today, and the
// lists that are about "today" are read again. It runs on the window thread, at the
// start of the paint that noticed midnight passing.
func (s *State) rollDay() {
	if !s.dayRolled.Swap(false) {
		return
	}
	if _, err := s.deps.Templates.Expand(s.today()); err != nil {
		s.toastError(err)
	}
	s.taskHistoryDay = s.today()
	s.markAll()
}

// loadRoute refreshes the data of a page from the application services. It runs on
// the window thread, before the page is declared, and reports the first failure it
// meets.
func (s *State) loadRoute(route Route) {
	var err error
	switch route {
	case RouteHome:
		err = s.loadHome()
	case RouteGoals:
		err = s.loadGoals()
	case RouteTasks:
		err = s.loadTasks()
	case RouteFocus:
		err = s.loadFocus()
	case RouteLedger:
		err = s.loadLedger()
	case RouteStats:
		err = s.loadStats()
	case RouteTemplates:
		err = s.loadTemplates()
	case RouteSettings:
		err = s.loadSchedules()
	case RouteData:
		err = s.loadExport()
	}
	if err != nil {
		s.toastError(err)
	}
}

func (s *State) loadSchedules() error {
	s.timerCfg = s.deps.Settings.Timer()
	s.economy = s.deps.Settings.Economy()
	// The settings page picks a reward account and two categories, so it needs the
	// lists itself: a user who opens it first never passes through the ledger.
	accounts, err := s.deps.Ledger.Accounts(true)
	if err != nil {
		return err
	}
	s.accounts = accounts
	categories, err := s.deps.Ledger.Categories()
	if err != nil {
		return err
	}
	s.categories = categories
	s.fillEconomyDefaults()
	return nil
}

func (s *State) loadHome() error {
	overview, err := s.deps.Dashboard.Load()
	if err != nil {
		return err
	}
	s.overview = dashboardView{
		day:       overview.Day,
		tasks:     overview.TodayTasks,
		open:      overview.OpenTasks,
		doneToday: overview.DoneToday,
		overdue:   overview.OverdueTasks,
		planned:   overview.PlannedPomodoros,
		completed: overview.CompletedFocus,
		minutes:   overview.FocusMinutes,
		income:    overview.Income,
		expense:   overview.Expense,
	}
	s.active = overview.ActiveSession
	s.running.Store(s.active != nil)
	return nil
}

func (s *State) loadTasks() error {
	today := s.today()
	tasks, err := s.deps.Tasks.Today(today)
	if err != nil {
		return err
	}
	s.taskToday = tasks
	overdue, err := s.deps.Tasks.OverdueCount()
	if err != nil {
		return err
	}
	s.taskOverdue = overdue
	groups, err := s.deps.Tasks.HistoryGroups(today)
	if err != nil {
		return err
	}
	s.taskGroups = groups
	// The row of the task a session runs on replaces its start button with the
	// session's own controls, so the list needs the session as well as the tasks.
	active, err := s.deps.Focus.Active()
	if err != nil {
		return err
	}
	s.active = active
	s.running.Store(active != nil)
	if s.taskTab == 0 {
		return nil
	}
	if s.taskHistoryDay.IsZero() {
		s.taskHistoryDay = today.AddDays(-1)
	}
	return nil
}

func (s *State) loadFocus() error {
	active, err := s.deps.Focus.Active()
	if err != nil {
		return err
	}
	s.active = active
	s.running.Store(active != nil)
	stats, err := s.deps.Focus.TodayStats()
	if err != nil {
		return err
	}
	s.focusStats = focusStats{
		completed: stats.CompletedFocus,
		minutes:   stats.FocusMinutes,
		breaks:    stats.Interruptions,
		planned:   stats.PlannedMinutes,
	}
	recent, err := s.deps.Focus.ListRecent(8)
	if err != nil {
		return err
	}
	s.focusRecent = recent
	s.loadSchedules()
	return nil
}

func (s *State) loadLedger() error {
	month := s.ledgerMonth
	entries, err := s.deps.Ledger.List(lapp.MonthFilter(month))
	if err != nil {
		return err
	}
	s.entries = entries
	summary, err := s.deps.Ledger.Summarise(month)
	if err != nil {
		return err
	}
	s.entrySummary = summary
	balances, err := s.deps.Ledger.Balances(true)
	if err != nil {
		return err
	}
	s.balances = balances
	accounts, err := s.deps.Ledger.Accounts(true)
	if err != nil {
		return err
	}
	s.accounts = accounts
	categories, err := s.deps.Ledger.Categories()
	if err != nil {
		return err
	}
	s.categories = categories
	budgets, err := s.deps.Ledger.Budgets(month)
	if err != nil {
		return err
	}
	s.budgets = budgets
	spending, err := s.deps.Ledger.CategorySpending(month)
	if err != nil {
		return err
	}
	s.spending = spending
	rows, err := s.deps.Ledger.BudgetRows(month)
	if err != nil {
		return err
	}
	s.budgetRows = rows
	s.syncDrafts()
	return nil
}

func (s *State) loadStats() error {
	if err := s.loadLedger(); err != nil {
		return err
	}
	from, to, ok := s.statsWindow()
	s.statsValid = ok
	if !ok {
		s.statsBars = nil
		s.statsTotals = focusStats{}
		return nil
	}
	days := int(to.DaysSince(from)) + 1
	bars := make([]monthBar, 0, days)
	totals := focusStats{}
	for day := from; !day.After(to); day = day.AddDays(1) {
		stats, err := s.deps.Focus.DayStats(day)
		if err != nil {
			return err
		}
		bars = append(bars, monthBar{day: day, pomodoros: stats.CompletedFocus})
		totals.completed += stats.CompletedFocus
		totals.minutes += stats.FocusMinutes
		totals.breaks += stats.Interruptions
		totals.planned += stats.PlannedMinutes
	}
	s.statsBars = bars
	s.statsTotals = totals
	s.statsLoadedFrom, s.statsLoadedTo = from, to
	return nil
}

// statsWindow is the span the statistics page counts, and whether it can be
// counted: a custom range needs two dates, in order, spanning at most a year.
func (s *State) statsWindow() (shared.LocalDate, shared.LocalDate, bool) {
	today := s.today()
	switch s.statsRange {
	case statsRangeWeek:
		return today.AddDays(-(statsRangeWeek - 1)), today, true
	case statsRangeFortnight:
		return today.AddDays(-(statsRangeFortnight - 1)), today, true
	case statsRangeMonth:
		return today.AddDays(-(statsRangeMonth - 1)), today, true
	}
	from, err := format.ParseDate(s.lang, s.statsFromText)
	if err != nil || from.IsZero() {
		return today, today, false
	}
	to, err := format.ParseDate(s.lang, s.statsToText)
	if err != nil || to.IsZero() || to.Before(from) {
		return today, today, false
	}
	if to.After(from.AddDays(statsRangeMaxDays - 1)) {
		return today, today, false
	}
	return from, to, true
}

// statsRangeChoices are the windows the statistics page offers, in the order of
// its tabs: the three usual spans, then the two dates the user picks.
var statsRangeChoices = []int{statsRangeWeek, statsRangeFortnight, statsRangeMonth, statsRangeCustom}

// loadGoals reads the countdown goals, nearest deadline first.
func (s *State) loadGoals() error {
	goals, err := s.deps.Goals.List()
	if err != nil {
		return err
	}
	s.goals = goals
	return nil
}

func (s *State) loadTemplates() error {
	templates, err := s.deps.Templates.List()
	if err != nil {
		return err
	}
	s.templates = templates
	logs, err := s.deps.Templates.RecentLogs(10)
	if err != nil {
		return err
	}
	s.templateLogs = logs
	return nil
}

func (s *State) loadExport() error {
	return s.buildPreview()
}

// syncDrafts keeps the selections of the ledger forms pointing at rows that still
// exist, so a deleted account or category cannot leave a form that cannot be saved.
func (s *State) syncDrafts() {
	if s.entryDraft != nil {
		draft := s.entryDraft
		if draft.account.IsZero() || !s.accountExists(draft.account) {
			draft.account = ""
			if len(s.accounts) > 0 {
				draft.account = s.accounts[0].ID
			}
		}
		// A destination account belongs to a transfer only: a leftover id on an
		// expense line is refused by the domain, so it is cleared instead of faked.
		if draft.kind != ledger.EntryTransfer {
			draft.toAccount = ""
		} else if draft.toAccount.IsZero() || !s.accountExists(draft.toAccount) {
			draft.toAccount = draft.account
		}
		if draft.category.IsZero() || !s.categoryExists(draft.category, draft.kind) {
			draft.category = s.firstCategoryFor(draft.kind)
		}
	}
	if s.budgetDraft != nil && !s.budgetDraft.category.IsZero() &&
		!s.categoryExists(s.budgetDraft.category, ledger.EntryExpense) {
		s.budgetDraft.category = ""
	}
}

// accountExists reports whether an account with this id is still in the list.
func (s *State) accountExists(id shared.ID) bool {
	for _, account := range s.accounts {
		if account.ID == id {
			return !account.Archived
		}
	}
	return false
}

// categoryExists reports whether the category is still there and on the right side.
func (s *State) categoryExists(id shared.ID, kind ledger.EntryKind) bool {
	for _, category := range s.categories {
		if category.ID == id {
			return s.matchesDirection(category, kind)
		}
	}
	return false
}

// firstCategoryFor is the default category of a direction: the first one listed.
func (s *State) firstCategoryFor(kind ledger.EntryKind) shared.ID {
	for _, category := range s.categories {
		if s.matchesDirection(category, kind) {
			return category.ID
		}
	}
	return ""
}
