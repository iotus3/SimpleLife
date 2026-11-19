// Package ingest reads an exported file back into the database: same tables, same column
// keys, same value formats. An unknown account name fails the row, it is never created.
package ingest

import (
	"strings"
	"time"

	"github.com/iotus3/simplelife/app/internal/domain/export"
	"github.com/iotus3/simplelife/app/internal/domain/ledger"
	"github.com/iotus3/simplelife/app/internal/domain/shared"
	"github.com/iotus3/simplelife/app/internal/domain/task"
	"github.com/iotus3/simplelife/app/internal/domain/template"
)

// Headers maps a column key to every header text the file may carry for it, because the
// exporter writes localized labels.
type Headers interface {
	Header(table, column string) []string
}

// Tables are the tables an import accepts, in dependency order. Accounts and focus
// sessions are absent: a balance is derived from entries, a session needs its task.
func Tables() []string {
	return []string{"categories", "tasks", "entries", "budgets", "templates"}
}

// Importable reports whether a table can be imported.
func Importable(table string) bool {
	for _, key := range Tables() {
		if key == table {
			return true
		}
	}
	return false
}

// Failure is one row that could not be imported, with the code the message layer
// translates.
type Failure struct {
	Row  int // 1-based row in the file, counting the header as row 1
	Code string
}

// Report is what one import did.
type Report struct {
	Table   string
	Created int
	Skipped int
	Failed  []Failure
}

// Service imports files into the database.
type Service struct {
	tasks      task.Repository
	entries    ledger.EntryRepository
	accounts   ledger.AccountRepository
	categories ledger.CategoryRepository
	budgets    ledger.BudgetRepository
	templates  template.Repository
	clock      shared.Clock
	ids        shared.IDGen
}

// New wires the service.
func New(
	tasks task.Repository,
	entries ledger.EntryRepository,
	accounts ledger.AccountRepository,
	categories ledger.CategoryRepository,
	budgets ledger.BudgetRepository,
	templates template.Repository,
	clock shared.Clock,
	ids shared.IDGen,
) *Service {
	return &Service{
		tasks: tasks, entries: entries, accounts: accounts, categories: categories,
		budgets: budgets, templates: templates, clock: clock, ids: ids,
	}
}

// Import reads one document as one table. A bad row is reported and the rest are
// imported; an id already in the database is skipped, so importing twice is safe.
func (s *Service) Import(table string, document export.Document, headers Headers) (Report, error) {
	report := Report{Table: table}
	if !Importable(table) {
		return report, shared.Invalid("import.table", "table", "this table cannot be imported: "+table)
	}
	columns, err := resolveColumns(table, document.Headers, headers)
	if err != nil {
		return report, err
	}
	now := s.clock.Now()
	for index, cells := range document.Rows {
		row := rowValues{columns: columns, cells: cells, file: document}
		created, err := s.importRow(table, row, now)
		switch {
		case err != nil:
			report.Failed = append(report.Failed, Failure{Row: index + 2, Code: shared.Code(err)})
		case created:
			report.Created++
		default:
			report.Skipped++
		}
	}
	return report, nil
}

// rowValues reads one row by column key, with the fallbacks the file needs: a
// missing cell is "", and a numeric cell keeps its literal text.
type rowValues struct {
	columns map[string]int
	cells   []string
	file    export.Document
}

func (r rowValues) get(key string) string {
	index, ok := r.columns[key]
	if !ok {
		return ""
	}
	if index < 0 || index >= len(r.cells) {
		return ""
	}
	return strings.TrimSpace(r.cells[index])
}

func (r rowValues) has(key string) bool {
	_, ok := r.columns[key]
	return ok
}

