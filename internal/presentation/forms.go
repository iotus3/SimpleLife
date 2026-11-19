package presentation

import (
	"strconv"
	"strings"
	"time"

	goalapp "github.com/iotus3/simplelife/app/internal/application/goal"
	lapp "github.com/iotus3/simplelife/app/internal/application/ledger"
	tapp "github.com/iotus3/simplelife/app/internal/application/task"
	tplapp "github.com/iotus3/simplelife/app/internal/application/template"
	"github.com/iotus3/simplelife/app/internal/domain/export"
	"github.com/iotus3/simplelife/app/internal/domain/focus"
	"github.com/iotus3/simplelife/app/internal/domain/ledger"
	"github.com/iotus3/simplelife/app/internal/domain/shared"
	"github.com/iotus3/simplelife/app/internal/domain/template"
	"github.com/iotus3/simplelife/app/internal/presentation/format"
	"github.com/iotus3/simplelife/app/internal/presentation/i18n"
)

// Every form binds its inputs to fields of a struct kept in State: a page is
// declared again on each paint, so a value kept in a local would be gone before
// the user presses Enter.

type taskDraft struct {
	open     bool
	id       shared.ID // empty = create
	parent   shared.ID // set when the draft is a subtask
	title    string
	notes    string
	due      string
	tags     string
	priority int
	estimate string
	// pomodoro is the length of one session for this task, in minutes, kept as text
	// like every other number in a form. Empty means "the focus page's length".
	pomodoro string
}

// newTaskDraft is the draft of a new task. Its day is today: the list a task is
// filed under is its due date, and a new task without one would be filed nowhere on
// this page. Clearing the field still makes it undated.
func (s *State) newTaskDraft(parent shared.ID) *taskDraft {
	return &taskDraft{
		priority: 0,
		estimate: "1",
		due:      format.Date(s.lang, s.today()),
		parent:   parent,
	}
}

func (s *State) resetTaskDraft() {
	s.taskDraft = s.newTaskDraft("")
}

func (s *State) openTaskEditor(id shared.ID) {
	draft := s.newTaskDraft("")
	if !id.IsZero() {
		// An existing task keeps the day it has, which may be none.
		draft.due = ""
		value, err := s.deps.Tasks.Get(id)
		if err != nil {
			s.toastError(err)
			s.resetTaskDraft()
			return
		}
		draft.id = value.ID
		draft.title = value.Title
		draft.notes = value.Notes
		draft.priority = int(value.Priority)
		draft.tags = strings.Join(value.Tags, ", ")
		if value.EstimatePomodoros > 0 {
			draft.estimate = strconv.Itoa(value.EstimatePomodoros)
		}
		if value.FocusMinutes > 0 {
			draft.pomodoro = strconv.Itoa(value.FocusMinutes)
		}
		if value.HasDueDate() {
			draft.due = format.Date(s.lang, value.DueDate())
		}
	}
	draft.open = true
	s.taskDraft = draft
}

// saveTask validates nothing of its own: the domain decides what a valid task is,
// and its refusal becomes a localized sentence.
func (s *State) saveTask() error {
	draft := s.taskDraft
	due, err := format.ParseDate(s.lang, draft.due)
	if err != nil {
		return err
	}
	estimate, err := parseOptionalInt(draft.estimate)
	if err != nil {
		return err
	}
	pomodoro, err := parseOptionalInt(draft.pomodoro)
	if err != nil {
		return err
	}
	if draft.id.IsZero() {
		_, err = s.deps.Tasks.Create(tapp.CreateInput{
			Title:        draft.title,
			Notes:        draft.notes,
			Priority:     draft.priority,
			DueDate:      due,
			Estimate:     estimate,
			FocusMinutes: pomodoro,
			Tags:         format.SplitTags(draft.tags),
			ParentID:     draft.parent,
		})
		return err
	}
	_, err = s.deps.Tasks.Update(tapp.UpdateInput{
		ID:           draft.id,
		Title:        draft.title,
		Notes:        draft.notes,
		Priority:     draft.priority,
		DueDate:      due,
		Estimate:     estimate,
		FocusMinutes: pomodoro,
		Tags:         format.SplitTags(draft.tags),
	})
	return err
}

