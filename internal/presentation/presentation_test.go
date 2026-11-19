package presentation

import (
	"testing"
	"time"

	"github.com/vivaui/viva/app"
	"github.com/vivaui/viva/gfx"
	"github.com/vivaui/viva/headless"
	"github.com/vivaui/viva/ui"

	dashboardapp "github.com/iotus3/simplelife/app/internal/application/dashboard"
	exportapp "github.com/iotus3/simplelife/app/internal/application/export"
	focusapp "github.com/iotus3/simplelife/app/internal/application/focus"
	goalapp "github.com/iotus3/simplelife/app/internal/application/goal"
	ingestapp "github.com/iotus3/simplelife/app/internal/application/ingest"
	ledgerapp "github.com/iotus3/simplelife/app/internal/application/ledger"
	settingsapp "github.com/iotus3/simplelife/app/internal/application/settings"
	taskapp "github.com/iotus3/simplelife/app/internal/application/task"
	templateapp "github.com/iotus3/simplelife/app/internal/application/template"
	"github.com/iotus3/simplelife/app/internal/apptest/harness"
	"github.com/iotus3/simplelife/app/internal/apptest/store"
	"github.com/iotus3/simplelife/app/internal/domain/shared"
	exportinfra "github.com/iotus3/simplelife/app/internal/infrastructure/export"
	"github.com/iotus3/simplelife/app/internal/presentation/i18n"
)

// testFixture is a whole application over the in-memory store: the interface is
// tested through the real services, never through fakes of them.
type testFixture struct {
	deps  Deps
	state *State
	store *store.Store
	clock *harness.Clock
	// system is the desktop's own light or dark look, which the appearance
	// preference "follow the system" reads.
	system *fakeAppearance
}

// fakeAppearance stands in for the desktop: a test sets Dark and paints, which is
// what the real window does when the system switches.
type fakeAppearance struct {
	Dark bool
	// Unknown makes the fake answer "this platform cannot tell".
	Unknown bool
}

func (f *fakeAppearance) read() app.Appearance {
	switch {
	case f.Unknown:
		return app.AppearanceUnknown
	case f.Dark:
		return app.AppearanceDark
	}
	return app.AppearanceLight
}

func newFixture(t *testing.T) *testFixture {
	t.Helper()
	memory := store.New()
	clock := harness.NewClock(harness.At("2026-11-17 09:00:00"))
	ids := harness.NewIDs()

	settings := settingsapp.New(memory.Settings())
	tasks := taskapp.New(memory.Tasks(), memory, clock, ids)
	ledger := ledgerapp.New(memory.Entries(), memory.Accounts(), memory.Categories(), memory.Budgets(), memory, clock, ids)
	templates := templateapp.New(memory.Templates(), memory.Tasks(), memory, clock, ids)
	goals := goalapp.New(memory.Goals(), memory, clock, ids)
	focus := focusapp.New(
		memory.Sessions(), memory.Tasks(), memory.Entries(), ledger,
		memory, clock, ids, settings, shared.NoopNotifier{}, shared.NoopSleepGuard{},
	)
	board := dashboardapp.New(tasks, focus, ledger, clock)
	importer := ingestapp.New(
		memory.Tasks(), memory.Entries(), memory.Accounts(), memory.Categories(),
		memory.Budgets(), memory.Templates(), clock, ids,
	)
	exporter := exportapp.New(
		memory.Tasks(), memory.Sessions(), memory.Entries(), memory.Accounts(),
		memory.Categories(), memory.Budgets(), memory.Templates(), clock,
	)

	system := &fakeAppearance{}
	deps := Deps{
		Tasks:            tasks,
		Focus:            focus,
		Ledger:           ledger,
		Templates:        templates,
		Goals:            goals,
		Settings:         settings,
		Export:           exporter,
		Import:           importer,
		Decode:           exportinfra.Decode,
		Dashboard:        board,
		Clock:            clock,
		SystemLanguages:  nil,
		SystemAppearance: system.read,
		Version:          "test",
		DataDir:          "/tmp/simplelife",
		Encode:           exportinfra.Encode,
	}
	return &testFixture{deps: deps, state: NewState(deps), store: memory, clock: clock, system: system}
}

// openWindow drives the real page through Viva's headless driver. A machine
// without the renderer skips the test rather than failing it.
func (f *testFixture) openWindow(t *testing.T) *headless.Session {
	t.Helper()
	return f.openWindowSize(t, appConfig())
}