// resolveColumns maps each file header to a column key, ignoring case and spacing and
// accepting either the localized label or the raw key.
func resolveColumns(table string, fileHeaders []string, headers Headers) (map[string]int, error) {
	definition, ok := tableDefinition(table)
	if !ok {
		return nil, shared.Invalid("import.table", "table", "unknown table: "+table)
	}
	candidates := make(map[string]string, len(definition.Columns)*2)
	for _, column := range definition.Columns {
		candidates[normalizeHeader(column.Key)] = column.Key
		if headers != nil {
			for _, label := range headers.Header(table, column.Key) {
				candidates[normalizeHeader(label)] = column.Key
			}
		}
	}
	columns := make(map[string]int, len(fileHeaders))
	for index, header := range fileHeaders {
		key, ok := candidates[normalizeHeader(header)]
		if !ok {
			continue
		}
		if _, seen := columns[key]; seen {
			continue // the first column with a name wins
		}
		columns[key] = index
	}
	if len(columns) == 0 {
		return nil, shared.Invalid("import.header", "file", "no column of this file belongs to the table "+table)
	}
	return columns, nil
}

// normalizeHeader folds a header so "Due date", "due_date" and "  DUE DATE "
// are the same column.
func normalizeHeader(text string) string {
	text = strings.ToLower(strings.TrimSpace(text))
	replacer := strings.NewReplacer(" ", "", "_", "", "-", "", "\u00a0", "")
	return replacer.Replace(text)
}

// tableDefinition finds the exported shape of a table, which is also its imported
// shape.
func tableDefinition(table string) (export.Table, bool) {
	for _, candidate := range export.DefaultTables() {
		if candidate.Key == table {
			return candidate, true
		}
	}
	return export.Table{}, false
}

// importRow dispatches on the table. It reports whether a row was created; a false
// with no error means the row was already there.
func (s *Service) importRow(table string, row rowValues, now time.Time) (bool, error) {
	switch table {
	case "tasks":
		return s.importTask(row, now)
	case "categories":
		return s.importCategory(row)
	case "entries":
		return s.importEntry(row, now)
	case "budgets":
		return s.importBudget(row, now)
	case "templates":
		return s.importTemplate(row, now)
	}
	return false, shared.Invalid("import.table", "table", "unknown table: "+table)
}

// importTask rebuilds one todo.
func (s *Service) importTask(row rowValues, now time.Time) (bool, error) {
	id, err := parseID(row.get("id"), "task")
	if err != nil {
		return false, err
	}
	exists, err := s.taskExists(id)
	if err != nil {
		return false, err
	}
	if exists {
		return false, nil
	}
	status := task.StatusTodo
	if text := row.get("status"); text != "" {
		if status, err = task.ParseStatus(text); err != nil {
			return false, err
		}
	}
	priority, err := parseInt(row, "priority", 0, int(task.PriorityMax))
	if err != nil {
		return false, err
	}
	estimate, err := parseInt(row, "estimate_pomodoros", 0, task.EstimatePomodorosMax)
	if err != nil {
		return false, err
	}
	focusMinutes, err := parseInt(row, "focus_minutes", 0, task.FocusMinutesMax)
	if err != nil {
		return false, err
	}
	done, err := parseInt(row, "done_pomodoros", 0, task.EstimatePomodorosMax)
	if err != nil {
		return false, err
	}
	due, err := parseDate(row.get("due_date"))
	if err != nil {
		return false, err
	}
	created, err := parseTime(row.get("created_at"))
	if err != nil {
		return false, err
	}
	completed, err := parseTime(row.get("completed_at"))
	if err != nil {
		return false, err
	}
	restored, err := task.Imported(task.ImportSpec{
		ID:           id,
		Title:        row.get("title"),
		Notes:        row.get("notes"),
		Status:       status,
		Priority:     task.Priority(priority),
		Tags:         splitList(row.get("tags")),
		DueDate:      due,
		Estimate:     estimate,
		FocusMinutes: focusMinutes,
		Done:         done,
		CreatedAt:    created,
		UpdatedAt:    created,
		Completed:    completed,
	}, now)
	if err != nil {
		return false, err
	}
	if err := s.tasks.Create(restored); err != nil {
		return false, err
	}
	return true, nil
}