type entryDraft struct {
	open      bool
	id        shared.ID
	kind      ledger.EntryKind
	amount    string
	account   shared.ID
	toAccount shared.ID
	category  shared.ID
	date      string
	note      string
}

func (s *State) resetEntryDraft() {
	s.entryDraft = &entryDraft{kind: ledger.EntryExpense}
	s.syncDrafts()
}

func (s *State) openEntryEditor(id shared.ID) {
	draft := &entryDraft{kind: ledger.EntryExpense}
	if !id.IsZero() {
		for _, entry := range s.entries {
			if entry.ID != id {
				continue
			}
			draft.id = entry.ID
			draft.kind = entry.Kind
			draft.amount = entry.Amount.String()
			draft.account = entry.AccountID
			draft.toAccount = entry.ToAccountID
			draft.category = entry.CategoryID
			draft.date = format.Date(s.lang, shared.DateOf(entry.OccurredAt))
			draft.note = entry.Note
			break
		}
	}
	draft.open = true
	s.entryDraft = draft
	s.syncDrafts()
}

func (s *State) saveEntry() error {
	draft := s.entryDraft
	currency := s.currencyOf(draft.account)
	amount, err := shared.ParseMoney(draft.amount, currency)
	if err != nil {
		return err
	}
	occurred, err := format.ParseDate(s.lang, draft.date)
	if err != nil {
		return err
	}
	when := s.now()
	if !occurred.IsZero() {
		when = occurred.At(12, 0)
	}
	input := lapp.EntryInput{
		Kind:        draft.kind,
		Amount:      amount,
		AccountID:   draft.account,
		ToAccountID: draft.toAccount,
		CategoryID:  draft.category,
		OccurredAt:  when,
		Note:        draft.note,
	}
	if draft.kind == ledger.EntryTransfer {
		input.CategoryID = ""
	}
	if draft.id.IsZero() {
		_, err = s.deps.Ledger.CreateEntry(input)
		return err
	}
	_, err = s.deps.Ledger.UpdateEntry(draft.id, input)
	return err
}

// currencyOf is the currency of the account a form points at, which decides how
// the typed amount is read: 12.34 is ¥12.34 or $12.34 according to it.
func (s *State) currencyOf(id shared.ID) shared.Currency {
	for _, account := range s.accounts {
		if account.ID == id {
			return account.Currency()
		}
	}
	return s.deps.Settings.DefaultCurrency()
}

// accountDraft is the account editor. id empty means "create"; an existing account
// keeps its currency and opening balance, so those two fields are only read there.
type accountDraft struct {
	open    bool
	id      shared.ID // empty = create
	name    string
	builtin bool
	kind    ledger.AccountKind
	// currency is the account's currency: chosen for a new one, shown for an old one.
	currency shared.Currency
	initial  string
}

func (s *State) resetAccountDraft() {
	s.accountDraft = &accountDraft{
		kind:     ledger.AccountCash,
		currency: s.deps.Settings.DefaultCurrency(),
		initial:  "0",
	}
}

// openAccountEditor fills the draft from an existing account, or starts a new one
// when id is empty.
func (s *State) openAccountEditor(id shared.ID) {
	draft := &accountDraft{kind: ledger.AccountCash, currency: s.deps.Settings.DefaultCurrency()}
	if !id.IsZero() {
		for _, account := range s.accounts {
			if account.ID != id {
				continue
			}
			draft.id = account.ID
			draft.name = i18n.DisplayName(s.lang, account.Name, account.BuiltinKey)
			draft.builtin = account.BuiltinKey != ""
			draft.kind = account.Kind
			draft.currency = account.Currency()
			break
		}
		if draft.id.IsZero() {
			s.toastError(shared.NotFound("ledger.account.not_found", "account not found"))
			return
		}
	}
	draft.open = true
	s.accountDraft = draft
}