// openWindowSize opens the window at a given size, for a test that needs a control
// the default window would not paint.
func (f *testFixture) openWindowSize(t *testing.T, config app.Config) *headless.Session {
	t.Helper()
	page := ui.NewPage(ui.PageConfig{View: f.state.View, Theme: f.state.themeSlot})
	var canvas gfx.Canvas
	session, err := headless.Open(page.Config(config), &canvas, headless.Options{Scale: 1})
	if err != nil {
		t.Skipf("headless driver unavailable: %v", err)
	}
	t.Cleanup(session.Close)
	if err := session.Frame(); err != nil {
		t.Skipf("headless paint unavailable: %v", err)
	}
	return session
}

// settle paints until the page stops changing, which is what runs the loads that
// were queued after the first frame.
func settle(t *testing.T, session *headless.Session) {
	t.Helper()
	for i := 0; i < 6; i++ {
		if err := session.Frame(); err != nil {
			t.Fatalf("frame: %v", err)
		}
	}
}

func TestEveryPageRenders(t *testing.T) {
	fixture := newFixture(t)
	session := fixture.openWindow(t)
	for _, route := range routes {
		fixture.state.route = route
		fixture.state.markAll()
		settle(t, session)
		title := i18n.T(fixture.state.lang, route.message())
		if !session.HasText(title) {
			t.Errorf("route %v does not show its title %q; texts: %v", route, title, session.Texts())
		}
	}
}

func TestLanguageSwitchChangesTheWholeWindow(t *testing.T) {
	fixture := newFixture(t)
	session := fixture.openWindow(t)
	fixture.state.route = RouteTasks
	settle(t, session)
	if !session.HasText(i18n.T(i18n.ZhCN, i18n.MsgNavHome)) {
		t.Fatalf("the window is not in Chinese: %v", session.Texts())
	}
	fixture.state.lang = i18n.En
	fixture.state.langPref = i18n.PrefEn
	fixture.state.markAll()
	settle(t, session)
	if session.HasText("今日") {
		t.Errorf("Chinese text survived the switch: %v", session.Texts())
	}
	if !session.HasText(i18n.T(i18n.En, i18n.MsgNavTasks)) {
		t.Errorf("English navigation is missing: %v", session.Texts())
	}
}

func TestAddingATaskThroughTheServiceShowsUp(t *testing.T) {
	fixture := newFixture(t)
	session := fixture.openWindow(t)
	fixture.state.route = RouteTasks
	settle(t, session)
	if _, err := fixture.deps.Tasks.Create(taskapp.CreateInput{Title: "写设计文档", DueDate: fixture.state.today()}); err != nil {
		t.Fatalf("create: %v", err)
	}
	fixture.state.markAll()
	settle(t, session)
	if !session.HasText("写设计文档") {
		t.Errorf("the new task is not on the page: %v", session.Texts())
	}
}

func TestFocusActionsReachTheStore(t *testing.T) {
	fixture := newFixture(t)
	session := fixture.openWindow(t)
	fixture.state.route = RouteFocus
	settle(t, session)

	if err := session.ClickLabel(i18n.T(fixture.state.lang, i18n.MsgFocusActionStart)); err != nil {
		t.Fatalf("click start: %v", err)
	}
	settle(t, session)
	active, err := fixture.deps.Focus.Active()
	if err != nil {
		t.Fatalf("active: %v", err)
	}
	if active == nil {
		t.Fatal("clicking start did not begin a session")
	}
	if active.Planned <= 0 {
		t.Errorf("the session has no planned length: %v", active.Planned)
	}

	// The clock moves under the session; the ring follows it.
	fixture.clock.Advance(60 * time.Second)
	settle(t, session)

	if err := session.ClickLabel(i18n.T(fixture.state.lang, i18n.MsgFocusActionCancel)); err != nil {
		t.Fatalf("click cancel: %v", err)
	}
	settle(t, session)
	active, err = fixture.deps.Focus.Active()
	if err != nil {
		t.Fatalf("active after cancel: %v", err)
	}
	if active != nil {
		t.Errorf("the session is still active: %+v", active)
	}
}