func (s *Service) taskExists(id shared.ID) (bool, error) {
	_, err := s.tasks.Get(id)
	switch {
	case err == nil:
		return true, nil
	case shared.IsKind(err, shared.KindNotFound):
		return false, nil
	}
	return false, err
}

// importCategory rebuilds a ledger category, resolving its parent by name.
func (s *Service) importCategory(row rowValues) (bool, error) {
	name := row.get("name")
	if name == "" {
		return false, shared.Invalid("ledger.category.name", "name", "category name is required")
	}
	existing, err := s.categories.List()
	if err != nil {
		return false, err
	}
	for _, category := range existing {
		if strings.EqualFold(category.Name, name) {
			return false, nil // categories are identified by name, not by id
		}
	}
	direction := ledger.CategoryExpense
	if text := row.get("direction"); text != "" {
		if direction, err = ledger.ParseCategoryDirection(text); err != nil {
			return false, err
		}
	}
	category, err := ledger.NewCategory(s.ids.New(), name, direction)
	if err != nil {
		return false, err
	}
	if parent := row.get("parent"); parent != "" {
		for _, candidate := range existing {
			if strings.EqualFold(candidate.Name, parent) {
				if !candidate.ParentID.IsZero() {
					return false, shared.Invalid("ledger.category.parent", "parent", "a category cannot be three levels deep")
				}
				category.ParentID = candidate.ID
				break
			}
		}
		if category.ParentID.IsZero() {
			return false, shared.Invalid("import.category.parent", "parent", "unknown parent category: "+parent)
		}
	}
	if err := s.categories.Create(category); err != nil {
		return false, err
	}
	return true, nil
}

// importEntry rebuilds one ledger line, resolving the account and category names
// the export wrote.
func (s *Service) importEntry(row rowValues, now time.Time) (bool, error) {
	id, err := parseID(row.get("id"), "entry")
	if err != nil {
		return false, err
	}
	if _, err := s.entries.Get(id); err == nil {
		return false, nil
	} else if !shared.IsKind(err, shared.KindNotFound) {
		return false, err
	}
	kindText := row.get("kind")
	if kindText == "" {
		return false, shared.Invalid("import.kind", "kind", "the row has no kind")
	}
	kind, err := ledger.ParseEntryKind(kindText)
	if err != nil {
		return false, err
	}
	currency, err := parseCurrency(row.get("currency"))
	if err != nil {
		return false, err
	}
	amount, err := parseMoney(row.get("amount"), currency)
	if err != nil {
		return false, err
	}
	account, err := s.accountByName(row.get("account"))
	if err != nil {
		return false, err
	}
	var toAccount shared.ID
	if name := row.get("to_account"); name != "" {
		if toAccount, err = s.accountByName(name); err != nil {
			return false, err
		}
	}
	var category shared.ID
	if name := row.get("category"); name != "" {
		if category, err = s.categoryByName(name); err != nil {
			return false, err
		}
	}
	occurred, err := parseTime(row.get("occurred_at"))
	if err != nil {
		return false, err
	}
	origin := ledger.OriginManual
	if text := row.get("origin"); text != "" {
		if origin, err = ledger.ParseOrigin(text); err != nil {
			return false, err
		}
	}
	entry, err := ledger.NewEntry(id, now, ledger.EntryInput{
		Kind:        kind,
		Amount:      amount,
		AccountID:   account,
		ToAccountID: toAccount,
		CategoryID:  category,
		OccurredAt:  occurred,
		Note:        row.get("note"),
		Tags:        splitList(row.get("tags")),
		Origin:      origin,
	})
	if err != nil {
		return false, err
	}
	if err := s.entries.Create(entry); err != nil {
		return false, err
	}
	return true, nil
}