func (s *State) saveAccount() error {
	draft := s.accountDraft
	if !draft.id.IsZero() {
		_, err := s.deps.Ledger.UpdateAccount(draft.id, draft.name, draft.kind)
		return err
	}
	initial, err := shared.ParseMoney(draft.initial, draft.currency)
	if err != nil {
		return err
	}
	_, err = s.deps.Ledger.CreateAccount(draft.name, draft.kind, initial)
	return err
}

// budgetDraft is the budget editor. editing marks a ceiling that already exists: it
// keeps its category and only its limit can change.
type budgetDraft struct {
	open     bool
	editing  bool
	category shared.ID
	limit    string
}

// categoryDraft is the category editor: one place to create a category, rename it
// and pick which side of the ledger it belongs to. It is kept as text and a
// direction so a half-typed name cannot reach the database.
type categoryDraft struct {
	open      bool
	id        shared.ID // empty = create
	name      string
	direction ledger.CategoryDirection
	builtin   bool
}

func (s *State) resetCategoryDraft() {
	s.categoryDraft = &categoryDraft{direction: ledger.CategoryExpense}
}

// openCategoryEditor edits an existing category, or starts a new one of the given
// direction when id is empty.
func (s *State) openCategoryEditor(id shared.ID, direction ledger.CategoryDirection) {
	draft := &categoryDraft{direction: direction}
	if direction != ledger.CategoryIncome {
		draft.direction = ledger.CategoryExpense
	}
	if !id.IsZero() {
		for _, category := range s.categories {
			if category.ID != id {
				continue
			}
			draft.id = category.ID
			draft.name = category.Name
			draft.direction = category.Direction
			draft.builtin = category.BuiltinKey != ""
			break
		}
	}
	draft.open = true
	s.categoryDraft = draft
}

func (s *State) saveCategory() error {
	draft := s.categoryDraft
	if draft.id.IsZero() {
		_, err := s.deps.Ledger.CreateCategory(draft.name, draft.direction, "")
		return err
	}
	_, err := s.deps.Ledger.RenameCategory(draft.id, draft.name)
	return err
}

// resetBudgetDraft starts an empty draft. The limit is left blank rather than
// pre-filled with a zero: a zero is refused, so a prefilled field would fail on save
// unless the user cleared it first.
func (s *State) resetBudgetDraft() {
	s.budgetDraft = &budgetDraft{}
}

// openBudgetEditor fills the draft for one category: with id set it edits that
// ceiling, otherwise it creates one for the category the row belongs to (or for the
// first category that has none, when the dialog is opened from the header).
func (s *State) openBudgetEditor(id shared.ID, limit shared.Money, category shared.ID) {
	draft := &budgetDraft{category: category}
	if !id.IsZero() {
		draft.editing = true
		draft.limit = limit.String()
	} else if draft.category.IsZero() {
		// Opened from the header, so nothing is preselected: starting on the first
		// category without a ceiling means "new budget, save" does what it looks like
		// it does instead of refusing an empty category.
		draft.category = s.firstBudgetableCategory()
	}
	draft.open = true
	s.budgetDraft = draft
}

// firstBudgetableCategory is the first expense category of the month that has no
// ceiling yet.
func (s *State) firstBudgetableCategory() shared.ID {
	for _, category := range s.categories {
		if category.Direction != ledger.CategoryExpense || s.budgetOf(category.ID) != nil {
			continue
		}
		return category.ID
	}
	return ""
}

func (s *State) saveBudget() error {
	draft := s.budgetDraft
	currency := s.deps.Settings.DefaultCurrency()
	if existing := s.budgetOf(draft.category); existing != nil {
		// An edited ceiling keeps the currency it was created in: the limit is a sum
		// of money, and re-reading it in the default currency would change what it
		// means the day the default changes.
		currency = existing.Limit.Currency()
	}
	limit, err := shared.ParseMoney(draft.limit, currency)
	if err != nil {
		return err
	}
	_, err = s.deps.Ledger.UpsertBudget(draft.category, s.ledgerMonth, limit)
	return err
}