func TestTaskCheckboxCompletesTheTask(t *testing.T) {
	fixture := newFixture(t)
	session := fixture.openWindow(t)
	fixture.state.route = RouteTasks
	created, err := fixture.deps.Tasks.Create(taskapp.CreateInput{Title: "Review", DueDate: fixture.state.today()})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	fixture.state.markAll()
	settle(t, session)

	// The row's checkbox has no name of its own: it is found by role and clicked
	// where it was drawn, as a user would.
	var box gfx.Rect
	found := false
	for _, node := range session.Access() {
		if node.Role == gfx.RoleCheckbox {
			box = node.Bounds
			found = true
			break
		}
	}
	if !found {
		t.Skip("the frame reports no checkbox")
	}
	if err := session.Click(box.X+box.Width/2, box.Y+box.Height/2); err != nil {
		t.Fatalf("click checkbox: %v", err)
	}
	settle(t, session)
	value, err := fixture.deps.Tasks.Get(created.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if value.Status.IsOpen() {
		t.Errorf("the task is still open after ticking it: %v", value.Status)
	}
}

func TestExportPlanFollowsTheSelection(t *testing.T) {
	fixture := newFixture(t)
	state := fixture.state
	state.resetExportDraft()
	state.exportDraft.tables["entries"] = true
	state.exportDraft.currency = string(shared.USD)
	plan, err := state.plan()
	if err != nil {
		t.Fatalf("plan: %v", err)
	}
	if plan.Currency != shared.USD {
		t.Errorf("currency filter = %q, want USD", plan.Currency)
	}
	if len(plan.Tables) != 2 {
		t.Fatalf("tables = %v, want tasks and entries", plan.Tables)
	}
	// The identifier columns are always exported, even with every box unticked.
	for _, table := range plan.Tables {
		if len(table.Columns) < 2 {
			t.Errorf("table %q exported %v, want at least its identity columns", table.Key, table.Columns)
		}
	}
	state.exportDraft.tables["tasks"] = false
	state.exportDraft.tables["entries"] = false
	if _, err := state.plan(); err == nil {
		t.Error("an empty selection was accepted")
	}
}

func TestLocalizedExportHeaders(t *testing.T) {
	fixture := newFixture(t)
	state := fixture.state
	if got, want := state.Sheet("tasks"), i18n.T(i18n.ZhCN, i18n.MsgExportSheetTasks); got != want {
		t.Errorf("Sheet(tasks) = %q, want %q", got, want)
	}
	if got, want := state.Column("tasks", "title"), i18n.T(i18n.ZhCN, i18n.MsgExportTasksTitle); got != want {
		t.Errorf("Column(tasks.title) = %q, want %q", got, want)
	}
	state.lang = i18n.En
	if got, want := state.Column("entries", "amount"), "Amount"; got != want {
		t.Errorf("Column(entries.amount) in English = %q, want %q", got, want)
	}
	// An unknown column keeps its neutral key rather than vanishing.
	if got, want := state.Column("entries", "unknown_column"), "unknown_column"; got != want {
		t.Errorf("Column(unknown) = %q, want %q", got, want)
	}
}

func TestFormParsingRefusesNonNumbers(t *testing.T) {
	if _, err := parseRequiredInt("  "); err == nil {
		t.Error("an empty number was accepted")
	}
	if _, err := parseRequiredInt("abc"); err == nil {
		t.Error("text was accepted as a number")
	}
	if value, err := parseRequiredInt(" 25 "); err != nil || value != 25 {
		t.Errorf("parseRequiredInt(25) = %v, %v", value, err)
	}
	if value, err := parseOptionalInt(""); err != nil || value != 0 {
		t.Errorf("parseOptionalInt(empty) = %v, %v", value, err)
	}
}

func TestSaveTaskRejectsBadInput(t *testing.T) {
	fixture := newFixture(t)
	state := fixture.state
	state.resetTaskDraft()
	state.taskDraft.title = ""
	if err := state.saveTask(); err == nil {
		t.Error("an empty title was accepted")
	}
	state.taskDraft.title = "Valid"
	state.taskDraft.due = "31/31/31"
	if err := state.saveTask(); err == nil {
		t.Error("a nonsense due date was accepted")
	}
	state.taskDraft.due = ""
	state.taskDraft.estimate = "999"
	if err := state.saveTask(); err == nil {
		t.Error("an out of range estimate was accepted")
	}
	state.taskDraft.estimate = "2"
	if err := state.saveTask(); err != nil {
		t.Errorf("saveTask: %v", err)
	}
}

func TestClockOfRendersTheTimer(t *testing.T) {
	fixture := newFixture(t)
	now := fixture.state.now()
	session, err := fixture.deps.Focus.Start(focusapp.StartInput{Kind: 1, Planned: 25 * time.Minute})
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	if got, want := clockOf(session, now), "25:00"; got != want {
		t.Errorf("clockOf = %q, want %q", got, want)
	}
	if got, want := clockOf(session, now.Add(10*time.Minute)), "15:00"; got != want {
		t.Errorf("clockOf(10m later) = %q, want %q", got, want)
	}
	if got, want := clockOf(nil, now), "00:00"; got != want {
		t.Errorf("clockOf(nil) = %q, want %q", got, want)
	}
}