func (s *Service) accountByName(name string) (shared.ID, error) {
	if strings.TrimSpace(name) == "" {
		return "", shared.Invalid("ledger.entry.account", "account", "an entry needs an account")
	}
	accounts, err := s.accounts.List(true)
	if err != nil {
		return "", err
	}
	for _, account := range accounts {
		if strings.EqualFold(account.Name, strings.TrimSpace(name)) {
			return account.ID, nil
		}
	}
	return "", shared.Invalid("import.entry.account", "account", "unknown account: "+name)
}

func (s *Service) categoryByName(name string) (shared.ID, error) {
	categories, err := s.categories.List()
	if err != nil {
		return "", err
	}
	for _, category := range categories {
		if strings.EqualFold(category.Name, strings.TrimSpace(name)) {
			return category.ID, nil
		}
	}
	// The seeded categories are translated, so a file exported in the other
	// language names a category the database stores under its key's translation.
	for _, category := range categories {
		if category.BuiltinKey != "" && strings.EqualFold(strings.ReplaceAll(category.BuiltinKey, "_", " "), name) {
			return category.ID, nil
		}
	}
	return "", shared.Invalid("import.entry.category", "category", "unknown category: "+name)
}

// importBudget rebuilds one monthly limit.
func (s *Service) importBudget(row rowValues, now time.Time) (bool, error) {
	monthText := row.get("month")
	if monthText == "" {
		return false, shared.Invalid("import.month", "month", "the row has no month")
	}
	month, err := shared.ParseMonth(monthText)
	if err != nil {
		return false, err
	}
	category, err := s.categoryByName(row.get("category"))
	if err != nil {
		return false, err
	}
	currency, err := parseCurrency(row.get("currency"))
	if err != nil {
		return false, err
	}
	limit, err := parseMoney(row.get("limit"), currency)
	if err != nil {
		return false, err
	}
	if existing, err := s.budgets.Get(category, month); err == nil && existing != nil {
		return false, nil
	} else if err != nil && !shared.IsKind(err, shared.KindNotFound) {
		return false, err
	}
	budget, err := ledger.NewBudget(s.ids.New(), category, month, limit)
	if err != nil {
		return false, err
	}
	if err := s.budgets.Upsert(budget); err != nil {
		return false, err
	}
	return true, nil
}

// importTemplate rebuilds a template and the one todo it writes.
func (s *Service) importTemplate(row rowValues, now time.Time) (bool, error) {
	name := row.get("name")
	if name == "" {
		return false, shared.Invalid("template.name.length", "name", "template name is required")
	}
	existing, err := s.templates.List()
	if err != nil {
		return false, err
	}
	for _, candidate := range existing {
		if strings.EqualFold(candidate.Name, name) {
			return false, nil
		}
	}
	rule := template.TriggerRule{Kind: template.TriggerDaily}
	if text := row.get("rule"); text != "" {
		if rule, err = template.ParseRule(text); err != nil {
			return false, err
		}
	}
	priority, err := parseInt(row, "priority", 0, 4)
	if err != nil {
		return false, err
	}
	estimate, err := parseInt(row, "estimate", 0, 20)
	if err != nil {
		return false, err
	}
	focusMinutes, err := parseInt(row, "focus_minutes", 0, task.FocusMinutesMax)
	if err != nil {
		return false, err
	}
	offset, err := parseInt(row, "due_offset", 0, 365)
	if err != nil {
		return false, err
	}
	created, err := template.New(s.ids.New(), name, now)
	if err != nil {
		return false, err
	}
	created.Enabled = strings.EqualFold(row.get("enabled"), "true")
	created.Rule = rule
	title := row.get("task")
	if title == "" {
		title = name
	}
	if err := created.SetTask(template.Task{
		Title:         title,
		Notes:         row.get("notes"),
		Priority:      priority,
		Estimate:      estimate,
		FocusMinutes:  focusMinutes,
		DueOffsetDays: offset,
	}, now); err != nil {
		return false, err
	}
	if err := s.templates.Save(created); err != nil {
		return false, err
	}
	return true, nil
}