// goalDraft is the goal editor. Its two days are kept as text, like every other
// date field, so a half-typed date stays in the dialog instead of becoming a goal.
type goalDraft struct {
	open  bool
	id    shared.ID // empty = create
	name  string
	note  string
	start string
	end   string
}

// goalDefaultDays is the span a new goal starts with: a month is long enough to be
// a plan and short enough that the deadline is worth picking.
const goalDefaultDays = 30

// resetGoalDraft opens an empty goal: it starts today and ends a month later, so a
// user who only types a name gets a working countdown.
func (s *State) resetGoalDraft() {
	today := s.today()
	s.goalDraft = &goalDraft{
		start: format.Date(s.lang, today),
		end:   format.Date(s.lang, today.AddDays(goalDefaultDays)),
	}
}

// openGoalEditor edits an existing goal, or starts a new one when id is empty.
func (s *State) openGoalEditor(id shared.ID) {
	s.resetGoalDraft()
	draft := s.goalDraft
	if !id.IsZero() {
		for _, value := range s.goals {
			if value.ID != id {
				continue
			}
			draft.id = value.ID
			draft.name = value.Name
			draft.note = value.Note
			draft.start = format.Date(s.lang, value.Start)
			draft.end = format.Date(s.lang, value.End)
			break
		}
	}
	draft.open = true
}

// saveGoal writes the draft through the application service. The dates are parsed
// here because text is what the fields hold; everything else is the domain's.
func (s *State) saveGoal() error {
	draft := s.goalDraft
	start, err := format.ParseDate(s.lang, draft.start)
	if err != nil || start.IsZero() {
		return shared.Invalid("goal.date.empty", "date", "the start day is required")
	}
	end, err := format.ParseDate(s.lang, draft.end)
	if err != nil || end.IsZero() {
		return shared.Invalid("goal.date.empty", "date", "the end day is required")
	}
	input := goalapp.Input{Name: draft.name, Note: draft.note, Start: start, End: end}
	if draft.id.IsZero() {
		_, err = s.deps.Goals.Create(input)
		return err
	}
	_, err = s.deps.Goals.Update(draft.id, input)
	return err
}

type templateDraft struct {
	open       bool
	id         shared.ID
	name       string
	enabled    bool
	kind       template.TriggerKind
	weekdays   [7]bool
	dayOfMonth string
	everyN     string
	dates      string
	// The one todo the template writes. Kept as text so a half-typed number does
	// not fight the keyboard, the same way the timer form does.
	taskTitle    string
	taskNotes    string
	taskOffset   string
	taskEstimate string
	// taskFocus is the length of one pomodoro the generated tasks will carry, kept as text like
	// every other number in a form. Empty means "no length of its own".
	taskFocus    string
	taskPriority int
	taskTags     string
}

func (s *State) resetTemplateDraft() {
	s.templateDraft = &templateDraft{
		enabled:      true,
		kind:         template.TriggerDaily,
		dayOfMonth:   "1",
		everyN:       "2",
		taskOffset:   "0",
		taskEstimate: "1",
	}
}

func (s *State) openTemplateEditor(id shared.ID) {
	draft := &templateDraft{
		enabled:      true,
		kind:         template.TriggerDaily,
		dayOfMonth:   "1",
		everyN:       "2",
		taskOffset:   "0",
		taskEstimate: "1",
	}
	if !id.IsZero() {
		value, err := s.deps.Templates.Get(id)
		if err != nil {
			s.toastError(err)
			s.resetTemplateDraft()
			return
		}
		draft.id = value.ID
		draft.name = value.Name
		draft.enabled = value.Enabled
		draft.kind = value.Rule.Kind
		for _, weekday := range value.Rule.Weekdays {
			draft.weekdays[int(weekday)%7] = true
		}
		if value.Rule.DayOfMonth > 0 {
			draft.dayOfMonth = strconv.Itoa(value.Rule.DayOfMonth)
		}
		if value.Rule.EveryNDays > 0 {
			draft.everyN = strconv.Itoa(value.Rule.EveryNDays)
		}
		if len(value.Rule.Dates) > 0 {
			parts := make([]string, 0, len(value.Rule.Dates))
			for _, day := range value.Rule.Dates {
				parts = append(parts, day.String())
			}
			draft.dates = strings.Join(parts, ", ")
		}
		draft.taskTitle = value.Task.Title
		draft.taskNotes = value.Task.Notes
		draft.taskOffset = strconv.Itoa(value.Task.DueOffsetDays)
		draft.taskEstimate = strconv.Itoa(value.Task.Estimate)
		// A template that carries no length writes plain tasks, and the field says
		// so by staying empty rather than by claiming a number nobody chose.
		draft.taskFocus = ""
		if value.Task.FocusMinutes > 0 {
			draft.taskFocus = strconv.Itoa(value.Task.FocusMinutes)
		}
		draft.taskPriority = value.Task.Priority
		draft.taskTags = strings.Join(value.Task.Tags, ", ")
	}
	draft.open = true
	s.templateDraft = draft
}

// saveTemplate turns the draft into a rule and validates it through the domain.
func (s *State) saveTemplate() error {
	draft := s.templateDraft
	rule := template.TriggerRule{Kind: draft.kind}
	switch draft.kind {
	case template.TriggerWeekdays:
		for index, on := range draft.weekdays {
			if on {
				rule.Weekdays = append(rule.Weekdays, time.Weekday(index))
			}
		}
	case template.TriggerMonthly:
		day, err := parseOptionalInt(draft.dayOfMonth)
		if err != nil {
			return err
		}
		rule.DayOfMonth = day
	case template.TriggerEveryNDays:
		every, err := parseOptionalInt(draft.everyN)
		if err != nil {
			return err
		}
		rule.EveryNDays = every
		rule.Anchor = s.today()
	case template.TriggerSpecificDates:
		parsed, err := format.ParseDateList(draft.dates)
		if err != nil {
			return err
		}
		rule.Dates = parsed
	}
	offset, err := parseOptionalInt(draft.taskOffset)
	if err != nil {
		return err
	}
	estimate, err := parseOptionalInt(draft.taskEstimate)
	if err != nil {
		return err
	}
	focusMinutes, err := parseOptionalInt(draft.taskFocus)
	if err != nil {
		return err
	}
	_, err = s.deps.Templates.Save(tplapp.SaveInput{
		ID:      draft.id,
		Name:    draft.name,
		Enabled: draft.enabled,
		Rule:    rule,
		Task: template.Task{
			Title:         draft.taskTitle,
			Notes:         draft.taskNotes,
			Priority:      draft.taskPriority,
			Estimate:      estimate,
			FocusMinutes:  focusMinutes,
			DueOffsetDays: offset,
			Tags:          format.SplitTags(draft.taskTags),
		},
	})
	return err
}

// timerForm holds the four lengths as text so a half-typed number does not fight
// the user's keyboard.
type timerForm struct {
	focus     string
	short     string
	long      string
	every     string
	autoBreak bool
	autoFocus bool
}

func (s *State) resetTimerForm() {
	settings := s.deps.Settings.Timer()
	s.timerForm = timerForm{
		focus:     strconv.Itoa(int(settings.FocusDuration / time.Minute)),
		short:     strconv.Itoa(int(settings.ShortBreak / time.Minute)),
		long:      strconv.Itoa(int(settings.LongBreak / time.Minute)),
		every:     strconv.Itoa(settings.LongBreakEvery),
		autoBreak: settings.AutoStartBreak,
		autoFocus: settings.AutoStartFocus,
	}
}

func (s *State) saveTimerForm() error {
	focusMinutes, err := parseRequiredInt(s.timerForm.focus)
	if err != nil {
		return err
	}
	short, err := parseRequiredInt(s.timerForm.short)
	if err != nil {
		return err
	}
	long, err := parseRequiredInt(s.timerForm.long)
	if err != nil {
		return err
	}
	every, err := parseRequiredInt(s.timerForm.every)
	if err != nil {
		return err
	}
	return s.deps.Settings.SetTimer(focus.TimerSettings{
		FocusDuration:  time.Duration(focusMinutes) * time.Minute,
		ShortBreak:     time.Duration(short) * time.Minute,
		LongBreak:      time.Duration(long) * time.Minute,
		LongBreakEvery: every,
		AutoStartBreak: s.timerForm.autoBreak,
		AutoStartFocus: s.timerForm.autoFocus,
	})
}

type economyForm struct {
	enabled         bool
	rate            string
	ratio           string
	minMinutes      string
	rewardCap       string
	penaltyCap      string
	rewardAccount   shared.ID
	rewardCategory  shared.ID
	penaltyCategory shared.ID
}

func (s *State) resetEconomyForm() {
	settings := s.deps.Settings.Economy()
	s.economyForm = economyForm{
		enabled:         settings.Enabled,
		rate:            settings.Rate.String(),
		ratio:           strconv.Itoa(settings.PenaltyRatioPermille / 10),
		minMinutes:      strconv.Itoa(settings.PenaltyMinMinutes),
		rewardCap:       settings.RewardCapDaily.String(),
		penaltyCap:      settings.PenaltyCapDaily.String(),
		rewardAccount:   settings.RewardAccountID,
		rewardCategory:  settings.RewardCategoryID,
		penaltyCategory: settings.PenaltyCategoryID,
	}
}

// fillEconomyDefaults points the economy at the built-in rows when the settings do
// not name any, so the form shows what will happen instead of an empty select, and
// enabling the economy does not fail on a missing account.
func (s *State) fillEconomyDefaults() {
	if s.economyForm.rewardAccount.IsZero() {
		s.economyForm.rewardAccount = s.builtinAccount(ledger.BuiltinAccountFocusPot)
	}
	if s.economyForm.rewardCategory.IsZero() {
		s.economyForm.rewardCategory = s.builtinCategory(ledger.BuiltinCategoryFocusAward)
	}
	if s.economyForm.penaltyCategory.IsZero() {
		s.economyForm.penaltyCategory = s.builtinCategory(ledger.BuiltinCategoryFocusFine)
	}
}

// builtinAccount finds a seeded account by its key.
func (s *State) builtinAccount(key string) shared.ID {
	for _, account := range s.accounts {
		if account.BuiltinKey == key {
			return account.ID
		}
	}
	return ""
}

// builtinCategory finds a seeded category by its key.
func (s *State) builtinCategory(key string) shared.ID {
	for _, category := range s.categories {
		if category.BuiltinKey == key {
			return category.ID
		}
	}
	return ""
}

func (s *State) saveEconomyForm() error {
	currency := s.deps.Settings.DefaultCurrency()
	if !s.economyForm.rewardAccount.IsZero() {
		currency = s.currencyOf(s.economyForm.rewardAccount)
	}
	rate, err := shared.ParseMoney(s.economyForm.rate, currency)
	if err != nil {
		return err
	}
	capReward, err := shared.ParseMoney(s.economyForm.rewardCap, currency)
	if err != nil {
		return err
	}
	capPenalty, err := shared.ParseMoney(s.economyForm.penaltyCap, currency)
	if err != nil {
		return err
	}
	ratio, err := parseRequiredInt(s.economyForm.ratio)
	if err != nil {
		return err
	}
	minMinutes, err := parseRequiredInt(s.economyForm.minMinutes)
	if err != nil {
		return err
	}
	return s.deps.Settings.SetEconomy(focus.EconomySettings{
		Enabled:              s.economyForm.enabled,
		Rate:                 rate,
		PenaltyRatioPermille: ratio * 10,
		PenaltyMinMinutes:    minMinutes,
		RewardCapDaily:       capReward,
		PenaltyCapDaily:      capPenalty,
		RewardAccountID:      s.economyForm.rewardAccount,
		RewardCategoryID:     s.economyForm.rewardCategory,
		PenaltyCategoryID:    s.economyForm.penaltyCategory,
	})
}

// exportDraft is the export wizard: which tables, which columns, which range and
// which layout.
type exportDraft struct {
	layout          export.Layout
	tables          map[string]bool
	columns         map[string]map[string]bool
	rangeMode       int // 0 all, 1 this month, 2 custom
	from            string
	to              string
	currency        string // empty = every currency
	includeArchived bool
	previewSheets   []previewSheet
	previewError    error
}

// previewSheet is one table of the file being prepared: its name, its rows and the first few for
// the card, so the preview can say which table is which.
type previewSheet struct {
	name    string
	headers []string
	rows    [][]string
	total   int
}

func (s *State) resetExportDraft() {
	draft := &exportDraft{
		layout:  export.LayoutXLSX,
		tables:  make(map[string]bool),
		columns: make(map[string]map[string]bool),
	}
	for _, table := range export.DefaultTables() {
		selected := make(map[string]bool, len(table.Columns))
		for _, column := range table.Columns {
			selected[column.Key] = column.Always
		}
		draft.columns[table.Key] = selected
	}
	draft.tables["tasks"] = true
	s.exportDraft = *draft
}

// plan turns the draft into a domain plan, which validates itself.
func (s *State) plan() (export.Plan, error) {
	draft := s.exportDraft
	plan := export.Plan{
		Layout:          draft.layout,
		IncludeArchived: draft.includeArchived,
	}
	if draft.currency != "" {
		plan.Currency = shared.Currency(draft.currency)
	}
	switch draft.rangeMode {
	case 1:
		plan.Range = shared.OfMonth(s.ledgerMonth)
	case 2:
		from, err := format.ParseDate(s.lang, draft.from)
		if err != nil {
			return plan, err
		}
		to, err := format.ParseDate(s.lang, draft.to)
		if err != nil {
			return plan, err
		}
		plan.Range = shared.DateRange{From: from, To: to}
	}
	for _, table := range export.DefaultTables() {
		if !draft.tables[table.Key] {
			continue
		}
		selection := export.TableSelection{Key: table.Key}
		for _, column := range table.Columns {
			if draft.columns[table.Key][column.Key] || column.Always {
				selection.Columns = append(selection.Columns, column.Key)
			}
		}
		plan.Tables = append(plan.Tables, selection)
	}
	if err := plan.Validate(); err != nil {
		return plan, err
	}
	return plan, nil
}

// previewRows is how many rows of each sheet the card shows before it stops: enough to
// recognise the table, few enough to keep every sheet on screen.
const previewRows = 5

// buildPreview materialises every sheet so the wizard shows what the file will
// contain, sheet by sheet, before it is written.
func (s *State) buildPreview() error {
	plan, err := s.plan()
	s.exportDraft.previewError = err
	s.exportDraft.previewSheets = nil
	if err != nil {
		return nil
	}
	sheets, err := s.deps.Export.Build(plan, s)
	if err != nil {
		s.exportDraft.previewError = err
		return nil
	}
	previews := make([]previewSheet, 0, len(sheets))
	for _, sheet := range sheets {
		rows := sheet.Rows
		if len(rows) > previewRows {
			rows = rows[:previewRows]
		}
		previews = append(previews, previewSheet{
			name:    sheet.Name,
			headers: sheet.Headers,
			rows:    rows,
			total:   len(sheet.Rows),
		})
	}
	s.exportDraft.previewSheets = previews
	return nil
}

// Sheet and Column implement application/export.Labels: the header row follows
// the interface language while every value stays neutral.
func (s *State) Sheet(table string) string {
	if id, ok := sheetNames[table]; ok {
		return i18n.T(s.lang, id)
	}
	return table
}

// Column is the localized header of one export column.
func (s *State) Column(table, column string) string {
	if byTable, ok := columnNames[table]; ok {
		if id, ok := byTable[column]; ok {
			return i18n.T(s.lang, id)
		}
	}
	return column
}

var sheetNames = map[string]i18n.MessageID{
	"tasks":      i18n.MsgExportSheetTasks,
	"sessions":   i18n.MsgExportSheetSessions,
	"entries":    i18n.MsgExportSheetEntries,
	"accounts":   i18n.MsgExportSheetAccounts,
	"categories": i18n.MsgExportSheetCategories,
	"budgets":    i18n.MsgExportSheetBudgets,
	"templates":  i18n.MsgExportSheetTemplates,
}

var columnNames = map[string]map[string]i18n.MessageID{
	"tasks": {
		"id": i18n.MsgExportTasksID, "title": i18n.MsgExportTasksTitle,
		"status": i18n.MsgExportTasksStatus, "priority": i18n.MsgExportTasksPriority,
		"due_date": i18n.MsgExportTasksDueDate, "estimate_pomodoros": i18n.MsgExportTasksEstimate,
		"focus_minutes":  i18n.MsgExportTasksPomodoro,
		"done_pomodoros": i18n.MsgExportTasksDone, "tags": i18n.MsgExportTasksTags,
		"notes": i18n.MsgExportTasksNotes, "created_at": i18n.MsgExportTasksCreated,
		"completed_at": i18n.MsgExportTasksCompleted, "template_id": i18n.MsgExportTasksTemplate,
	},
	"sessions": {
		"id": i18n.MsgExportSessionsID, "kind": i18n.MsgExportSessionsKind,
		"phase": i18n.MsgExportSessionsPhase, "title": i18n.MsgExportSessionsTitle,
		"started_at": i18n.MsgExportSessionsStarted, "ended_at": i18n.MsgExportSessionsEnded,
		"planned_minutes": i18n.MsgExportSessionsPlanned, "elapsed_minutes": i18n.MsgExportSessionsElapsed,
		"interruptions": i18n.MsgExportSessionsInterrupt, "note": i18n.MsgExportSessionsNote,
	},
	"entries": {
		"id": i18n.MsgExportEntriesID, "kind": i18n.MsgExportEntriesKind,
		"amount": i18n.MsgExportEntriesAmount, "currency": i18n.MsgExportEntriesCurrency,
		"account": i18n.MsgExportEntriesAccount, "to_account": i18n.MsgExportEntriesTo,
		"category": i18n.MsgExportEntriesCategory, "occurred_at": i18n.MsgExportEntriesOccurred,
		"origin": i18n.MsgExportEntriesOrigin, "note": i18n.MsgExportEntriesNote,
		"tags": i18n.MsgExportEntriesTags,
	},
	"accounts": {
		"name": i18n.MsgExportAccountsName, "kind": i18n.MsgExportAccountsKind,
		"currency": i18n.MsgExportAccountsCurrency, "balance": i18n.MsgExportAccountsBalance,
		"archived": i18n.MsgExportAccountsArchived,
	},
	"categories": {
		"name": i18n.MsgExportCategoriesName, "direction": i18n.MsgExportCategoriesDirection,
		"parent": i18n.MsgExportCategoriesParent,
	},
	"budgets": {
		"month": i18n.MsgExportBudgetsMonth, "category": i18n.MsgExportBudgetsCategory,
		"limit": i18n.MsgExportBudgetsLimit, "currency": i18n.MsgExportBudgetsCurrency,
	},
	"templates": {
		"name": i18n.MsgExportTemplatesName, "enabled": i18n.MsgExportTemplatesEnabled,
		"rule": i18n.MsgExportTemplatesRule, "task": i18n.MsgExportTemplatesTask,
		"notes": i18n.MsgExportTemplatesNotes, "estimate": i18n.MsgExportTemplatesEstimate,
		"focus_minutes": i18n.MsgExportTemplatesFocus,
		"due_offset":    i18n.MsgExportTemplatesOffset,
	},
}

func parseOptionalInt(text string) (int, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return 0, nil
	}
	value, err := strconv.Atoi(text)
	if err != nil {
		return 0, shared.Invalid("ui.number", "", "not a number: "+text)
	}
	return value, nil
}

func parseRequiredInt(text string) (int, error) {
	if strings.TrimSpace(text) == "" {
		return 0, shared.Invalid("ui.number", "", "a number is required")
	}
	return parseOptionalInt(text)
}
